// Package audiosplitter 提供音频智能切分功能，基于 VAD 算法检测静音段并智能切分
package audiosplitter

import (
	"encoding/json"
	"time"
)

// SplitterConfig 音频切分器配置
type SplitterConfig struct {
	InputPaths          []string `json:"input_paths"`          // 输入音频文件路径列表
	OutputDir           string   `json:"output_dir"`           // 输出目录路径
	TargetSliceDuration float64  `json:"target_slice_duration"` // 目标切片时长，默认 10.0s
	MinSliceDuration    float64  `json:"min_slice_duration"`    // 最小切片时长，默认 3.0s
	MaxSliceDuration    float64  `json:"max_slice_duration"`    // 最大切片时长，默认 20.0s
	PaddingDuration     float64  `json:"padding_duration"`      // 边界 padding 时长，默认 0.3s
	EnergyThreshold     float64  `json:"energy_threshold"`      // 能量门限，默认 -40.0dB
	ZCRThreshold        float64  `json:"zcr_threshold"`         // 过零率门限，默认 0.05
	MinSilenceDuration  float64  `json:"min_silence_duration"`  // 最小静音段时长，默认 0.3s
	FrameSize           int      `json:"frame_size"`            // 帧长（采样点数），默认 30ms
	HopSize             int      `json:"hop_size"`              // 帧移（采样点数），默认 10ms
}

// SilenceSegment 静音段
type SilenceSegment struct {
	StartFrame int     `json:"start_frame"` // 起始帧索引
	EndFrame   int     `json:"end_frame"`   // 结束帧索引
	StartTime  float64 `json:"start_time"`  // 起始时间（秒）
	EndTime    float64 `json:"end_time"`    // 结束时间（秒）
	Duration   float64 `json:"duration"`    // 持续时间（秒）
}

// AudioSlice 音频切片
type AudioSlice struct {
	Index       int     `json:"index"`        // 切片索引
	StartTime   float64 `json:"start_time"`   // 起始时间（秒，含 padding）
	EndTime     float64 `json:"end_time"`     // 结束时间（秒，含 padding）
	Duration    float64 `json:"duration"`     // 时长（秒）
	StartSample int     `json:"start_sample"` // 起始采样点
	EndSample   int     `json:"end_sample"`   // 结束采样点
	Filename    string  `json:"filename"`     // 输出文件名
}

// SliceMetadata 切分元数据
type SliceMetadata struct {
	OriginalFile     string         `json:"original_file"`      // 原始音频文件名
	OriginalDuration float64        `json:"original_duration"`  // 原始音频时长
	SampleRate       int            `json:"sample_rate"`        // 采样率
	TotalSlices      int            `json:"total_slices"`       // 有效切片总数
	Slices           []AudioSlice   `json:"slices"`             // 有效切片列表
	FilteredSlices   []FilteredInfo `json:"filtered_slices"`    // 被过滤的切片信息
}

// FilteredInfo 被过滤切片信息
type FilteredInfo struct {
	Index    int     `json:"index"`    // 切片索引
	Reason   string  `json:"reason"`   // 过滤原因
	Duration float64 `json:"duration"` // 切片时长
}

// FileResult 单个文件处理结果
type FileResult struct {
	FilePath      string        `json:"file_path"`      // 原始文件路径
	Status        string        `json:"status"`         // 状态: success/failed
	OutputDir     string        `json:"output_dir"`     // 输出目录
	SliceCount    int           `json:"slice_count"`    // 生成的切片数量
	TotalDuration float64       `json:"total_duration"` // 原始音频时长
	SlicesInfo    string        `json:"slices_info"`    // 元数据文件路径
	Error         string        `json:"error"`          // 错误信息
	ProcessTime   time.Duration `json:"process_time"`   // 处理耗时
}

// SplitResult 批处理结果
type SplitResult struct {
	TotalFiles    int          `json:"total_files"`     // 总文件数
	SuccessCount  int          `json:"success_count"`   // 成功处理数
	FailedCount   int          `json:"failed_count"`    // 处理失败数
	FilteredCount int          `json:"filtered_count"`  // 过滤切片数
	Results       []FileResult `json:"results"`         // 每个文件的详细结果
	StartTime     time.Time    `json:"start_time"`      // 开始时间
	EndTime       time.Time    `json:"end_time"`        // 结束时间
	Duration      time.Duration `json:"duration"`       // 总耗时
}

// AudioData 音频数据结构
type AudioData struct {
	Samples    []float64 `json:"samples"`     // 单声道浮点采样数据
	SampleRate int       `json:"sample_rate"` // 采样率
	Duration   float64   `json:"duration"`    // 时长（秒）
	Format     string    `json:"format"`      // 原始格式
}

// FrameFeatures 帧特征结构
type FrameFeatures struct {
	Energy    float64 `json:"energy"`     // 帧能量（dB）
	ZCR       float64 `json:"zcr"`        // 过零率（0-1）
	IsSilence bool    `json:"is_silence"` // 是否为静音帧
}

// 状态常量
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// ToJSON 将 SliceMetadata 转换为 JSON 字符串
func (sm *SliceMetadata) ToJSON() (string, error) {
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ToJSON 将 SplitResult 转换为 JSON 字符串
func (sr *SplitResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(sr, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsSuccess 检查处理结果是否成功
func (fr *FileResult) IsSuccess() bool {
	return fr.Status == StatusSuccess
}

// Duration 计算 SilenceSegment 的持续时间
func (ss *SilenceSegment) CalcDuration() float64 {
	return ss.EndTime - ss.StartTime
}