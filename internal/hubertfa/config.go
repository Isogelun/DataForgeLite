package hubertfa

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MelSpecConfig holds mel spectrogram parameters from config.json.
type MelSpecConfig struct {
	NMels      int     `json:"n_mels"`
	SampleRate int     `json:"sample_rate"`
	WindowSize int     `json:"window_size"`
	HopSize    int     `json:"hop_size"`
	NFFT       int     `json:"n_fft"`
	FMin       float64 `json:"f_min"`
	FMax       float64 `json:"f_max"`
	Clamp      float64 `json:"clamp"`
}

// ModelConfig is the top-level config.json structure.
type ModelConfig struct {
	MelSpec MelSpecConfig `json:"mel_spec_config"`
}

// VocabConfig represents vocab.json.
type VocabConfig struct {
	NonLexicalPhonemes     []string          `json:"non_lexical_phonemes"`
	NonLexicalPhonemesDict map[string]int    `json:"non_lexical_phonemes_dict"`
	Dictionaries           map[string]string `json:"dictionaries"`
	LanguagePrefix         bool              `json:"language_prefix"`
	MergedPhonemeGroups    [][]string        `json:"merged_phoneme_groups"`
	SilentPhonemes         []string          `json:"silent_phonemes"`
	Vocab                  map[string]int    `json:"vocab"`
	VocabSize              int               `json:"vocab_size"`
}

// FAModelPaths holds resolved paths for a model directory.
type FAModelPaths struct {
	ModelDir   string
	OnnxPath   string
	ConfigPath string
	VocabPath  string
	VersionPath string
}

// ResolveModelPaths finds required files inside the model directory.
func ResolveModelPaths(modelDir string) (*FAModelPaths, error) {
	p := &FAModelPaths{ModelDir: modelDir}
	p.OnnxPath = filepath.Join(modelDir, "model.onnx")
	p.ConfigPath = filepath.Join(modelDir, "config.json")
	p.VocabPath = filepath.Join(modelDir, "vocab.json")
	p.VersionPath = filepath.Join(modelDir, "VERSION")

	for _, f := range []string{p.OnnxPath, p.ConfigPath, p.VocabPath, p.VersionPath} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return nil, fmt.Errorf("required file not found: %s", f)
		}
	}
	return p, nil
}

// LoadConfig reads config.json.
func LoadConfig(path string) (*ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.json: %w", err)
	}
	var cfg ModelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	return &cfg, nil
}

// LoadVocab reads vocab.json.
func LoadVocab(path string) (*VocabConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vocab.json: %w", err)
	}
	var vocab VocabConfig
	if err := json.Unmarshal(data, &vocab); err != nil {
		return nil, fmt.Errorf("parse vocab.json: %w", err)
	}
	return &vocab, nil
}

// CheckVersion reads VERSION file and asserts version == 5.
func CheckVersion(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read VERSION: %w", err)
	}
	ver := strings.TrimSpace(string(data))
	v, err := strconv.Atoi(ver)
	if err != nil {
		return fmt.Errorf("parse VERSION %q: %w", ver, err)
	}
	if v != 5 {
		return fmt.Errorf("onnx model version must be 5, got %d", v)
	}
	return nil
}

// DictionaryPath resolves the dictionary file for a given language.
func DictionaryPath(vocab *VocabConfig, modelDir, language string) (string, error) {
	dictFile, ok := vocab.Dictionaries[language]
	if !ok {
		return "", fmt.Errorf("language %q not found in vocab dictionaries", language)
	}
	p := filepath.Join(modelDir, dictFile)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return "", fmt.Errorf("dictionary file not found: %s", p)
	}
	return p, nil
}
