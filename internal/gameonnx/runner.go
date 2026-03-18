package gameonnx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

type RunOptions struct {
	ModelDir     string
	WavPath      string
	ORTLib       string
	Lang         string
	SegThreshold float32
	SegRadius    int64
	T0           float32
	NSteps       int
	EstThreshold float32
	// KnownDurations: 可选。用于 align：按 “词级/片段级” 已知时长（秒）约束边界。
	// 为空则默认只有一个区域=整段 duration。
	KnownDurations []float32
	// AlignCSV：true 时写出与 infer.py align 一致：每个 duration>0 的槽位一条（含 rest）。
	AlignCSV bool
}

func DefaultRunOptions() RunOptions {
	return RunOptions{
		// 默认使用与 DataForgeLite.exe 同目录的 Gameonnx
		//（WPF 构建时会复制模型到 bin\Debug\Gameonnx）
		ModelDir:     filepath.FromSlash("Gameonnx"),
		Lang:         "zh",
		SegThreshold: 0.2,
		SegRadius:    2,
		T0:           0.0,
		NSteps:       8,
		EstThreshold: 0.2,
	}
}

func Run(opt RunOptions) (*Result, error) {
	if opt.ModelDir == "" {
		return nil, fmt.Errorf("缺少参数: model_dir")
	}
	if opt.WavPath == "" {
		return nil, fmt.Errorf("缺少参数: wav")
	}
	if opt.ORTLib == "" {
		return nil, fmt.Errorf("缺少参数: ort")
	}
	if opt.NSteps < 0 {
		return nil, fmt.Errorf("nsteps 不能为负数")
	}
	if opt.SegRadius < 0 {
		return nil, fmt.Errorf("seg_radius 不能为负数")
	}

	// 兼容 WPF/双击运行：如果给的是相对路径，则以 DataForgeLite.exe 所在目录为基准解析
	if !filepath.IsAbs(opt.ModelDir) {
		if exe, err := os.Executable(); err == nil && exe != "" {
			exeDir := filepath.Dir(exe)
			candidate := filepath.Join(exeDir, opt.ModelDir)
			opt.ModelDir = candidate
		}
	}
	opt.ModelDir = filepath.Clean(opt.ModelDir)

	cfg, err := loadModelConfig(opt.ModelDir)
	if err != nil {
		return nil, err
	}

	langID := int64(0)
	if cfg.Languages != nil {
		langKey := strings.TrimSpace(opt.Lang)
		// 与 infer.py：未指定 -l 时 language_id=0（Embedding padding_idx=0）
		if langKey != "" {
			v, ok := cfg.Languages[langKey]
			if !ok {
				return nil, fmt.Errorf("language=%q 不在 config.json.languages 中（留空则 id=0，对齐 infer.py 默认）", langKey)
			}
			langID = int64(v)
		}
	}

	wave, sr, err := readWavMonoFloat32(opt.WavPath)
	if err != nil {
		return nil, err
	}
	if sr != cfg.SampleRate {
		return nil, fmt.Errorf("WAV 采样率=%d 与模型 samplerate=%d 不一致；请先重采样到 %dHz", sr, cfg.SampleRate, cfg.SampleRate)
	}
	durationSec := float32(float64(len(wave)) / float64(sr))

	ort.SetSharedLibraryPath(opt.ORTLib)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, err
	}
	defer ort.DestroyEnvironment()
	return RunInferenceAfterWav(opt, cfg, langID, wave, durationSec)
}

// RunInferenceAfterWav 在已调用 ort.InitializeEnvironment() 的前提下执行推理（供 Align 多行复用同一 ORT 环境）。
func RunInferenceAfterWav(opt RunOptions, cfg *ModelConfig, langID int64, wave []float32, durationSec float32) (*Result, error) {
	encoderPath := filepath.Join(opt.ModelDir, "encoder.onnx")
	dur2bdPath := filepath.Join(opt.ModelDir, "dur2bd.onnx")
	segmenterPath := filepath.Join(opt.ModelDir, "segmenter.onnx")
	bd2durPath := filepath.Join(opt.ModelDir, "bd2dur.onnx")
	estimatorPath := filepath.Join(opt.ModelDir, "estimator.onnx")

	xSeg, xEst, maskT, err := runEncoderDynamic(encoderPath, wave, durationSec)
	if err != nil {
		return nil, err
	}

	knownDur := opt.KnownDurations
	if len(knownDur) == 0 {
		knownDur = []float32{durationSec}
	}
	knownBoundaries, err := runDur2BDDynamic(dur2bdPath, knownDur, maskT)
	if err != nil {
		return nil, err
	}

	boundaries := knownBoundaries
	if cfg.Loop {
		ts := d3pmSchedule(opt.T0, opt.NSteps)
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
				opt.SegThreshold,
				opt.SegRadius,
			)
			if err != nil {
				return nil, err
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
			opt.SegThreshold,
			opt.SegRadius,
		)
		if err != nil {
			return nil, err
		}
	}

	durations, maskN, err := runBD2DurDynamic(bd2durPath, boundaries, maskT)
	if err != nil {
		return nil, err
	}

	presence, scores, err := runEstimatorDynamic(estimatorPath, xEst, boundaries, maskT, maskN, opt.EstThreshold)
	if err != nil {
		return nil, err
	}

	out := &Result{
		SampleRate: cfg.SampleRate,
		TimeStep:   fmt.Sprintf("%.6g", cfg.TimeStep),
		Language:   opt.Lang,
	}

	if len(maskN.Shape) < 2 {
		return nil, fmt.Errorf("maskN shape 异常: %v", maskN.Shape)
	}
	N := int(maskN.Shape[1])
	for i := 0; i < N; i++ {
		if i >= len(maskN.Data) || !maskN.Data[i] {
			break
		}
		var dur float32
		if i < len(durations.Data) {
			dur = durations.Data[i]
		}
		if dur <= 0 {
			continue
		}
		pres := i < len(presence.Data) && presence.Data[i]
		var pitch float32
		if i < len(scores.Data) {
			pitch = scores.Data[i]
		}
		if opt.AlignCSV {
			out.Notes = append(out.Notes, Note{
				Index: i, Duration: dur, Pitch: pitch, Rest: !pres,
			})
		} else if pres {
			out.Notes = append(out.Notes, Note{
				Index: i, Duration: dur, Pitch: pitch, Rest: false,
			})
		}
	}

	return out, nil
}

// ResolveModelDir 将相对 ModelDir 解析为绝对路径（相对 DataForgeLite.exe 目录）。
func ResolveModelDir(modelDir string) string {
	if strings.TrimSpace(modelDir) == "" {
		return modelDir
	}
	if !filepath.IsAbs(modelDir) {
		if exe, err := os.Executable(); err == nil && exe != "" {
			modelDir = filepath.Join(filepath.Dir(exe), modelDir)
		}
	}
	return filepath.Clean(modelDir)
}
