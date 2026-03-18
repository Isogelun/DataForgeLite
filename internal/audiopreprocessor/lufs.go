package audiopreprocessor

import (
	"math"
)

// LUFSAnalyzer LUFS 响度分析器
type LUFSAnalyzer struct {
	sampleRate int
	channels   int
}

// NewLUFSAnalyzer 创建 LUFS 分析器
func NewLUFSAnalyzer(sampleRate, channels int) *LUFSAnalyzer {
	return &LUFSAnalyzer{
		sampleRate: sampleRate,
		channels:   channels,
	}
}

// CalculateLUFS 计算音频的 LUFS 响度值
// 基于 ITU-R BS.1770-4 标准
func (a *LUFSAnalyzer) CalculateLUFS(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	// K 加权滤波器系数
	// 预滤波器（高通）
	preFilterB := []float64{1.0, -2.0, 1.0}
	preFilterA := []float64{1.0, -1.99004745483398, 0.99007225036621}

	// 加权滤波器（搁架滤波器）
	weightFilterB := []float64{1.53512485958697, -2.69169618940638, 1.19839281085285}
	weightFilterA := []float64{1.0, -1.69065929318241, 0.73248077421585}

	// 应用 K 加权滤波器
	filtered := applyFilter(samples, preFilterB, preFilterA)
	filtered = applyFilter(filtered, weightFilterB, weightFilterA)

	// 计算均方能量
	var sumSquared float64
	for _, sample := range filtered {
		sumSquared += sample * sample
	}
	meanSquared := sumSquared / float64(len(filtered))

	// 计算 LUFS: -0.691 + 10*log10(均方能量)
	if meanSquared <= 0 {
		return -70 // 最小值
	}

	lufs := -0.691 + 10*math.Log10(meanSquared)

	return lufs
}

// applyFilter 应用 IIR 数字滤波器: y[n] = (b[0]x[n]+b[1]x[n-1]+... - a[1]y[n-1]-...) / a[0]
func applyFilter(samples []float64, b, a []float64) []float64 {
	if len(samples) == 0 {
		return samples
	}
	result := make([]float64, len(samples))
	a0 := a[0]
	if a0 == 0 {
		a0 = 1
	}
	for n := 0; n < len(result); n++ {
		var out float64
		for k := 0; k < len(b) && n-k >= 0; k++ {
			out += b[k] * samples[n-k]
		}
		for k := 1; k < len(a) && n-k >= 0; k++ {
			out -= a[k] * result[n-k]
		}
		result[n] = out / a0
	}
	return result
}

// CalculateGainAdjustment 计算增益调整值
func CalculateGainAdjustment(currentLUFS, targetLUFS float64) float64 {
	return targetLUFS - currentLUFS
}

// ApplyGain 应用增益到音频样本
func ApplyGain(samples []float64, gainDB float64) []float64 {
	if gainDB == 0 {
		return samples
	}

	gainLinear := math.Pow(10, gainDB/20)
	result := make([]float64, len(samples))

	for i, sample := range samples {
		result[i] = sample * gainLinear
	}

	return result
}

// ApplyTruePeakLimit 应用真峰值限制（软限幅 tanh，避免硬截断导致爆音/方波失真）
// limitDB 为天花板 dB，如 -1 表示最大真峰 -1 dBTP
func ApplyTruePeakLimit(samples []float64, limitDB float64) []float64 {
	if limitDB >= 0 {
		limitDB = -1.0
	}
	limitLinear := math.Pow(10, limitDB/20)
	if limitLinear <= 0 {
		return samples
	}
	result := make([]float64, len(samples))
	for i, sample := range samples {
		result[i] = limitLinear * math.Tanh(sample/limitLinear)
	}
	return result
}

// FindPeakLevel 查找音频峰值电平（dB）
func FindPeakLevel(samples []float64) float64 {
	if len(samples) == 0 {
		return -96.0
	}

	maxAbs := 0.0
	for _, sample := range samples {
		abs := math.Abs(sample)
		if abs > maxAbs {
			maxAbs = abs
		}
	}

	if maxAbs <= 0 {
		return -96.0
	}

	return 20 * math.Log10(maxAbs)
}

// NormalizeAudio 标准化音频响度（类似 AU：目标 LUFS + 真峰天花板，增益受天花板约束避免爆音）
func NormalizeAudio(samples []float64, sampleRate, channels int, targetLUFS, truePeakLimit float64) ([]float64, float64) {
	if len(samples) == 0 {
		return samples, 0
	}
	// 天花板必须为负值，类似 AU 的「最大真峰」
	if truePeakLimit >= 0 {
		truePeakLimit = -1.0
	}
	analyzer := NewLUFSAnalyzer(sampleRate, channels)
	currentLUFS := analyzer.CalculateLUFS(samples)

	// 无论是否做响度增益，最终都应用真峰限制，避免输出超过天花板
	applyCeiling := func(s []float64) []float64 {
		return ApplyTruePeakLimit(s, truePeakLimit)
	}

	// 若当前为静音或无效，只做天花板限制，不做增益
	if currentLUFS <= -69 || currentLUFS > 10 {
		return applyCeiling(samples), currentLUFS
	}

	gain := CalculateGainAdjustment(currentLUFS, targetLUFS)
	const maxGainDB = 24.0
	const minGainDB = -24.0
	if gain > maxGainDB {
		gain = maxGainDB
	}
	if gain < minGainDB {
		gain = minGainDB
	}

	// 天花板感知：若完整增益会导致峰值超过天花板，则压低增益，避免大量信号被限幅引发爆音
	peakLinear := 0.0
	for _, s := range samples {
		if abs := math.Abs(s); abs > peakLinear {
			peakLinear = abs
		}
	}
	if peakLinear > 1e-10 {
		limitLinear := math.Pow(10, truePeakLimit/20)
		gainLinear := math.Pow(10, gain/20)
		if peakLinear*gainLinear > limitLinear {
			gainLinear = limitLinear / peakLinear
			gain = 20 * math.Log10(gainLinear)
		}
	}

	normalized := ApplyGain(samples, gain)
	normalized = applyCeiling(normalized)
	finalLUFS := analyzer.CalculateLUFS(normalized)
	return normalized, finalLUFS
}