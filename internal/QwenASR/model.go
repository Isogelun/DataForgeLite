package asrinfer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	ort "github.com/yalue/onnxruntime_go"
)

// OnnxASRConfig stores ONNX model configuration exported alongside the models.
type OnnxASRConfig struct {
	NumLayers         int `json:"num_layers"`
	NumKVHeads        int `json:"num_kv_heads"`
	HeadDim           int `json:"head_dim"`
	HiddenSize        int `json:"hidden_size"`
	VocabSize         int `json:"vocab_size"`
	NumAttentionHeads int `json:"num_attention_heads"`
}

// ASRModel holds the loaded ONNX sessions for encoder + decoder.
type ASRModel struct {
	Config      OnnxASRConfig
	Tokenizer   *BPETokenizer
	encoder     *ort.DynamicAdvancedSession
	decoderInit *ort.DynamicAdvancedSession
	decoder     *ort.DynamicAdvancedSession
	onnxDir     string
}

// LoadASRModel loads the encoder, decoder_init, and decoder ONNX sessions.
func LoadASRModel(onnxDir string, modelDir string) (*ASRModel, error) {
	cfgPath := filepath.Join(onnxDir, "onnx_config.json")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read onnx_config.json: %w", err)
	}
	var cfg OnnxASRConfig
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("parse onnx_config.json: %w", err)
	}

	tok, err := LoadTokenizer(modelDir)
	if err != nil {
		return nil, err
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create session options: %w", err)
	}
	defer opts.Destroy()
	opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)
	tryAppendCuda(opts)

	// Encoder session
	encPath := filepath.Join(onnxDir, "encoder.onnx")
	encSession, err := ort.NewDynamicAdvancedSession(
		encPath,
		[]string{"mel_features"},
		[]string{"encoder_hidden_states"},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("load encoder.onnx: %w", err)
	}

	// Decoder init session
	decInitPath := filepath.Join(onnxDir, "decoder_init.onnx")
	decInitInputs := []string{"input_ids", "encoder_hidden_states"}
	numKV := cfg.NumLayers * 2
	decInitOutputs := []string{"logits"}
	for i := 0; i < numKV; i++ {
		decInitOutputs = append(decInitOutputs, fmt.Sprintf("present_kv_%d", i))
	}
	decInitSession, err := ort.NewDynamicAdvancedSession(
		decInitPath,
		decInitInputs,
		decInitOutputs,
		opts,
	)
	if err != nil {
		encSession.Destroy()
		return nil, fmt.Errorf("load decoder_init.onnx: %w", err)
	}

	// Decoder session (with KV cache)
	decPath := filepath.Join(onnxDir, "decoder.onnx")
	decInputs := []string{"input_ids"}
	for i := 0; i < numKV; i++ {
		decInputs = append(decInputs, fmt.Sprintf("past_kv_%d", i))
	}
	decOutputs := []string{"logits"}
	for i := 0; i < numKV; i++ {
		decOutputs = append(decOutputs, fmt.Sprintf("present_kv_%d", i))
	}
	decSession, err := ort.NewDynamicAdvancedSession(
		decPath,
		decInputs,
		decOutputs,
		opts,
	)
	if err != nil {
		encSession.Destroy()
		decInitSession.Destroy()
		return nil, fmt.Errorf("load decoder.onnx: %w", err)
	}

	return &ASRModel{
		Config:      cfg,
		Tokenizer:   tok,
		encoder:     encSession,
		decoderInit: decInitSession,
		decoder:     decSession,
		onnxDir:     onnxDir,
	}, nil
}

// Destroy releases all ONNX sessions.
func (m *ASRModel) Destroy() {
	if m.encoder != nil {
		m.encoder.Destroy()
	}
	if m.decoderInit != nil {
		m.decoderInit.Destroy()
	}
	if m.decoder != nil {
		m.decoder.Destroy()
	}
}

// RunEncoder runs the audio encoder on mel features.
// melFeatures shape: [numMelBins][T] -> ONNX input [1, numMelBins, T]
func (m *ASRModel) RunEncoder(melFeatures [][]float32) ([]float32, []int64, error) {
	numMels := len(melFeatures)
	T := len(melFeatures[0])
	flat := make([]float32, numMels*T)
	for i := 0; i < numMels; i++ {
		copy(flat[i*T:], melFeatures[i])
	}

	inputShape := ort.NewShape(1, int64(numMels), int64(T))
	inputTensor, err := ort.NewTensor(inputShape, flat)
	if err != nil {
		return nil, nil, fmt.Errorf("create encoder input: %w", err)
	}
	defer inputTensor.Destroy()

	outputs := []ort.Value{nil}
	if err := m.encoder.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, nil, fmt.Errorf("run encoder: %w", err)
	}

	outTensor := outputs[0].(*ort.Tensor[float32])
	defer outTensor.Destroy()

	shape := outTensor.GetShape()
	data := outTensor.GetData()
	result := make([]float32, len(data))
	copy(result, data)
	return result, shape, nil
}

// Generate runs autoregressive decoding.
// Returns generated token IDs (excluding prompt).
func (m *ASRModel) Generate(melFeatures [][]float32, maxTokens int) ([]int, error) {
	// 1. Encode audio
	encData, encShape, err := m.RunEncoder(melFeatures)
	if err != nil {
		return nil, err
	}

	// encShape: [1, seqLen, hiddenSize]
	encSeqLen := int(encShape[1])

	// 2. Build prompt
	promptIDs := BuildPromptTokens(m.Tokenizer, encSeqLen)

	// 3. Decoder init (first step)
	numKV := m.Config.NumLayers * 2
	kvCache, nextLogits, err := m.runDecoderInit(promptIDs, encData, encShape)
	if err != nil {
		return nil, fmt.Errorf("decoder init: %w", err)
	}

	// 4. Greedy decode loop
	var generated []int
	for step := 0; step < maxTokens; step++ {
		nextToken := argmax(nextLogits)
		if IsEOS(nextToken) {
			break
		}
		generated = append(generated, nextToken)

		// Run decoder step
		var newKV []ort.Value
		newKV, nextLogits, err = m.runDecoderStep(nextToken, kvCache)
		if err != nil {
			destroyValues(kvCache)
			return generated, fmt.Errorf("decoder step %d: %w", step, err)
		}
		destroyValues(kvCache)
		kvCache = newKV
	}
	destroyValues(kvCache)

	_ = numKV
	return generated, nil
}

func (m *ASRModel) runDecoderInit(promptIDs []int, encData []float32, encShape []int64) (
	kvCache []ort.Value, lastLogits []float32, err error,
) {
	numKV := m.Config.NumLayers * 2

	// Input IDs tensor [1, seqLen]
	seqLen := len(promptIDs)
	ids64 := make([]int64, seqLen)
	for i, id := range promptIDs {
		ids64[i] = int64(id)
	}
	idsShape := ort.NewShape(1, int64(seqLen))
	idsTensor, err := ort.NewTensor(idsShape, ids64)
	if err != nil {
		return nil, nil, err
	}
	defer idsTensor.Destroy()

	// Encoder hidden states tensor
	encTensor, err := ort.NewTensor(ort.NewShape(encShape...), encData)
	if err != nil {
		return nil, nil, err
	}
	defer encTensor.Destroy()

	outputs := make([]ort.Value, 1+numKV)
	if err := m.decoderInit.Run([]ort.Value{idsTensor, encTensor}, outputs); err != nil {
		return nil, nil, err
	}

	// Extract logits (last position)
	logitsTensor := outputs[0].(*ort.Tensor[float32])
	logitsData := logitsTensor.GetData()
	logitsShape := logitsTensor.GetShape()
	vocabSize := int(logitsShape[2])
	lastPos := int(logitsShape[1]) - 1
	lastLogits = make([]float32, vocabSize)
	copy(lastLogits, logitsData[lastPos*vocabSize:(lastPos+1)*vocabSize])
	logitsTensor.Destroy()

	// KV cache = outputs[1:]
	kvCache = outputs[1:]
	return kvCache, lastLogits, nil
}

func (m *ASRModel) runDecoderStep(tokenID int, kvCache []ort.Value) (
	newKV []ort.Value, lastLogits []float32, err error,
) {
	numKV := m.Config.NumLayers * 2

	ids64 := []int64{int64(tokenID)}
	idsShape := ort.NewShape(1, 1)
	idsTensor, err := ort.NewTensor(idsShape, ids64)
	if err != nil {
		return nil, nil, err
	}
	defer idsTensor.Destroy()

	inputs := make([]ort.Value, 1+numKV)
	inputs[0] = idsTensor
	copy(inputs[1:], kvCache)

	outputs := make([]ort.Value, 1+numKV)
	if err := m.decoder.Run(inputs, outputs); err != nil {
		return nil, nil, err
	}

	// Extract logits
	logitsTensor := outputs[0].(*ort.Tensor[float32])
	logitsData := logitsTensor.GetData()
	logitsShape := logitsTensor.GetShape()
	vocabSize := int(logitsShape[2])
	lastLogits = make([]float32, vocabSize)
	copy(lastLogits, logitsData[:vocabSize])
	logitsTensor.Destroy()

	newKV = outputs[1:]
	return newKV, lastLogits, nil
}

func argmax(logits []float32) int {
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

func destroyValues(vals []ort.Value) {
	for _, v := range vals {
		if v != nil {
			v.Destroy()
		}
	}
}
