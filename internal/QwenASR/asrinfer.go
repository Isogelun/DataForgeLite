// Package asrinfer provides Go-native ASR inference.
// 支持三种后端（按优先级）：
//   1. 纯 ONNX（andrewleech 导出格式：encoder.onnx + decoder_init/decoder_step + embed_tokens.bin）
//   2. Python（qwen3asrinfer 目录含 .venv 和 main.py，通过子进程调用）
//   3. GGUF（ONNX encoder frontend/backend + llama_asr_decode 子进程）— 旧路径，作为 fallback
package asrinfer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/go-audio/wav"
	"github.com/mozillazg/go-pinyin"
	ort "github.com/yalue/onnxruntime_go"
)

// ASRResult holds recognition result for a single audio file.
type ASRResult struct {
	File         string `json:"file"`
	Text         string `json:"text,omitempty"`
	Pinyin       string `json:"pinyin,omitempty"`
	TxtOutput    string `json:"txt_output,omitempty"`
	PinyinOutput string `json:"pinyin_output,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ASRResponse is the JSON-serialisable inference response.
type ASRResponse struct {
	Success         bool        `json:"success"`
	TotalFiles      int         `json:"total_files,omitempty"`
	SuccessCount    int         `json:"success_count,omitempty"`
	ErrorCount      int         `json:"error_count,omitempty"`
	OutputDirectory string      `json:"output_directory,omitempty"`
	Results         []ASRResult `json:"results,omitempty"`
	Error           string      `json:"error,omitempty"`
	Timestamp       string      `json:"timestamp,omitempty"`
}

// Config holds ASR inference configuration. 仅使用 GGUF 纯 Go 路径（无 Python）。
type Config struct {
	InputDir   string
	OutputDir  string
	Language   string
	ModelDir   string
	OnnxDir    string
	MaxTokens  int
	Device     string
	WorkDir    string
	Prefix     string
	Dtype      string
	PythonCmd  string
	Backend    string
}

// DefaultConfig creates a config with sensible defaults.
func DefaultConfig(inputDir, outputDir string) Config {
	return Config{
		InputDir:  inputDir,
		OutputDir: outputDir,
		MaxTokens: 256,
		Device:    "cuda:0",
		Prefix:    "output",
		Dtype:     "float32",
		PythonCmd: "",
	}
}

// ASRInferencer manages the ASR model and performs transcription.
type ASRInferencer struct {
	config Config
	model  *ASRModel
}

// NewASRInferencer creates an inferencer from the given config.
func NewASRInferencer(config Config) *ASRInferencer {
	return &ASRInferencer{config: config}
}

// resolveOnnxModelDir 定位 andrewleech 格式的 ONNX 模型目录（含 encoder.onnx + decoder_init + embed_tokens.bin）。
func (a *ASRInferencer) resolveOnnxModelDir() (string, error) {
	checkDir := func(dir string) bool {
		_, e1 := os.Stat(filepath.Join(dir, "encoder.onnx"))
		_, e2 := os.Stat(filepath.Join(dir, "embed_tokens.bin"))
		// decoder_init 可能是 .onnx 或 .int4.onnx
		hasInit := false
		for _, suffix := range []string{".int4.onnx", ".int8.onnx", ".onnx"} {
			if _, err := os.Stat(filepath.Join(dir, "decoder_init"+suffix)); err == nil {
				hasInit = true
				break
			}
		}
		return e1 == nil && e2 == nil && hasInit
	}

	// 1. 先查 Config.OnnxDir / Config.ModelDir
	for _, dir := range []string{a.config.OnnxDir, a.config.ModelDir} {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			exePath, _ := os.Executable()
			dir = filepath.Join(filepath.Dir(exePath), dir)
		}
		if checkDir(dir) {
			return filepath.Abs(dir)
		}
	}

	// 2. 搜索 exe/cwd 附近，包括 qwen3-asr-*-onnx 子目录
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cwd, _ := os.Getwd()
	roots := []string{exeDir, filepath.Dir(exeDir), cwd, filepath.Dir(cwd)}
	for _, root := range roots {
		if checkDir(root) {
			return filepath.Abs(root)
		}
		// 查找 qwen3-asr-*-onnx 子目录
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			lower := strings.ToLower(e.Name())
			if strings.Contains(lower, "qwen") && strings.Contains(lower, "onnx") {
				candidate := filepath.Join(root, e.Name())
				if checkDir(candidate) {
					return filepath.Abs(candidate)
				}
			}
		}
	}
	return "", fmt.Errorf("未找到 ONNX 模型目录（需含 encoder.onnx + decoder_init.onnx + embed_tokens.bin）")
}

// Transcribe 批量识别。优先使用纯 ONNX 路径，其次 Python，最后 GGUF。
func (a *ASRInferencer) Transcribe() (*ASRResponse, error) {
	inputDir := strings.TrimSpace(a.config.InputDir)
	outputDir := strings.TrimSpace(a.config.OutputDir)
	if inputDir == "" || outputDir == "" {
		return nil, fmt.Errorf("输入/输出目录不能为空")
	}
	if absInput, err := filepath.Abs(inputDir); err == nil {
		inputDir = absInput
	}
	if absOutput, err := filepath.Abs(outputDir); err == nil {
		outputDir = absOutput
	}
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("输入目录不存在: %s", inputDir)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建输出目录: %w", err)
	}

	// 1. 纯 ONNX 路径
	if a.config.Backend != "python" {
		if onnxDir, err := a.resolveOnnxModelDir(); err == nil {
			return a.transcribeOnnx(onnxDir, inputDir, outputDir)
		}
	}

	// 2. Python 路径（qwen3asrinfer 目录含 .venv 和 main.py）
	if pyDir, err := a.resolvePythonDir(); err == nil {
		return a.transcribePython(pyDir, inputDir, outputDir)
	}

	return nil, fmt.Errorf("未找到可用的 ASR 后端：请确保 qwen3-asr-*-onnx 或 qwen3asrinfer 目录存在于程序附近")
}

// transcribeOnnx 使用 andrewleech ONNX 模型进行推理。
func (a *ASRInferencer) transcribeOnnx(modelDir, inputDir, outputDir string) (*ASRResponse, error) {
	if err := configureOnnxRuntime(); err != nil {
		return nil, err
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("初始化 ONNX Runtime: %w", err)
	}
	defer ort.DestroyEnvironment()

	enc, err := LoadOnnxEncoder(modelDir)
	if err != nil {
		return nil, fmt.Errorf("加载 ONNX 编码器: %w", err)
	}
	defer enc.Destroy()

	dec, err := LoadOnnxDecoder(modelDir)
	if err != nil {
		return nil, fmt.Errorf("加载 ONNX 解码器: %w", err)
	}
	defer dec.Destroy()

	tok, err := LoadTokenizer(modelDir)
	if err != nil {
		return nil, fmt.Errorf("加载 tokenizer: %w", err)
	}

	wavPaths, err := scanWAVFiles(inputDir)
	if err != nil {
		return nil, err
	}
	if len(wavPaths) == 0 {
		return &ASRResponse{Success: false, Error: "未找到任何 WAV 文件", TotalFiles: 0}, nil
	}

	melCfg := DefaultMelConfig()
	prefix := a.config.Prefix
	if prefix == "" {
		prefix = "output"
	}
	maxTokens := a.config.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}

	var results []ASRResult
	for _, wavPath := range wavPaths {
		baseName := filepath.Base(wavPath)
		baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
		txtPath := filepath.Join(outputDir, prefix+"_"+baseName+".txt")
		pinyinPath := filepath.Join(outputDir, prefix+"_"+baseName+"_pinyin.txt")
		labPath := filepath.Join(outputDir, prefix+"_"+baseName+".lab")

		samples, err := loadAndResampleWAV(wavPath, melCfg.SampleRate)
		if err != nil {
			results = append(results, ASRResult{File: wavPath, Error: err.Error()})
			continue
		}

		mel := ComputeMelSpectrogram(samples, melCfg)
		if len(mel) == 0 || len(mel[0]) == 0 {
			results = append(results, ASRResult{File: wavPath, Error: "Mel 特征为空"})
			continue
		}

		audioFeatures, audioSeqLen, _, err := enc.Encode(mel)
		if err != nil {
			results = append(results, ASRResult{File: wavPath, Error: err.Error()})
			continue
		}

		text, err := dec.Decode(audioFeatures, audioSeqLen, tok, maxTokens)
		if err != nil {
			results = append(results, ASRResult{File: wavPath, Error: err.Error()})
			continue
		}
		if strings.TrimSpace(text) == "" {
			text = "(识别为空)"
		}
		pinyinText := textToPinyin(text)

		_ = os.WriteFile(txtPath, []byte(text), 0644)
		_ = os.WriteFile(pinyinPath, []byte(pinyinText), 0644)
		_ = os.WriteFile(labPath, []byte(pinyinText), 0644)

		results = append(results, ASRResult{
			File:         wavPath,
			Text:         text,
			Pinyin:       pinyinText,
			TxtOutput:    txtPath,
			PinyinOutput: pinyinPath,
		})
	}

	return a.buildResponse(wavPaths, results, outputDir)
}

// buildResponse 构建最终响应并写入 summary/json 文件。
func (a *ASRInferencer) buildResponse(wavPaths []string, results []ASRResult, outputDir string) (*ASRResponse, error) {
	prefix := a.config.Prefix
	if prefix == "" {
		prefix = "output"
	}
	successCount := 0
	for _, r := range results {
		if r.Error == "" {
			successCount++
		}
	}
	summaryPath := filepath.Join(outputDir, prefix+"_summary.txt")
	writeSummaryFile(summaryPath, results)

	resp := &ASRResponse{
		Success:         successCount == len(wavPaths),
		TotalFiles:      len(wavPaths),
		SuccessCount:    successCount,
		ErrorCount:      len(wavPaths) - successCount,
		OutputDirectory: outputDir,
		Results:         results,
	}
	jsonBytes, _ := json.MarshalIndent(resp, "", "  ")
	_ = os.WriteFile(filepath.Join(outputDir, "asr_result.json"), jsonBytes, 0644)
	return resp, nil
}

func writeSummaryFile(path string, results []ASRResult) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("处理了 %d 个文件\n", len(results)))
	b.WriteString(strings.Repeat("=", 50) + "\n\n")
	for _, r := range results {
		b.WriteString("文件：" + r.File + "\n")
		if r.Error != "" {
			b.WriteString("错误：" + r.Error + "\n")
		} else {
			b.WriteString("文本：" + r.Text + "\n")
			b.WriteString("拼音：" + r.Pinyin + "\n")
		}
		b.WriteString(strings.Repeat("-", 50) + "\n")
	}
	_ = os.WriteFile(path, []byte(b.String()), 0644)
}

// QuickTranscribe is a convenience function.
func QuickTranscribe(inputDir, outputDir string) (*ASRResponse, error) {
	config := DefaultConfig(inputDir, outputDir)
	inferencer := NewASRInferencer(config)
	return inferencer.Transcribe()
}

// SimpleTranscribe is a convenience function.
func SimpleTranscribe(inputDir, outputDir string) (*ASRResponse, error) {
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("输入目录不存在: %s", inputDir)
	}
	return QuickTranscribe(inputDir, outputDir)
}

func scanWAVFiles(dir string) ([]string, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.wav"))
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	return entries, nil
}

func loadAndResampleWAV(path string, targetSR int) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, fmt.Errorf("invalid wav: %s", path)
	}

	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, fmt.Errorf("decode wav: %w", err)
	}

	sr := int(dec.SampleRate)
	nCh := int(dec.NumChans)
	bitDepth := int(dec.BitDepth)
	data := buf.Data

	// Mix to mono
	if nCh > 1 {
		n := len(data) / nCh
		mono := make([]int, n)
		for i := 0; i < n; i++ {
			sum := 0
			for c := 0; c < nCh; c++ {
				sum += data[i*nCh+c]
			}
			mono[i] = sum / nCh
		}
		data = mono
	}

	// Normalize to float32
	scale := float32(1.0 / math.Pow(2, float64(bitDepth-1)))
	samples := make([]float32, len(data))
	for i, s := range data {
		samples[i] = float32(s) * scale
	}

	// Resample if needed
	if sr != targetSR {
		samples = resampleLinear(samples, sr, targetSR)
	}
	return samples, nil
}

func resampleLinear(samples []float32, srcRate, dstRate int) []float32 {
	ratio := float64(srcRate) / float64(dstRate)
	outLen := int(float64(len(samples)) / ratio)
	out := make([]float32, outLen)
	for i := 0; i < outLen; i++ {
		srcIdx := float64(i) * ratio
		idx := int(srcIdx)
		frac := float32(srcIdx - float64(idx))
		if idx+1 < len(samples) {
			out[i] = samples[idx]*(1-frac) + samples[idx+1]*frac
		} else if idx < len(samples) {
			out[i] = samples[idx]
		}
	}
	return out
}

// textToPinyin converts Chinese text to pinyin (no tones, space-separated).
func textToPinyin(text string) string {
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal

	var parts []string
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			result := pinyin.SinglePinyin(r, args)
			if len(result) > 0 {
				parts = append(parts, result[0])
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			parts = append(parts, string(r))
		}
	}

	result := strings.Join(parts, " ")
	re := regexp.MustCompile(`\s+`)
	result = re.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

func configureOnnxRuntime() error {
	dllPath, err := locateOnnxRuntimeDll()
	if err != nil {
		return err
	}
	ort.SetSharedLibraryPath(dllPath)
	return nil
}

func tryAppendCuda(opts *ort.SessionOptions) {
	dllPath, err := locateOnnxRuntimeDll()
	if err != nil {
		return
	}
	cudaDll := filepath.Join(filepath.Dir(dllPath), "onnxruntime_providers_cuda.dll")
	if _, err := os.Stat(cudaDll); err == nil {
		_ = opts.AppendExecutionProviderCUDA(nil)
	}
}

func locateOnnxRuntimeDll() (string, error) {
	if envPath := strings.TrimSpace(os.Getenv("ORT_DLL_PATH")); envPath != "" {
		if _, err := os.Stat(envPath); err != nil {
			return "", fmt.Errorf("ORT_DLL_PATH 指向的文件不存在: %s", envPath)
		}
		return envPath, nil
	}

	var roots []string
	addRoot := func(p string) {
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		for _, r := range roots {
			if r == p {
				return
			}
		}
		roots = append(roots, p)
	}

	if exe, err := os.Executable(); err == nil && exe != "" {
		exeDir := filepath.Dir(exe)
		addRoot(exeDir)
		addRoot(filepath.Dir(exeDir))
		addRoot(filepath.Dir(filepath.Dir(exeDir)))
	}
	if cwd, err := os.Getwd(); err == nil {
		addRoot(cwd)
		addRoot(filepath.Dir(cwd))
		addRoot(filepath.Dir(filepath.Dir(cwd)))
	}

	candidates := []string{
		filepath.Join("Microsoft.ML.OnnxRuntime.DirectML.1.21.0", "runtimes", "win-x64", "native", "onnxruntime.dll"),
		filepath.Join("onnxruntime-win-x64-directml-1.24.4", "lib", "onnxruntime.dll"),
		filepath.Join("onnxruntime-win-x64-gpu-1.24.4", "lib", "onnxruntime.dll"),
		filepath.Join("onnxruntime-win-x64-gpu-1.24.3", "lib", "onnxruntime.dll"),
		filepath.Join("onnxruntime-win-x64-1.24.3", "lib", "onnxruntime.dll"),
		filepath.Join("onnxruntime-win-x64-1.19.0", "lib", "onnxruntime.dll"),
		"onnxruntime.dll",
	}

	for _, root := range roots {
		for _, rel := range candidates {
			full := filepath.Join(root, rel)
			if _, err := os.Stat(full); err == nil {
				return full, nil
			}
		}
	}

	return "", fmt.Errorf("未找到 onnxruntime.dll，请将 onnxruntime-win-x64-1.24.3\\lib 放在可执行文件附近或设置 ORT_DLL_PATH")
}

// resolvePythonDir 定位 Python ASR 目录（含 main.py 和 .venv）。
func (a *ASRInferencer) resolvePythonDir() (string, error) {
	checkDir := func(dir string) bool {
		_, e1 := os.Stat(filepath.Join(dir, "main.py"))
		_, e2 := os.Stat(filepath.Join(dir, ".venv"))
		return e1 == nil && e2 == nil
	}
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cwd, _ := os.Getwd()
	for _, root := range []string{exeDir, filepath.Dir(exeDir), cwd, filepath.Dir(cwd)} {
		candidate := filepath.Join(root, "qwen3asrinfer")
		if checkDir(candidate) {
			return filepath.Abs(candidate)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			c := filepath.Join(root, e.Name())
			if checkDir(c) {
				return filepath.Abs(c)
			}
		}
	}
	return "", fmt.Errorf("未找到 Python ASR 目录（需含 main.py 和 .venv）")
}

// transcribePython 通过子进程调用 Python ASR 推理。
func (a *ASRInferencer) transcribePython(pyDir, inputDir, outputDir string) (*ASRResponse, error) {
	// 找 Python 可执行文件（.venv 内）
	pythonExe := filepath.Join(pyDir, ".venv", "Scripts", "python.exe")
	if _, err := os.Stat(pythonExe); err != nil {
		pythonExe = filepath.Join(pyDir, ".venv", "bin", "python")
		if _, err := os.Stat(pythonExe); err != nil {
			return nil, fmt.Errorf("找不到 .venv Python: %s", pyDir)
		}
	}

	// 写结果到临时 JSON 文件
	tmpJSON, err := os.CreateTemp("", "asr_result_*.json")
	if err != nil {
		return nil, err
	}
	tmpJSONPath := tmpJSON.Name()
	tmpJSON.Close()
	defer os.Remove(tmpJSONPath)

	mainPy := filepath.Join(pyDir, "main.py")
	cfg := a.config

	args := []string{mainPy,
		"--input", inputDir,
		"--output", outputDir,
		"--output-json", tmpJSONPath,
	}
	if cfg.Language != "" {
		args = append(args, "--language", cfg.Language)
	}
	if cfg.Dtype != "" {
		args = append(args, "--dtype", cfg.Dtype)
	}
	if cfg.Device != "" {
		args = append(args, "--device", cfg.Device)
	}
	// 查找模型目录（Qwen3-ASR-* 子目录）
	modelDir := cfg.ModelDir
	if modelDir == "" {
		entries, _ := os.ReadDir(pyDir)
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "qwen3-asr") {
				modelDir = filepath.Join(pyDir, e.Name())
				break
			}
		}
	}
	if modelDir != "" {
		args = append(args, "--model", modelDir)
	}

	cmd := exec.Command(pythonExe, args...)
	cmd.Dir = pyDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Python ASR 执行失败: %v\n%s", err, strings.TrimSpace(string(out)))
	}

	// 读取结果 JSON
	data, err := os.ReadFile(tmpJSONPath)
	if err != nil {
		return nil, fmt.Errorf("读取 Python ASR 结果失败: %w", err)
	}

	// Python 返回的 JSON 结构和 ASRResponse 兼容
	var resp ASRResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析 Python ASR 结果失败: %w", err)
	}

	// 写 summary（Python 已写，但格式可能不同，这里统一）
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "output"
	}
	summaryPath := filepath.Join(outputDir, prefix+"_summary.txt")
	writeSummaryFile(summaryPath, resp.Results)

	return &resp, nil
}
