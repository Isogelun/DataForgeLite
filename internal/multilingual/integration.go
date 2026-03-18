package multilingual

import (
	"encoding/csv"
	"os"
	"strconv"

	"DataForgeLite/internal/addphnum"
)

// ProcessWithPhNum 集成函数：文本处理 + 音素数量计算
// 处理流程：text → multilingual → pinyin → addphnum → ph_num
//
// 参数:
//   text: 原始文本
//   textProc: 文本处理器
//   phNumProc: 音素数量处理器
//
// 返回:
//   convertedText: 转换后的文本（拼音/罗马音）
//   phNum: 音素数量
//   err: 执行错误
func ProcessWithPhNum(text string, textProc *TextProcessorImpl, phNumProc *addphnum.PhNumProcessor) (string, int, error) {
	// 1. 文本转换（多语言 → 拼音/罗马音）
	record, err := textProc.Process(text, "auto")
	if err != nil {
		return "", 0, err
	}

	// 2. 计算音素数量（使用简化计算：统计空格分隔的单词数）
	phNum := countPhonemes(record.ConvertedText)

	return record.ConvertedText, phNum, nil
}

// countPhonemes 统计音素数量（简化版本：统计空格分隔的单元数）
func countPhonemes(text string) int {
	if text == "" {
		return 0
	}
	count := 0
	inWord := false
	for _, r := range text {
		if r == ' ' {
			if inWord {
				count++
				inWord = false
			}
		} else {
			inWord = true
		}
	}
	if inWord {
		count++
	}
	return count
}

// TextProcessorWithPhNum 带音素数量计算的文本处理器
type TextProcessorWithPhNum struct {
	textProc  *TextProcessorImpl
	phNumProc *addphnum.PhNumProcessor
}

// NewTextProcessorWithPhNum 创建带音素数量计算的文本处理器
//
// 参数:
//   textConfig: 文本处理器配置
//   dictPath: 音素词典路径
//
// 返回:
//   *TextProcessorWithPhNum: 处理器实例
//   error: 初始化错误
func NewTextProcessorWithPhNum(textConfig *Config, dictPath string) (*TextProcessorWithPhNum, error) {
	// 创建文本处理器
	textProc, err := NewTextProcessor(textConfig)
	if err != nil {
		return nil, err
	}

	// 创建音素词典
	phNumDict := addphnum.NewPhonemeDictionary()
	if dictPath != "" {
		if err := phNumDict.Load(dictPath); err != nil {
			return nil, err
		}
	}

	// 创建音素数量处理器
	phNumConfig := &addphnum.ProcessorConfig{
		PhSeqCol: "ph_seq",
	}
	phNumProc := addphnum.NewPhNumProcessor(phNumDict, phNumConfig)

	return &TextProcessorWithPhNum{
		textProc:  textProc,
		phNumProc: phNumProc,
	}, nil
}

// Process 处理文本并返回转换结果和音素数量
func (p *TextProcessorWithPhNum) Process(text string) (string, int, error) {
	return ProcessWithPhNum(text, p.textProc, p.phNumProc)
}

// ProcessText 处理文本（仅返回转换后的文本）
func (p *TextProcessorWithPhNum) ProcessText(text string) (*TextRecord, error) {
	return p.textProc.Process(text, "auto")
}

// ProcessCSVWithPhNum 批量处理 CSV 文件并计算音素数量
//
// 参数:
//   inputPath: 输入 CSV 路径
//   outputPath: 输出 CSV 路径
//   textColumn: 文本列名
//   outputColumn: 输出列名
//   phNumColumn: 音素数量列名
//
// 返回:
//   error: 执行错误
func (p *TextProcessorWithPhNum) ProcessCSVWithPhNum(inputPath, outputPath, textColumn, outputColumn, phNumColumn string) error {
	// 打开输入文件
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return NewError(ErrFileOperation, "无法打开输入文件："+inputPath, err)
	}
	defer inputFile.Close()

	// 创建 CSV 读取器
	reader := csv.NewReader(inputFile)

	// 读取表头
	headers, err := reader.Read()
	if err != nil {
		return NewError(ErrFileOperation, "无法读取 CSV 表头", err)
	}

	// 查找列索引
	textColIndex := findColumnIndex(headers, textColumn)
	if textColIndex == -1 {
		return NewError(ErrInvalidConfig, "未找到文本列："+textColumn, nil)
	}

	// 读取所有记录
	var records [][]string
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		records = append(records, record)
	}

	// 确定输出列和音素数量列索引
	outputColIndex := findColumnIndex(headers, outputColumn)
	phNumColIndex := findColumnIndex(headers, phNumColumn)

	newHeaders := make([]string, len(headers))
	copy(newHeaders, headers)

	if outputColIndex == -1 {
		newHeaders = append(newHeaders, outputColumn)
		outputColIndex = len(headers)
	}

	if phNumColIndex == -1 {
		newHeaders = append(newHeaders, phNumColumn)
		phNumColIndex = len(headers)
	}

	// 处理每条记录
	newRecords := make([][]string, 0, len(records))
	for _, record := range records {
		var convertedText string
		var phNum int

		if textColIndex < len(record) {
			text := record[textColIndex]
			convertedText, phNum, _ = p.Process(text)
			// 处理失败时保留空值和 0
		}

		// 扩展记录
		newRecord := make([]string, len(newHeaders))
		for i := range newRecord {
			if i < len(record) {
				newRecord[i] = record[i]
			}
		}

		if outputColIndex < len(newRecord) {
			newRecord[outputColIndex] = convertedText
		}
		if phNumColIndex < len(newRecord) {
			newRecord[phNumColIndex] = strconv.Itoa(phNum)
		}

		newRecords = append(newRecords, newRecord)
	}

	// 创建输出文件
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return NewError(ErrFileOperation, "无法创建输出文件："+outputPath, err)
	}
	defer outputFile.Close()

	// 创建 CSV 写入器
	writer := csv.NewWriter(outputFile)

	// 写入表头
	if err := writer.Write(newHeaders); err != nil {
		return NewError(ErrFileOperation, "写入 CSV 表头失败", err)
	}

	// 写入记录
	for _, record := range newRecords {
		if err := writer.Write(record); err != nil {
			return NewError(ErrFileOperation, "写入 CSV 记录失败", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return NewError(ErrFileOperation, "刷新 CSV 写入失败", err)
	}

	return nil
}

// findColumnIndex 查找列索引
func findColumnIndex(headers []string, columnName string) int {
	for i, h := range headers {
		if h == columnName {
			return i
		}
	}
	return -1
}
