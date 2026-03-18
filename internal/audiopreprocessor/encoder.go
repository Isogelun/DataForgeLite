package audiopreprocessor

import (
	"encoding/binary"
	"fmt"
	"os"
)

// Encoder 音频编码器
type Encoder struct {
	sampleRate int
	channels   int
	bitDepth   int
}

// NewEncoder 创建音频编码器
func NewEncoder(sampleRate, channels, bitDepth int) *Encoder {
	if bitDepth != 16 && bitDepth != 24 && bitDepth != 32 {
		bitDepth = 16 // 默认 16 位
	}
	return &Encoder{
		sampleRate: sampleRate,
		channels:   channels,
		bitDepth:   bitDepth,
	}
}

// EncodeToWAV 将采样数据编码为 WAV 文件
func (e *Encoder) EncodeToWAV(samples []float64, outputPath string) error {
	// 转换浮点采样为整数采样
	intSamples := e.floatToIntSamples(samples)

	// 创建 WAV 文件
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	// 计算数据大小
	dataSize := len(intSamples) * e.bitDepth / 8
	byteRate := e.sampleRate * e.channels * e.bitDepth / 8
	blockAlign := e.channels * e.bitDepth / 8

	// 写入 WAV 头部
	// RIFF chunk
	file.WriteString("RIFF")
	binary.Write(file, binary.LittleEndian, uint32(36+dataSize)) // ChunkSize
	file.WriteString("WAVE")

	// fmt sub-chunk
	file.WriteString("fmt ")
	binary.Write(file, binary.LittleEndian, uint32(16))     // Subchunk1Size (PCM)
	binary.Write(file, binary.LittleEndian, uint16(1))      // AudioFormat (PCM)
	binary.Write(file, binary.LittleEndian, uint16(e.channels))
	binary.Write(file, binary.LittleEndian, uint32(e.sampleRate))
	binary.Write(file, binary.LittleEndian, uint32(byteRate))
	binary.Write(file, binary.LittleEndian, uint16(blockAlign))
	binary.Write(file, binary.LittleEndian, uint16(e.bitDepth))

	// data sub-chunk
	file.WriteString("data")
	binary.Write(file, binary.LittleEndian, uint32(dataSize))

	// 写入采样数据
	switch e.bitDepth {
	case 16:
		e.writeInt16Samples(file, intSamples)
	case 24:
		e.writeInt24Samples(file, intSamples)
	case 32:
		e.writeInt32Samples(file, intSamples)
	}

	return nil
}

// floatToIntSamples 将浮点采样 [-1.0, 1.0] 转换为整数采样
func (e *Encoder) floatToIntSamples(samples []float64) []int32 {
	maxVal := int32(1 << (e.bitDepth - 1))
	result := make([]int32, len(samples))

	for i, sample := range samples {
		// 限制在 [-1.0, 1.0] 范围内
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}

		// 转换为整数
		result[i] = int32(sample * float64(maxVal-1))
	}

	return result
}

// writeInt16Samples 写入 16 位采样（批量写入，避免逐 sample 系统调用）
func (e *Encoder) writeInt16Samples(file *os.File, samples []int32) error {
	buf := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(sample)))
	}
	_, err := file.Write(buf)
	return err
}

// writeInt24Samples 写入 24 位采样（批量写入）
func (e *Encoder) writeInt24Samples(file *os.File, samples []int32) error {
	buf := make([]byte, len(samples)*3)
	for i, sample := range samples {
		base := i * 3
		buf[base] = byte(sample)
		buf[base+1] = byte(sample >> 8)
		buf[base+2] = byte(sample >> 16)
	}
	_, err := file.Write(buf)
	return err
}

// writeInt32Samples 写入 32 位采样（批量写入）
func (e *Encoder) writeInt32Samples(file *os.File, samples []int32) error {
	buf := make([]byte, len(samples)*4)
	for i, sample := range samples {
		binary.LittleEndian.PutUint32(buf[i*4:], uint32(sample))
	}
	_, err := file.Write(buf)
	return err
}

// EncodeToWAVFile 便捷函数：直接编码为 WAV 文件
func EncodeToWAVFile(samples []float64, sampleRate, channels, bitDepth int, outputPath string) error {
	encoder := NewEncoder(sampleRate, channels, bitDepth)
	return encoder.EncodeToWAV(samples, outputPath)
}

// GetWAVHeaderSize 获取 WAV 文件头部大小
func GetWAVHeaderSize() int {
	return 44 // 标准 WAV 头部大小
}

// CalculateWAVFileSize 计算 WAV 文件大小
func CalculateWAVFileSize(sampleCount, bitDepth, channels int) int {
	dataSize := sampleCount * bitDepth / 8
	return 44 + dataSize // 头部 + 数据
}