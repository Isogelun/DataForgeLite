// 示例：纯 Go GGUF ASR，无需 Python。需在 exe 同目录或当前目录放置 model/ 与 llama_asr_decode。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	asrinfer "DataForgeLite/internal/QwenASR"
)

func main() {
	inputDir := flag.String("input", "./audio_files", "音频文件所在文件夹")
	outputDir := flag.String("output", "./results", "输出结果文件夹")
	flag.Parse()

	if _, err := os.Stat(*inputDir); os.IsNotExist(err) {
		log.Fatalf("输入目录不存在：%s", *inputDir)
	}
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败：%v", err)
	}

	config := asrinfer.DefaultConfig(*inputDir, *outputDir)
	inferencer := asrinfer.NewASRInferencer(config)

	fmt.Println("=== ASR 推理（纯 Go GGUF）===")
	fmt.Printf("输入：%s\n", *inputDir)
	fmt.Printf("输出：%s\n", *outputDir)
	fmt.Println()

	response, err := inferencer.Transcribe()
	if err != nil {
		log.Fatalf("推理失败：%v", err)
	}
	if !response.Success {
		log.Fatalf("推理失败：%s", response.Error)
	}

	fmt.Println("=== 完成 ===")
	fmt.Printf("成功：%d，失败：%d，输出：%s\n", response.SuccessCount, response.ErrorCount, response.OutputDirectory)
	for i, r := range response.Results {
		if r.Error != "" {
			fmt.Printf("  [%d] %s 错误：%s\n", i+1, r.File, r.Error)
		} else {
			fmt.Printf("  [%d] %s -> %s\n", i+1, r.File, r.Text)
		}
	}
}
