// Package exporter 提供数据集导出功能
package exporter

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// AudioData 表示音频数据
type AudioData struct {
	// Samples 音频采样数据（归一化浮点，单声道）
	Samples []float32
	// SampleRate 采样率
	SampleRate int
	// Channels 原始声道数
	Channels int
}

// WAVProcessor WAV音频处理器
type WAVProcessor struct {
	// rng 随机数生成器
	rng *rand.Rand
}

// NewWAVProcessor 创建新的WAV处理器
func NewWAVProcessor() *WAVProcessor {
	return &WAVProcessor{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NewWAVProcessorWithSeed 使用指定种子创建WAV处理器（用于测试）
func NewWAVProcessorWithSeed(seed int64) *WAVProcessor {
	return &WAVProcessor{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// LoadWAV 读取WAV文件，返回音频数据和采样率
func (p *WAVProcessor) LoadWAV(filePath string) (*AudioData, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, NewFileError(fmt.Sprintf("WAV文件不存在: %s", filePath), err)
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, NewFileError(fmt.Sprintf("无法打开WAV文件: %s", filePath), err)
	}
	defer file.Close()

	// 创建WAV解码器
	decoder := wav.NewDecoder(file)

	// 读取WAV文件信息
	decoder.ReadInfo()

	if !decoder.IsValidFile() {
		return nil, NewParseError(fmt.Sprintf("无效的WAV文件格式: %s", filePath), nil)
	}

	// 获取格式信息
	format := decoder.Format()
	if format == nil {
		return nil, NewParseError(fmt.Sprintf("无法获取WAV格式信息: %s", filePath), nil)
	}

	numChannels := format.NumChannels
	sampleRate := int(format.SampleRate)
	bitDepth := int(decoder.BitDepth)

	// 读取完整的PCM数据
	buf, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, NewParseError(fmt.Sprintf("读取WAV数据失败: %s", filePath), err)
	}

	if buf == nil || len(buf.Data) == 0 {
		return nil, NewParseError(fmt.Sprintf("WAV文件没有音频数据: %s", filePath), nil)
	}

	// 将整数采样转换为浮点（归一化到[-1, 1]）
	maxVal := float32(math.Pow(2, float64(bitDepth))) / 2
	samples := make([]float32, len(buf.Data))
	for i, sample := range buf.Data {
		samples[i] = float32(sample) / maxVal
	}

	return &AudioData{
		Samples:    samples,
		SampleRate: sampleRate,
		Channels:   numChannels,
	}, nil
}

// Resample 重采样音频数据
// 使用线性插值算法实现
func (p *WAVProcessor) Resample(audio []float32, fromRate, toRate int) []float32 {
	if fromRate == toRate {
		// 无需重采样，返回副本
		result := make([]float32, len(audio))
		copy(result, audio)
		return result
	}

	ratio := float64(fromRate) / float64(toRate)
	outputLen := int(float64(len(audio)) / ratio)
	result := make([]float32, outputLen)

	for i := 0; i < outputLen; i++ {
		// 计算原始音频中的位置
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := float32(srcPos - float64(srcIdx))

		if srcIdx+1 < len(audio) {
			// 线性插值
			result[i] = audio[srcIdx]*(1-frac) + audio[srcIdx+1]*frac
		} else if srcIdx < len(audio) {
			result[i] = audio[srcIdx]
		} else {
			result[i] = 0
		}
	}

	return result
}

// ToMono 将多声道音频转换为单声道
// 输入: 按声道交错的音频数据 [L0, R0, L1, R1, ...]
// 输出: 单声道音频数据
func (p *WAVProcessor) ToMono(audio []float32, channels int) []float32 {
	if channels <= 1 {
		// 已经是单声道，返回副本
		result := make([]float32, len(audio))
		copy(result, audio)
		return result
	}

	numFrames := len(audio) / channels
	mono := make([]float32, numFrames)

	for i := 0; i < numFrames; i++ {
		var sum float32
		for ch := 0; ch < channels; ch++ {
			sum += audio[i*channels+ch]
		}
		mono[i] = sum / float32(channels)
	}

	return mono
}

// InsertSilence 在音频前后随机插入静音
// minSil, maxSil: 静音时长范围（秒）
// 返回: 插入静音后的音频数据，前静音时长（秒），后静音时长（秒）
func (p *WAVProcessor) InsertSilence(audioData []float32, sampleRate int, minSil, maxSil float64) ([]float32, float64, float64) {
	minSilSamples := int(minSil * float64(sampleRate))
	maxSilSamples := int(maxSil * float64(sampleRate))

	var frontSilenceDuration, backSilenceDuration float64

	// 随机决定是否在前面插入静音
	var frontSilence []float32
	if p.rng.Float64() < 0.5 && maxSilSamples > minSilSamples {
		lenSil := p.rng.Intn(maxSilSamples-minSilSamples) + minSilSamples
		frontSilence = make([]float32, lenSil)
		for i := range frontSilence {
			frontSilence[i] = 0
		}
		frontSilenceDuration = float64(lenSil) / float64(sampleRate)
	}

	// 随机决定是否在后面插入静音
	var backSilence []float32
	if p.rng.Float64() < 0.5 && maxSilSamples > minSilSamples {
		lenSil := p.rng.Intn(maxSilSamples-minSilSamples) + minSilSamples
		backSilence = make([]float32, lenSil)
		for i := range backSilence {
			backSilence[i] = 0
		}
		backSilenceDuration = float64(lenSil) / float64(sampleRate)
	}

	// 拼接音频
	result := make([]float32, 0, len(frontSilence)+len(audioData)+len(backSilence))
	result = append(result, frontSilence...)
	result = append(result, audioData...)
	result = append(result, backSilence...)

	return result, frontSilenceDuration, backSilenceDuration
}

// WriteWAV 将音频数据写入WAV文件
// subtype: WAV子类型格式（如 "PCM_16", "PCM_24", "PCM_32"）
func (p *WAVProcessor) WriteWAV(filePath string, audioData []float32, sampleRate int, subtype string) error {
	// 确保输出目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return NewFileError(fmt.Sprintf("无法创建输出目录: %s", dir), err)
	}

	// 创建文件
	file, err := os.Create(filePath)
	if err != nil {
		return NewFileError(fmt.Sprintf("无法创建WAV文件: %s", filePath), err)
	}
	defer file.Close()

	// 解析子类型，确定位深度
	bitDepth := 16
	switch subtype {
	case "PCM_8", "PCM_U8":
		bitDepth = 8
	case "PCM_16":
		bitDepth = 16
	case "PCM_24":
		bitDepth = 24
	case "PCM_32":
		bitDepth = 32
	case "PCM_F32":
		bitDepth = 32
	case "PCM_F64":
		bitDepth = 64
	}

	// 创建WAV编码器
	format := &audio.Format{
		NumChannels: 1,
		SampleRate:  sampleRate,
	}

	// 将浮点数据转换为整数
	maxVal := float32(math.Pow(2, float64(bitDepth))) / 2
	intSamples := make([]int, len(audioData))
	for i, sample := range audioData {
		// 限制范围在[-1, 1]
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}
		intSamples[i] = int(sample * (maxVal - 1))
	}

	buf := &audio.IntBuffer{
		Data:   intSamples,
		Format: format,
	}

	encoder := wav.NewEncoder(file, format.SampleRate, bitDepth, format.NumChannels, 1)
	if err := encoder.Write(buf); err != nil {
		return NewFileError(fmt.Sprintf("写入WAV数据失败: %s", filePath), err)
	}

	if err := encoder.Close(); err != nil {
		return NewFileError(fmt.Sprintf("关闭WAV编码器失败: %s", filePath), err)
	}

	return nil
}

// CopyOrConvertWAV 如果 WAV 已符合目标采样率则直接复制，否则解码重采样后写出
func (p *WAVProcessor) CopyOrConvertWAV(srcPath, dstPath string, targetSampleRate int, subtype string) error {
	// 快速读取 WAV 头判断是否需要转换
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	dec := wav.NewDecoder(f)
	dec.ReadInfo()
	srcRate := int(dec.SampleRate)
	srcChannels := int(dec.NumChans)
	f.Close()

	// 如果采样率和声道数都符合，直接文件复制
	if srcRate == targetSampleRate && srcChannels == 1 {
		src, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = io.Copy(dst, src)
		return err
	}

	// 否则走完整解码/转换流程
	audioData, err := p.LoadWAV(srcPath)
	if err != nil {
		return err
	}
	if audioData.SampleRate != targetSampleRate {
		audioData.Samples = p.Resample(audioData.Samples, audioData.SampleRate, targetSampleRate)
		audioData.SampleRate = targetSampleRate
	}
	if audioData.Channels > 1 {
		audioData.Samples = p.ToMono(audioData.Samples, audioData.Channels)
		audioData.Channels = 1
	}
	return p.WriteWAV(dstPath, audioData.Samples, audioData.SampleRate, subtype)
}

// 用于处理一些非标准格式的WAV文件
func (p *WAVProcessor) LoadWAVRaw(filePath string) (*AudioData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, NewFileError(fmt.Sprintf("无法打开WAV文件: %s", filePath), err)
	}
	defer file.Close()

	// 读取WAV头
	header := make([]byte, 44)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, NewParseError("读取WAV头失败", err)
	}

	// 验证RIFF头
	if string(header[0:4]) != "RIFF" {
		return nil, NewParseError("无效的WAV文件：缺少RIFF头", nil)
	}
	if string(header[8:12]) != "WAVE" {
		return nil, NewParseError("无效的WAV文件：缺少WAVE标识", nil)
	}

	// 解析格式信息
	sampleRate := int(binary.LittleEndian.Uint32(header[24:28]))
	bitDepth := int(binary.LittleEndian.Uint16(header[34:36]))

	// 跳过可能存在的额外块，查找data块
	var dataSize int32
	for {
		chunkID := make([]byte, 4)
		if _, err := io.ReadFull(file, chunkID); err != nil {
			return nil, NewParseError("查找data块失败", err)
		}
		chunkSize := make([]byte, 4)
		if _, err := io.ReadFull(file, chunkSize); err != nil {
			return nil, NewParseError("读取块大小失败", err)
		}

		if string(chunkID) == "data" {
			dataSize = int32(binary.LittleEndian.Uint32(chunkSize))
			break
		}

		// 跳过其他块
		skipSize := int32(binary.LittleEndian.Uint32(chunkSize))
		if skipSize > 0 {
			file.Seek(int64(skipSize), 1)
		}
	}

	// 读取音频数据
	data := make([]byte, dataSize)
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, NewParseError("读取音频数据失败", err)
	}

	// 转换为浮点
	var samples []float32
	switch bitDepth {
	case 16:
		samples = p.bytesToFloat32_16(data)
	case 24:
		samples = p.bytesToFloat32_24(data)
	case 32:
		samples = p.bytesToFloat32_32(data)
	default:
		return nil, NewParseError(fmt.Sprintf("不支持的位深度: %d", bitDepth), nil)
	}

	return &AudioData{
		Samples:    samples,
		SampleRate: sampleRate,
		Channels:   1,
	}, nil
}

// bytesToFloat32_16 将16位PCM数据转换为浮点
func (p *WAVProcessor) bytesToFloat32_16(data []byte) []float32 {
	numSamples := len(data) / 2
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}

// bytesToFloat32_24 将24位PCM数据转换为浮点
func (p *WAVProcessor) bytesToFloat32_24(data []byte) []float32 {
	numSamples := len(data) / 3
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		b := data[i*3 : i*3+3]
		sample := int32(b[0]) | (int32(b[1]) << 8) | (int32(b[2]) << 16)
		if sample >= 0x800000 {
			sample -= 0x1000000
		}
		samples[i] = float32(sample) / 8388608.0
	}
	return samples
}

// bytesToFloat32_32 将32位PCM数据转换为浮点
func (p *WAVProcessor) bytesToFloat32_32(data []byte) []float32 {
	numSamples := len(data) / 4
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		sample := int32(binary.LittleEndian.Uint32(data[i*4 : i*4+4]))
		samples[i] = float32(sample) / 2147483648.0
	}
	return samples
}

// String 返回AudioData的字符串表示
func (ad *AudioData) String() string {
	duration := float64(len(ad.Samples)) / float64(ad.SampleRate)
	return fmt.Sprintf("AudioData{Samples: %d, SampleRate: %d, Channels: %d, Duration: %.2fs}",
		len(ad.Samples), ad.SampleRate, ad.Channels, duration)
}
