package audiosplitter

import (
	"fmt"
	"os"
	"path/filepath"
)

// 默认配置常量
const (
	DefaultTargetSliceDuration = 10.0  // 默认目标切片时长 10秒
	DefaultMinSliceDuration    = 3.0   // 默认最小切片时长 3秒
	DefaultMaxSliceDuration    = 20.0  // 默认最大切片时长 20秒
	DefaultPaddingDuration     = 0.3   // 默认边界padding 0.3秒
	DefaultEnergyThreshold     = -45.0 // 默认能量门限 -45dB，更多低能量判为静音，只保留有声音片段
	DefaultZCRThreshold        = 0.08  // 默认过零率门限（略放宽减少误判静音）
	DefaultMinSilenceDuration  = 0.4   // 默认最小静音段时长 0.4秒
)

// SetDefaults 为配置设置默认值
func (c *SplitterConfig) SetDefaults() {
	if c.TargetSliceDuration == 0 {
		c.TargetSliceDuration = DefaultTargetSliceDuration
	}
	if c.MinSliceDuration == 0 {
		c.MinSliceDuration = DefaultMinSliceDuration
	}
	if c.MaxSliceDuration == 0 {
		c.MaxSliceDuration = DefaultMaxSliceDuration
	}
	if c.PaddingDuration == 0 {
		c.PaddingDuration = DefaultPaddingDuration
	}
	if c.EnergyThreshold == 0 {
		c.EnergyThreshold = DefaultEnergyThreshold
	}
	if c.ZCRThreshold == 0 {
		c.ZCRThreshold = DefaultZCRThreshold
	}
	if c.MinSilenceDuration == 0 {
		c.MinSilenceDuration = DefaultMinSilenceDuration
	}
}

// Validate 验证配置有效性
func (c *SplitterConfig) Validate() error {
	// 验证输入路径
	if len(c.InputPaths) == 0 {
		return fmt.Errorf("输入路径列表不能为空")
	}

	// 验证输出目录
	if c.OutputDir == "" {
		return fmt.Errorf("输出目录不能为空")
	}

	// 创建输出目录
	if err := os.MkdirAll(c.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 验证时长参数
	if c.MinSliceDuration <= 0 {
		return fmt.Errorf("最小切片时长必须大于 0")
	}
	if c.MaxSliceDuration <= c.MinSliceDuration {
		return fmt.Errorf("最大切片时长必须大于最小切片时长")
	}
	if c.TargetSliceDuration < c.MinSliceDuration || c.TargetSliceDuration > c.MaxSliceDuration {
		return fmt.Errorf("目标切片时长必须在最小和最大切片时长之间")
	}

	// 验证门限参数
	if c.EnergyThreshold > 0 {
		return fmt.Errorf("能量门限必须小于等于 0 dB")
	}
	if c.ZCRThreshold < 0 || c.ZCRThreshold > 1 {
		return fmt.Errorf("过零率门限必须在 0 到 1 之间")
	}

	// 验证静音段时长
	if c.MinSilenceDuration <= 0 {
		return fmt.Errorf("最小静音段时长必须大于 0")
	}

	return nil
}

// GetFrameSize 获取帧长（采样点数）
func (c *SplitterConfig) GetFrameSize(sampleRate int) int {
	if c.FrameSize > 0 {
		return c.FrameSize
	}
	// 默认 30ms
	return int(float64(sampleRate) * 0.03)
}

// GetHopSize 获取帧移（采样点数）
func (c *SplitterConfig) GetHopSize(sampleRate int) int {
	if c.HopSize > 0 {
		return c.HopSize
	}
	// 默认 10ms
	return int(float64(sampleRate) * 0.01)
}

// GetSliceOutputPath 获取切片输出路径
func (c *SplitterConfig) GetSliceOutputPath(baseName string, index int) string {
	fileName := fmt.Sprintf("%s_slice_%03d.wav", baseName, index)
	return filepath.Join(c.OutputDir, fileName)
}

// GetMetadataOutputPath 获取元数据输出路径
func (c *SplitterConfig) GetMetadataOutputPath(baseName string) string {
	fileName := fmt.Sprintf("%s_slices.json", baseName)
	return filepath.Join(c.OutputDir, fileName)
}