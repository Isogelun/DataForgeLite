package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/go-audio/wav"
	ort "github.com/yalue/onnxruntime_go"
)

// 这个 main.go 是“拷贝即用”的 GAME ONNX 推理样例。
// 模型目录应包含：encoder.onnx segmenter.onnx estimator.onnx dur2bd.onnx bd2dur.onnx config.json

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

func main() {
	var (
		modelDir     = flag.String("model_dir", filepath.FromSlash("GAME-1.0.3-large-onnx"), "模型目录（相对/绝对路径）")
		wavPath      = flag.String("wav", "", "输入 WAV 文件（建议单声道，采样率需匹配模型）")
		ortLib       = flag.String("ort", "", "onnxruntime 共享库路径（Windows: onnxruntime.dll）")
		langCode     = flag.String("lang", "zh", "语言代码（如 zh/en/ja/yue）")
		segThreshold = flag.Float64("seg_threshold", 0.2, "边界解码阈值（推荐 0.2）")
		segRadius    = flag.Int("seg_radius", 2, "边界解码半径（帧数，推荐 2）")
		t0           = flag.Float64("t0", 0.0, "D3PM 起始 t0")
		nsteps       = flag.Int("nsteps", 8, "D3PM 步数")
		estThreshold = flag.Float64("est_threshold", 0.2, "音符存在阈值（推荐 0.2）")
	)
	flag.Parse()

	if *wavPath == "" || *ortLib == "" {
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := loadModelConfig(filepath.Join(*modelDir, "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	langID := int64(0)
	if cfg.Languages != nil {
		v, ok := cfg.Languages[*langCode]
		if !ok {
			log.Fatalf("language=%q 不在 config.json.languages 中", *langCode)
		}
		langID = int64(v)
	}

	wave, sr, err := readWavMonoFloat32(*wavPath)
	if err != nil {
		log.Fatal(err)
	}
	if sr != cfg.SampleRate {
		log.Fatalf("WAV 采样率=%d 与模型 samplerate=%d 不一致；请先重采样到 %dHz", sr, cfg.SampleRate, cfg.SampleRate)
	}
	durationSec := float32(float64(len(wave)) / float64(sr))

	ort.SetSharedLibraryPath(*ortLib)
	if err := ort.InitializeEnvironment(); err != nil {
		log.Fatal(err)
	}
	defer ort.DestroyEnvironment()

	encoderPath := filepath.Join(*modelDir, "encoder.onnx")
	dur2bdPath := filepath.Join(*modelDir, "dur2bd.onnx")
	segmenterPath := filepath.Join(*modelDir, "segmenter.onnx")
	bd2durPath := filepath.Join(*modelDir, "bd2dur.onnx")
	estimatorPath := filepath.Join(*modelDir, "estimator.onnx")

	// encoder
	xSeg, xEst, maskT, err := runEncoderDynamic(encoderPath, wave, durationSec)
	if err != nil {
		log.Fatal(err)
	}

	// known_durations：没提供外部已知区域时，给 1 个区域=整段时长
	knownDur := []float32{durationSec} // [1, 1]

	// dur2bd -> known_boundaries
	knownBoundaries, err := runDur2BDDynamic(dur2bdPath, knownDur, maskT)
	if err != nil {
		log.Fatal(err)
	}

	// segmenter loop（D3PM）
	boundaries := knownBoundaries
	if cfg.Loop {
		ts := d3pmSchedule(float32(*t0), *nsteps)
		for _, ti := range ts {
			boundaries, err = runSegmenterOnceDynamic(
				segmenterPath,
				xSeg,
				cfg.Languages != nil,
				langID,
				knownBoundaries,
				boundaries,
				ti,
				maskT,
				float32(*segThreshold),
				int64(*segRadius),
			)
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {
		boundaries, err = runSegmenterOnceDynamic(
			segmenterPath,
			xSeg,
			cfg.Languages != nil,
			langID,
			knownBoundaries,
			BoolTensor{},
			0,
			maskT,
			float32(*segThreshold),
			int64(*segRadius),
		)
		if err != nil {
			log.Fatal(err)
		}
	}

	// bd2dur
	durations, maskN, err := runBD2DurDynamic(bd2durPath, boundaries, maskT)
	if err != nil {
		log.Fatal(err)
	}

	// estimator
	presence, scores, err := runEstimatorDynamic(estimatorPath, xEst, boundaries, maskT, maskN, float32(*estThreshold))
	if err != nil {
		log.Fatal(err)
	}

	// 输出：有效且 voiced 的音符
	type Note struct {
		Index    int     `json:"index"`
		Duration float32 `json:"duration_sec"`
		Pitch    float32 `json:"pitch_midi"`
	}
	out := struct {
		SampleRate int    `json:"samplerate"`
		TimeStep   string `json:"timestep"`
		Language   string `json:"language"`
		Notes      []Note `json:"notes"`
	}{
		SampleRate: cfg.SampleRate,
		TimeStep:   fmt.Sprintf("%.6g", cfg.TimeStep),
		Language:   *langCode,
	}
	N := int(maskN.Shape[1])
	for i := 0; i < N; i++ {
		if !maskN.Data[i] {
			break
		}
		if presence.Data[i] {
			out.Notes = append(out.Notes, Note{
				Index:    i,
				Duration: durations.Data[i],
				Pitch:    scores.Data[i],
			})
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

func loadModelConfig(path string) (*ModelConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ModelConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func readWavMonoFloat32(path string) (samples []float32, sampleRate int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, 0, fmt.Errorf("invalid wav file")
	}
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}
	sampleRate = buf.Format.SampleRate
	ch := buf.Format.NumChannels
	if ch <= 0 {
		return nil, 0, fmt.Errorf("invalid channels=%d", ch)
	}

	maxInt := float32(float64(int64(1)<<(uint(buf.SourceBitDepth)-1)) - 1)
	if maxInt <= 0 {
		maxInt = 32767
	}

	nFrames := len(buf.Data) / ch
	out := make([]float32, nFrames)
	for i := 0; i < nFrames; i++ {
		var sum float32
		for c := 0; c < ch; c++ {
			sum += float32(buf.Data[i*ch+c]) / maxInt
		}
		out[i] = sum / float32(ch)
	}
	return out, sampleRate, nil
}

func d3pmSchedule(t0 float32, steps int) []float32 {
	if steps <= 0 {
		return nil
	}
	step := (1 - t0) / float32(steps)
	ts := make([]float32, steps)
	for i := 0; i < steps; i++ {
		ts[i] = t0 + float32(i)*step
	}
	return ts
}

func tensorShapeToSlice(s ort.Shape) []int64 {
	// Shape 在不同版本 API 里实现细节可能不同；这里使用 fmt.Sprint 的方式兜底不可取。
	// v1.27.0 支持 s.Dimensions() 返回 []int64。
	return s.Dimensions()
}

func runEncoderDynamic(onnxPath string, waveform []float32, durationSec float32) (xSeg F32Tensor, xEst F32Tensor, maskT BoolTensor, err error) {
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"waveform", "duration"},
		[]string{"x_seg", "x_est", "maskT"},
		nil,
	)
	if err != nil {
		return xSeg, xEst, maskT, err
	}
	defer sess.Destroy()

	waveTensor, err := ort.NewTensor[float32](ort.NewShape(1, int64(len(waveform))), waveform)
	if err != nil {
		return xSeg, xEst, maskT, err
	}
	defer waveTensor.Destroy()

	durTensor, err := ort.NewTensor[float32](ort.NewShape(1), []float32{durationSec})
	if err != nil {
		return xSeg, xEst, maskT, err
	}
	defer durTensor.Destroy()

	var out0 ort.ArbitraryTensor
	var out1 ort.ArbitraryTensor
	var out2 ort.ArbitraryTensor
	outputs := []ort.ArbitraryTensor{out0, out1, out2} // nil -> 让 session 分配
	if err := sess.Run([]ort.Value{waveTensor, durTensor}, outputs); err != nil {
		return xSeg, xEst, maskT, err
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
	}()

	xSegT := outputs[0].(*ort.Tensor[float32])
	xEstT := outputs[1].(*ort.Tensor[float32])
	maskTT := outputs[2].(*ort.Tensor[bool])

	xSeg = F32Tensor{Shape: tensorShapeToSlice(xSegT.GetShape()), Data: append([]float32(nil), xSegT.GetData()...)}
	xEst = F32Tensor{Shape: tensorShapeToSlice(xEstT.GetShape()), Data: append([]float32(nil), xEstT.GetData()...)}
	maskT = BoolTensor{Shape: tensorShapeToSlice(maskTT.GetShape()), Data: append([]bool(nil), maskTT.GetData()...)}
	return xSeg, xEst, maskT, nil
}

func runDur2BDDynamic(onnxPath string, knownDurations []float32, maskT BoolTensor) (boundaries BoolTensor, err error) {
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"durations", "maskT"},
		[]string{"boundaries"},
		nil,
	)
	if err != nil {
		return boundaries, err
	}
	defer sess.Destroy()

	durTensor, err := ort.NewTensor[float32](ort.NewShape(1, int64(len(knownDurations))), knownDurations)
	if err != nil {
		return boundaries, err
	}
	defer durTensor.Destroy()

	maskTensor, err := ort.NewTensor[bool](ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return boundaries, err
	}
	defer maskTensor.Destroy()

	outputs := []ort.ArbitraryTensor{nil}
	if err := sess.Run([]ort.Value{durTensor, maskTensor}, outputs); err != nil {
		return boundaries, err
	}
	defer outputs[0].Destroy()

	bd := outputs[0].(*ort.Tensor[bool])
	boundaries = BoolTensor{Shape: tensorShapeToSlice(bd.GetShape()), Data: append([]bool(nil), bd.GetData()...)}
	return boundaries, nil
}

func runSegmenterOnceDynamic(
	onnxPath string,
	xSeg F32Tensor,
	useLanguage bool,
	langID int64,
	knownBoundaries BoolTensor,
	prevBoundaries BoolTensor,
	t float32,
	maskT BoolTensor,
	threshold float32,
	radius int64,
) (boundaries BoolTensor, err error) {
	// loop 模型输入名：x_seg, [language], known_boundaries, prev_boundaries, t, maskT, threshold, radius
	inputNames := []string{"x_seg"}
	if useLanguage {
		inputNames = append(inputNames, "language")
	}
	inputNames = append(inputNames, "known_boundaries")
	// 如果 prevBoundaries 为空（shape nil），就不传（用于非 loop 模型的兼容）
	hasPrev := len(prevBoundaries.Shape) != 0
	if hasPrev {
		inputNames = append(inputNames, "prev_boundaries", "t")
	}
	inputNames = append(inputNames, "maskT", "threshold", "radius")

	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		inputNames,
		[]string{"boundaries"},
		nil,
	)
	if err != nil {
		return boundaries, err
	}
	defer sess.Destroy()

	xTensor, err := ort.NewTensor[float32](ort.NewShape(xSeg.Shape...), xSeg.Data)
	if err != nil {
		return boundaries, err
	}
	defer xTensor.Destroy()

	var inputs []ort.Value
	inputs = append(inputs, xTensor)

	var langTensor *ort.Tensor[int64]
	if useLanguage {
		langTensor, err = ort.NewTensor[int64](ort.NewShape(1), []int64{langID})
		if err != nil {
			return boundaries, err
		}
		defer langTensor.Destroy()
		inputs = append(inputs, langTensor)
	}

	knownTensor, err := ort.NewTensor[bool](ort.NewShape(knownBoundaries.Shape...), knownBoundaries.Data)
	if err != nil {
		return boundaries, err
	}
	defer knownTensor.Destroy()
	inputs = append(inputs, knownTensor)

	var prevTensor *ort.Tensor[bool]
	var tTensor *ort.Tensor[float32]
	if hasPrev {
		prevTensor, err = ort.NewTensor[bool](ort.NewShape(prevBoundaries.Shape...), prevBoundaries.Data)
		if err != nil {
			return boundaries, err
		}
		defer prevTensor.Destroy()

		tTensor, err = ort.NewTensor[float32](ort.NewShape(1), []float32{t})
		if err != nil {
			return boundaries, err
		}
		defer tTensor.Destroy()

		inputs = append(inputs, prevTensor, tTensor)
	}

	maskTensor, err := ort.NewTensor[bool](ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return boundaries, err
	}
	defer maskTensor.Destroy()

	thTensor, err := ort.NewTensor[float32](ort.NewShape(), []float32{threshold})
	if err != nil {
		return boundaries, err
	}
	defer thTensor.Destroy()

	rTensor, err := ort.NewTensor[int64](ort.NewShape(), []int64{radius})
	if err != nil {
		return boundaries, err
	}
	defer rTensor.Destroy()

	inputs = append(inputs, maskTensor, thTensor, rTensor)

	outputs := []ort.ArbitraryTensor{nil}
	if err := sess.Run(inputs, outputs); err != nil {
		return boundaries, err
	}
	defer outputs[0].Destroy()

	bd := outputs[0].(*ort.Tensor[bool])
	boundaries = BoolTensor{Shape: tensorShapeToSlice(bd.GetShape()), Data: append([]bool(nil), bd.GetData()...)}
	return boundaries, nil
}

func runBD2DurDynamic(onnxPath string, boundaries BoolTensor, maskT BoolTensor) (durations F32Tensor, maskN BoolTensor, err error) {
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"boundaries", "maskT"},
		[]string{"durations", "maskN"},
		nil,
	)
	if err != nil {
		return durations, maskN, err
	}
	defer sess.Destroy()

	bTensor, err := ort.NewTensor[bool](ort.NewShape(boundaries.Shape...), boundaries.Data)
	if err != nil {
		return durations, maskN, err
	}
	defer bTensor.Destroy()

	mTensor, err := ort.NewTensor[bool](ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return durations, maskN, err
	}
	defer mTensor.Destroy()

	outputs := []ort.ArbitraryTensor{nil, nil}
	if err := sess.Run([]ort.Value{bTensor, mTensor}, outputs); err != nil {
		return durations, maskN, err
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
	}()

	durT := outputs[0].(*ort.Tensor[float32])
	maskNT := outputs[1].(*ort.Tensor[bool])

	durations = F32Tensor{Shape: tensorShapeToSlice(durT.GetShape()), Data: append([]float32(nil), durT.GetData()...)}
	maskN = BoolTensor{Shape: tensorShapeToSlice(maskNT.GetShape()), Data: append([]bool(nil), maskNT.GetData()...)}
	return durations, maskN, nil
}

func runEstimatorDynamic(
	onnxPath string,
	xEst F32Tensor,
	boundaries BoolTensor,
	maskT BoolTensor,
	maskN BoolTensor,
	threshold float32,
) (presence BoolTensor, scores F32Tensor, err error) {
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"x_est", "boundaries", "maskT", "maskN", "threshold"},
		[]string{"presence", "scores"},
		nil,
	)
	if err != nil {
		return presence, scores, err
	}
	defer sess.Destroy()

	xTensor, err := ort.NewTensor[float32](ort.NewShape(xEst.Shape...), xEst.Data)
	if err != nil {
		return presence, scores, err
	}
	defer xTensor.Destroy()

	bTensor, err := ort.NewTensor[bool](ort.NewShape(boundaries.Shape...), boundaries.Data)
	if err != nil {
		return presence, scores, err
	}
	defer bTensor.Destroy()

	tTensor, err := ort.NewTensor[bool](ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return presence, scores, err
	}
	defer tTensor.Destroy()

	nTensor, err := ort.NewTensor[bool](ort.NewShape(maskN.Shape...), maskN.Data)
	if err != nil {
		return presence, scores, err
	}
	defer nTensor.Destroy()

	thTensor, err := ort.NewTensor[float32](ort.NewShape(), []float32{threshold})
	if err != nil {
		return presence, scores, err
	}
	defer thTensor.Destroy()

	outputs := []ort.ArbitraryTensor{nil, nil}
	if err := sess.Run([]ort.Value{xTensor, bTensor, tTensor, nTensor, thTensor}, outputs); err != nil {
		return presence, scores, err
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
	}()

	pT := outputs[0].(*ort.Tensor[bool])
	sT := outputs[1].(*ort.Tensor[float32])

	presence = BoolTensor{Shape: tensorShapeToSlice(pT.GetShape()), Data: append([]bool(nil), pT.GetData()...)}
	scores = F32Tensor{Shape: tensorShapeToSlice(sT.GetShape()), Data: append([]float32(nil), sT.GetData()...)}
	return presence, scores, nil
}

