package multilingual

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sync"
)

// BatchProcessor 批量处理器
type BatchProcessor struct {
	textProcessor *TextProcessorImpl
	config        *Config
}

// NewBatchProcessor 创建新的批量处理器
func NewBatchProcessor(config *Config) (*BatchProcessor, error) {
	if config == nil {
		config = DefaultConfig()
	}

	textProc, err := NewTextProcessor(config)
	if err != nil {
		return nil, err
	}

	return &BatchProcessor{
		textProcessor: textProc,
		config:        config,
	}, nil
}

// ProcessCSV 批量处理 CSV 文件
// 参数:
//   inputPath: 输入 CSV 文件路径
//   outputPath: 输出 CSV 文件路径
//   textColumn: 文本列名
//   outputColumn: 输出列名
// 返回:
//   error: 执行错误
func (bp *BatchProcessor) ProcessCSV(inputPath, outputPath, textColumn, outputColumn string) error {
	// 打开输入文件
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return NewError(ErrFileOperation, fmt.Sprintf("无法打开输入文件：%s", inputPath), err)
	}
	defer inputFile.Close()

	// 创建 CSV 读取器
	reader := csv.NewReader(inputFile)

	// 读取表头
	headers, err := reader.Read()
	if err != nil {
		return NewError(ErrFileOperation, "无法读取 CSV 表头", err)
	}

	// 查找文本列索引
	textColIndex := -1
	for i, h := range headers {
		if h == textColumn {
			textColIndex = i
			break
		}
	}

	if textColIndex == -1 {
		return NewError(ErrInvalidConfig, fmt.Sprintf("未找到文本列：%s", textColumn), nil)
	}

	// 读取所有记录
	var records [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return NewError(ErrFileOperation, "读取 CSV 记录失败", err)
		}
		records = append(records, record)
	}

	// 确定输出列索引（如果已存在则更新，否则追加）
	outputColIndex := -1
	for i, h := range headers {
		if h == outputColumn {
			outputColIndex = i
			break
		}
	}

	// 如果输出列不存在，追加到表头
	newHeaders := make([]string, len(headers))
	copy(newHeaders, headers)
	if outputColIndex == -1 {
		newHeaders = append(newHeaders, outputColumn)
		outputColIndex = len(headers)
	}

	// 并发处理文本
	results := make([]string, len(records))
	var wg sync.WaitGroup
	errChan := make(chan error, len(records))

	// 限制并发数
	maxWorkers := 4
	sem := make(chan struct{}, maxWorkers)

	for i, record := range records {
		wg.Add(1)
		go func(idx int, rec []string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if textColIndex >= len(rec) {
				errChan <- NewError(ErrFileOperation, fmt.Sprintf("记录 %d 的文本列索引超出范围", idx), nil)
				return
			}

			text := rec[textColIndex]
			result, err := bp.textProcessor.Process(text, bp.config.DefaultLanguage)
			if err != nil {
				errChan <- err
				return
			}

			results[idx] = result.ConvertedText
		}(i, record)
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	// 创建输出文件
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return NewError(ErrFileOperation, fmt.Sprintf("无法创建输出文件：%s", outputPath), err)
	}
	defer outputFile.Close()

	// 创建 CSV 写入器
	writer := csv.NewWriter(outputFile)

	// 写入表头
	if err := writer.Write(newHeaders); err != nil {
		return NewError(ErrFileOperation, "写入 CSV 表头失败", err)
	}

	// 写入记录
	for i, record := range records {
		// 确保记录有足够的列
		newRecord := make([]string, len(newHeaders))
		for j := range newRecord {
			if j < len(record) {
				newRecord[j] = record[j]
			}
		}

		// 设置输出列
		if outputColIndex < len(newRecord) {
			newRecord[outputColIndex] = results[i]
		}

		if err := writer.Write(newRecord); err != nil {
			return NewError(ErrFileOperation, "写入 CSV 记录失败", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return NewError(ErrFileOperation, "刷新 CSV 写入失败", err)
	}

	return nil
}

// ProcessBatch 批量处理文本记录
// 参数:
//   texts: 文本列表
// 返回:
//   []*TextRecord: 处理结果
//   error: 执行错误
func (bp *BatchProcessor) ProcessBatch(texts []string) ([]*TextRecord, error) {
	results := make([]*TextRecord, len(texts))
	var wg sync.WaitGroup
	errChan := make(chan error, len(texts))

	maxWorkers := 4
	sem := make(chan struct{}, maxWorkers)

	for i, text := range texts {
		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := bp.textProcessor.Process(t, bp.config.DefaultLanguage)
			if err != nil {
				errChan <- err
				return
			}

			results[idx] = result
		}(i, text)
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// ProcessCSV 包级便捷函数：批量处理 CSV 文件
func ProcessCSV(inputPath, outputPath, textColumn, outputColumn string, config *Config) error {
	bp, err := NewBatchProcessor(config)
	if err != nil {
		return err
	}
	return bp.ProcessCSV(inputPath, outputPath, textColumn, outputColumn)
}

// ProcessBatch 包级便捷函数：批量处理文本
func ProcessBatch(texts []string, config *Config) ([]*TextRecord, error) {
	bp, err := NewBatchProcessor(config)
	if err != nil {
		return nil, err
	}
	return bp.ProcessBatch(texts)
}
