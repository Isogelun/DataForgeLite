package exporter

import (
	"fmt"
	"os"
)

// Config 包含导出工具的所有配置参数
type Config struct {
	// WavsDir WAV文件输入目录
	WavsDir string
	// TgDir TextGrid文件目录
	TgDir string
	// OutputDir 输出目录
	OutputDir string
	// SkipSilenceInsertion 是否跳过静音插入
	SkipSilenceInsertion bool
	// WavSubtype WAV子类型格式（如 "PCM_16", "PCM_24", "PCM_32"）
	WavSubtype string
	// TargetSampleRate 目标采样率，默认44100
	TargetSampleRate int
	// SilenceMin 静音最小时长（秒），默认0.1
	SilenceMin float64
	// SilenceMax 静音最大时长（秒），默认0.5
	SilenceMax float64
	// PhoneTierName 音素层级名称，默认 "ph"
	PhoneTierName string
	// DictPath 音素词典文件路径，用于计算 ph_num（可选）
	DictPath string
	// EnablePhNum 是否启用 ph_num 计算（需要设置 DictPath）
	EnablePhNum bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		TargetSampleRate:     44100,
		SilenceMin:           0.1,
		SilenceMax:           0.5,
		WavSubtype:           "PCM_16",
		SkipSilenceInsertion: false,
		PhoneTierName:        "phones",
	}
}

// NewConfig 使用选项模式创建配置
func NewConfig(opts ...Option) *Config {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Option 配置选项函数类型
type Option func(*Config)

// WithWavsDir 设置WAV文件目录
func WithWavsDir(dir string) Option {
	return func(c *Config) {
		c.WavsDir = dir
	}
}

// WithTgDir 设置TextGrid文件目录
func WithTgDir(dir string) Option {
	return func(c *Config) {
		c.TgDir = dir
	}
}

// WithOutputDir 设置输出目录
func WithOutputDir(dir string) Option {
	return func(c *Config) {
		c.OutputDir = dir
	}
}

// WithSkipSilenceInsertion 设置是否跳过静音插入
func WithSkipSilenceInsertion(skip bool) Option {
	return func(c *Config) {
		c.SkipSilenceInsertion = skip
	}
}

// WithWavSubtype 设置WAV子类型格式
func WithWavSubtype(subtype string) Option {
	return func(c *Config) {
		c.WavSubtype = subtype
	}
}

// WithTargetSampleRate 设置目标采样率
func WithTargetSampleRate(rate int) Option {
	return func(c *Config) {
		c.TargetSampleRate = rate
	}
}

// WithSilenceRange 设置静音时长范围
func WithSilenceRange(min, max float64) Option {
	return func(c *Config) {
		c.SilenceMin = min
		c.SilenceMax = max
	}
}

// WithPhoneTierName 设置音素层级名称
func WithPhoneTierName(name string) Option {
	return func(c *Config) {
		c.PhoneTierName = name
	}
}

// WithDictPath 设置音素词典文件路径
func WithDictPath(path string) Option {
	return func(c *Config) {
		c.DictPath = path
	}
}

// WithEnablePhNum 设置是否启用 ph_num 计算
func WithEnablePhNum(enable bool) Option {
	return func(c *Config) {
		c.EnablePhNum = enable
	}
}

// Validate 验证配置参数
func (c *Config) Validate() error {
	// 验证目录
	if c.WavsDir == "" {
		return NewConfigError("WAV文件目录不能为空", nil)
	}
	if c.TgDir == "" {
		return NewConfigError("TextGrid文件目录不能为空", nil)
	}
	if c.OutputDir == "" {
		return NewConfigError("输出目录不能为空", nil)
	}

	// 检查输入目录是否存在
	if _, err := os.Stat(c.WavsDir); os.IsNotExist(err) {
		return NewConfigError(fmt.Sprintf("WAV文件目录不存在: %s", c.WavsDir), err)
	}
	if _, err := os.Stat(c.TgDir); os.IsNotExist(err) {
		return NewConfigError(fmt.Sprintf("TextGrid文件目录不存在: %s", c.TgDir), err)
	}

	// 验证采样率
	if c.TargetSampleRate <= 0 {
		return NewConfigError(fmt.Sprintf("无效的采样率: %d", c.TargetSampleRate), nil)
	}

	// 验证静音时长范围
	if c.SilenceMin < 0 {
		return NewConfigError(fmt.Sprintf("静音最小时长不能为负数: %f", c.SilenceMin), nil)
	}
	if c.SilenceMax < c.SilenceMin {
		return NewConfigError(fmt.Sprintf("静音最大时长(%f)不能小于最小时长(%f)", c.SilenceMax, c.SilenceMin), nil)
	}

	// 验证WAV子类型
	validSubtypes := map[string]bool{
		"PCM_16":  true,
		"PCM_24":  true,
		"PCM_32":  true,
		"PCM_U8":  true,
		"PCM_F32": true,
		"PCM_F64": true,
	}
	if !validSubtypes[c.WavSubtype] {
		return NewConfigError(fmt.Sprintf("无效的WAV子类型: %s", c.WavSubtype), nil)
	}

	// 验证音素层级名称
	if c.PhoneTierName == "" {
		return NewConfigError("音素层级名称不能为空", nil)
	}

	// 如果启用 ph_num 计算，验证词典文件
	if c.EnablePhNum {
		if c.DictPath == "" {
			return NewConfigError("启用 ph_num 计算时，词典文件路径不能为空", nil)
		}
		if _, err := os.Stat(c.DictPath); os.IsNotExist(err) {
			return NewConfigError(fmt.Sprintf("词典文件不存在: %s", c.DictPath), err)
		}
	}

	return nil
}

// String 返回配置的字符串表示
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{WavsDir: %q, TgDir: %q, OutputDir: %q, TargetSampleRate: %d, "+
			"SkipSilenceInsertion: %v, SilenceMin: %.2f, SilenceMax: %.2f, "+
			"WavSubtype: %q, PhoneTierName: %q, DictPath: %q, EnablePhNum: %v}",
		c.WavsDir, c.TgDir, c.OutputDir, c.TargetSampleRate,
		c.SkipSilenceInsertion, c.SilenceMin, c.SilenceMax,
		c.WavSubtype, c.PhoneTierName, c.DictPath, c.EnablePhNum,
	)
}
