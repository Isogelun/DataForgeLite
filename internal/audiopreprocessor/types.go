// Package audiopreprocessor 提供音频预处理功能，包括响度标准化、采样率转换、质量检测等
package audiopreprocessor

import (
	"encoding/json"
	"time"
)

// ProcessorConfig 音频处理器配置
type ProcessorConfig struct {
	InputPaths         []string `json:"input_paths"`          // 输入音频文件路径列表
	OutputDir          string   `json:"output_dir"`           // 输出目录路径
	TargetLUFS         float64  `json:"target_lufs"`          // 目标响度值，默认 -18.0
	TargetSampleRate   int      `json:"target_sample_rate"`   // 目标采样率，默认 48000
	TargetChannels     int      `json:"target_channels"`      // 目标声道数，默认 1
	TruePeakLimit      float64  `json:"true_peak_limit"`      // 真峰值限制，默认 -1.0
	EnableQualityCheck bool     `json:"enable_quality_check"` // 是否启用质量检测，默认 true
	SilenceThreshold   float64  `json:"silence_threshold"`    // 静音检测阈值(dB)，默认 -50.0
	ClippingThreshold  float64  `json:"clipping_threshold"`   // 截幅检测阈值(dB)，默认 -0.5
	Concurrency        int      `json:"concurrency"`          // 并发数，默认 CPU 核心数
}

// AudioMeta 音频元信息
type AudioMeta struct {
	SampleRate int     `json:"sample_rate"` // 采样率（Hz）
	Channels   int     `json:"channels"`    // 声道数
	Duration   float64 `json:"duration"`    // 时长（秒）
	BitDepth   int     `json:"bit_depth"`   // 位深度
	Format     string  `json:"format"`      // 格式（WAV/MP3/FLAC/M4A/OGG）
	LUFS       float64 `json:"lufs"`        // 响度值
}

// ProcessingResult 批处理结果
type ProcessingResult struct {
	TotalFiles   int           `json:"total_files"`   // 总文件数
	SuccessCount int           `json:"success_count"` // 成功处理数
	FailedCount  int           `json:"failed_count"`  // 处理失败数
	SkippedCount int           `json:"skipped_count"` // 跳过数
	Results      []FileResult  `json:"results"`       // 每个文件的详细结果
	StartTime    time.Time     `json:"start_time"`    // 开始时间
	EndTime      time.Time     `json:"end_time"`      // 结束时间
	Duration     time.Duration `json:"duration"`      // 总耗时
}

// FileResult 单个文件处理结果
type FileResult struct {
	FilePath      string        `json:"file_path"`      // 原始文件路径
	Status        string        `json:"status"`         // 状态: success/failed/skipped
	OutputPath    string        `json:"output_path"`    // 输出文件路径
	Error         string        `json:"error"`          // 错误信息
	OriginalMeta  AudioMeta     `json:"original_meta"`  // 原始音频元信息
	ProcessedMeta AudioMeta     `json:"processed_meta"` // 处理后音频元信息
	QualityReport QualityReport `json:"quality_report"` // 质量检测报告
	ProcessTime   time.Duration `json:"process_time"`   // 处理耗时
}

// QualityReport 质量检测报告
type QualityReport struct {
	SilenceSegments []Segment `json:"silence_segments"` // 静音段列表
	ClippingCount   int       `json:"clipping_count"`   // 截幅样本数
	ClippingRatio   float64   `json:"clipping_ratio"`   // 截幅比例
	HasQualityIssue bool      `json:"has_quality_issue"` // 是否存在质量问题
}

// Segment 时间段
type Segment struct {
	StartTime float64 `json:"start_time"` // 开始时间（秒）
	EndTime   float64 `json:"end_time"`   // 结束时间（秒）
}

// ProcessStatus 处理状态常量
const (
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusSkipped  = "skipped"
)

// ToJSON 将 ProcessingResult 转换为 JSON 字符串
func (r *ProcessingResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsSuccess 检查处理结果是否成功
func (fr *FileResult) IsSuccess() bool {
	return fr.Status == StatusSuccess
}

// IsFailed 检查处理是否失败
func (fr *FileResult) IsFailed() bool {
	return fr.Status == StatusFailed
}

// IsSkipped 检查处理是否被跳过
func (fr *FileResult) IsSkipped() bool {
	return fr.Status == StatusSkipped
}

// Duration 计算 Segment 的持续时间
func (s *Segment) Duration() float64 {
	return s.EndTime - s.StartTime
}

// TotalSilenceDuration 计算总静音时长
func (qr *QualityReport) TotalSilenceDuration() float64 {
	var total float64
	for _, seg := range qr.SilenceSegments {
		total += seg.Duration()
	}
	return total
}