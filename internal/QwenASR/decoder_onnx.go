// Package asrinfer: ONNX decoder（andrewleech 导出格式）。
// decoder_init (prefill) + decoder_step (自回归) + embed_tokens.bin (embedding lookup)。
package asrinfer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// onnxModelConfig mirrors the config.json from andrewleech's export.
type onnxModelConfig struct {
	Decoder struct {
		NumLayers         int `json:"num_layers"`
		HiddenSize        int `json:"hidden_size"`
		NumAttentionHeads int `json:"num_attention_heads"`
		NumKVHeads        int `json:"num_key_value_heads"`
		HeadDim           int `json:"head_dim"`
		VocabSize         int `json:"vocab_size"`
	} `json:"decoder"`
	EmbedTokensShape []int  `json:"embed_tokens_shape"`
	EmbedTokensDtype string `json:"embed_tokens_dtype"`
}

// OnnxDecoder holds decoder_init + decoder_step sessions and the embedding table.
type OnnxDecoder struct {
	decoderInit *ort.DynamicAdvancedSession
	decoderStep *ort.DynamicAdvancedSession
	embedTokens []float32 // [vocabSize * hiddenSize]
	vocabSize   int
	hiddenSize  int
	numLayers   int
	numKVHeads  int
	headDim     int
}

// LoadOnnxDecoder loads decoder_init, decoder_step ONNX sessions and embed_tokens.bin.
// Prefers int4 quantized variants when available.
func LoadOnnxDecoder(modelDir string) (*OnnxDecoder, error) {
	// Read config.json
	cfgData, err := os.ReadFile(filepath.Join(modelDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("读取 config.json: %w", err)
	}
	var cfg onnxModelConfig
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("解析 config.json: %w", err)
	}

	d := &OnnxDecoder{
		vocabSize:  cfg.Decoder.VocabSize,
		hiddenSize: cfg.Decoder.HiddenSize,
		numLayers:  cfg.Decoder.NumLayers,
		numKVHeads: cfg.Decoder.NumKVHeads,
		headDim:    cfg.Decoder.HeadDim,
	}

	// Pick decoder_init ONNX (prefer int4)
	initPath := pickOnnxFile(modelDir, "decoder_init")
	if initPath == "" {
		return nil, fmt.Errorf("找不到 decoder_init.onnx 或 decoder_init.int4.onnx")
	}
	// Pick decoder_step ONNX (prefer int4)
	stepPath := pickOnnxFile(modelDir, "decoder_step")
	if stepPath == "" {
		return nil, fmt.Errorf("找不到 decoder_step.onnx 或 decoder_step.int4.onnx")
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)
	if strings.ToLower(strings.TrimSpace(os.Getenv("ORT_DEVICE"))) == "dml" {
		tryAppendDML(opts)
	}

	// decoder_init session: input_embeds [batch, seq_len, hidden] + position_ids [batch, seq_len]
	d.decoderInit, err = ort.NewDynamicAdvancedSession(
		initPath,
		[]string{"input_embeds", "position_ids"},
		[]string{"logits", "present_keys", "present_values"},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("加载 decoder_init: %w", err)
	}

	// decoder_step session
	d.decoderStep, err = ort.NewDynamicAdvancedSession(
		stepPath,
		[]string{"input_embeds", "position_ids", "past_keys", "past_values"},
		[]string{"logits", "present_keys", "present_values"},
		opts,
	)
	if err != nil {
		d.decoderInit.Destroy()
		return nil, fmt.Errorf("加载 decoder_step: %w", err)
	}

	// Load embed_tokens.bin
	embedPath := filepath.Join(modelDir, "embed_tokens.bin")
	d.embedTokens, err = loadEmbedTokens(embedPath, d.vocabSize, d.hiddenSize)
	if err != nil {
		d.decoderInit.Destroy()
		d.decoderStep.Destroy()
		return nil, fmt.Errorf("加载 embed_tokens.bin: %w", err)
	}

	return d, nil
}

// Decode runs prefill + autoregressive decoding, returns recognized text.
func (d *OnnxDecoder) Decode(audioFeatures []float32, audioSeqLen int, tok *BPETokenizer, maxTokens int) (string, error) {
	// 1. Build prompt tokens
	promptIDs := BuildPromptTokens(tok, audioSeqLen)
	seqLen := len(promptIDs)

	// 2. Build position_ids [1, seqLen]
	posIDs := make([]int64, seqLen)
	for i := 0; i < seqLen; i++ {
		posIDs[i] = int64(i)
	}

	// 3. Find audio_offset: index of first audio_pad token in prompt
	audioOffset := -1
	for i, id := range promptIDs {
		if id == AudioPadTokenID {
			audioOffset = i
			break
		}
	}
	if audioOffset < 0 {
		return "", fmt.Errorf("prompt 中未找到 audio_pad token")
	}

	// 4. Build input_embeds [1, seqLen, hiddenSize]:
	//    - embed_tokens lookup for all tokens
	//    - scatter audio features over audio_pad positions
	inputEmbeds := make([]float32, seqLen*d.hiddenSize)
	for i, id := range promptIDs {
		var emb []float32
		if id == AudioPadTokenID && i >= audioOffset && i < audioOffset+audioSeqLen {
			// Use audio feature for this position
			audioIdx := i - audioOffset
			emb = audioFeatures[audioIdx*d.hiddenSize : (audioIdx+1)*d.hiddenSize]
		} else {
			emb = d.embedTokens[id*d.hiddenSize : (id+1)*d.hiddenSize]
		}
		copy(inputEmbeds[i*d.hiddenSize:], emb)
	}

	// 5. Create tensors for decoder_init
	embedsTensor, err := ort.NewTensor(ort.NewShape(1, int64(seqLen), int64(d.hiddenSize)), inputEmbeds)
	if err != nil {
		return "", fmt.Errorf("创建 input_embeds: %w", err)
	}
	defer embedsTensor.Destroy()

	posTensor, err := ort.NewTensor(ort.NewShape(1, int64(seqLen)), posIDs)
	if err != nil {
		return "", fmt.Errorf("创建 position_ids: %w", err)
	}
	defer posTensor.Destroy()

	// 6. Run decoder_init (prefill)
	initOutputs := []ort.Value{nil, nil, nil}
	if err := d.decoderInit.Run(
		[]ort.Value{embedsTensor, posTensor},
		initOutputs,
	); err != nil {
		return "", fmt.Errorf("运行 decoder_init: %w", err)
	}

	// Extract logits from last position
	logitsTensor := initOutputs[0].(*ort.Tensor[float32])
	logitsData := logitsTensor.GetData()
	lastLogits := make([]float32, d.vocabSize)
	copy(lastLogits, logitsData[(seqLen-1)*d.vocabSize:seqLen*d.vocabSize])
	logitsTensor.Destroy()

	// KV cache tensors
	pastKeys := initOutputs[1]
	pastValues := initOutputs[2]

	// 7. Autoregressive decode loop
	var generated []int
	nextPos := int64(seqLen)

	for step := 0; step < maxTokens; step++ {
		nextToken := argmaxOnnx(lastLogits)
		if IsEOS(nextToken) {
			break
		}
		generated = append(generated, nextToken)

		// Embedding lookup for next token
		embed := d.lookupEmbed(nextToken)

		embedTensor, err := ort.NewTensor(ort.NewShape(1, 1, int64(d.hiddenSize)), embed)
		if err != nil {
			pastKeys.Destroy()
			pastValues.Destroy()
			return tok.Decode(generated), fmt.Errorf("创建 input_embeds: %w", err)
		}

		stepPosTensor, err := ort.NewTensor(ort.NewShape(1, 1), []int64{nextPos})
		if err != nil {
			embedTensor.Destroy()
			pastKeys.Destroy()
			pastValues.Destroy()
			return tok.Decode(generated), fmt.Errorf("创建 step position_ids: %w", err)
		}

		stepOutputs := []ort.Value{nil, nil, nil}
		err = d.decoderStep.Run(
			[]ort.Value{embedTensor, stepPosTensor, pastKeys, pastValues},
			stepOutputs,
		)
		embedTensor.Destroy()
		stepPosTensor.Destroy()
		pastKeys.Destroy()
		pastValues.Destroy()

		if err != nil {
			return tok.Decode(generated), fmt.Errorf("decoder_step %d: %w", step, err)
		}

		stepLogits := stepOutputs[0].(*ort.Tensor[float32])
		stepLogitsData := stepLogits.GetData()
		lastLogits = make([]float32, d.vocabSize)
		copy(lastLogits, stepLogitsData[:d.vocabSize])
		stepLogits.Destroy()

		pastKeys = stepOutputs[1]
		pastValues = stepOutputs[2]
		nextPos++
	}

	pastKeys.Destroy()
	pastValues.Destroy()

	return tok.Decode(generated), nil
}

// Destroy releases ONNX sessions.
func (d *OnnxDecoder) Destroy() {
	if d.decoderInit != nil {
		d.decoderInit.Destroy()
		d.decoderInit = nil
	}
	if d.decoderStep != nil {
		d.decoderStep.Destroy()
		d.decoderStep = nil
	}
	d.embedTokens = nil
}

// lookupEmbed returns the embedding vector for a token ID.
func (d *OnnxDecoder) lookupEmbed(tokenID int) []float32 {
	offset := tokenID * d.hiddenSize
	embed := make([]float32, d.hiddenSize)
	copy(embed, d.embedTokens[offset:offset+d.hiddenSize])
	return embed
}

func argmaxOnnx(logits []float32) int {
	best := 0
	bestVal := float32(-math.MaxFloat32)
	for i, v := range logits {
		if v > bestVal {
			bestVal = v
			best = i
		}
	}
	return best
}

// pickOnnxFile returns the path to a quantized or FP32 ONNX file.
// Prefers int4 > int8 > fp16 > fp32.
func pickOnnxFile(modelDir, baseName string) string {
	suffixes := []string{".int4.onnx", ".int8.onnx", ".fp16.onnx", ".onnx"}
	for _, suffix := range suffixes {
		p := filepath.Join(modelDir, baseName+suffix)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// loadEmbedTokens reads embed_tokens.bin as raw float32 [vocabSize * hiddenSize].
func loadEmbedTokens(path string, vocabSize, hiddenSize int) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	expected := vocabSize * hiddenSize
	data := make([]float32, expected)
	if err := binary.Read(f, binary.LittleEndian, data); err != nil {
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			return nil, fmt.Errorf("embed_tokens.bin 大小不匹配，期望 %d 个 float32", expected)
		}
		return nil, err
	}
	return data, nil
}
