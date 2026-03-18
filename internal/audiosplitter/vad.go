package audiosplitter

import (
	"math"
)

// VADDetector VAD 静音检测器
type VADDetector struct {
	config *SplitterConfig
}

// NewVADDetector 创建 VAD 检测器
func NewVADDetector(config *SplitterConfig) *VADDetector {
	return &VADDetector{
		config: config,
	}
}

// Detect 检测音频中的静音段
func (vd *VADDetector) Detect(audio *AudioData) ([]SilenceSegment, error) {
	// 提取帧特征
	features := vd.ExtractFeatures(audio)

	// 基于特征检测静音段
	silences := vd.detectSilences(features, audio.SampleRate)

	return silences, nil
}

// ExtractFeatures 提取每帧的 VAD 特征
func (vd *VADDetector) ExtractFeatures(audio *AudioData) []FrameFeatures {
	frameSize := vd.config.GetFrameSize(audio.SampleRate)
	hopSize := vd.config.GetHopSize(audio.SampleRate)

	// 计算帧数
	numFrames := (len(audio.Samples) - frameSize) / hopSize
	if numFrames < 0 {
		numFrames = 0
	}

	features := make([]FrameFeatures, numFrames)

	for i := 0; i < numFrames; i++ {
		start := i * hopSize
		end := start + frameSize

		if end > len(audio.Samples) {
			break
		}

		frame := audio.Samples[start:end]

		// 计算能量（dB）
		energy := vd.calculateEnergy(frame)

		// 计算过零率
		zcr := vd.calculateZCR(frame)

		// 判断是否为静音帧（双门限）
		isSilence := energy < vd.config.EnergyThreshold || zcr > vd.config.ZCRThreshold

		features[i] = FrameFeatures{
			Energy:    energy,
			ZCR:       zcr,
			IsSilence: isSilence,
		}
	}

	return features
}

// calculateEnergy 计算帧能量（dB）
func (vd *VADDetector) calculateEnergy(frame []float64) float64 {
	if len(frame) == 0 {
		return -100.0
	}

	sumSquares := 0.0
	for _, sample := range frame {
		sumSquares += sample * sample
	}

	meanSquare := sumSquares / float64(len(frame))
	if meanSquare <= 0 {
		return -100.0
	}

	// 转换为 dB
	return 10 * math.Log10(meanSquare)
}

// calculateZCR 计算帧过零率
func (vd *VADDetector) calculateZCR(frame []float64) float64 {
	if len(frame) < 2 {
		return 0
	}

	zeroCrossings := 0
	for i := 1; i < len(frame); i++ {
		if (frame[i-1] >= 0 && frame[i] < 0) || (frame[i-1] < 0 && frame[i] >= 0) {
			zeroCrossings++
		}
	}

	return float64(zeroCrossings) / float64(len(frame)-1)
}

// detectSilences 基于特征检测静音段
func (vd *VADDetector) detectSilences(features []FrameFeatures, sampleRate int) []SilenceSegment {
	hopSize := vd.config.GetHopSize(sampleRate)
	minSilenceFrames := int(vd.config.MinSilenceDuration * float64(sampleRate) / float64(hopSize))

	var silences []SilenceSegment
	inSilence := false
	silenceStart := 0

	for i, feat := range features {
		if feat.IsSilence {
			if !inSilence {
				inSilence = true
				silenceStart = i
			}
		} else {
			if inSilence {
				inSilence = false
				silenceEnd := i
				silenceDuration := float64(silenceEnd-silenceStart) * float64(hopSize) / float64(sampleRate)

				// 只记录超过最小时长的静音段
				if silenceEnd-silenceStart >= minSilenceFrames {
					silences = append(silences, SilenceSegment{
						StartFrame: silenceStart,
						EndFrame:   silenceEnd,
						StartTime:  float64(silenceStart*hopSize) / float64(sampleRate),
						EndTime:    float64(silenceEnd*hopSize) / float64(sampleRate),
						Duration:   silenceDuration,
					})
				}
			}
		}
	}

	// 处理结尾的静音
	if inSilence {
		silenceEnd := len(features)
		silenceDuration := float64(silenceEnd-silenceStart) * float64(hopSize) / float64(sampleRate)

		if silenceEnd-silenceStart >= minSilenceFrames {
			silences = append(silences, SilenceSegment{
				StartFrame: silenceStart,
				EndFrame:   silenceEnd,
				StartTime:  float64(silenceStart*hopSize) / float64(sampleRate),
				EndTime:    float64(silenceEnd*hopSize) / float64(sampleRate),
				Duration:   silenceDuration,
			})
		}
	}

	return silences
}

// FilterShortSilences 过滤过短的静音段
func FilterShortSilences(silences []SilenceSegment, minDuration float64) []SilenceSegment {
	var filtered []SilenceSegment
	for _, s := range silences {
		if s.Duration >= minDuration {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// MergeCloseSilences 合并相邻的静音段
func MergeCloseSilences(silences []SilenceSegment, maxGap float64) []SilenceSegment {
	if len(silences) <= 1 {
		return silences
	}

	var merged []SilenceSegment
	current := silences[0]

	for i := 1; i < len(silences); i++ {
		if silences[i].StartTime-current.EndTime <= maxGap {
			// 合并
			current.EndFrame = silences[i].EndFrame
			current.EndTime = silences[i].EndTime
			current.Duration = current.EndTime - current.StartTime
		} else {
			merged = append(merged, current)
			current = silences[i]
		}
	}
	merged = append(merged, current)

	return merged
}