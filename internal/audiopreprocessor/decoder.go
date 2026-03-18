package audiopreprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
)

// Decoder 音频解码器
type Decoder struct {
	filePath string
	format   beep.Format
	streamer beep.StreamSeekCloser
}

// DecodedAudio 解码后的音频数据
type DecodedAudio struct {
	Samples    []float64 // 采样数据（归一化到 [-1.0, 1.0]）
	SampleRate int       // 采样率
	Channels   int       // 声道数
	Duration   float64   // 时长（秒）
}

// NewDecoder 创建音频解码器
func NewDecoder(filePath string) *Decoder {
	return &Decoder{
		filePath: filePath,
	}
}

// Decode 解码音频文件
func (d *Decoder) Decode() (*DecodedAudio, *AudioMeta, error) {
	// 打开文件
	f, err := os.Open(d.filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	// 根据格式选择解码器
	ext := filepath.Ext(d.filePath)
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
		return nil, nil, fmt.Errorf("不支持的音频格式: %s", ext)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("解码失败: %w", err)
	}

	d.streamer = streamer
	d.format = format

	// 读取所有采样数据
	samples := d.readAllSamples()

	// 计算时长
	duration := time.Duration(streamer.Len()) * time.Second / time.Duration(format.SampleRate)

	// 创建解码后音频对象
	decoded := &DecodedAudio{
		Samples:    samples,
		SampleRate: int(format.SampleRate),
		Channels:   format.NumChannels,
		Duration:   duration.Seconds(),
	}

	// 创建元信息
	meta := &AudioMeta{
		SampleRate: int(format.SampleRate),
		Channels:   format.NumChannels,
		Duration:   duration.Seconds(),
		BitDepth:   16, // beep 库内部使用 16 位深度
		Format:     GetFormatFromPath(d.filePath),
		LUFS:       0, // 将在后续步骤计算
	}

	return decoded, meta, nil
}

const decodeBufferFrames = 8192

// readAllSamples 读取所有采样数据并转换为 float64 切片（大缓冲区 + 预分配，减少分配与拷贝）
func (d *Decoder) readAllSamples() []float64 {
	if d.streamer == nil {
		return nil
	}

	totalFrames := d.streamer.Len()
	ch := d.format.NumChannels
	totalSamples := totalFrames * ch
	samples := make([]float64, totalSamples)

	buffer := make([][2]float64, decodeBufferFrames)
	pos := 0
	for {
		n, ok := d.streamer.Stream(buffer)
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

// Close 关闭解码器
func (d *Decoder) Close() error {
	if d.streamer != nil {
		return d.streamer.Close()
	}
	return nil
}

// GetAudioMeta 获取音频元信息（不读取采样数据）
func GetAudioMeta(filePath string) (*AudioMeta, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
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
		return nil, fmt.Errorf("不支持的音频格式: %s", ext)
	}

	if err != nil {
		return nil, fmt.Errorf("解码失败: %w", err)
	}
	defer streamer.Close()

	duration := time.Duration(streamer.Len()) * time.Second / time.Duration(format.SampleRate)

	return &AudioMeta{
		SampleRate: int(format.SampleRate),
		Channels:   format.NumChannels,
		Duration:   duration.Seconds(),
		BitDepth:   16,
		Format:     GetFormatFromPath(filePath),
		LUFS:       0,
	}, nil
}

// InitSpeaker 初始化音频播放（用于测试）
func InitSpeaker(sampleRate beep.SampleRate) error {
	return speaker.Init(sampleRate, sampleRate.N(time.Second/10))
}