package main

import (
	asrinfer "DataForgeLite/internal/QwenASR"
	"DataForgeLite/internal/addphnum"
	"DataForgeLite/internal/audiopreprocessor"
	"DataForgeLite/internal/audiosplitter"
	"DataForgeLite/internal/exporter"
	"DataForgeLite/internal/gameonnx"
	"DataForgeLite/internal/hubertfa"
	"DataForgeLite/internal/tgannotation"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// jsonOutput 供客户端解析的 JSON 输出（ASR / AddPhNum 等统一用可扩展结构）
type jsonOutput struct {
	Success      bool        `json:"success"`
	SuccessCount int         `json:"success_count,omitempty"`
	ErrorCount   int         `json:"error_count,omitempty"`
	Error        string      `json:"error,omitempty"`
	OutputPath   string      `json:"output_path,omitempty"`
	Results      interface{} `json:"results,omitempty"`
}

func main() {
	// 命令行参数解析
	asrMode := flag.Bool("asr", false, "Run ASR mode")
	gameMode := flag.Bool("game", false, "Run GAME ONNX 推理模式（音符/边界推理）")
	addPhNumMode := flag.Bool("addphnum", false, "Run AddPhNum mode")
	outputJSON := flag.Bool("json", false, "Output result as single JSON line (for DataForgeLiteClient)")
	inputDir := flag.String("input", "", "Input directory for ASR")
	outputDir := flag.String("output", "", "Output directory for ASR")
	asrLanguage := flag.String("language", "", "ASR 语言: 空=默认中文, auto=自动检测, 或 Chinese/English 等")
	asrBackend := flag.String("backend", "qwen", "ASR 后端: qwen=官方 PyTorch, gguf=Qwen3-ASR-GGUF(ONNX+llama.cpp，更快)")
	gameModelDir := flag.String("game-model-dir", gameonnx.DefaultRunOptions().ModelDir, "GAME 模型目录（含 encoder.onnx segmenter.onnx estimator.onnx dur2bd.onnx bd2dur.onnx config.json）")
	gameWav := flag.String("game-wav", "", "GAME 输入 WAV 文件（建议单声道，采样率需匹配模型）")
	gameInputDir := flag.String("game-input-dir", "", "GAME 输入目录：批量推理目录下所有 WAV（与 --game-wav 二选一）")
	gameOutJSON := flag.String("game-out", "", "GAME 批量推理输出 JSON 路径（可选；为空则只输出到 stdout）")
	gameOrt := flag.String("game-ort", "", "onnxruntime 共享库路径（Windows: onnxruntime.dll）")
	gameLang := flag.String("game-lang", gameonnx.DefaultRunOptions().Lang, "GAME 语言代码（如 zh/en/ja/yue）")
	gameSegThreshold := flag.Float64("game-seg-threshold", float64(gameonnx.DefaultRunOptions().SegThreshold), "GAME 边界解码阈值（推荐 0.2）")
	gameSegRadius := flag.Int("game-seg-radius", int(gameonnx.DefaultRunOptions().SegRadius), "GAME 边界解码半径（帧数，推荐 2）")
	gameT0 := flag.Float64("game-t0", float64(gameonnx.DefaultRunOptions().T0), "GAME D3PM 起始 t0")
	gameNSteps := flag.Int("game-nsteps", gameonnx.DefaultRunOptions().NSteps, "GAME D3PM 步数")
	gameEstThreshold := flag.Float64("game-est-threshold", float64(gameonnx.DefaultRunOptions().EstThreshold), "GAME 音符存在阈值（推荐 0.2）")
	gameAlignMode := flag.Bool("game-align", false, "Run GAME align：读取 DiffSinger transcriptions CSV，写回 note_seq/note_dur")
	gameAlignCSVIn := flag.String("game-align-in", "", "GAME align 输入 CSV（DiffSinger transcriptions）")
	gameAlignCSVOut := flag.String("game-align-out", "", "GAME align 输出 CSV（写入 note_seq/note_dur）")
	gameAlignWavDir := flag.String("game-align-wavs", "", "GAME align wavs 目录（可选；默认: 输入CSV同级/wavs）")
	gameAlignLang := flag.String("game-align-lang", "", "GAME align 语言；空=与 infer.py align 无 -l 一致(language_id=0)，勿填 zh 除非 Python 也用了 -l zh")
	inputCSV := flag.String("input-csv", "", "Input CSV file for AddPhNum")
	outputCSV := flag.String("output-csv", "", "Output CSV file for AddPhNum")
	dictPath := flag.String("dict", "", "Dictionary file path for AddPhNum")
	phSeqCol := flag.String("ph-col", "ph_seq", "Phoneme sequence column name")
	exportMode := flag.Bool("export", false, "Run export dataset mode")
	wavsDir := flag.String("wavs-dir", "", "WAV files directory for export")
	tgDir := flag.String("tg-dir", "", "TextGrid files directory for export")
	exportOutDir := flag.String("export-out", "", "Output directory for export")
	preprocessMode := flag.Bool("preprocess", false, "Run audio preprocess mode")
	preprocessInput := flag.String("preprocess-input", "", "Input directory for preprocess")
	preprocessOutput := flag.String("preprocess-output", "", "Output directory for preprocess")
	targetLufs := flag.Float64("target-lufs", -18, "目标响度 LUFS，如 -18")
	truePeakLimit := flag.Float64("true-peak-limit", -1, "最大真峰 dB，如 -1，防止爆音")
	splitMode := flag.Bool("split", false, "Run audio split mode")
	splitInput := flag.String("split-input", "", "Input directory for split")
	splitOutput := flag.String("split-output", "", "Output directory for split")
	faMode := flag.Bool("fa", false, "Run forced-alignment mode")
	faModelDir := flag.String("model-dir", "", "HubertFA model directory (contains model.onnx, config.json, vocab.json)")
	faLanguage := flag.String("fa-language", "zh", "FA language (zh, en, ja, ko, yue)")
	faNonLex := flag.String("non-lexical", "AP,EP", "Non-lexical phonemes, comma-separated")
	faG2P := flag.String("g2p", "dictionary", "G2P type: dictionary or phoneme")
	faDictPath := flag.String("dictionary", "", "Custom dictionary path (optional)")
	faPadTimes := flag.Int("pad-times", 1, "Number of pad iterations for FA")
	faPadLength := flag.Int("pad-length", 5, "Max pad length in seconds for FA")
	showHelp := flag.Bool("help", false, "Show help")
	combineMode := flag.Bool("combine", false, "Combine segmented TextGrids and WAVs into 3-tier TextGrids")
	combineWavs := flag.String("combine-wavs", "", "WAV directory for combine")
	combineTg := flag.String("combine-tg", "", "TextGrid directory for combine (defaults to wavs dir)")
	combineOut := flag.String("combine-out", "", "Output directory for combine (used when wavs-out/tg-out not set)")
	combineWavsOut := flag.String("combine-wavs-out", "", "WAV output directory for combine (overrides combine-out)")
	combineTgOut := flag.String("combine-tg-out", "", "TextGrid output directory for combine (overrides combine-out)")
	combineSuffix := flag.String("combine-suffix", `_\d+`, "Filename suffix pattern for combine")
	combineOverwrite := flag.Bool("combine-overwrite", false, "Overwrite existing files in combine")
	sliceMode := flag.Bool("slice-tg", false, "Slice 3-tier TextGrids and WAVs into segments")
	sliceIn := flag.String("slice-in", "", "Input parent directory for slice (contains wavs/ and TextGrid/)")
	sliceOut := flag.String("slice-out", "", "Output parent directory for slice")
	sliceDigits := flag.Int("slice-digits", 3, "Number of suffix digits for slice")
	slicePreserveName := flag.Bool("slice-preserve-name", false, "Use sentence marks as filenames")
	sliceOverwrite := flag.Bool("slice-overwrite", false, "Overwrite existing files in slice")

	// 避免参数错误时向 stderr 刷屏（WPF 会捕获并显示），仅输出简短提示
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法: DataForgeLite.exe --asr|--preprocess|--split|--export|--addphnum ... 详细参数请加 --help")
	}
	flag.Parse()

	// 显示帮助信息
	if *showHelp || len(os.Args) == 1 {
		fmt.Println("DataForgeLite - 数据处理和语音识别工具")
		fmt.Println("")
		fmt.Println("用法:")
		fmt.Println("  DataForgeLite.exe --asr --input <输入目录> --output <输出目录> [--json]")
		fmt.Println("  DataForgeLite.exe --game --game-wav <输入wav> --game-ort <onnxruntime.dll路径> [--game-model-dir <模型目录>] [--json]")
		fmt.Println("  DataForgeLite.exe --game --game-input-dir <输入目录> --game-ort <onnxruntime.dll路径> [--game-model-dir <模型目录>] [--game-out <out.json>] [--json]")
		fmt.Println("  DataForgeLite.exe --fa --model-dir <模型目录> --input <输入目录> --output <输出目录> [--json]")
		fmt.Println("  DataForgeLite.exe --addphnum --input-csv <输入CSV> --output-csv <输出CSV> --dict <词典文件> [--json]")
		fmt.Println("  DataForgeLite.exe --export --wavs-dir <WAV目录> --tg-dir <TextGrid目录> --export-out <输出目录> [--json]")
		fmt.Println("  DataForgeLite.exe --preprocess --preprocess-input <输入目录> --preprocess-output <输出目录> [--json]")
		fmt.Println("  DataForgeLite.exe --split --split-input <输入目录> --split-output <输出目录> [--json]")
		fmt.Println("")
		fmt.Println("模式:")
		fmt.Println("  --asr           启用ASR模式（语音识别）")
		fmt.Println("  --game          启用 GAME ONNX 推理模式（输出音符/边界推理结果 JSON）")
		fmt.Println("  --fa            启用强制对齐模式（Forced Alignment）")
		fmt.Println("  --addphnum      启用AddPhNum模式（计算音素数量）")
		fmt.Println("  --export        导出数据集（WAV+TextGrid → 数据集目录）")
		fmt.Println("  --preprocess    音频预处理（响度/重采样/单声道）")
		fmt.Println("  --split          音频智能切分（VAD 切片）")
		fmt.Println("  --json          输出一行 JSON 供客户端解析（与 --asr/--fa/--addphnum/--export 同用）")
		fmt.Println("")
		fmt.Println("ASR模式参数:")
		fmt.Println("  --input         输入音频文件夹路径")
		fmt.Println("  --output        输出结果文件夹路径")
		fmt.Println("  --language      ASR 语言: 空=中文, auto=自动检测, 或 Chinese/English 等")
		fmt.Println("  --backend       ASR 后端（当前仅支持 gguf，纯 Go 实现，无 Python）")
		fmt.Println("")
		fmt.Println("GAME模式参数:")
		fmt.Println("  --game-wav           输入 WAV 文件")
		fmt.Println("  --game-input-dir     输入目录（批量推理目录下所有 WAV）")
		fmt.Println("  --game-ort           onnxruntime.dll 路径")
		fmt.Println("  --game-model-dir     模型目录（含 encoder.onnx/segmenter.onnx/estimator.onnx/dur2bd.onnx/bd2dur.onnx/config.json）")
		fmt.Println("  --game-out           批量推理输出 JSON 路径（可选）")
		fmt.Println("  --game-lang          语言代码（如 zh/en/ja/yue）")
		fmt.Println("  --game-seg-threshold 边界阈值（默认 0.2）")
		fmt.Println("  --game-seg-radius    边界半径（默认 2）")
		fmt.Println("  --game-t0            D3PM 起始 t0（默认 0）")
		fmt.Println("  --game-nsteps        D3PM 步数（默认 8）")
		fmt.Println("  --game-est-threshold 音符存在阈值（默认 0.2）")
		fmt.Println("")
		fmt.Println("AddPhNum模式参数:")
		fmt.Println("  --input-csv     输入CSV文件路径")
		fmt.Println("  --output-csv    输出CSV文件路径")
		fmt.Println("  --dict          音素词典文件路径")
		fmt.Println("  --ph-col        音素序列列名（默认: ph_seq）")
		fmt.Println("")
		fmt.Println("导出模式参数:")
		fmt.Println("  --wavs-dir      WAV 文件目录")
		fmt.Println("  --tg-dir        TextGrid 文件目录")
		fmt.Println("  --export-out    导出输出目录")
		fmt.Println("")
		fmt.Println("")
		fmt.Println("强制对齐模式参数:")
		fmt.Println("  --model-dir     HubertFA 模型目录（含 model.onnx, config.json, vocab.json）")
		fmt.Println("  --input         输入目录（含 .wav 与同名 .lab 文件）")
		fmt.Println("  --output        TextGrid 输出目录")
		fmt.Println("  --fa-language   语言: zh, en, ja, ko, yue（默认 zh）")
		fmt.Println("  --non-lexical   非词汇音素，逗号分隔（默认 AP,EP）")
		fmt.Println("  --g2p           G2P 类型: dictionary 或 phoneme（默认 dictionary）")
		fmt.Println("  --dictionary    自定义词典路径（可选）")
		fmt.Println("")
		fmt.Println("预处理/切分参数:")
		fmt.Println("  --preprocess-input   预处理输入目录")
		fmt.Println("  --preprocess-output  预处理输出目录")
		fmt.Println("  --target-lufs        目标响度 LUFS（默认 -18）")
		fmt.Println("  --true-peak-limit    最大真峰 dB（默认 -1，防爆音）")
		fmt.Println("  --split-input        切分输入目录")
		fmt.Println("  --split-output       切分输出目录")
		fmt.Println("")
		fmt.Println("示例:")
		fmt.Println("  DataForgeLite.exe --asr --input ./audio --output ./results")
		fmt.Println("  DataForgeLite.exe --asr --input ./audio --output ./results --json")
		fmt.Println("  DataForgeLite.exe --addphnum --input-csv input.csv --output-csv output.csv --dict dict.txt")
		os.Exit(0)
	}

	// 如果是ASR模式，执行ASR并退出
	if *asrMode {
		runASRMode(*inputDir, *outputDir, *asrLanguage, *asrBackend, *outputJSON)
		return
	}

	// 如果是 GAME 模式，执行推理并退出
	if *gameMode {
		runGameMode(
			*gameModelDir,
			*gameWav,
			*gameInputDir,
			*gameOutJSON,
			*gameOrt,
			*gameLang,
			float32(*gameSegThreshold),
			int64(*gameSegRadius),
			float32(*gameT0),
			*gameNSteps,
			float32(*gameEstThreshold),
			*outputJSON,
		)
		return
	}

	if *gameAlignMode {
		runGameAlignMode(
			*gameModelDir,
			*gameOrt,
			*gameAlignLang,
			float32(*gameSegThreshold),
			int64(*gameSegRadius),
			float32(*gameEstThreshold),
			*gameAlignCSVIn,
			*gameAlignCSVOut,
			*gameAlignWavDir,
			*outputJSON,
		)
		return
	}

	// 如果是FA模式，执行强制对齐并退出
	if *faMode {
		runFAMode(*faModelDir, *inputDir, *outputDir, *faLanguage, *faNonLex, *faG2P, *faDictPath, *faPadTimes, *faPadLength, *outputJSON)
		return
	}

	// 如果是AddPhNum模式，执行音素数量计算并退出
	if *addPhNumMode {
		runAddPhNumMode(*inputCSV, *outputCSV, *dictPath, *phSeqCol, *outputJSON)
		return
	}

	// 如果是导出模式
	if *exportMode {
		runExportMode(*wavsDir, *tgDir, *exportOutDir, *outputJSON)
		return
	}

	// 如果是预处理模式
	if *preprocessMode {
		runPreprocessMode(*preprocessInput, *preprocessOutput, *targetLufs, *truePeakLimit, *outputJSON)
		return
	}

	// 如果是切分模式
	if *splitMode {
		runSplitMode(*splitInput, *splitOutput, *outputJSON)
		return
	}

	// 如果是合并标注模式
	if *combineMode {
		runCombineMode(*combineWavs, *combineTg, *combineOut, *combineWavsOut, *combineTgOut, *combineSuffix, *combineOverwrite, *outputJSON)
		return
	}

	// 如果是拆分标注模式
	if *sliceMode {
		runSliceTgMode(*sliceIn, *sliceOut, *sliceDigits, *slicePreserveName, *sliceOverwrite, *outputJSON)
		return
	}

	// 默认显示帮助
	fmt.Println("请使用 --help 查看帮助信息")
	os.Exit(0)
}

func runCombineMode(wavsDir, tgDir, outDir, wavsOutDir, tgOutDir, suffix string, overwrite bool, outputJSON bool) {
	if wavsDir == "" || outDir == "" {
		msg := "缺少参数: --combine-wavs 和 --combine-out 必须提供"
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: msg})
		} else {
			fmt.Fprintln(os.Stderr, "错误: "+msg)
		}
		os.Exit(1)
	}
	cfg := tgannotation.CombineConfig{
		WavsDir:   wavsDir,
		TgDir:     tgDir,
		OutDir:    outDir,
		Suffix:    suffix,
		Overwrite: overwrite,
	}
	if err := tgannotation.Combine(cfg); err != nil {
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "合并失败: %v\n", err)
		}
		os.Exit(1)
	}
	if outputJSON {
		effectiveOut := wavsOutDir
		if effectiveOut == "" {
			effectiveOut = outDir
		}
		_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: true, OutputPath: effectiveOut})
	} else {
		fmt.Println("合并完成")
	}
}

func runSliceTgMode(inDir, outDir string, digits int, preserveName, overwrite bool, outputJSON bool) {
	if inDir == "" || outDir == "" {
		msg := "缺少参数: --slice-in 和 --slice-out 必须提供"
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: msg})
		} else {
			fmt.Fprintln(os.Stderr, "错误: "+msg)
		}
		os.Exit(1)
	}
	cfg := tgannotation.SliceConfig{
		InDir:                 inDir,
		OutDir:                outDir,
		Digits:                digits,
		PreserveSentenceNames: preserveName,
		Overwrite:             overwrite,
	}
	if err := tgannotation.Slice(cfg); err != nil {
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "拆分失败: %v\n", err)
		}
		os.Exit(1)
	}
	if outputJSON {
		_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: true, OutputPath: outDir})
	} else {
		fmt.Println("拆分完成: " + outDir)
	}
}

func runASRMode(inputDir, outputDir, language, backend string, outputJSON bool) {
	if inputDir == "" || outputDir == "" {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "缺少参数: --input 和 --output 必须提供"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: --input 和 --output 参数必须提供，详细用法请加 --help")
		}
		os.Exit(1)
	}

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "无法创建输出目录: " + err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "错误: 无法创建输出目录: %v\n", err)
		}
		os.Exit(1)
	}

	if !outputJSON {
		fmt.Println("===========================================")
		fmt.Println("  DataForgeLite - Qwen3 ASR 语音识别")
		fmt.Println("===========================================")
		fmt.Printf("输入目录: %s\n", inputDir)
		fmt.Printf("输出目录: %s\n", outputDir)
		fmt.Println("")
		fmt.Println("正在调用Qwen3-ASR模型进行语音识别...")
		fmt.Println("")
	}

	// 创建ASR推理器
	config := asrinfer.DefaultConfig(inputDir, outputDir)
	if language == "auto" {
		config.Language = ""
	} else if language != "" {
		config.Language = language
	}
	if backend == "gguf" {
		config.Backend = "gguf"
	} else if backend == "python" {
		config.Backend = "python"
	}
	inferencer := asrinfer.NewASRInferencer(config)

	response, err := inferencer.Transcribe()
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "ASR执行失败: %v\n", err)
		}
		os.Exit(1)
	}

	if outputJSON {
		out := jsonOutput{
			Success:      response.Success,
			SuccessCount: response.SuccessCount,
			ErrorCount:   response.ErrorCount,
			Error:        response.Error,
			Results:      response.Results,
		}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "输出JSON失败: %v\n", err)
			os.Exit(1)
		}
		if !response.Success {
			os.Exit(1)
		}
		return
	}

	fmt.Println("")
	fmt.Println("===========================================")
	if response.Success {
		fmt.Printf("识别完成！成功: %d, 失败: %d\n", response.SuccessCount, response.ErrorCount)
		fmt.Println("===========================================")
		for _, result := range response.Results {
			if result.Error != "" {
				fmt.Printf("[%s] 错误: %s\n", result.File, result.Error)
			} else {
				fmt.Printf("[%s]\n  -> %s\n", result.File, result.Text)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "识别失败: %s\n", response.Error)
		os.Exit(1)
	}
}

func runGameMode(modelDir, wavPath, inputDir, outJSON, ortLib, lang string, segThreshold float32, segRadius int64, t0 float32, nsteps int, estThreshold float32, outputJSON bool) {
	opt := gameonnx.DefaultRunOptions()
	opt.ModelDir = modelDir
	opt.ORTLib = ortLib
	opt.Lang = lang
	opt.SegThreshold = segThreshold
	opt.SegRadius = segRadius
	opt.T0 = t0
	opt.NSteps = nsteps
	opt.EstThreshold = estThreshold

	// 单文件模式
	if strings.TrimSpace(wavPath) != "" {
		opt.WavPath = wavPath
		res, err := gameonnx.Run(opt)
		if err != nil {
			if outputJSON {
				_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: err.Error()})
			} else {
				fmt.Fprintf(os.Stderr, "GAME 推理失败: %v\n", err)
			}
			os.Exit(1)
		}

		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: true, Results: res})
			return
		}

		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	// 批量目录模式
	if strings.TrimSpace(inputDir) == "" {
		msg := "缺少参数: --game-wav 或 --game-input-dir 必须提供"
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: msg})
		} else {
			fmt.Fprintln(os.Stderr, "GAME 推理失败: "+msg)
		}
		os.Exit(1)
	}

	wavs, err := listWavs(inputDir)
	if err != nil {
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "GAME 推理失败: %v\n", err)
		}
		os.Exit(1)
	}
	type item struct {
		File   string           `json:"file"`
		Result *gameonnx.Result `json:"result,omitempty"`
		Error  string           `json:"error,omitempty"`
	}
	outItems := make([]item, 0, len(wavs))
	okCount := 0
	errCount := 0
	for _, w := range wavs {
		o := opt
		o.WavPath = w
		r, e := gameonnx.Run(o)
		if e != nil {
			errCount++
			outItems = append(outItems, item{File: filepath.Base(w), Error: e.Error()})
			continue
		}
		okCount++
		outItems = append(outItems, item{File: filepath.Base(w), Result: r})
	}

	payload := jsonOutput{
		Success:      errCount == 0,
		SuccessCount: okCount,
		ErrorCount:   errCount,
		Results:      outItems,
		OutputPath:   outJSON,
	}

	line, _ := json.Marshal(payload)
	if outJSON != "" {
		_ = os.WriteFile(outJSON, append(line, '\n'), 0644)
	}
	fmt.Println(string(line))
}

func listWavs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".wav") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func runGameAlignMode(modelDir, ortLib, lang string, segThreshold float32, segRadius int64, estThreshold float32, csvIn, csvOut, wavDir string, outputJSON bool) {
	// 与 infer.py align 的 @shared_options(defaults={t0:0.5, nsteps:4}) 一致（勿用普通 --game 的 t0/nsteps）
	opt := gameonnx.AlignOptions{
		ModelDir:     modelDir,
		ORTLib:       ortLib,
		Lang:         lang,
		SegThreshold: segThreshold,
		SegRadius:    segRadius,
		T0:           0.5,
		NSteps:       4,
		EstThreshold: estThreshold,
		CSVIn:        csvIn,
		CSVOut:       csvOut,
		WavDir:       wavDir,
	}
	ok, fail, err := gameonnx.Align(opt)
	if err != nil {
		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: false, Error: err.Error(), SuccessCount: ok, ErrorCount: fail})
		} else {
			fmt.Fprintf(os.Stderr, "GAME align 失败: %v\n", err)
		}
		os.Exit(1)
	}
	// 只要写出过 CSV 且至少一行成功，即视为成功（与 infer.py 部分条目失败仍可出结果一致）
	success := ok > 0
	var errMsg string
	if ok == 0 && fail > 0 {
		success = false
		errMsg = fmt.Sprintf("全部 %d 行未成功（请检查 wavs 下是否有对应 .wav、ph_num 与 ph_dur 是否匹配、采样率是否与模型一致）", fail)
	} else if ok == 0 && fail == 0 {
		success = false
		errMsg = "输入 CSV 无有效数据行"
	} else if ok > 0 && fail > 0 {
		errMsg = fmt.Sprintf("已完成：%d 行成功，%d 行跳过（无 wav/校验失败/推理失败）", ok, fail)
	}
	if outputJSON {
		_ = json.NewEncoder(os.Stdout).Encode(jsonOutput{Success: success, SuccessCount: ok, ErrorCount: fail, Error: errMsg, OutputPath: csvOut})
		return
	}
	fmt.Printf("GAME align 完成: 成功 %d, 失败 %d, 输出 %s\n", ok, fail, csvOut)
}

func runAddPhNumMode(inputCSV, outputCSV, dictPath, phSeqCol string, outputJSON bool) {
	// 验证参数
	if inputCSV == "" || outputCSV == "" || dictPath == "" {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "缺少参数: --input-csv, --output-csv 和 --dict 必须提供"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: --input-csv, --output-csv 和 --dict 参数必须提供，详细用法请加 --help")
		}
		os.Exit(1)
	}

	// 转换为绝对路径
	inputCSV, _ = filepath.Abs(inputCSV)
	outputCSV, _ = filepath.Abs(outputCSV)
	dictPath, _ = filepath.Abs(dictPath)

	if !outputJSON {
		fmt.Println("===========================================")
		fmt.Println("  DataForgeLite - AddPhNum 音素数量计算")
		fmt.Println("===========================================")
		fmt.Printf("输入CSV:  %s\n", inputCSV)
		fmt.Printf("输出CSV:  %s\n", outputCSV)
		fmt.Printf("词典文件: %s\n", dictPath)
		fmt.Printf("音素列名: %s\n", phSeqCol)
		fmt.Println("")
		fmt.Println("正在加载词典并处理CSV文件...")
	}

	// 创建配置
	config := &addphnum.ProcessorConfig{
		InputCSV:  inputCSV,
		OutputCSV: outputCSV,
		DictPath:  dictPath,
		PhSeqCol:  phSeqCol,
	}

	if err := addphnum.AddPhNumToCSV(config); err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "处理失败: %v\n", err)
		}
		os.Exit(1)
	}

	if outputJSON {
		out := jsonOutput{Success: true, OutputPath: outputCSV}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	fmt.Println("")
	fmt.Println("===========================================")
	fmt.Println("  处理完成！")
	fmt.Printf("  输出文件: %s\n", outputCSV)
	fmt.Println("===========================================")
}

func runExportMode(wavsDir, tgDir, outputDir string, outputJSON bool) {
	if wavsDir == "" || tgDir == "" || outputDir == "" {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "缺少参数: --wavs-dir, --tg-dir, --export-out 必须提供"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: --wavs-dir, --tg-dir, --export-out 必须提供")
		}
		os.Exit(1)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "创建输出目录失败: " + err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		}
		os.Exit(1)
	}
	exp := exporter.NewExporter()
	result, err := exp.Export(wavsDir, tgDir, outputDir)
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
		}
		os.Exit(1)
	}
	if outputJSON {
		out := jsonOutput{
			Success:      result.ErrorCount == 0,
			SuccessCount: result.SuccessCount,
			ErrorCount:   result.ErrorCount,
			Error:        "",
			OutputPath:   outputDir,
		}
		if len(result.Errors) > 0 {
			out.Error = result.Errors[0]
		}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	fmt.Printf("导出完成: 成功 %d, 失败 %d\n", result.SuccessCount, result.ErrorCount)
}

// listAudioFiles 列出目录下支持的音频文件
func listAudioFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if audiopreprocessor.IsSupportedFormat(full) {
			paths = append(paths, full)
		}
	}
	return paths, nil
}

func runPreprocessMode(inputDir, outputDir string, targetLufs, truePeakLimit float64, outputJSON bool) {
	if inputDir == "" || outputDir == "" {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "缺少参数: --preprocess-input 和 --preprocess-output 必须提供"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: --preprocess-input 和 --preprocess-output 必须提供")
		}
		os.Exit(1)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "创建输出目录失败: " + err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		}
		os.Exit(1)
	}
	paths, err := listAudioFiles(inputDir)
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		}
		os.Exit(1)
	}
	if len(paths) == 0 {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "输入目录下没有支持的音频文件"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: 输入目录下没有支持的音频文件")
		}
		os.Exit(1)
	}
	concurrency := runtime.NumCPU() * 2
	if concurrency > len(paths) {
		concurrency = len(paths)
	}
	if concurrency < 1 {
		concurrency = 1
	}
	opts := []audiopreprocessor.ProcessorOption{
		audiopreprocessor.WithTargetLUFS(targetLufs),
		audiopreprocessor.WithTruePeakLimit(truePeakLimit),
		audiopreprocessor.WithConcurrency(concurrency),
	}
	result, err := audiopreprocessor.ProcessBatch(paths, outputDir, opts...)
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "预处理失败: %v\n", err)
		}
		os.Exit(1)
	}
	if outputJSON {
		out := jsonOutput{
			Success:      result.FailedCount == 0,
			SuccessCount: result.SuccessCount,
			ErrorCount:   result.FailedCount,
			OutputPath:   outputDir,
		}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		if result.FailedCount > 0 {
			os.Exit(1)
		}
		return
	}
	fmt.Printf("预处理完成: 成功 %d, 失败 %d, 跳过 %d\n", result.SuccessCount, result.FailedCount, result.SkippedCount)
}

func runFAMode(modelDir, inputDir, outputDir, language, nonLex, g2pType, dictPath string, padTimes, padLength int, outputJSON bool) {
	if inputDir == "" || outputDir == "" {
		msg := "缺少参数: --input 和 --output 必须提供"
		if outputJSON {
			out := jsonOutput{Success: false, Error: msg}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: "+msg)
		}
		os.Exit(1)
	}
	if modelDir == "" {
		msg := "缺少参数: --model-dir 必须提供（HubertFA 模型目录）"
		if outputJSON {
			out := jsonOutput{Success: false, Error: msg}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: "+msg)
		}
		os.Exit(1)
	}

	if !outputJSON {
		fmt.Println("===========================================")
		fmt.Println("  DataForgeLite - HubertFA 强制对齐")
		fmt.Println("===========================================")
		fmt.Printf("模型目录: %s\n", modelDir)
		fmt.Printf("输入目录: %s\n", inputDir)
		fmt.Printf("输出目录: %s\n", outputDir)
		fmt.Printf("语言: %s\n", language)
		fmt.Println("")
	}

	cfg := &hubertfa.FAConfig{
		ModelDir:           modelDir,
		InputDir:           inputDir,
		OutputDir:          outputDir,
		Language:           language,
		G2PType:            g2pType,
		DictionaryPath:     dictPath,
		NonLexicalPhonemes: nonLex,
		PadTimes:           padTimes,
		PadLength:          padLength,
		Quiet:              outputJSON,
	}

	response, err := hubertfa.RunFA(cfg)
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "FA 执行失败: %v\n", err)
		}
		os.Exit(1)
	}

	if outputJSON {
		out := jsonOutput{
			Success:      response.Success,
			SuccessCount: response.SuccessCount,
			ErrorCount:   response.ErrorCount,
			Error:        response.Error,
			OutputPath:   response.OutputPath,
		}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		if !response.Success {
			os.Exit(1)
		}
		return
	}

	fmt.Println("")
	fmt.Println("===========================================")
	if response.Success {
		fmt.Printf("对齐完成！成功: %d, 失败: %d\n", response.SuccessCount, response.ErrorCount)
		fmt.Printf("输出目录: %s\n", response.OutputPath)
	} else {
		fmt.Fprintf(os.Stderr, "对齐失败: %s\n", response.Error)
		os.Exit(1)
	}
}

func runSplitMode(inputDir, outputDir string, outputJSON bool) {
	if inputDir == "" || outputDir == "" {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "缺少参数: --split-input 和 --split-output 必须提供"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: --split-input 和 --split-output 必须提供")
		}
		os.Exit(1)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "创建输出目录失败: " + err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		}
		os.Exit(1)
	}
	paths, err := listAudioFiles(inputDir)
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		}
		os.Exit(1)
	}
	if len(paths) == 0 {
		if outputJSON {
			out := jsonOutput{Success: false, Error: "输入目录下没有支持的音频文件"}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintln(os.Stderr, "错误: 输入目录下没有支持的音频文件")
		}
		os.Exit(1)
	}
	result, err := audiosplitter.SplitFiles(paths, outputDir)
	if err != nil {
		if outputJSON {
			out := jsonOutput{Success: false, Error: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Fprintf(os.Stderr, "切分失败: %v\n", err)
		}
		os.Exit(1)
	}
	if outputJSON {
		out := jsonOutput{
			Success:      result.FailedCount == 0,
			SuccessCount: result.SuccessCount,
			ErrorCount:   result.FailedCount,
			OutputPath:   outputDir,
		}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		if result.FailedCount > 0 {
			os.Exit(1)
		}
		return
	}
	fmt.Printf("切分完成: 成功 %d, 失败 %d\n", result.SuccessCount, result.FailedCount)
}
