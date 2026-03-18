package audiopreprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// 默认配置常量
const (
	DefaultTargetLUFS         = -18.0
	DefaultTargetSampleRate   = 48000
	DefaultTargetChannels     = 1
	DefaultTruePeakLimit      = -1.0
	DefaultSilenceThreshold   = -50.0
	DefaultClippingThreshold  = -0.5
)

// SetDefaults 为配置设置默认值
func (c *ProcessorConfig) SetDefaults() {
	if c.TargetLUFS == 0 {
		c.TargetLUFS = DefaultTargetLUFS
	}
	if c.TargetSampleRate == 0 {
		c.TargetSampleRate = DefaultTargetSampleRate
	}
	if c.TargetChannels == 0 {
		c.TargetChannels = DefaultTargetChannels
	}
	if c.TruePeakLimit == 0 {
		c.TruePeakLimit = DefaultTruePeakLimit
	}
	if c.SilenceThreshold == 0 {
		c.SilenceThreshold = DefaultSilenceThreshold
	}
	if c.ClippingThreshold == 0 {
		c.ClippingThreshold = DefaultClippingThreshold
	}
	if c.Concurrency == 0 {
		c.Concurrency = runtime.NumCPU()
	}
	if !c.EnableQualityCheck {
		c.EnableQualityCheck = true
	}
}

// Validate 验证配置有效性
func (c *ProcessorConfig) Validate() error {
	// 验证输入路径
	if len(c.InputPaths) == 0 {
		return fmt.Errorf("输入路径列表不能为空")
	}

	for _, path := range c.InputPaths {
		if path == "" {
			return fmt.Errorf("输入路径不能为空字符串")
		}
	}

	// 验证输出目录
	if c.OutputDir == "" {
		return fmt.Errorf("输出目录不能为空")
	}

	// 检查输出目录是否存在，不存在则创建
	if err := os.MkdirAll(c.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 验证目标响度值
	if c.TargetLUFS < -70 || c.TargetLUFS > 0 {
		return fmt.Errorf("目标响度值 %.1f 超出有效范围 [-70, 0]", c.TargetLUFS)
	}

	// 验证目标采样率
	validSampleRates := []int{8000, 16000, 22050, 24000, 32000, 44100, 48000, 96000}
	if !containsInt(validSampleRates, c.TargetSampleRate) {
		return fmt.Errorf("不支持的目标采样率 %d，有效值: %v", c.TargetSampleRate, validSampleRates)
	}

	// 验证目标声道数
	if c.TargetChannels != 1 && c.TargetChannels != 2 {
		return fmt.Errorf("不支持的目标声道数 %d，仅支持单声道(1)或立体声(2)", c.TargetChannels)
	}

	// 验证真峰值限制
	if c.TruePeakLimit > 0 {
		return fmt.Errorf("真峰值限制 %.1f 必须小于等于 0", c.TruePeakLimit)
	}

	// 验证静音检测阈值
	if c.SilenceThreshold > 0 {
		return fmt.Errorf("静音检测阈值 %.1f 必须小于等于 0", c.SilenceThreshold)
	}

	// 验证截幅检测阈值
	if c.ClippingThreshold > 0 {
		return fmt.Errorf("截幅检测阈值 %.1f 必须小于等于 0", c.ClippingThreshold)
	}

	// 验证并发数
	if c.Concurrency < 1 {
		return fmt.Errorf("并发数 %d 必须大于等于 1", c.Concurrency)
	}

	return nil
}

// GetOutputPath 根据输入文件路径生成输出文件路径
func (c *ProcessorConfig) GetOutputPath(inputPath string) string {
	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := baseName[:len(baseName)-len(ext)]
	outputFileName := nameWithoutExt + "_processed.wav"
	return filepath.Join(c.OutputDir, outputFileName)
}

// IsSupportedFormat 检查是否为支持的音频格式
func IsSupportedFormat(filePath string) bool {
	ext := filepath.Ext(filePath)
	supportedExts := []string{".wav", ".mp3", ".flac", ".m4a", ".ogg", ".oga", ".ogv", ".aac", ".wma"}
	for _, supported := range supportedExts {
		if ext == supported {
			return true
		}
	}
	return false
}

// GetFormatFromPath 从文件路径获取音频格式
func GetFormatFromPath(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".wav":
		return "WAV"
	case ".mp3":
		return "MP3"
	case ".flac":
		return "FLAC"
	case ".m4a", ".aac":
		return "M4A"
	case ".ogg", ".oga":
		return "OGG"
	case ".wma":
		return "WMA"
	default:
		return "UNKNOWN"
	}
}

// containsInt 检查切片中是否包含指定整数
func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}