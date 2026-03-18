// Package addphnum 提供音素数量计算功能
package addphnum

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"DataForgeLite/internal/exporter"
)

// PhNumProcessor 音素数量处理器
// 负责读取CSV、执行最长前缀匹配计算、写入结果
type PhNumProcessor struct {
	// dict 音素词典引用
	dict *PhonemeDictionary
	// config 配置引用
	config *ProcessorConfig
}

// NewPhNumProcessor 创建新的音素数量处理器
// dict: 已加载的音素词典
// config: 处理器配置
func NewPhNumProcessor(dict *PhonemeDictionary, config *ProcessorConfig) *PhNumProcessor {
	return &PhNumProcessor{
		dict:   dict,
		config: config,
	}
}

// Process 处理CSV文件，计算ph_num并写入输出文件
// inputPath: 输入CSV文件路径
// outputPath: 输出CSV文件路径
func (p *PhNumProcessor) Process(inputPath, outputPath string) error {
	// 打开输入文件
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return exporter.NewFileError(fmt.Sprintf("无法打开输入CSV文件: %s", inputPath), err)
	}
	defer inputFile.Close()

	// 创建CSV reader
	reader := csv.NewReader(inputFile)

	// 读取表头
	headers, err := reader.Read()
	if err != nil {
		return exporter.NewParseError(fmt.Sprintf("读取CSV表头失败: %s", inputPath), err)
	}

	// 查找 ph_seq 列索引
	phSeqIdx := -1
	for i, header := range headers {
		if header == p.config.PhSeqCol {
			phSeqIdx = i
			break
		}
	}

	if phSeqIdx == -1 {
		return exporter.NewParseError(
			fmt.Sprintf("CSV文件中未找到音素序列列 '%s': %s", p.config.PhSeqCol, inputPath),
			nil,
		)
	}

	// 查找 ph_num 列是否已存在
	phNumIdx := -1
	for i, header := range headers {
		if header == "ph_num" {
			phNumIdx = i
			break
		}
	}

	// 准备输出表头
	outputHeaders := make([]string, len(headers))
	copy(outputHeaders, headers)

	// 如果 ph_num 列不存在，添加该列（插入到 ph_seq 之后）
	if phNumIdx == -1 {
		// 在 ph_seq 列后插入 ph_num 列
		newHeaders := make([]string, 0, len(headers)+1)
		for i, h := range headers {
			newHeaders = append(newHeaders, h)
			if i == phSeqIdx {
				newHeaders = append(newHeaders, "ph_num")
			}
		}
		outputHeaders = newHeaders
		phNumIdx = phSeqIdx + 1
	}

	// 读取所有数据行
	records, err := reader.ReadAll()
	if err != nil {
		return exporter.NewParseError(fmt.Sprintf("读取CSV数据失败: %s", inputPath), err)
	}

	// 处理每一行
	processedRecords := make([][]string, 0, len(records))
	for rowNum, record := range records {
		// 检查行数据完整性
		if len(record) <= phSeqIdx {
			return exporter.NewParseError(
				fmt.Sprintf("第 %d 行数据不完整，缺少音素序列列", rowNum+2),
				nil,
			)
		}

		// 获取音素序列
		phSeq := record[phSeqIdx]

		// 计算音素数量序列（与训练管线一致：每个词/片段对应一个数字）
		phNum, err := p.calculatePhNumSeq(phSeq)
		if err != nil {
			return exporter.NewParseError(
				fmt.Sprintf("第 %d 行音素序列计算失败: %s", rowNum+2, phSeq),
				err,
			)
		}

		// 构建输出行
		outputRecord := make([]string, len(outputHeaders))

		// 复制原始数据
		srcIdx := 0
		dstIdx := 0
		for dstIdx < len(outputHeaders) {
			if dstIdx == phNumIdx {
				// 插入 ph_num 值
				outputRecord[dstIdx] = phNum
			} else if srcIdx < len(record) {
				outputRecord[dstIdx] = record[srcIdx]
				srcIdx++
			}
			dstIdx++
		}

		processedRecords = append(processedRecords, outputRecord)
	}

	// 创建输出文件
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return exporter.NewFileError(fmt.Sprintf("无法创建输出CSV文件: %s", outputPath), err)
	}
	defer outputFile.Close()

	// 创建CSV writer
	writer := csv.NewWriter(outputFile)

	// 写入表头
	if err := writer.Write(outputHeaders); err != nil {
		return exporter.NewFileError("写入CSV表头失败", err)
	}

	// 写入数据行
	if err := writer.WriteAll(processedRecords); err != nil {
		return exporter.NewFileError("写入CSV数据失败", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return exporter.NewFileError("刷新CSV写入器失败", err)
	}

	return nil
}

// calculatePhNumSeq 计算音素序列的 ph_num 序列（空格分隔）。
// 规则与用户提供的 Python 一致：
// - AP / SP: 输出 1，指针前进 1
// - 其它：从当前位置起，寻找在词典中存在的“最长音素序列”匹配，输出其长度，指针前进该长度
// - 若无匹配：输出 1，指针前进 1
func (p *PhNumProcessor) calculatePhNumSeq(phSeq string) (string, error) {
	// 分割音素序列
	phonemes := strings.Fields(phSeq)
	if len(phonemes) == 0 {
		return "", nil
	}

	out := make([]string, 0, len(phonemes))
	i := 0
	maxDictLen := p.dict.GetMaxLen()

	for i < len(phonemes) {
		current := phonemes[i]

		// 处理特殊标记 AP 和 SP
		if current == "AP" || current == "SP" {
			out = append(out, "1")
			i++
			continue
		}

		// 最长匹配（从最长到最短）
		bestLen := 0

		// 计算最大可能匹配长度
		maxPossibleLen := maxDictLen
		remaining := len(phonemes) - i
		if remaining < maxPossibleLen {
			maxPossibleLen = remaining
		}

		for length := maxPossibleLen; length >= 1; length-- {
			seq := strings.Join(phonemes[i:i+length], " ")
			if p.dict.Contains(seq) {
				bestLen = length
				break
			}
		}

		if bestLen > 0 {
			out = append(out, fmt.Sprintf("%d", bestLen))
			i += bestLen
		} else {
			out = append(out, "1")
			i++
		}
	}

	return strings.Join(out, " "), nil
}
