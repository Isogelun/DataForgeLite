package audiosplitter

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AudioExporter 音频导出器
type AudioExporter struct {
	config *SplitterConfig
}

// NewAudioExporter 创建音频导出器
func NewAudioExporter(config *SplitterConfig) *AudioExporter {
	return &AudioExporter{
		config: config,
	}
}

// ExportSlices 导出切片为 WAV 文件
func (ae *AudioExporter) ExportSlices(audio *AudioData, slices []AudioSlice, baseName string) error {
	n := len(audio.Samples)
	if n == 0 {
		return fmt.Errorf("音频无采样数据，无法导出切片")
	}
	for i := range slices {
		slices[i].Filename = ae.config.GetSliceOutputPath(baseName, slices[i].Index)
		start := slices[i].StartSample
		end := slices[i].EndSample
		if start < 0 {
			start = 0
		}
		if end > n {
			end = n
		}
		if start >= end {
			end = start + ae.config.GetHopSize(audio.SampleRate)*10
			if end > n {
				end = n
			}
			if start >= end {
				start = 0
				end = ae.config.GetHopSize(audio.SampleRate) * 3
				if end > n {
					end = n
				}
			}
		}
		sliceSamples := audio.Samples[start:end]
		if err := ae.exportWAV(sliceSamples, audio.SampleRate, slices[i].Filename); err != nil {
			return fmt.Errorf("导出切片 %d 失败: %w", slices[i].Index, err)
		}
	}
	return nil
}

// ExportMetadata 导出切片元数据为 JSON
func (ae *AudioExporter) ExportMetadata(metadata *SliceMetadata, baseName string) error {
	outputPath := ae.config.GetMetadataOutputPath(baseName)

	jsonData, err := metadata.ToJSON()
	if err != nil {
		return fmt.Errorf("序列化元数据失败: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(jsonData), 0644); err != nil {
		return fmt.Errorf("写入元数据文件失败: %w", err)
	}

	return nil
}

// exportWAV 导出 WAV 文件
func (ae *AudioExporter) exportWAV(samples []float64, sampleRate int, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	// 转换浮点采样为 16 位整数
	intSamples := make([]int16, len(samples))
	for i, sample := range samples {
		// 限制在 [-1.0, 1.0] 范围内
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		intSamples[i] = int16(sample * 32767)
	}

	// 计算数据大小
	dataSize := len(intSamples) * 2 // 16 位 = 2 字节
	byteRate := sampleRate * 1 * 2  // 单声道，16 位
	blockAlign := 1 * 2             // 单声道，16 位

	// 写入 WAV 头部
	// RIFF chunk
	file.WriteString("RIFF")
	binary.Write(file, binary.LittleEndian, uint32(36+dataSize))
	file.WriteString("WAVE")

	// fmt sub-chunk
	file.WriteString("fmt ")
	binary.Write(file, binary.LittleEndian, uint32(16)) // PCM
	binary.Write(file, binary.LittleEndian, uint16(1))  // 音频格式
	binary.Write(file, binary.LittleEndian, uint16(1))  // 单声道
	binary.Write(file, binary.LittleEndian, uint32(sampleRate))
	binary.Write(file, binary.LittleEndian, uint32(byteRate))
	binary.Write(file, binary.LittleEndian, uint16(blockAlign))
	binary.Write(file, binary.LittleEndian, uint16(16)) // 位深度

	// data sub-chunk
	file.WriteString("data")
	binary.Write(file, binary.LittleEndian, uint32(dataSize))

	// 批量写入采样数据（避免逐 sample 系统调用）
	buf := make([]byte, len(intSamples)*2)
	for i, sample := range intSamples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
	}
	if _, err := file.Write(buf); err != nil {
		return fmt.Errorf("写入采样数据失败: %w", err)
	}
	return nil
}

// GetBaseName 从文件路径获取基础文件名（不含扩展名）
func GetBaseName(filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// CreateSliceMetadata 创建切片元数据
func CreateSliceMetadata(originalFile string, audio *AudioData, slices []AudioSlice, filtered []FilteredInfo) *SliceMetadata {
	return &SliceMetadata{
		OriginalFile:     filepath.Base(originalFile),
		OriginalDuration: audio.Duration,
		SampleRate:       audio.SampleRate,
		TotalSlices:      len(slices),
		Slices:           slices,
		FilteredSlices:   filtered,
	}
}