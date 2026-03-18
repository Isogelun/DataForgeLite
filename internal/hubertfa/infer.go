package hubertfa

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// FAConfig holds all parameters for a forced-alignment run.
type FAConfig struct {
	ModelDir           string
	InputDir           string
	OutputDir          string
	Language           string
	G2PType            string // "dictionary" or "phoneme"
	DictionaryPath     string // optional override
	NonLexicalPhonemes string // comma-separated, e.g. "AP,EP"
	PadTimes           int
	PadLength          int
	Quiet              bool
}

// FAResponse is the JSON-serialisable result.
type FAResponse struct {
	Success      bool   `json:"success"`
	SuccessCount int    `json:"success_count,omitempty"`
	ErrorCount   int    `json:"error_count,omitempty"`
	Error        string `json:"error,omitempty"`
	OutputPath   string `json:"output_path,omitempty"`
}

type datasetItem struct {
	WavPath        string
	PhSeq          []string
	WordSeq        []string
	PhIdxToWordIdx []int
}

// RunFA executes the full forced-alignment pipeline.
func RunFA(cfg *FAConfig) (*FAResponse, error) {
	logf := func(format string, args ...interface{}) {
		if !cfg.Quiet {
			fmt.Printf(format, args...)
		}
	}

	// Resolve model files
	paths, err := ResolveModelPaths(cfg.ModelDir)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}
	if err := CheckVersion(paths.VersionPath); err != nil {
		return nil, err
	}

	modelCfg, err := LoadConfig(paths.ConfigPath)
	if err != nil {
		return nil, err
	}
	vocab, err := LoadVocab(paths.VocabPath)
	if err != nil {
		return nil, err
	}

	melCfg := modelCfg.MelSpec

	// Determine language prefix
	language := cfg.Language
	if !vocab.LanguagePrefix {
		language = ""
	}

	// G2P
	g2p, err := NewG2P(cfg.G2PType, language, cfg.DictionaryPath, vocab, cfg.ModelDir)
	if err != nil {
		return nil, fmt.Errorf("init g2p: %w", err)
	}

	// Scan dataset
	wavDir := cfg.InputDir
	dataset, scanErr := scanDataset(wavDir, g2p)
	if scanErr != nil {
		return nil, fmt.Errorf("scan dataset: %w", scanErr)
	}
	if len(dataset) == 0 {
		return &FAResponse{Success: false, Error: "no valid wav+lab pairs found"}, nil
	}
	logf("Loaded %d samples.\n", len(dataset))

	// Init ONNX Runtime
	if err := configureOnnxRuntime(); err != nil {
		return nil, err
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("init onnx runtime: %w", err)
	}
	defer ort.DestroyEnvironment()

	session, err := createOnnxSession(paths.OnnxPath)
	if err != nil {
		return nil, fmt.Errorf("create onnx session: %w", err)
	}
	defer session.Destroy()

	// Create decoders
	alignDec := NewAlignmentDecoder(vocab, melCfg.SampleRate, melCfg.HopSize)
	nllDec := NewNonLexicalDecoder(vocab, melCfg.SampleRate, melCfg.HopSize)

	nlPhonemes := parseNonLexical(cfg.NonLexicalPhonemes)

	padTimes := cfg.PadTimes
	if padTimes < 1 {
		padTimes = 1
	}
	padLength := cfg.PadLength
	if padLength < 0 {
		padLength = 5
	}

	os.MkdirAll(cfg.OutputDir, 0755)

	successCount := 0
	errorCount := 0

	for idx, item := range dataset {
		logf("[%d/%d] Processing: %s\n", idx+1, len(dataset), filepath.Base(item.WavPath))

		wav, err := LoadAndResample(item.WavPath, melCfg.SampleRate)
		if err != nil {
			logf("  Error loading audio: %v\n", err)
			errorCount++
			continue
		}
		wavLength := float64(len(wav)) / float64(melCfg.SampleRate)

		padLengths := computePadLengths(padLength, padTimes)

		var allWords []*WordList
		for _, pl := range padLengths {
			paddedSamples := int(pl * float64(melCfg.SampleRate))
			paddedFrames := paddedSamples / melCfg.HopSize
			paddedWav := PadWav(wav, paddedSamples)

			phFrameLogits, phEdgeLogits, cvntLogits, err := runOnnx(session, paddedWav, vocab.VocabSize, len(nllDec.ClassNames))
			if err != nil {
				logf("  ONNX inference error: %v\n", err)
				errorCount++
				continue
			}

			// Trim padded frames from outputs
			trimPhFrame := trimLogits3D(phFrameLogits, paddedFrames)
			trimPhEdge := trimLogits1D(phEdgeLogits, paddedFrames)
			trimCvnt := trimLogits3D(cvntLogits, paddedFrames)

			result := alignDec.Decode(trimPhFrame, trimPhEdge, wavLength, item.PhSeq, item.WordSeq, item.PhIdxToWordIdx)
			nlResults := nllDec.Decode(trimCvnt, wavLength, nlPhonemes)

			for _, nlWl := range nlResults {
				for _, w := range nlWl.Words {
					result.Words.AddAP(w, 0.1)
				}
			}
			result.Words.ClearLanguagePrefix()
			allWords = append(allWords, result.Words)
		}

		if len(allWords) == 0 {
			errorCount++
			continue
		}

		// Find best duplicate set
		phLists := make([][]string, len(allWords))
		for i, wl := range allWords {
			phLists[i] = wl.PhonemeTexts()
		}
		indices := findDuplicatePhonemes(phLists)
		selected := make([]*WordList, len(indices))
		for i, idx := range indices {
			selected[i] = allWords[idx]
		}

		// Average timestamps, remove outliers
		finalWords := averageWordLists(selected, wavLength)

		baseName := strings.TrimSuffix(filepath.Base(item.WavPath), filepath.Ext(item.WavPath))
		if err := WriteTextGrid(cfg.OutputDir, baseName, wavLength, finalWords); err != nil {
			logf("  Error writing TextGrid: %v\n", err)
			errorCount++
			continue
		}
		successCount++
	}

	logf("Done. Success: %d, Errors: %d\n", successCount, errorCount)
	return &FAResponse{
		Success:      true,
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		OutputPath:   cfg.OutputDir,
	}, nil
}

func scanDataset(wavDir string, g2p G2P) ([]datasetItem, error) {
	var items []datasetItem
	entries, err := filepath.Glob(filepath.Join(wavDir, "*.wav"))
	if err != nil {
		return nil, err
	}
	// Also search subdirectories
	subEntries, _ := filepath.Glob(filepath.Join(wavDir, "**", "*.wav"))
	entries = append(entries, subEntries...)
	sort.Strings(entries)

	seen := make(map[string]bool)
	for _, wavPath := range entries {
		abs, _ := filepath.Abs(wavPath)
		if seen[abs] {
			continue
		}
		seen[abs] = true

		labPath := strings.TrimSuffix(wavPath, filepath.Ext(wavPath)) + ".lab"
		if _, err := os.Stat(labPath); os.IsNotExist(err) {
			continue
		}
		labData, err := os.ReadFile(labPath)
		if err != nil {
			continue
		}
		labText := strings.TrimSpace(string(labData))
		if labText == "" {
			continue
		}
		result, err := g2p.Convert(labText)
		if err != nil {
			fmt.Printf("  G2P error for %s: %v\n", filepath.Base(wavPath), err)
			continue
		}
		items = append(items, datasetItem{
			WavPath:        wavPath,
			PhSeq:          result.PhSeq,
			WordSeq:        result.WordSeq,
			PhIdxToWordIdx: result.PhIdxToWordIdx,
		})
	}
	return items, nil
}

func parseNonLexical(s string) []string {
	var result []string
	for _, ph := range strings.Split(s, ",") {
		ph = strings.TrimSpace(ph)
		if ph != "" {
			result = append(result, ph)
		}
	}
	return result
}

func computePadLengths(padLength, padTimes int) []float64 {
	if padTimes <= 1 {
		return []float64{0}
	}
	lengths := make([]float64, padTimes)
	for i := 0; i < padTimes; i++ {
		lengths[i] = math.Round(float64(padLength)/float64(padTimes)*float64(i)*10) / 10
	}
	return lengths
}

// ---- ONNX helpers ----

func createOnnxSession(onnxPath string) (*ort.DynamicAdvancedSession, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)

	device := strings.ToLower(strings.TrimSpace(os.Getenv("ORT_DEVICE")))
	if device == "dml" {
		tryAppendDML(opts)
	} else {
		tryAppendCuda(opts)
	}

	inputNames := []string{"waveform"}
	outputNames := []string{"ph_frame_logits", "ph_edge_logits", "cvnt_logits"}

	session, err := ort.NewDynamicAdvancedSession(onnxPath, inputNames, outputNames, opts)
	if err != nil {
		return nil, fmt.Errorf("load onnx model: %w", err)
	}
	return session, nil
}

func runOnnx(session *ort.DynamicAdvancedSession, wav []float32, vocabSize, numClasses int) (
	phFrameLogits [][]float32, phEdgeLogits []float32, cvntLogits [][]float32, err error,
) {
	inputShape := ort.NewShape(1, int64(len(wav)))
	inputTensor, err := ort.NewTensor(inputShape, wav)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	// Prepare output slice with nil entries to let the runtime auto-allocate
	outputs := []ort.Value{nil, nil, nil}

	err = session.Run([]ort.Value{inputTensor}, outputs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("run onnx: %w", err)
	}

	// Parse outputs - the runtime allocated these, we must destroy them
	phFrameOut, ok0 := outputs[0].(*ort.Tensor[float32])
	phEdgeOut, ok1 := outputs[1].(*ort.Tensor[float32])
	cvntOut, ok2 := outputs[2].(*ort.Tensor[float32])
	if !ok0 || !ok1 || !ok2 {
		// Clean up any successfully cast tensors
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
		return nil, nil, nil, fmt.Errorf("unexpected output tensor types")
	}

	// ph_frame_logits: [1, vocabSize, T]
	phFrameShape := phFrameOut.GetShape()
	T := int(phFrameShape[2])
	phFrameData := phFrameOut.GetData()
	phFrameLogits = make([][]float32, vocabSize)
	for v := 0; v < vocabSize; v++ {
		phFrameLogits[v] = make([]float32, T)
		for t := 0; t < T; t++ {
			phFrameLogits[v][t] = phFrameData[v*T+t]
		}
	}

	// ph_edge_logits: [1, T]
	phEdgeData := phEdgeOut.GetData()
	phEdgeLogits = make([]float32, T)
	copy(phEdgeLogits, phEdgeData[:T])

	// cvnt_logits: [1, numClasses, T]
	cvntData := cvntOut.GetData()
	cvntLogits = make([][]float32, numClasses)
	for c := 0; c < numClasses; c++ {
		cvntLogits[c] = make([]float32, T)
		for t := 0; t < T; t++ {
			cvntLogits[c][t] = cvntData[c*T+t]
		}
	}

	phFrameOut.Destroy()
	phEdgeOut.Destroy()
	cvntOut.Destroy()

	return phFrameLogits, phEdgeLogits, cvntLogits, nil
}

func trimLogits3D(logits [][]float32, paddedFrames int) [][]float32 {
	if paddedFrames <= 0 {
		return logits
	}
	result := make([][]float32, len(logits))
	for i, row := range logits {
		if paddedFrames < len(row) {
			result[i] = row[paddedFrames:]
		} else {
			result[i] = nil
		}
	}
	return result
}

func trimLogits1D(logits []float32, paddedFrames int) []float32 {
	if paddedFrames <= 0 || paddedFrames >= len(logits) {
		return logits
	}
	return logits[paddedFrames:]
}

// ---- Multi-pad averaging ----

func findDuplicatePhonemes(phLists [][]string) []int {
	if len(phLists) == 1 {
		return []int{0}
	}
	type key struct{ s string }
	groups := make(map[string][]int)
	for i, phs := range phLists {
		k := strings.Join(phs, "|")
		groups[k] = append(groups[k], i)
	}

	var bestKey string
	bestCount := 0
	bestLen := 0
	for k, indices := range groups {
		if len(indices) > bestCount || (len(indices) == bestCount && len(k) > bestLen) {
			bestKey = k
			bestCount = len(indices)
			bestLen = len(k)
		}
	}
	if bestCount > 1 {
		return groups[bestKey]
	}
	return []int{0}
}

func averageWordLists(selected []*WordList, wavLength float64) *WordList {
	if len(selected) == 1 {
		wl := selected[0]
		wl.FillSmallGaps(wavLength, 0.1)
		wl.AddSP(wavLength)
		return wl
	}

	ref := selected[0]
	result := NewWordList()
	var allPhonemes []Phoneme

	for wIdx := 0; wIdx < ref.Len(); wIdx++ {
		refWord := ref.Get(wIdx)
		var phonemes []Phoneme
		for phIdx := 0; phIdx < len(refWord.Phonemes); phIdx++ {
			var starts, ends []float64
			for _, wl := range selected {
				if wIdx < wl.Len() && phIdx < len(wl.Get(wIdx).Phonemes) {
					starts = append(starts, wl.Get(wIdx).Phonemes[phIdx].Start)
					ends = append(ends, wl.Get(wIdx).Phonemes[phIdx].End)
				}
			}
			phStart := medianF64(starts)
			phEnd := medianF64(ends)
			if len(allPhonemes) > 0 {
				phStart = math.Max(phStart, allPhonemes[len(allPhonemes)-1].End)
			}
			phEnd = math.Max(phStart+0.0001, phEnd)

			ph, err := NewPhoneme(phStart, phEnd, refWord.Phonemes[phIdx].Text)
			if err != nil {
				continue
			}
			phonemes = append(phonemes, ph)
			allPhonemes = append(allPhonemes, ph)
		}
		if len(phonemes) == 0 {
			continue
		}
		w, err := NewWord(phonemes[0].Start, phonemes[len(phonemes)-1].End, refWord.Text, false)
		if err != nil {
			continue
		}
		for _, ph := range phonemes {
			w.Phonemes = append(w.Phonemes, ph)
		}
		result.Append(w)
	}

	result.FillSmallGaps(wavLength, 0.1)
	result.AddSP(wavLength)
	return result
}

func medianF64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
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

func tryAppendDML(opts *ort.SessionOptions) bool {
	deviceID := 0
	if v := strings.TrimSpace(os.Getenv("ORT_DML_DEVICE_ID")); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &deviceID); n != 1 || err != nil {
			deviceID = 0
		}
	}
	return opts.AppendExecutionProviderDirectML(deviceID) == nil
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
