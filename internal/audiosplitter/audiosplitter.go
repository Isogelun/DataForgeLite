package audiosplitter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// AudioSplitter 音频切分器
type AudioSplitter struct {
	config *SplitterConfig
}

// NewAudioSplitter 创建音频切分器
func NewAudioSplitter(config *SplitterConfig) (*AudioSplitter, error) {
	// 设置默认值
	config.SetDefaults()

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &AudioSplitter{
		config: config,
	}, nil
}

// Split 执行单文件切分
func (as *AudioSplitter) Split(filePath string) (*FileResult, error) {
	startTime := time.Now()

	result := &FileResult{
		FilePath:  filePath,
		Status:    StatusSuccess,
		OutputDir: as.config.OutputDir,
	}

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		result.Status = StatusFailed
		result.Error = "文件不存在"
		return result, fmt.Errorf(result.Error)
	}

	// 加载音频
	loader := NewAudioLoader(filePath)
	audio, err := loader.Load()
	if err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("加载音频失败: %v", err)
		return result, err
	}

	result.TotalDuration = audio.Duration

	// VAD 检测静音段
	vadDetector := NewVADDetector(as.config)
	silences, err := vadDetector.Detect(audio)
	if err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("VAD检测失败: %v", err)
		return result, err
	}

	// 选择切分点
	selector := NewSliceSelector(as.config)
	slices := selector.SelectSlices(audio, silences)

	// 获取被过滤的信息
	filtered := selector.GetFilteredInfo(slices)

	// 导出切片
	exporter := NewAudioExporter(as.config)
	baseName := GetBaseName(filePath)

	if err := exporter.ExportSlices(audio, slices, baseName); err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("导出切片失败: %v", err)
		return result, err
	}

	// 创建并导出元数据
	metadata := CreateSliceMetadata(filePath, audio, slices, filtered)
	if err := exporter.ExportMetadata(metadata, baseName); err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("导出元数据失败: %v", err)
		return result, err
	}

	result.SliceCount = len(slices)
	result.SlicesInfo = as.config.GetMetadataOutputPath(baseName)
	result.ProcessTime = time.Since(startTime)

	return result, nil
}

// SplitBatch 批量切分（并行处理多文件，提升速度）
func (as *AudioSplitter) SplitBatch(ctx context.Context) (*SplitResult, error) {
	startTime := time.Now()
	paths := as.config.InputPaths
	result := &SplitResult{
		TotalFiles: len(paths),
		Results:    make([]FileResult, len(paths)),
		StartTime:  startTime,
	}

	workers := runtime.NumCPU() * 2
	if workers > len(paths) {
		workers = len(paths)
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	work := make(chan int, len(paths))
	for i := range paths {
		work <- i
	}
	close(work)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				select {
				case <-ctx.Done():
					return
				default:
				}
				fr, _ := as.Split(paths[i])
				result.Results[i] = *fr
			}
		}()
	}
	wg.Wait()
	for i := range result.Results {
		if result.Results[i].Status == StatusSuccess {
			result.SuccessCount++
			result.FilteredCount += len(result.Results[i].SlicesInfo)
		} else {
			result.FailedCount++
		}
	}
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// SplitFile 便捷函数：切分单个文件
func SplitFile(filePath, outputDir string, options ...SplitterOption) (*FileResult, error) {
	config := &SplitterConfig{
		InputPaths: []string{filePath},
		OutputDir:  outputDir,
	}

	// 应用选项
	for _, option := range options {
		option(config)
	}

	splitter, err := NewAudioSplitter(config)
	if err != nil {
		return nil, err
	}

	return splitter.Split(filePath)
}

// SplitFiles 便捷函数：批量切分文件
func SplitFiles(filePaths []string, outputDir string, options ...SplitterOption) (*SplitResult, error) {
	config := &SplitterConfig{
		InputPaths: filePaths,
		OutputDir:  outputDir,
	}

	// 应用选项
	for _, option := range options {
		option(config)
	}

	splitter, err := NewAudioSplitter(config)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	return splitter.SplitBatch(ctx)
}

// SplitterOption 切分器选项函数类型
type SplitterOption func(*SplitterConfig)

// WithTargetSliceDuration 设置目标切片时长
func WithTargetSliceDuration(duration float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.TargetSliceDuration = duration
	}
}

// WithMinSliceDuration 设置最小切片时长
func WithMinSliceDuration(duration float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.MinSliceDuration = duration
	}
}

// WithMaxSliceDuration 设置最大切片时长
func WithMaxSliceDuration(duration float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.MaxSliceDuration = duration
	}
}

// WithPaddingDuration 设置边界 padding 时长
func WithPaddingDuration(duration float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.PaddingDuration = duration
	}
}

// WithEnergyThreshold 设置能量门限
func WithEnergyThreshold(threshold float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.EnergyThreshold = threshold
	}
}

// WithZCRThreshold 设置过零率门限
func WithZCRThreshold(threshold float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.ZCRThreshold = threshold
	}
}

// WithMinSilenceDuration 设置最小静音段时长
func WithMinSilenceDuration(duration float64) SplitterOption {
	return func(c *SplitterConfig) {
		c.MinSilenceDuration = duration
	}
}

// ScanAudioFiles 扫描目录获取音频文件
func ScanAudioFiles(dirPath string, recursive bool) ([]string, error) {
	var files []string

	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("访问目录失败: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("路径不是目录: %s", dirPath)
	}

	if recursive {
		err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && IsValidAudioFile(path) {
				files = append(files, path)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, fmt.Errorf("读取目录失败: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				path := filepath.Join(dirPath, entry.Name())
				if IsValidAudioFile(path) {
					files = append(files, path)
				}
			}
		}
	}

	if err != nil {
		return nil, err
	}

	return files, nil
}