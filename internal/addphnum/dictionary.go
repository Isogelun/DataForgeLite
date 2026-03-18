// Package addphnum 提供音素数量计算功能
package addphnum

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"DataForgeLite/internal/exporter"
)

// PhonemeDictionary 音素词典
// 用于存储和查询音素序列，支持最长前缀匹配
type PhonemeDictionary struct {
	// entries 音素序列集合，key为"空格拼接的音素序列"
	entries map[string]struct{}
	// maxLen 词典中最长音素序列的音素个数
	maxLen int
}

// NewPhonemeDictionary 创建新的音素词典实例
// 返回空的词典实例，需要调用 Load 方法加载数据
func NewPhonemeDictionary() *PhonemeDictionary {
	return &PhonemeDictionary{
		entries: make(map[string]struct{}),
		maxLen:  0,
	}
}

// Load 从文件加载词典
// 词典文件格式：每行 "key\tvalue"，value 是空格分隔的音素序列
// 只使用 value 部分构建查询索引
func (d *PhonemeDictionary) Load(dictPath string) error {
	// 打开词典文件
	file, err := os.Open(dictPath)
	if err != nil {
		return exporter.NewFileError(fmt.Sprintf("无法打开词典文件: %s", dictPath), err)
	}
	defer file.Close()

	// 使用 Scanner 逐行读取
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 跳过空行和注释行
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 按制表符分割
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			// 格式错误，跳过该行
			continue
		}

		// 取 value 部分（音素序列）
		phonemeSeq := strings.TrimSpace(parts[1])
		if phonemeSeq == "" {
			continue
		}

		// 存入词典
		d.entries[phonemeSeq] = struct{}{}

		// 更新最大长度
		phonemes := strings.Split(phonemeSeq, " ")
		if len(phonemes) > d.maxLen {
			d.maxLen = len(phonemes)
		}
	}

	if err := scanner.Err(); err != nil {
		return exporter.NewParseError(fmt.Sprintf("读取词典文件失败: %s", dictPath), err)
	}

	// 检查词典是否为空
	if len(d.entries) == 0 {
		return exporter.NewParseError(fmt.Sprintf("词典文件为空或格式错误: %s", dictPath), nil)
	}

	return nil
}

// Contains 检查音素序列是否在词典中
// 时间复杂度 O(1)
func (d *PhonemeDictionary) Contains(phonemeSeq string) bool {
	if phonemeSeq == "" {
		return false
	}
	_, exists := d.entries[phonemeSeq]
	return exists
}

// GetMaxLen 获取词典中最长音素序列的音素个数
// 用于最长前缀匹配时确定最大尝试长度
func (d *PhonemeDictionary) GetMaxLen() int {
	return d.maxLen
}

// Size 返回词典条目数量
func (d *PhonemeDictionary) Size() int {
	return len(d.entries)
}
