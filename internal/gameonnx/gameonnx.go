package gameonnx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ModelConfig mirrors Game's config.json.
type ModelConfig struct {
	SampleRate   int            `json:"samplerate"`
	TimeStep     float64        `json:"timestep"`
	Languages    map[string]int `json:"languages"`
	Loop         bool           `json:"loop"`
	EmbeddingDim int            `json:"embedding_dim"`
}

type BoolTensor struct {
	Shape []int64
	Data  []bool
}

type F32Tensor struct {
	Shape []int64
	Data  []float32
}

type Note struct {
	Index    int     `json:"index"`
	Duration float32 `json:"duration_sec"`
	Pitch    float32 `json:"pitch_midi"`
	// Rest true：无发声（对应 infer.py presence=False → "rest"）
	Rest bool `json:"rest,omitempty"`
}

type Result struct {
	SampleRate int    `json:"samplerate"`
	TimeStep   string `json:"timestep"`
	Language   string `json:"language"`
	Notes      []Note `json:"notes"`
}

func loadModelConfig(modelDir string) (*ModelConfig, error) {
	b, err := os.ReadFile(filepath.Join(modelDir, "config.json"))
	if err != nil {
		return nil, err
	}
	var cfg ModelConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.SampleRate <= 0 {
		return nil, fmt.Errorf("config.json samplerate 无效: %d", cfg.SampleRate)
	}
	return &cfg, nil
}
