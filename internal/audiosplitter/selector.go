package audiosplitter

import (
	"math"
	"sort"
)

// frameEnergyDB 计算一段采样的能量 (dB)，与 VAD 一致
func frameEnergyDB(samples []float64) float64 {
	if len(samples) == 0 {
		return -100.0
	}
	var sumSq float64
	for _, s := range samples {
		sumSq += s * s
	}
	ms := sumSq / float64(len(samples))
	if ms <= 0 {
		return -100.0
	}
	return 10 * math.Log10(ms)
}

// SliceSelector 切分点选择器
type SliceSelector struct {
	config *SplitterConfig
}

// NewSliceSelector 创建切分点选择器
func NewSliceSelector(config *SplitterConfig) *SliceSelector {
	return &SliceSelector{
		config: config,
	}
}

// SelectSlices 基于静音段选择切片：只保留有声音的片段，静音直接丢掉
func (ss *SliceSelector) SelectSlices(audio *AudioData, silences []SilenceSegment) []AudioSlice {
	// 按时间排序静音段
	sort.Slice(silences, func(i, j int) bool {
		return silences[i].StartTime < silences[j].StartTime
	})

	// 语音段 = 静音段之间的间隙（静音不写入任何切片）
	speechSlices := ss.speechSegmentsToSlices(audio, silences)

	// 每个切片裁掉首尾静音，只保留有声音的部分
	speechSlices = ss.trimSilenceEdges(audio, speechSlices)

	// 过滤过短切片并合并/切分（传入 audio 以便在静音/低能量处切分，避免从音中间断）
	return ss.filterAndMergeSlices(audio, speechSlices)
}

// speechSegmentsToSlices 将静音段取反得到语音段，并转为 AudioSlice（静音全部丢弃）
func (ss *SliceSelector) speechSegmentsToSlices(audio *AudioData, silences []SilenceSegment) []AudioSlice {
	sr := float64(audio.SampleRate)
	duration := audio.Duration
	audioLen := len(audio.Samples)

	if len(silences) == 0 {
		return []AudioSlice{{
			Index:       0,
			StartTime:   0,
			EndTime:     duration,
			Duration:    duration,
			StartSample: 0,
			EndSample:   audioLen,
		}}
	}

	var slices []AudioSlice
	idx := 0

	// 第一段：开头到第一个静音前
	if silences[0].StartTime > 0.01 {
		startTime := 0.0
		endTime := silences[0].StartTime
		startSample := 0
		endSample := int(endTime * sr)
		if endSample > audioLen {
			endSample = audioLen
		}
		if endSample > startSample {
			slices = append(slices, AudioSlice{
				Index:       idx,
				StartTime:   startTime,
				EndTime:     endTime,
				Duration:    endTime - startTime,
				StartSample: startSample,
				EndSample:   endSample,
			})
			idx++
		}
	}

	// 中间：静音与静音之间的语音
	for i := 0; i+1 < len(silences); i++ {
		startTime := silences[i].EndTime
		endTime := silences[i+1].StartTime
		if endTime <= startTime+0.01 {
			continue
		}
		startSample := int(startTime * sr)
		endSample := int(endTime * sr)
		if startSample < 0 {
			startSample = 0
		}
		if endSample > audioLen {
			endSample = audioLen
		}
		if endSample > startSample {
			slices = append(slices, AudioSlice{
				Index:       idx,
				StartTime:   startTime,
				EndTime:     endTime,
				Duration:    endTime - startTime,
				StartSample: startSample,
				EndSample:   endSample,
			})
			idx++
		}
	}

	// 最后一段：最后一个静音后到结尾
	lastEnd := silences[len(silences)-1].EndTime
	if duration > lastEnd+0.01 {
		startTime := lastEnd
		endTime := duration
		startSample := int(startTime * sr)
		endSample := audioLen
		if startSample < 0 {
			startSample = 0
		}
		if endSample > startSample {
			slices = append(slices, AudioSlice{
				Index:       idx,
				StartTime:   startTime,
				EndTime:     endTime,
				Duration:    endTime - startTime,
				StartSample: startSample,
				EndSample:   endSample,
			})
		}
	}

	return slices
}

// trimSilenceEdges 裁掉每个切片首尾的静音，只保留有声音的部分
func (ss *SliceSelector) trimSilenceEdges(audio *AudioData, slices []AudioSlice) []AudioSlice {
	sr := audio.SampleRate
	hopSize := ss.config.GetHopSize(sr)
	frameSize := ss.config.GetFrameSize(sr)
	samples := audio.Samples
	threshold := ss.config.EnergyThreshold // 高于此 dB 视为有声音

	var out []AudioSlice
	for _, sl := range slices {
		start, end := sl.StartSample, sl.EndSample
		if start < 0 {
			start = 0
		}
		if end > len(samples) {
			end = len(samples)
		}
		if end <= start+frameSize {
			continue
		}
		// 从开头找第一帧有声音
		firstLoud := start
		for i := start; i+frameSize <= end; i += hopSize {
			frame := samples[i : i+frameSize]
			if frameEnergyDB(frame) > threshold {
				firstLoud = i
				break
			}
		}
		// 从结尾找最后一帧有声音
		lastLoud := end - frameSize
		if lastLoud < firstLoud {
			lastLoud = firstLoud
		}
		for i := end - frameSize; i >= firstLoud && i >= start; i -= hopSize {
			frame := samples[i : i+frameSize]
			if frameEnergyDB(frame) > threshold {
				lastLoud = i + frameSize
				if lastLoud > end {
					lastLoud = end
				}
				break
			}
		}
		if lastLoud <= firstLoud {
			lastLoud = firstLoud + hopSize
			if lastLoud > end {
				lastLoud = end
			}
		}
		dur := float64(lastLoud-firstLoud) / float64(sr)
		out = append(out, AudioSlice{
			Index:       sl.Index,
			StartTime:   float64(firstLoud) / float64(sr),
			EndTime:     float64(lastLoud) / float64(sr),
			Duration:    dur,
			StartSample: firstLoud,
			EndSample:   lastLoud,
		})
	}
	return out
}

// filterAndMergeSlices 过滤过短切片并合并相邻切片；切分过长切片时在静音/低能量处下刀，避免从音中间断
func (ss *SliceSelector) filterAndMergeSlices(audio *AudioData, slices []AudioSlice) []AudioSlice {
	if len(slices) == 0 {
		return slices
	}

	// 第一轮：只保留过短过滤
	var validSlices []AudioSlice
	for _, slice := range slices {
		if slice.Duration >= ss.config.MinSliceDuration {
			validSlices = append(validSlices, slice)
		}
	}

	// 第二轮：合并过短相邻切片
	var mergedSlices []AudioSlice
	var currentSlice *AudioSlice

	for i := range validSlices {
		if currentSlice == nil {
			currentSlice = &AudioSlice{
				Index:       validSlices[i].Index,
				StartTime:   validSlices[i].StartTime,
				EndTime:     validSlices[i].EndTime,
				Duration:    validSlices[i].Duration,
				StartSample: validSlices[i].StartSample,
				EndSample:   validSlices[i].EndSample,
			}
		} else {
			if currentSlice.Duration < ss.config.TargetSliceDuration {
				currentSlice.EndTime = validSlices[i].EndTime
				currentSlice.EndSample = validSlices[i].EndSample
				currentSlice.Duration = currentSlice.EndTime - currentSlice.StartTime
			} else {
				mergedSlices = append(mergedSlices, *currentSlice)
				currentSlice = &AudioSlice{
					Index:       validSlices[i].Index,
					StartTime:   validSlices[i].StartTime,
					EndTime:     validSlices[i].EndTime,
					Duration:    validSlices[i].Duration,
					StartSample: validSlices[i].StartSample,
					EndSample:   validSlices[i].EndSample,
				}
			}
		}
	}

	if currentSlice != nil {
		mergedSlices = append(mergedSlices, *currentSlice)
	}

	// 第三轮：过长切片在低能量/静音处切分，避免从音中间断
	var finalSlices []AudioSlice
	index := 0
	for _, slice := range mergedSlices {
		if slice.Duration <= ss.config.MaxSliceDuration {
			slice.Index = index
			finalSlices = append(finalSlices, slice)
			index++
		} else {
			cutPoints := ss.findCutPointsInSlice(audio, &slice)
			start := slice.StartSample
			startTime := slice.StartTime
			sr := float64(audio.SampleRate)
			for i, cut := range cutPoints {
				endSample := cut
				if i == len(cutPoints)-1 {
					endSample = slice.EndSample
				}
				endTime := float64(endSample) / sr
				if endSample > start {
					finalSlices = append(finalSlices, AudioSlice{
						Index:       index,
						StartTime:   startTime,
						EndTime:     endTime,
						Duration:    endTime - startTime,
						StartSample: start,
						EndSample:   endSample,
					})
					index++
				}
				start = endSample
				startTime = endTime
			}
		}
	}

	return finalSlices
}

// 切分时只允许在「真实静音」处下刀，避免在元音/辅音之间的能量谷切
const (
	cutSearchRadiusSec = 0.6  // 目标位置两侧搜索范围（秒）
	cutWindowSec       = 0.25 // 判定静音用的窗口（秒）
)

// findCutPointsInSlice 在过长切片内找切分点：只在能量低于静音阈值的帧处切，避免元音辅音中间断
// 若某段在搜索范围内找不到静音，则不在该处下刀，保留为较长一段
func (ss *SliceSelector) findCutPointsInSlice(audio *AudioData, slice *AudioSlice) []int {
	samples := audio.Samples
	sr := audio.SampleRate
	start, end := slice.StartSample, slice.EndSample
	if start >= end {
		return nil
	}
	total := end - start
	numSub := int(math.Ceil(slice.Duration / ss.config.TargetSliceDuration))
	if numSub <= 1 {
		return []int{end}
	}
	hopSize := ss.config.GetHopSize(sr)
	searchRadius := int(float64(sr) * cutSearchRadiusSec)
	windowLen := int(float64(sr) * cutWindowSec)
	if windowLen > total/4 {
		windowLen = total / 4
	}
	if searchRadius > total/4 {
		searchRadius = total / 4
	}
	silenceThreshold := ss.config.EnergyThreshold // 只有低于此 dB 才视为可切分静音（真实静音）

	var cutPoints []int
	for k := 1; k < numSub; k++ {
		targetRatio := float64(k) / float64(numSub)
		target := start + int(float64(total)*targetRatio)
		low := target - searchRadius
		if low < start {
			low = start
		}
		high := target + searchRadius
		if high+windowLen > end {
			high = end - windowLen
		}
		if high <= low {
			continue
		}

		// 只在「真实静音」处下刀：能量 < 静音阈值，选离 target 最近的那一帧
		bestSilencePos := -1
		bestSilenceDist := 1 << 30
		for pos := low; pos <= high && pos+windowLen <= end; pos += hopSize {
			e := frameEnergyDB(samples[pos : pos+windowLen])
			if e >= silenceThreshold {
				continue
			}
			dist := pos - target
			if dist < 0 {
				dist = -dist
			}
			if dist < bestSilenceDist {
				bestSilenceDist = dist
				bestSilencePos = pos + windowLen/2
			}
		}
		if bestSilencePos >= 0 {
			cutPoints = append(cutPoints, bestSilencePos)
		}
		// 找不到静音则不在该处切，避免元音辅音中间断；可能得到较少、较长的切片
	}
	cutPoints = append(cutPoints, end)
	// 保证切点按样本递增，避免重叠
	sort.Ints(cutPoints)
	// 去重并保证 end 在末尾
	seen := make(map[int]bool)
	var out []int
	for _, c := range cutPoints {
		if seen[c] || c <= start {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	if len(out) == 0 || out[len(out)-1] != end {
		out = append(out, end)
	}
	return out
}

// GetFilteredInfo 获取被过滤的切片信息
func (ss *SliceSelector) GetFilteredInfo(slices []AudioSlice) []FilteredInfo {
	// 这里可以记录被过滤的原因
	// 简化实现，返回空列表
	return []FilteredInfo{}
}
