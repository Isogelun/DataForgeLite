package audiopreprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AudioProcessor 单文件音频处理器
type AudioProcessor struct {
	config *ProcessorConfig
}

// NewAudioProcessor 创建音频处理器
func NewAudioProcessor(config *ProcessorConfig) *AudioProcessor {
	return &AudioProcessor{
		config: config,
	}
}

// Process 处理单个音频文件
func (p *AudioProcessor) Process(inputPath string) (*FileResult, error) {
	startTime := time.Now()

	result := &FileResult{
		FilePath:   inputPath,
		Status:     StatusSuccess,
		OutputPath: p.config.GetOutputPath(inputPath),
	}

	// 检查文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		result.Status = StatusFailed
		result.Error = "文件不存在"
		return result, fmt.Errorf("文件不存在: %s", inputPath)
	}

	// 检查是否为支持的格式
	if !IsSupportedFormat(inputPath) {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("不支持的音频格式: %s", filepath.Ext(inputPath))
		return result, fmt.Errorf(result.Error)
	}

	// 解码音频
	decoder := NewDecoder(inputPath)
	decodedAudio, originalMeta, err := decoder.Decode()
	if err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("解码失败: %v", err)
		return result, err
	}
	decoder.Close()

	result.OriginalMeta = *originalMeta

	// 处理音频
	processedSamples, processedMeta, err := p.processAudio(decodedAudio)
	if err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("处理失败: %v", err)
		return result, err
	}

	result.ProcessedMeta = *processedMeta

	// 编码输出
	encoder := NewEncoder(
		p.config.TargetSampleRate,
		p.config.TargetChannels,
		16, // 输出 16 位深度
	)

	if err := encoder.EncodeToWAV(processedSamples, result.OutputPath); err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("编码失败: %v", err)
		return result, err
	}

	// 质量检测
	if p.config.EnableQualityCheck {
		qualityChecker := NewQualityChecker(
			p.config.SilenceThreshold,
			p.config.ClippingThreshold,
			p.config.TargetSampleRate,
		)
		result.QualityReport = *qualityChecker.Check(processedSamples, p.config.TargetChannels)
	}

	result.ProcessTime = time.Since(startTime)
	return result, nil
}

// processAudio 处理音频数据
func (p *AudioProcessor) processAudio(decodedAudio *DecodedAudio) ([]float64, *AudioMeta, error) {
	samples := decodedAudio.Samples
	channels := decodedAudio.Channels
	sampleRate := decodedAudio.SampleRate

	// 1. 重采样
	if sampleRate != p.config.TargetSampleRate {
		resampler := NewResampler(sampleRate, p.config.TargetSampleRate)
		samples = resampler.Resample(samples, channels)
		sampleRate = p.config.TargetSampleRate
	}

	// 2. 声道转换
	if channels != p.config.TargetChannels {
		samples = ConvertChannels(samples, channels, p.config.TargetChannels)
		channels = p.config.TargetChannels
	}

	// 3. 计算当前 LUFS 并进行响度标准化
	finalSamples, finalLUFS := NormalizeAudio(
		samples,
		sampleRate,
		channels,
		p.config.TargetLUFS,
		p.config.TruePeakLimit,
	)

	// 创建处理后元信息
	processedMeta := &AudioMeta{
		SampleRate: sampleRate,
		Channels:   channels,
		Duration:   float64(len(finalSamples)/channels) / float64(sampleRate),
		BitDepth:   16,
		Format:     "WAV",
		LUFS:       finalLUFS,
	}

	return finalSamples, processedMeta, nil
}

// ProcessSingle 便捷函数：处理单个文件
func ProcessSingle(inputPath, outputDir string, config *ProcessorConfig) (*FileResult, error) {
	// 设置默认值
	config.SetDefaults()

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 创建处理器并处理
	processor := NewAudioProcessor(config)
	return processor.Process(inputPath)
}

// ValidateInputFiles 验证输入文件列表
func ValidateInputFiles(paths []string) ([]string, error) {
	validPaths := make([]string, 0, len(paths))

	for _, path := range paths {
		// 检查文件是否存在
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue // 跳过不存在的文件
		}

		// 检查格式
		if !IsSupportedFormat(path) {
			continue // 跳过不支持的格式
		}

		validPaths = append(validPaths, path)
	}

	if len(validPaths) == 0 {
		return nil, fmt.Errorf("没有有效的输入文件")
	}

	return validPaths, nil
}