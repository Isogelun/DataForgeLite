// Package asrinfer: GGUF 格式编码器（ONNX 前段 + 后段），与 Qwen3-ASR-GGUF 兼容。
package asrinfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	ggufChunkFrames   = 100
	ggufPadToSec      = 40
	ggufFramesPerSec  = 13
	ggufMelBins       = 128
	ggufTargetPadLen  = ggufPadToSec * ggufFramesPerSec // 520
)

// getFeatExtractOutputLengths 与 Qwen3 前端逻辑一致，计算有效输出帧数。
func getFeatExtractOutputLengths(inputLength int) int {
	inputLengthsLeave := inputLength % 100
	featLengths := (inputLengthsLeave-1)/2 + 1
	outputLengths := ((featLengths-1)/2+1-1)/2 + 1 + (inputLength/100)*13
	return outputLengths
}

// GGUFEncoder 使用 frontend + backend 两个 ONNX，输出与 Qwen3-ASR-GGUF 一致的 audio embedding。
type GGUFEncoder struct {
	frontend *ort.DynamicAdvancedSession
	backend  *ort.DynamicAdvancedSession
	padTo    int
}

// LoadGGUFEncoder 从模型目录加载 frontend 与 backend ONNX。
// 默认文件名: qwen3_asr_encoder_frontend.int4.onnx, qwen3_asr_encoder_backend.int4.onnx
func LoadGGUFEncoder(modelDir string) (*GGUFEncoder, error) {
	frontPath := filepath.Join(modelDir, "qwen3_asr_encoder_frontend.int4.onnx")
	backPath := filepath.Join(modelDir, "qwen3_asr_encoder_backend.int4.onnx")
	for _, p := range []string{frontPath, backPath} {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("找不到 %s: %w", p, err)
		}
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)
	tryAppendDML(opts)

	front, err := ort.NewDynamicAdvancedSession(
		frontPath,
		[]string{"chunk_mel"},
		[]string{"chunk_out"},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("加载 frontend: %w", err)
	}

	back, err := ort.NewDynamicAdvancedSession(
		backPath,
		[]string{"hidden_states", "attention_mask"},
		[]string{"last_hidden_state"},
		opts,
	)
	if err != nil {
		front.Destroy()
		return nil, fmt.Errorf("加载 backend: %w", err)
	}

	return &GGUFEncoder{
		frontend: front,
		backend:  back,
		padTo:    ggufTargetPadLen,
	}, nil
}

func tryAppendDML(opts *ort.SessionOptions) {
	deviceID := 0
	if v := strings.TrimSpace(os.Getenv("ORT_DML_DEVICE_ID")); v != "" {
		fmt.Sscanf(v, "%d", &deviceID)
	}
	_ = opts.AppendExecutionProviderDirectML(deviceID)
}

// Destroy 释放 ONNX 会话。
func (e *GGUFEncoder) Destroy() {
	if e.frontend != nil {
		e.frontend.Destroy()
		e.frontend = nil
	}
	if e.backend != nil {
		e.backend.Destroy()
		e.backend = nil
	}
}

// Encode 将 Mel 特征 [128][T] 编码为 (T_out, dim) 的 embedding。
// Mel 需与 Qwen3 一致：128 bins, hop 160, 16kHz。
func (e *GGUFEncoder) Encode(mel [][]float32) ([]float32, int, int, error) {
	T := len(mel[0])
	// Pad 到 100 的倍数
	padLen := (100 - (T % 100)) % 100
	Tpadded := T + padLen
	if Tpadded == 0 {
		return nil, 0, 0, fmt.Errorf("mel 长度为 0")
	}

	flat := make([]float32, ggufMelBins*Tpadded)
	for i := 0; i < ggufMelBins; i++ {
		for j := 0; j < T; j++ {
			flat[i*Tpadded+j] = mel[i][j]
		}
	}

	numChunks := Tpadded / ggufChunkFrames
	var hiddenList [][]float32
	for i := 0; i < numChunks; i++ {
		start := i * ggufChunkFrames
		chunk := make([]float32, ggufMelBins*ggufChunkFrames)
		for m := 0; m < ggufMelBins; m++ {
			copy(chunk[m*ggufChunkFrames:], flat[m*Tpadded+start:m*Tpadded+start+ggufChunkFrames])
		}
		inputShape := ort.NewShape(1, int64(ggufMelBins), int64(ggufChunkFrames))
		inputTensor, err := ort.NewTensor(inputShape, chunk)
		if err != nil {
			return nil, 0, 0, err
		}
		outputs := []ort.Value{nil}
		if err := e.frontend.Run([]ort.Value{inputTensor}, outputs); err != nil {
			inputTensor.Destroy()
			return nil, 0, 0, err
		}
		inputTensor.Destroy()
		outT := outputs[0].(*ort.Tensor[float32])
		data := outT.GetData()
		dup := make([]float32, len(data))
		copy(dup, data)
		outT.Destroy()
		// shape (1, 13, dim)
		hiddenList = append(hiddenList, dup)
	}

	// 拼接并截取有效长度
	tOut := getFeatExtractOutputLengths(T)
	dim := 0
	for _, h := range hiddenList {
		if len(h) > 0 {
			dim = len(h) / (1 * 13)
			break
		}
	}
	if dim == 0 {
		return nil, 0, 0, fmt.Errorf("frontend 输出为空")
	}
	totalLen := numChunks * 13
	if tOut > totalLen {
		tOut = totalLen
	}
	hiddenStates := make([]float32, totalLen*dim)
	offset := 0
	for _, h := range hiddenList {
		n := 13 * dim
		if offset+n <= len(hiddenStates) && n <= len(h) {
			copy(hiddenStates[offset:offset+n], h[:n])
		}
		offset += n
	}
	hiddenStates = hiddenStates[:tOut*dim]

	// Backend: 可选 pad 到固定长度（DML 友好）
	seqLen := tOut
	hTarget := e.padTo
	if seqLen < hTarget {
		padded := make([]float32, hTarget*dim)
		copy(padded, hiddenStates)
		hiddenStates = padded
		seqLen = hTarget
	}

	// attention_mask: (1, 1, seqLen, seqLen)，有效位置 0，pad 位置 -10000
	mask := make([]float32, 1*1*seqLen*seqLen)
	for i := seqLen * seqLen; i < len(mask); i++ {
		mask[i] = -10000.0
	}
	// 有效部分为 0（已零初始化）
	tOutOrig := getFeatExtractOutputLengths(T)
	for row := 0; row < seqLen; row++ {
		for col := tOutOrig; col < seqLen; col++ {
			mask[row*seqLen+col] = -10000.0
		}
	}

	hiddenShape := ort.NewShape(1, int64(seqLen), int64(dim))
	maskShape := ort.NewShape(1, 1, int64(seqLen), int64(seqLen))
	hiddenTensor, err := ort.NewTensor(hiddenShape, hiddenStates)
	if err != nil {
		return nil, 0, 0, err
	}
	defer hiddenTensor.Destroy()
	maskTensor, err := ort.NewTensor(maskShape, mask)
	if err != nil {
		return nil, 0, 0, err
	}
	defer maskTensor.Destroy()

	outputs := []ort.Value{nil}
	if err := e.backend.Run([]ort.Value{hiddenTensor, maskTensor}, outputs); err != nil {
		return nil, 0, 0, err
	}
	outT := outputs[0].(*ort.Tensor[float32])
	outData := outT.GetData()
	outShape := outT.GetShape()
	outT.Destroy()
	// 截回有效长度
	actualSeq := int(outShape[1])
	if actualSeq > tOutOrig {
		actualSeq = tOutOrig
	}
	result := make([]float32, actualSeq*dim)
	copy(result, outData[:actualSeq*dim])
	return result, actualSeq, dim, nil
}
