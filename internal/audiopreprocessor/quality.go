package audiopreprocessor

import (
	"math"
)

// QualityChecker 质量检测器
type QualityChecker struct {
	silenceThreshold  float64 // 静音阈值 (dB)
	clippingThreshold float64 // 截幅阈值 (dB)
	minSilenceDuration float64 // 最小静音段时长（秒）
	sampleRate        int
}

// NewQualityChecker 创建质量检测器
func NewQualityChecker(silenceThreshold, clippingThreshold float64, sampleRate int) *QualityChecker {
	return &QualityChecker{
		silenceThreshold:   silenceThreshold,
		clippingThreshold:  clippingThreshold,
		minSilenceDuration: 0.1, // 默认 100ms
		sampleRate:         sampleRate,
	}
}

// Check 执行质量检测
func (qc *QualityChecker) Check(samples []float64, channels int) *QualityReport {
	report := &QualityReport{
		SilenceSegments: make([]Segment, 0),
	}

	// 检测静音段
	report.SilenceSegments = qc.detectSilence(samples, channels)

	// 检测截幅
	clippingCount, clippingRatio := qc.detectClipping(samples)
	report.ClippingCount = clippingCount
	report.ClippingRatio = clippingRatio

	// 判断是否存在质量问题
	report.HasQualityIssue = len(report.SilenceSegments) > 0 || clippingRatio > 0.001

	return report
}

// detectSilence 检测静音段
func (qc *QualityChecker) detectSilence(samples []float64, channels int) []Segment {
	if len(samples) == 0 {
		return nil
	}

	silenceThresholdLinear := math.Pow(10, qc.silenceThreshold/20)
	sampleCount := len(samples) / channels

	var segments []Segment
	inSilence := false
	silenceStart := 0

	for i := 0; i < sampleCount; i++ {
		// 计算当前帧的最大振幅
		maxAmp := 0.0
		for ch := 0; ch < channels; ch++ {
			amp := math.Abs(samples[i*channels+ch])
			if amp > maxAmp {
				maxAmp = amp
			}
		}

		isSilent := maxAmp < silenceThresholdLinear

		if isSilent && !inSilence {
			// 静音开始
			inSilence = true
			silenceStart = i
		} else if !isSilent && inSilence {
			// 静音结束
			inSilence = false
			silenceEnd := i
			duration := float64(silenceEnd-silenceStart) / float64(qc.sampleRate)

			// 只记录超过最小时长的静音段
			if duration >= qc.minSilenceDuration {
				segments = append(segments, Segment{
					StartTime: float64(silenceStart) / float64(qc.sampleRate),
					EndTime:   float64(silenceEnd) / float64(qc.sampleRate),
				})
			}
		}
	}

	// 处理结尾的静音
	if inSilence {
		silenceEnd := sampleCount
		duration := float64(silenceEnd-silenceStart) / float64(qc.sampleRate)
		if duration >= qc.minSilenceDuration {
			segments = append(segments, Segment{
				StartTime: float64(silenceStart) / float64(qc.sampleRate),
				EndTime:   float64(silenceEnd) / float64(qc.sampleRate),
			})
		}
	}

	return segments
}

// detectClipping 检测截幅
func (qc *QualityChecker) detectClipping(samples []float64) (int, float64) {
	if len(samples) == 0 {
		return 0, 0
	}

	clippingThresholdLinear := math.Pow(10, qc.clippingThreshold/20)
	// 截幅阈值通常是接近 0 dBFS，即接近 1.0 的线性值
	// 使用一个稍微小一点的值作为实际截幅判断
	actualThreshold := 0.99 // 接近 0 dBFS

	if clippingThresholdLinear < 1.0 {
		actualThreshold = clippingThresholdLinear
	}

	clippingCount := 0
	for _, sample := range samples {
		if math.Abs(sample) >= actualThreshold {
			clippingCount++
		}
	}

	clippingRatio := float64(clippingCount) / float64(len(samples))

	return clippingCount, clippingRatio
}

// SetMinSilenceDuration 设置最小静音时长
func (qc *QualityChecker) SetMinSilenceDuration(duration float64) {
	qc.minSilenceDuration = duration
}

// CalculateSNR 计算信噪比 (dB)
func CalculateSNR(samples []float64, sampleRate int, silenceThreshold float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	// 检测静音段
	qc := NewQualityChecker(silenceThreshold, -0.5, sampleRate)
	report := qc.Check(samples, 1)

	// 计算信号能量
	signalEnergy := 0.0
	silenceEnergy := 0.0
	silenceSamples := 0

	// 标记静音样本
	isSilence := make([]bool, len(samples))
	for _, seg := range report.SilenceSegments {
		start := int(seg.StartTime * float64(sampleRate))
		end := int(seg.EndTime * float64(sampleRate))
		for i := start; i < end && i < len(samples); i++ {
			isSilence[i] = true
		}
	}

	// 计算能量
	for i, sample := range samples {
		if isSilence[i] {
			silenceEnergy += sample * sample
			silenceSamples++
		} else {
			signalEnergy += sample * sample
		}
	}

	if silenceSamples == 0 || silenceEnergy == 0 {
		return 60.0 // 没有明显静音，假设高信噪比
	}

	avgSignalPower := signalEnergy / float64(len(samples)-silenceSamples)
	avgNoisePower := silenceEnergy / float64(silenceSamples)

	if avgNoisePower == 0 {
		return 60.0
	}

	snr := 10 * math.Log10(avgSignalPower/avgNoisePower)
	return snr
}

// AnalyzeDynamicRange 分析动态范围
func AnalyzeDynamicRange(samples []float64) (minDB, maxDB, dynamicRange float64) {
	if len(samples) == 0 {
		return -96, 0, 0
	}

	minVal := 1.0
	maxVal := 0.0

	for _, sample := range samples {
		abs := math.Abs(sample)
		if abs > 0 && abs < minVal {
			minVal = abs
		}
		if abs > maxVal {
			maxVal = abs
		}
	}

	if minVal <= 0 {
		minVal = 0.00001 // -100 dB
	}

	minDB = 20 * math.Log10(minVal)
	maxDB = 20 * math.Log10(maxVal)
	dynamicRange = maxDB - minDB

	return minDB, maxDB, dynamicRange
}