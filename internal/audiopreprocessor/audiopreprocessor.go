package audiopreprocessor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BatchProcessor 批量音频处理器
type BatchProcessor struct {
	config *ProcessorConfig
}

// NewBatchProcessor 创建批量处理器
func NewBatchProcessor(config *ProcessorConfig) (*BatchProcessor, error) {
	// 设置默认值
	config.SetDefaults()

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &BatchProcessor{
		config: config,
	}, nil
}

// Process 执行批量处理
func (bp *BatchProcessor) Process(ctx context.Context) (*ProcessingResult, error) {
	startTime := time.Now()

	result := &ProcessingResult{
		TotalFiles:   len(bp.config.InputPaths),
		Results:      make([]FileResult, 0, len(bp.config.InputPaths)),
		StartTime:    startTime,
		SuccessCount: 0,
		FailedCount:  0,
		SkippedCount: 0,
	}

	// 验证输入文件
	validPaths, err := ValidateInputFiles(bp.config.InputPaths)
	if err != nil {
		return nil, err
	}

	// 创建工作池
	jobs := make(chan string, len(validPaths))
	results := make(chan FileResult, len(validPaths))

	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < bp.config.Concurrency; i++ {
		wg.Add(1)
		go bp.worker(ctx, &wg, jobs, results)
	}

	// 发送任务
	go func() {
		for _, path := range validPaths {
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- path:
			}
		}
		close(jobs)
	}()

	// 等待所有工作协程完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	for fileResult := range results {
		result.Results = append(result.Results, fileResult)

		switch fileResult.Status {
		case StatusSuccess:
			result.SuccessCount++
		case StatusFailed:
			result.FailedCount++
		case StatusSkipped:
			result.SkippedCount++
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)

	return result, nil
}

// worker 工作协程
func (bp *BatchProcessor) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan string, results chan<- FileResult) {
	defer wg.Done()

	processor := NewAudioProcessor(bp.config)

	for {
		select {
		case <-ctx.Done():
			return
		case path, ok := <-jobs:
			if !ok {
				return
			}

			result, err := processor.Process(path)
			if err != nil {
				// 处理失败已在 Process 中设置状态
				results <- *result
				continue
			}

			results <- *result
		}
	}
}

// ProcessBatch 便捷函数：批量处理音频文件
func ProcessBatch(inputPaths []string, outputDir string, options ...ProcessorOption) (*ProcessingResult, error) {
	config := &ProcessorConfig{
		InputPaths: inputPaths,
		OutputDir:  outputDir,
	}

	// 应用选项
	for _, option := range options {
		option(config)
	}

	// 创建处理器并执行
	processor, err := NewBatchProcessor(config)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	return processor.Process(ctx)
}

// ProcessorOption 处理器选项函数类型
type ProcessorOption func(*ProcessorConfig)

// WithTargetLUFS 设置目标响度
func WithTargetLUFS(lufs float64) ProcessorOption {
	return func(c *ProcessorConfig) {
		c.TargetLUFS = lufs
	}
}

// WithTargetSampleRate 设置目标采样率
func WithTargetSampleRate(rate int) ProcessorOption {
	return func(c *ProcessorConfig) {
		c.TargetSampleRate = rate
	}
}

// WithTargetChannels 设置目标声道数
func WithTargetChannels(channels int) ProcessorOption {
	return func(c *ProcessorConfig) {
		c.TargetChannels = channels
	}
}

// WithTruePeakLimit 设置真峰值限制
func WithTruePeakLimit(limit float64) ProcessorOption {
	return func(c *ProcessorConfig) {
		c.TruePeakLimit = limit
	}
}

// WithQualityCheck 设置是否启用质量检测
func WithQualityCheck(enable bool) ProcessorOption {
	return func(c *ProcessorConfig) {
		c.EnableQualityCheck = enable
	}
}

// WithConcurrency 设置并发数
func WithConcurrency(n int) ProcessorOption {
	return func(c *ProcessorConfig) {
		c.Concurrency = n
	}
}

// ScanDirectory 扫描目录获取音频文件
func ScanDirectory(dirPath string, recursive bool) ([]string, error) {
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
			if !info.IsDir() && IsSupportedFormat(path) {
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
				if IsSupportedFormat(path) {
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