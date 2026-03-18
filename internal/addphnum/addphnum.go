// Package addphnum 提供音素数量计算功能
//
// 本包用于对 CSV 文件中的音素序列列进行最长前缀匹配处理，
// 计算每个音素序列包含的音素数量，并将结果作为 ph_num 列添加到 CSV 文件中。
//
// 基本使用示例：
//
//	config := &addphnum.ProcessorConfig{
//	    InputCSV:  "input.csv",
//	    OutputCSV: "output.csv",
//	    DictPath:  "dict.txt",
//	    PhSeqCol:  "ph_seq",
//	}
//	
//	if err := addphnum.AddPhNumToCSV(config); err != nil {
//	    log.Fatal(err)
//	}
//
// 主要类型：
//   - ProcessorConfig: 处理器配置
//   - PhonemeDictionary: 音素词典
//   - PhNumProcessor: 音素数量处理器
package addphnum

import (
	"fmt"

	"DataForgeLite/internal/exporter"
)

// AddPhNumToCSV 执行完整的音素数量计算流程
//
// 该函数是包的主要入口点，执行以下操作：
//  1. 验证配置参数
//  2. 加载音素词典
//  3. 创建处理器实例
//  4. 处理CSV文件，计算ph_num
//  5. 写入输出CSV文件
//
// 参数:
//   - config: 处理器配置，包含输入/输出文件路径和词典路径
//
// 返回:
//   - error: 处理过程中的错误，nil 表示成功
//
// 示例:
//
//	config := addphnum.NewConfig("input.csv", "output.csv", "dict.txt")
//	if err := addphnum.AddPhNumToCSV(config); err != nil {
//	    log.Printf("处理失败: %v", err)
//	}
func AddPhNumToCSV(config *ProcessorConfig) error {
	// 验证配置
	if err := config.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 加载词典
	dict := NewPhonemeDictionary()
	if err := dict.Load(config.DictPath); err != nil {
		return fmt.Errorf("加载词典失败: %w", err)
	}

	// 创建处理器
	processor := NewPhNumProcessor(dict, config)

	// 执行处理
	if err := processor.Process(config.InputCSV, config.OutputCSV); err != nil {
		return fmt.Errorf("处理CSV文件失败: %w", err)
	}

	return nil
}

// AddPhNumToCSVWithDict 使用已加载的词典执行音素数量计算
//
// 适用于需要重复使用同一词典处理多个CSV文件的场景，
// 避免重复加载词典的开销。
//
// 参数:
//   - config: 处理器配置
//   - dict: 已加载的音素词典
//
// 返回:
//   - error: 处理过程中的错误，nil 表示成功
//
// 示例:
//
//	dict := addphnum.NewPhonemeDictionary()
//	if err := dict.Load("dict.txt"); err != nil {
//	    log.Fatal(err)
//	}
//	
//	for _, file := range files {
//	    config := addphnum.NewConfig(file, file+".out", "dict.txt")
//	    if err := addphnum.AddPhNumToCSVWithDict(config, dict); err != nil {
//	        log.Printf("处理 %s 失败: %v", file, err)
//	    }
//	}
func AddPhNumToCSVWithDict(config *ProcessorConfig, dict *PhonemeDictionary) error {
	// 验证配置
	if err := config.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 检查词典是否已加载
	if dict == nil || dict.Size() == 0 {
		return exporter.NewConfigError("词典未加载或为空", nil)
	}

	// 创建处理器
	processor := NewPhNumProcessor(dict, config)

	// 执行处理
	if err := processor.Process(config.InputCSV, config.OutputCSV); err != nil {
		return fmt.Errorf("处理CSV文件失败: %w", err)
	}

	return nil
}
