package audiosplitter

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/faiface/beep"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
)

// AudioLoader 音频加载器
type AudioLoader struct {
	filePath string
}

// NewAudioLoader 创建音频加载器
func NewAudioLoader(filePath string) *AudioLoader {
	return &AudioLoader{
		filePath: filePath,
	}
}

// Load 加载音频文件并转换为单声道浮点数据
func (al *AudioLoader) Load() (*AudioData, error) {
	// 打开文件
	f, err := os.Open(al.filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	// 根据格式解码
	ext := filepath.Ext(al.filePath)
	var streamer beep.StreamSeekCloser
	var format beep.Format

	switch ext {
	case ".wav":
		streamer, format, err = wav.Decode(f)
	case ".mp3":
		streamer, format, err = mp3.Decode(f)
	case ".flac":
		streamer, format, err = flac.Decode(f)
	case ".ogg", ".oga":
		streamer, format, err = vorbis.Decode(f)
	default:
		return nil, fmt.Errorf("不支持的音频格式: %s", ext)
	}

	if err != nil {
		return nil, fmt.Errorf("解码失败: %w", err)
	}
	defer streamer.Close()

	// 读取所有采样
	samples := al.readAllSamples(streamer, format)

	// 转换为单声道
	monoSamples := al.toMono(samples, format.NumChannels)

	// 计算时长
	duration := float64(len(monoSamples)) / float64(format.SampleRate)

	return &AudioData{
		Samples:    monoSamples,
		SampleRate: int(format.SampleRate),
		Duration:   duration,
		Format:     ext,
	}, nil
}

const loadBufferFrames = 8192

// readAllSamples 读取所有采样数据（大缓冲区 + 预分配）
func (al *AudioLoader) readAllSamples(streamer beep.StreamSeekCloser, format beep.Format) []float64 {
	totalFrames := streamer.Len()
	ch := format.NumChannels
	totalSamples := totalFrames * ch
	samples := make([]float64, totalSamples)

	buffer := make([][2]float64, loadBufferFrames)
	pos := 0
	for {
		n, ok := streamer.Stream(buffer)
		if !ok || n == 0 {
			break
		}
		for i := 0; i < n && pos < totalSamples; i++ {
			samples[pos] = buffer[i][0]
			pos++
			if ch > 1 && pos < totalSamples {
				samples[pos] = buffer[i][1]
				pos++
			}
		}
	}
	return samples[:pos]
}

// toMono 将多声道转换为单声道
func (al *AudioLoader) toMono(samples []float64, channels int) []float64 {
	if channels == 1 {
		return samples
	}

	sampleCount := len(samples) / channels
	mono := make([]float64, sampleCount)

	for i := 0; i < sampleCount; i++ {
		sum := 0.0
		for ch := 0; ch < channels; ch++ {
			sum += samples[i*channels+ch]
		}
		mono[i] = sum / float64(channels)
	}

	return mono
}

// LoadAudio 便捷函数：加载音频文件
func LoadAudio(filePath string) (*AudioData, error) {
	loader := NewAudioLoader(filePath)
	return loader.Load()
}

// IsValidAudioFile 检查是否为有效的音频文件
func IsValidAudioFile(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}

	ext := filepath.Ext(filePath)
	validExts := []string{".wav", ".mp3", ".flac", ".ogg", ".oga"}
	for _, valid := range validExts {
		if ext == valid {
			return true
		}
	}
	return false
}

// GetAudioInfo 获取音频信息（不加载采样数据）
func GetAudioInfo(filePath string) (duration float64, sampleRate int, channels int, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()

	ext := filepath.Ext(filePath)
	var streamer beep.StreamSeekCloser
	var format beep.Format

	switch ext {
	case ".wav":
		streamer, format, err = wav.Decode(f)
	case ".mp3":
		streamer, format, err = mp3.Decode(f)
	case ".flac":
		streamer, format, err = flac.Decode(f)
	case ".ogg", ".oga":
		streamer, format, err = vorbis.Decode(f)
	default:
		return 0, 0, 0, fmt.Errorf("不支持的格式")
	}

	if err != nil {
		return 0, 0, 0, err
	}
	defer streamer.Close()

	duration = float64(streamer.Len()) / float64(format.SampleRate)
	return duration, int(format.SampleRate), format.NumChannels, nil
}

// CalculateRMS 计算音频的 RMS 能量
func CalculateRMS(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	sumSquares := 0.0
	for _, sample := range samples {
		sumSquares += sample * sample
	}

	return math.Sqrt(sumSquares / float64(len(samples)))
}