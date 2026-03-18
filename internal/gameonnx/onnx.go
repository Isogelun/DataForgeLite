package gameonnx

import (
	"fmt"
	"os"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

func buildSessionOptions() *ort.SessionOptions {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil
	}
	opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)
	device := strings.ToLower(strings.TrimSpace(os.Getenv("ORT_DEVICE")))
	if device == "dml" {
		deviceID := 0
		if v := strings.TrimSpace(os.Getenv("ORT_DML_DEVICE_ID")); v != "" {
			fmt.Sscanf(v, "%d", &deviceID)
		}
		_ = opts.AppendExecutionProviderDirectML(deviceID)
	}
	return opts
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

func shapeToSlice(s []int64) []int64 {
	// 兼容当前仓库使用的 onnxruntime_go 版本：GetShape() 直接返回 []int64
	return append([]int64(nil), s...)
}

func runEncoderDynamic(onnxPath string, waveform []float32, durationSec float32) (xSeg F32Tensor, xEst F32Tensor, maskT BoolTensor, err error) {
	opts := buildSessionOptions()
	if opts != nil {
		defer opts.Destroy()
	}
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"waveform", "duration"},
		[]string{"x_seg", "x_est", "maskT"},
		opts,
	)
	if err != nil {
		return xSeg, xEst, maskT, err
	}
	defer sess.Destroy()

	waveTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(waveform))), waveform)
	if err != nil {
		return xSeg, xEst, maskT, err
	}
	defer waveTensor.Destroy()

	durTensor, err := ort.NewTensor(ort.NewShape(1), []float32{durationSec})
	if err != nil {
		return xSeg, xEst, maskT, err
	}
	defer durTensor.Destroy()

	outputs := []ort.Value{nil, nil, nil}
	if err := sess.Run([]ort.Value{waveTensor, durTensor}, outputs); err != nil {
		return xSeg, xEst, maskT, err
	}
	defer destroyValues(outputs)

	xSegT, ok0 := outputs[0].(*ort.Tensor[float32])
	xEstT, ok1 := outputs[1].(*ort.Tensor[float32])
	maskTT, ok2 := outputs[2].(*ort.Tensor[bool])
	if !ok0 || !ok1 || !ok2 {
		return xSeg, xEst, maskT, fmt.Errorf("encoder 输出类型不匹配")
	}

	xSeg = F32Tensor{Shape: shapeToSlice(xSegT.GetShape()), Data: append([]float32(nil), xSegT.GetData()...)}
	xEst = F32Tensor{Shape: shapeToSlice(xEstT.GetShape()), Data: append([]float32(nil), xEstT.GetData()...)}
	maskT = BoolTensor{Shape: shapeToSlice(maskTT.GetShape()), Data: append([]bool(nil), maskTT.GetData()...)}
	return xSeg, xEst, maskT, nil
}

func runDur2BDDynamic(onnxPath string, knownDurations []float32, maskT BoolTensor) (boundaries BoolTensor, err error) {
	opts := buildSessionOptions()
	if opts != nil {
		defer opts.Destroy()
	}
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"durations", "maskT"},
		[]string{"boundaries"},
		opts,
	)
	if err != nil {
		return boundaries, err
	}
	defer sess.Destroy()

	durTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(knownDurations))), knownDurations)
	if err != nil {
		return boundaries, err
	}
	defer durTensor.Destroy()

	maskTensor, err := ort.NewTensor(ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return boundaries, err
	}
	defer maskTensor.Destroy()

	outputs := []ort.Value{nil}
	if err := sess.Run([]ort.Value{durTensor, maskTensor}, outputs); err != nil {
		return boundaries, err
	}
	defer destroyValues(outputs)

	bd, ok := outputs[0].(*ort.Tensor[bool])
	if !ok {
		return boundaries, fmt.Errorf("dur2bd 输出类型不匹配")
	}
	boundaries = BoolTensor{Shape: shapeToSlice(bd.GetShape()), Data: append([]bool(nil), bd.GetData()...)}
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
	inputNames := []string{"x_seg"}
	if useLanguage {
		inputNames = append(inputNames, "language")
	}
	inputNames = append(inputNames, "known_boundaries")

	hasPrev := len(prevBoundaries.Shape) != 0
	if hasPrev {
		inputNames = append(inputNames, "prev_boundaries", "t")
	}
	inputNames = append(inputNames, "maskT", "threshold", "radius")

	opts := buildSessionOptions()
	if opts != nil {
		defer opts.Destroy()
	}
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		inputNames,
		[]string{"boundaries"},
		opts,
	)
	if err != nil {
		return boundaries, err
	}
	defer sess.Destroy()

	xTensor, err := ort.NewTensor(ort.NewShape(xSeg.Shape...), xSeg.Data)
	if err != nil {
		return boundaries, err
	}
	defer xTensor.Destroy()

	var inputs []ort.Value
	inputs = append(inputs, xTensor)

	var langTensor *ort.Tensor[int64]
	if useLanguage {
		langTensor, err = ort.NewTensor(ort.NewShape(1), []int64{langID})
		if err != nil {
			return boundaries, err
		}
		defer langTensor.Destroy()
		inputs = append(inputs, langTensor)
	}

	knownTensor, err := ort.NewTensor(ort.NewShape(knownBoundaries.Shape...), knownBoundaries.Data)
	if err != nil {
		return boundaries, err
	}
	defer knownTensor.Destroy()
	inputs = append(inputs, knownTensor)

	var prevTensor *ort.Tensor[bool]
	var tTensor *ort.Tensor[float32]
	if hasPrev {
		prevTensor, err = ort.NewTensor(ort.NewShape(prevBoundaries.Shape...), prevBoundaries.Data)
		if err != nil {
			return boundaries, err
		}
		defer prevTensor.Destroy()

		tTensor, err = ort.NewTensor(ort.NewShape(1), []float32{t})
		if err != nil {
			return boundaries, err
		}
		defer tTensor.Destroy()

		inputs = append(inputs, prevTensor, tTensor)
	}

	maskTensor, err := ort.NewTensor(ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return boundaries, err
	}
	defer maskTensor.Destroy()

	thTensor, err := ort.NewScalar(threshold)
	if err != nil {
		return boundaries, err
	}
	defer thTensor.Destroy()

	rTensor, err := ort.NewScalar(radius)
	if err != nil {
		return boundaries, err
	}
	defer rTensor.Destroy()

	inputs = append(inputs, maskTensor, thTensor, rTensor)

	outputs := []ort.Value{nil}
	if err := sess.Run(inputs, outputs); err != nil {
		return boundaries, err
	}
	defer destroyValues(outputs)

	bd, ok := outputs[0].(*ort.Tensor[bool])
	if !ok {
		return boundaries, fmt.Errorf("segmenter 输出类型不匹配")
	}
	boundaries = BoolTensor{Shape: shapeToSlice(bd.GetShape()), Data: append([]bool(nil), bd.GetData()...)}
	return boundaries, nil
}

func runBD2DurDynamic(onnxPath string, boundaries BoolTensor, maskT BoolTensor) (durations F32Tensor, maskN BoolTensor, err error) {
	opts := buildSessionOptions()
	if opts != nil {
		defer opts.Destroy()
	}
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"boundaries", "maskT"},
		[]string{"durations", "maskN"},
		opts,
	)
	if err != nil {
		return durations, maskN, err
	}
	defer sess.Destroy()

	bTensor, err := ort.NewTensor(ort.NewShape(boundaries.Shape...), boundaries.Data)
	if err != nil {
		return durations, maskN, err
	}
	defer bTensor.Destroy()

	mTensor, err := ort.NewTensor(ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return durations, maskN, err
	}
	defer mTensor.Destroy()

	outputs := []ort.Value{nil, nil}
	if err := sess.Run([]ort.Value{bTensor, mTensor}, outputs); err != nil {
		return durations, maskN, err
	}
	defer destroyValues(outputs)

	durT, ok0 := outputs[0].(*ort.Tensor[float32])
	maskNT, ok1 := outputs[1].(*ort.Tensor[bool])
	if !ok0 || !ok1 {
		return durations, maskN, fmt.Errorf("bd2dur 输出类型不匹配")
	}
	durations = F32Tensor{Shape: shapeToSlice(durT.GetShape()), Data: append([]float32(nil), durT.GetData()...)}
	maskN = BoolTensor{Shape: shapeToSlice(maskNT.GetShape()), Data: append([]bool(nil), maskNT.GetData()...)}
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
	opts := buildSessionOptions()
	if opts != nil {
		defer opts.Destroy()
	}
	sess, err := ort.NewDynamicAdvancedSession(
		onnxPath,
		[]string{"x_est", "boundaries", "maskT", "maskN", "threshold"},
		[]string{"presence", "scores"},
		opts,
	)
	if err != nil {
		return presence, scores, err
	}
	defer sess.Destroy()

	xTensor, err := ort.NewTensor(ort.NewShape(xEst.Shape...), xEst.Data)
	if err != nil {
		return presence, scores, err
	}
	defer xTensor.Destroy()

	bTensor, err := ort.NewTensor(ort.NewShape(boundaries.Shape...), boundaries.Data)
	if err != nil {
		return presence, scores, err
	}
	defer bTensor.Destroy()

	tTensor, err := ort.NewTensor(ort.NewShape(maskT.Shape...), maskT.Data)
	if err != nil {
		return presence, scores, err
	}
	defer tTensor.Destroy()

	nTensor, err := ort.NewTensor(ort.NewShape(maskN.Shape...), maskN.Data)
	if err != nil {
		return presence, scores, err
	}
	defer nTensor.Destroy()

	thTensor, err := ort.NewScalar(threshold)
	if err != nil {
		return presence, scores, err
	}
	defer thTensor.Destroy()

	outputs := []ort.Value{nil, nil}
	if err := sess.Run([]ort.Value{xTensor, bTensor, tTensor, nTensor, thTensor}, outputs); err != nil {
		return presence, scores, err
	}
	defer destroyValues(outputs)

	pT, ok0 := outputs[0].(*ort.Tensor[bool])
	sT, ok1 := outputs[1].(*ort.Tensor[float32])
	if !ok0 || !ok1 {
		return presence, scores, fmt.Errorf("estimator 输出类型不匹配")
	}

	presence = BoolTensor{Shape: shapeToSlice(pT.GetShape()), Data: append([]bool(nil), pT.GetData()...)}
	scores = F32Tensor{Shape: shapeToSlice(sT.GetShape()), Data: append([]float32(nil), sT.GetData()...)}
	return presence, scores, nil
}

func destroyValues(vals []ort.Value) {
	for _, v := range vals {
		if v != nil {
			v.Destroy()
		}
	}
}
