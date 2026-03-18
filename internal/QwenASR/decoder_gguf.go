// Package asrinfer: GGUF 解码器通过子进程调用 llama_asr_decode 可执行文件（需单独编译，见 cmd/llama_asr_decode）。
package asrinfer

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindLLamaASRDecode 在 exe 同目录、上级目录、当前工作目录、以及打包用的 llama-*-bin-* 子目录中查找 llama_asr_decode.exe。
func FindLLamaASRDecode() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exe)
	cwd, _ := os.Getwd()
	name := "llama_asr_decode"
	if filepath.Ext(strings.ToLower(exe)) == ".exe" {
		name = "llama_asr_decode.exe"
	}
	// 1) 先查 exe 同目录及上级、cwd 及上级
	for _, dir := range []string{exeDir, filepath.Dir(exeDir), filepath.Dir(filepath.Dir(exeDir)), cwd, filepath.Dir(cwd), filepath.Dir(filepath.Dir(cwd))} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// 2) 再查 exe/cwd 下名为 llama-*-bin-* 的子目录（WPF 打包复制后的 llama-b8389-bin-win-cpu-x64 等）
	for _, root := range []string{exeDir, cwd} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			lower := strings.ToLower(e.Name())
			if strings.Contains(lower, "llama") && (strings.Contains(lower, "bin") || strings.Contains(lower, "b8389")) {
				p := filepath.Join(root, e.Name(), name)
				if _, err := os.Stat(p); err == nil {
					return p, nil
				}
			}
		}
	}
	return "", fmt.Errorf("未找到 %s，请将编译好的可执行文件放在 DataForgeLite 同目录或当前目录，或放入 llama-b8389-bin-win-cpu-x64 子目录", name)
}

// WriteEmbeddingsFile 将 audio embedding (seqLen, dim) 写入二进制文件：4 字节 seqLen, 4 字节 dim, 再 seqLen*dim 个 float32。
func WriteEmbeddingsFile(path string, emb []float32, seqLen, dim int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := binary.Write(f, binary.LittleEndian, int32(seqLen)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, int32(dim)); err != nil {
		return err
	}
	return binary.Write(f, binary.LittleEndian, emb)
}

// RunLLamaASRDecode 调用 llama_asr_decode --model <modelDir> --embeddings <embPath> [--max-tokens 256]，返回 stdout 作为识别文本。
func RunLLamaASRDecode(modelDir, embeddingsPath string, maxTokens int) (string, error) {
	decExe, err := FindLLamaASRDecode()
	if err != nil {
		return "", err
	}
	args := []string{
		"--model", modelDir,
		"--embeddings", embeddingsPath,
	}
	if maxTokens > 0 {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", maxTokens))
	}
	cmd := exec.Command(decExe, args...)
	cmd.Dir = filepath.Dir(decExe)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("llama_asr_decode 执行失败: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
