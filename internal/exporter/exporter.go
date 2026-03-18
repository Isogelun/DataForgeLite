// Package exporter 提供数据集导出功能
// 这是整个导出模块的统一入口，封装了音频处理、TextGrid解析和数据集构建的完整流程
package exporter

import (
	"fmt"
)

// ExportResult 导出结果
type ExportResult struct {
	// TotalFiles 处理文件总数
	TotalFiles int
	// SuccessCount 成功处理数量
	SuccessCount int
	// ErrorCount 失败数量
	ErrorCount int
	// Errors 错误信息列表
	Errors []string
}

// Exporter 数据集导出器
// 作为整个模块的统一入口，封装 DatasetBuilder 提供简化的导出接口
type Exporter struct {
	// builder 数据集构建器
	builder *DatasetBuilder
}

// NewExporter 创建新的导出器
func NewExporter() *Exporter {
	return &Exporter{
		builder: NewDatasetBuilder(),
	}
}

// Export 执行数据集导出
// wavsDir: WAV文件输入目录
// tgDir: TextGrid文件目录
// outputDir: 输出目录
// opts: 配置选项列表
func (e *Exporter) Export(wavsDir, tgDir, outputDir string, opts ...Option) (*ExportResult, error) {
	// 创建配置
	cfg := NewConfig(opts...)
	cfg.WavsDir = wavsDir
	cfg.TgDir = tgDir
	cfg.OutputDir = outputDir

	return e.ExportWithConfig(cfg)
}

// ExportWithConfig 使用完整配置执行导出
// cfg: 完整的配置对象
func (e *Exporter) ExportWithConfig(cfg *Config) (*ExportResult, error) {
	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 调用构建器执行导出
	buildResult, err := e.builder.Export(cfg.WavsDir, cfg.TgDir, cfg.OutputDir, cfg)
	if err != nil {
		return nil, err
	}

	// 转换结果
	result := &ExportResult{
		TotalFiles:   buildResult.TotalFiles,
		SuccessCount: buildResult.SuccessCount,
		ErrorCount:   buildResult.ErrorCount,
		Errors:       buildResult.Errors,
	}

	return result, nil
}

// String 返回导出结果的字符串表示
func (r *ExportResult) String() string {
	return fmt.Sprintf("ExportResult{TotalFiles: %d, SuccessCount: %d, ErrorCount: %d}",
		r.TotalFiles, r.SuccessCount, r.ErrorCount)
}

// Summary 返回导出结果的摘要信息
func (r *ExportResult) Summary() string {
	return fmt.Sprintf("导出完成: 共处理 %d 个文件, 成功 %d 个, 失败 %d 个",
		r.TotalFiles, r.SuccessCount, r.ErrorCount)
}

// HasErrors 检查是否有错误
func (r *ExportResult) HasErrors() bool {
	return r.ErrorCount > 0
}

// ============================================================
// 包级便捷函数
// ============================================================

// defaultExporter 默认导出器实例，用于包级函数
var defaultExporter = NewExporter()

// Export 执行数据集导出（包级便捷函数）
// 使用默认导出器执行导出操作
//
// 参数:
//   - wavsDir: WAV文件输入目录
//   - tgDir: TextGrid文件目录
//   - outputDir: 输出目录
//   - opts: 配置选项列表（可选）
//
// 返回:
//   - ExportResult: 导出结果统计
//   - error: 执行过程中的错误
//
// 示例:
//
//	// 基本用法
//	result, err := exporter.Export("/path/to/wavs", "/path/to/tg", "/path/to/output")
//
//	// 使用选项配置
//	result, err := exporter.Export(
//	    "/path/to/wavs", "/path/to/tg", "/path/to/output",
//	    exporter.WithSkipSilenceInsertion(true),
//	    exporter.WithWavSubtype("PCM_24"),
//	)
func Export(wavsDir, tgDir, outputDir string, opts ...Option) (*ExportResult, error) {
	return defaultExporter.Export(wavsDir, tgDir, outputDir, opts...)
}

// ExportWithConfig 使用完整配置执行导出（包级便捷函数）
// 使用默认导出器和完整配置对象执行导出操作
//
// 参数:
//   - cfg: 完整的配置对象，必须设置 WavsDir, TgDir, OutputDir
//
// 返回:
//   - ExportResult: 导出结果统计
//   - error: 执行过程中的错误
//
// 示例:
//
//	cfg := exporter.NewConfig(
//	    exporter.WithWavsDir("/path/to/wavs"),
//	    exporter.WithTgDir("/path/to/tg"),
//	    exporter.WithOutputDir("/path/to/output"),
//	    exporter.WithSkipSilenceInsertion(true),
//	    exporter.WithTargetSampleRate(22050),
//	)
//	result, err := exporter.ExportWithConfig(cfg)
func ExportWithConfig(cfg *Config) (*ExportResult, error) {
	return defaultExporter.ExportWithConfig(cfg)
}

// ============================================================
// 兼容性别名（保持与Python原始代码参数名一致）
// ============================================================

// ExportDataset 导出数据集（兼容性别名）
// 提供与 Python 原始代码类似的接口
//
// 参数:
//   - wavs: WAV文件目录（对应 Python 的 --wavs）
//   - tg: TextGrid文件目录（对应 Python 的 --tg）
//   - dataset: 输出目录（对应 Python 的 --dataset）
//   - skipSilenceInsertion: 是否跳过静音插入（对应 Python 的 --skip_silence_insertion）
//   - wavSubtype: WAV子类型（对应 Python 的 --wav_subtype，默认 "PCM_16"）
func ExportDataset(wavs, tg, dataset string, skipSilenceInsertion bool, wavSubtype string) (*ExportResult, error) {
	if wavSubtype == "" {
		wavSubtype = "PCM_16"
	}

	return Export(
		wavs, tg, dataset,
		WithSkipSilenceInsertion(skipSilenceInsertion),
		WithWavSubtype(wavSubtype),
	)
}
