// Package asrinfer: 单体 ONNX encoder（andrewleech 导出格式）。
// 输入 mel [1, 128, T] → 输出 audio_features [1, tokens, output_dim]。
package asrinfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// OnnxEncoder wraps the single-file encoder.onnx from andrewleech's export.
type OnnxEncoder struct {
	session *ort.DynamicAdvancedSession
}

// LoadOnnxEncoder loads encoder.onnx from modelDir.
func LoadOnnxEncoder(modelDir string) (*OnnxEncoder, error) {
	encPath := filepath.Join(modelDir, "encoder.onnx")
	if _, err := os.Stat(encPath); err != nil {
		return nil, fmt.Errorf("找不到 encoder.onnx: %w", err)
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

	session, err := ort.NewDynamicAdvancedSession(
		encPath,
		[]string{"mel"},
		[]string{"audio_features"},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("加载 encoder.onnx: %w", err)
	}
	return &OnnxEncoder{session: session}, nil
}

// Encode runs the encoder on mel features [128][T].
// Returns (flatData, seqLen, dim, error).
func (e *OnnxEncoder) Encode(mel [][]float32) ([]float32, int, int, error) {
	numMels := len(mel)
	if numMels == 0 || len(mel[0]) == 0 {
		return nil, 0, 0, fmt.Errorf("mel 特征为空")
	}
	T := len(mel[0])

	// Flatten [128][T] → [1, 128, T] row-major
	flat := make([]float32, numMels*T)
	for i := 0; i < numMels; i++ {
		copy(flat[i*T:], mel[i])
	}

	inputShape := ort.NewShape(1, int64(numMels), int64(T))
	inputTensor, err := ort.NewTensor(inputShape, flat)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("创建 encoder 输入: %w", err)
	}
	defer inputTensor.Destroy()

	outputs := []ort.Value{nil}
	if err := e.session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, 0, 0, fmt.Errorf("运行 encoder: %w", err)
	}

	outTensor := outputs[0].(*ort.Tensor[float32])
	shape := outTensor.GetShape() // [1, tokens, dim]
	data := outTensor.GetData()
	result := make([]float32, len(data))
	copy(result, data)
	outTensor.Destroy()

	seqLen := int(shape[1])
	dim := int(shape[2])
	return result, seqLen, dim, nil
}

// Destroy releases the ONNX session.
func (e *OnnxEncoder) Destroy() {
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
}
