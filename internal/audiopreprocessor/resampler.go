package audiopreprocessor

import (
	"math"
)

// Resampler 采样率转换器
type Resampler struct {
	sourceRate int
	targetRate int
}

// NewResampler 创建采样率转换器
func NewResampler(sourceRate, targetRate int) *Resampler {
	return &Resampler{
		sourceRate: sourceRate,
		targetRate: targetRate,
	}
}

// Resample 重采样音频数据
func (r *Resampler) Resample(samples []float64, channels int) []float64 {
	if r.sourceRate == r.targetRate {
		return samples
	}

	// 计算重采样后的采样数
	resampledLen := int(float64(len(samples)/channels) * float64(r.targetRate) / float64(r.sourceRate) * float64(channels))
	resampled := make([]float64, resampledLen)

	// 线性插值重采样
	ratio := float64(r.sourceRate) / float64(r.targetRate)

	for i := 0; i < resampledLen/channels; i++ {
		srcPos := float64(i) * ratio
		srcIndex := int(srcPos)
		frac := srcPos - float64(srcIndex)

		for ch := 0; ch < channels; ch++ {
			srcIdx1 := srcIndex*channels + ch
			srcIdx2 := (srcIndex+1)*channels + ch

			if srcIdx2 >= len(samples) {
				srcIdx2 = srcIdx1
			}

			// 线性插值
			val1 := samples[srcIdx1]
			val2 := samples[srcIdx2]
			resampled[i*channels+ch] = val1 + (val2-val1)*frac
		}
	}

	return resampled
}

// ConvertChannels 转换声道数
func ConvertChannels(samples []float64, sourceChannels, targetChannels int) []float64 {
	if sourceChannels == targetChannels {
		return samples
	}

	sampleCount := len(samples) / sourceChannels
	result := make([]float64, sampleCount*targetChannels)

	if sourceChannels == 2 && targetChannels == 1 {
		// 立体声转单声道：混合左右声道
		for i := 0; i < sampleCount; i++ {
			left := samples[i*2]
			right := samples[i*2+1]
			result[i] = (left + right) / 2
		}
	} else if sourceChannels == 1 && targetChannels == 2 {
		// 单声道转立体声：复制到左右声道
		for i := 0; i < sampleCount; i++ {
			val := samples[i]
			result[i*2] = val
			result[i*2+1] = val
		}
	}

	return result
}

// ChangeBitDepth 改变位深度（量化）
func ChangeBitDepth(samples []float64, targetBitDepth int) []float64 {
	if targetBitDepth == 32 {
		// 32位浮点，无需量化
		return samples
	}

	maxVal := math.Pow(2, float64(targetBitDepth-1)) - 1
	result := make([]float64, len(samples))

	for i, sample := range samples {
		// 归一化到目标位深度范围
		quantized := math.Round(sample * maxVal)
		result[i] = quantized / maxVal
	}

	return result
}