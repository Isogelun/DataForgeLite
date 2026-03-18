// Package addphnum 提供音素数量计算功能
package addphnum

import (
	"fmt"
	"os"

	"DataForgeLite/internal/exporter"
)

// ProcessorConfig 处理器配置
// 包含音素数量计算所需的全部配置参数
type ProcessorConfig struct {
	// InputCSV 输入CSV文件路径
	InputCSV string
	// OutputCSV 输出CSV文件路径
	OutputCSV string
	// DictPath 词典文件路径
	DictPath string
	// PhSeqCol 音素序列列名，默认 "ph_seq"
	PhSeqCol string
}

// DefaultConfig 返回默认配置
// 默认音素序列列名为 "ph_seq"
func DefaultConfig() *ProcessorConfig {
	return &ProcessorConfig{
		PhSeqCol: "ph_seq",
	}
}

// NewConfig 创建新的配置实例
// 使用提供的参数创建配置，未提供的字段使用默认值
func NewConfig(inputCSV, outputCSV, dictPath string) *ProcessorConfig {
	cfg := DefaultConfig()
	cfg.InputCSV = inputCSV
	cfg.OutputCSV = outputCSV
	cfg.DictPath = dictPath
	return cfg
}

// Validate 验证配置参数
// 检查所有必填字段和文件存在性
func (c *ProcessorConfig) Validate() error {
	// 验证输入CSV路径
	if c.InputCSV == "" {
		return exporter.NewConfigError("输入CSV文件路径不能为空", nil)
	}

	// 检查输入文件是否存在
	if _, err := os.Stat(c.InputCSV); os.IsNotExist(err) {
		return exporter.NewConfigError(fmt.Sprintf("输入CSV文件不存在: %s", c.InputCSV), err)
	}

	// 验证输出CSV路径
	if c.OutputCSV == "" {
		return exporter.NewConfigError("输出CSV文件路径不能为空", nil)
	}

	// 验证词典路径
	if c.DictPath == "" {
		return exporter.NewConfigError("词典文件路径不能为空", nil)
	}

	// 检查词典文件是否存在
	if _, err := os.Stat(c.DictPath); os.IsNotExist(err) {
		return exporter.NewConfigError(fmt.Sprintf("词典文件不存在: %s", c.DictPath), err)
	}

	// 验证音素序列列名
	if c.PhSeqCol == "" {
		c.PhSeqCol = "ph_seq"
	}

	return nil
}
