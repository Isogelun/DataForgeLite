package gameonnx

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

type AlignOptions struct {
	ModelDir     string
	ORTLib       string
	Lang         string
	SegThreshold float32
	SegRadius    int64
	T0           float32
	NSteps       int
	EstThreshold float32

	CSVIn  string
	CSVOut string

	// WavDir: 若为空，默认使用 CSV 所在目录下的 wavs/
	WavDir string
}

func Align(opt AlignOptions) (successCount, errorCount int, err error) {
	if strings.TrimSpace(opt.CSVIn) == "" {
		return 0, 0, fmt.Errorf("缺少参数: csv_in")
	}
	if strings.TrimSpace(opt.CSVOut) == "" {
		return 0, 0, fmt.Errorf("缺少参数: csv_out")
	}
	if strings.TrimSpace(opt.ORTLib) == "" {
		return 0, 0, fmt.Errorf("缺少参数: ort")
	}

	inF, err := os.Open(opt.CSVIn)
	if err != nil {
		return 0, 0, err
	}
	defer inF.Close()

	r := csv.NewReader(inF)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return 0, 0, err
	}
	if len(records) < 2 {
		return 0, 0, fmt.Errorf("CSV 为空或无数据行: %s", opt.CSVIn)
	}
	header := records[0]
	for i, h := range header {
		header[i] = strings.TrimPrefix(strings.TrimSpace(h), "\ufeff")
	}
	col := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		return -1
	}
	nameIdx := col("name")
	phDurIdx := col("ph_dur")
	phNumIdx := col("ph_num")
	if nameIdx < 0 || phDurIdx < 0 || phNumIdx < 0 {
		return 0, 0, fmt.Errorf("CSV 缺少列（需要 name/ph_dur/ph_num）: %v", header)
	}

	// note_seq / note_dur：若不存在则追加
	noteSeqIdx := col("note_seq")
	noteDurIdx := col("note_dur")
	outHeader := append([]string(nil), header...)
	if noteSeqIdx < 0 {
		noteSeqIdx = len(outHeader)
		outHeader = append(outHeader, "note_seq")
	}
	if noteDurIdx < 0 {
		noteDurIdx = len(outHeader)
		outHeader = append(outHeader, "note_dur")
	}
	// 去掉 note_glide（和 python callback 一致）
	noteGlideIdx := col("note_glide")
	if noteGlideIdx >= 0 {
		tmp := make([]string, 0, len(outHeader)-1)
		for i, h := range outHeader {
			if i == noteGlideIdx {
				continue
			}
			tmp = append(tmp, h)
		}
		outHeader = tmp
		// 重新计算索引
		header = outHeader
		nameIdx, phDurIdx, phNumIdx = col("name"), col("ph_dur"), col("ph_num")
		noteSeqIdx, noteDurIdx = col("note_seq"), col("note_dur")
	}

	csvDir := filepath.Dir(opt.CSVIn)
	wavDir := opt.WavDir
	if strings.TrimSpace(wavDir) == "" {
		wavDir = filepath.Join(csvDir, "wavs")
	}

	modelDir := ResolveModelDir(opt.ModelDir)
	cfg, err := loadModelConfig(modelDir)
	if err != nil {
		return 0, 0, fmt.Errorf("GAME 模型目录无效: %w", err)
	}
	langID := int64(0)
	if cfg.Languages != nil && strings.TrimSpace(opt.Lang) != "" {
		v, ok := cfg.Languages[strings.TrimSpace(opt.Lang)]
		if !ok {
			return 0, 0, fmt.Errorf("language=%q 不在 config.json.languages", opt.Lang)
		}
		langID = int64(v)
	}

	var ortInited bool
	defer func() {
		if ortInited {
			ort.DestroyEnvironment()
		}
	}()

	outRows := make([][]string, 0, len(records))
	outRows = append(outRows, outHeader)

	for _, row := range records[1:] {
		// pad to header length
		outRow := make([]string, len(outHeader))
		for i := range outRow {
			if i < len(row) {
				outRow[i] = row[i]
			} else {
				outRow[i] = ""
			}
		}

		name := strings.TrimSpace(outRow[nameIdx])
		if name == "" {
			errorCount++
			outRows = append(outRows, outRow)
			continue
		}
		phDur, err := parseFloatList(outRow[phDurIdx])
		if err != nil {
			errorCount++
			outRows = append(outRows, outRow)
			continue
		}
		phNum, err := parseIntList(outRow[phNumIdx])
		if err != nil {
			errorCount++
			outRows = append(outRows, outRow)
			continue
		}
		sumNum := 0
		for _, n := range phNum {
			sumNum += n
		}
		if sumNum != len(phDur) {
			errorCount++
			outRows = append(outRows, outRow)
			continue
		}
		wordDur := make([]float32, 0, len(phNum))
		idx := 0
		for _, n := range phNum {
			s := 0.0
			for _, d := range phDur[idx : idx+n] {
				s += d
			}
			wordDur = append(wordDur, float32(s))
			idx += n
		}

		wavPath := filepath.Join(wavDir, name+".wav")
		if _, err := os.Stat(wavPath); err != nil {
			errorCount++
			outRow[noteSeqIdx] = "[GAME_Align_Error]"
			outRow[noteDurIdx] = alignErrCSV(fmt.Errorf("找不到 WAV: %s", wavPath))
			outRows = append(outRows, outRow)
			continue
		}

		if !ortInited {
			ort.SetSharedLibraryPath(opt.ORTLib)
			if err := ort.InitializeEnvironment(); err != nil {
				return successCount, errorCount, fmt.Errorf("ONNX Runtime 初始化失败（请确认 onnxruntime.dll 路径正确）: %w", err)
			}
			ortInited = true
		}

		wave, sr, err := readWavMonoFloat32(wavPath)
		if err != nil {
			errorCount++
			outRow[noteSeqIdx] = "[GAME_Align_Error]"
			outRow[noteDurIdx] = alignErrCSV(err)
			outRows = append(outRows, outRow)
			continue
		}
		if sr != cfg.SampleRate {
			errorCount++
			outRow[noteSeqIdx] = "[GAME_Align_Error]"
			outRow[noteDurIdx] = alignErrCSV(fmt.Errorf("WAV 采样率=%d，模型要求=%dHz", sr, cfg.SampleRate))
			outRows = append(outRows, outRow)
			continue
		}
		durationSec := float32(float64(len(wave)) / float64(sr))

		runOpt := DefaultRunOptions()
		runOpt.ModelDir = modelDir
		runOpt.ORTLib = opt.ORTLib
		runOpt.Lang = opt.Lang
		runOpt.SegThreshold = opt.SegThreshold
		runOpt.SegRadius = opt.SegRadius
		runOpt.T0 = opt.T0
		runOpt.NSteps = opt.NSteps
		runOpt.EstThreshold = opt.EstThreshold
		runOpt.WavPath = wavPath
		runOpt.KnownDurations = wordDur
		runOpt.AlignCSV = true

		res, err := RunInferenceAfterWav(runOpt, cfg, langID, wave, durationSec)
		if err != nil {
			errorCount++
			outRow[noteSeqIdx] = "[GAME_Align_Error]"
			outRow[noteDurIdx] = alignErrCSV(err)
			outRows = append(outRows, outRow)
			continue
		}

		noteSeq, noteDur := formatNotesForTranscription(res)
		if noteSeq == "" {
			errorCount++
			outRow[noteSeqIdx] = "[GAME_Align_Error]"
			outRow[noteDurIdx] = "推理成功但未解析到任何 note（duration>0 的格为空），请检查 ONNX 与 model.pt 是否同源"
			outRows = append(outRows, outRow)
			continue
		}
		outRow[noteSeqIdx] = noteSeq
		outRow[noteDurIdx] = noteDur
		successCount++
		outRows = append(outRows, outRow)
	}

	// 写出 csv
	if err := os.MkdirAll(filepath.Dir(opt.CSVOut), 0755); err != nil {
		return successCount, errorCount, err
	}
	outF, err := os.Create(opt.CSVOut)
	if err != nil {
		return successCount, errorCount, err
	}
	defer outF.Close()
	w := csv.NewWriter(outF)
	if err := w.WriteAll(outRows); err != nil {
		return successCount, errorCount, err
	}
	w.Flush()
	return successCount, errorCount, w.Error()
}

func alignErrCSV(e error) string {
	if e == nil {
		return ""
	}
	s := e.Error()
	if len(s) > 220 {
		return s[:217] + "..."
	}
	return s
}

func parseFloatList(s string) ([]float64, error) {
	parts := strings.Fields(s)
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func parseIntList(s string) ([]int, error) {
	parts := strings.Fields(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func formatNotesForTranscription(res *Result) (noteSeq string, noteDur string) {
	seq := make([]string, 0, len(res.Notes))
	dur := make([]string, 0, len(res.Notes))
	for _, n := range res.Notes {
		if n.Rest {
			seq = append(seq, "rest")
		} else {
			seq = append(seq, midiToNoteName(float64(n.Pitch), true))
		}
		dur = append(dur, fmt.Sprintf("%.3f", float64(n.Duration)))
	}
	return strings.Join(seq, " "), strings.Join(dur, " ")
}

var noteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// midiToNoteName: 参考 librosa.midi_to_note(..., cents=True) 的输出风格（近似）
func midiToNoteName(midi float64, cents bool) string {
	if math.IsNaN(midi) || math.IsInf(midi, 0) {
		return "rest"
	}
	if cents {
		base := math.Floor(midi)
		c := int(math.Round((midi - base) * 100))
		n := int(base)
		name := noteNames[mod(n, 12)]
		oct := n/12 - 1
		if c == 0 {
			return fmt.Sprintf("%s%d", name, oct)
		}
		if c > 50 {
			// 进位到下一音
			n2 := n + 1
			name2 := noteNames[mod(n2, 12)]
			oct2 := n2/12 - 1
			return fmt.Sprintf("%s%d-%.0f", name2, oct2, float64(100-c))
		}
		return fmt.Sprintf("%s%d+%d", name, oct, c)
	}
	n := int(math.Round(midi))
	name := noteNames[mod(n, 12)]
	oct := n/12 - 1
	return fmt.Sprintf("%s%d", name, oct)
}

func mod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}
