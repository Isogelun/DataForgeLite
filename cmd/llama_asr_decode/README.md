# llama_asr_decode

供 DataForgeLite 纯 Go ASR 调用的 GGUF 解码器：读取 Go 写入的 audio embedding 文件，用 llama.cpp 做 prefill + 自回归解码，输出识别文本。

## 依赖

- [llama.cpp](https://github.com/ggerganov/llama.cpp)（需支持 `llama_batch` 的 `embd` 自定义 embedding 输入）
- CMake 3.14+
- C++17 编译器（MSVC / GCC / Clang）

## 构建（Windows 示例）

1. 克隆并构建 llama.cpp（生成 `llama.dll` 或静态库）：
   ```powershell
   git clone https://github.com/ggerganov/llama.cpp
   cd llama.cpp
   cmake -B build -DGGML_VULKAN=ON
   cmake --build build --config Release
   ```

2. 将本目录中的 `main.cpp` 与 llama.cpp 的 `include`、库链接后编译，或将本目录复制到 llama.cpp 的 `examples` 下用 CMake 集成。

3. 将生成的 `llama_asr_decode.exe` 放在 **DataForgeLite.exe 同目录**、或**当前工作目录**（或它们的上级目录）即可。

## 用法（由 DataForgeLite 自动调用）

```text
llama_asr_decode.exe --model <GGUF模型目录> --embeddings <embedding二进制文件> [--max-tokens 256]
```

- `--model`: 含 `qwen3_asr_llm.q4_k.gguf` 的目录。
- `--embeddings`: Go 写入的 .bin 文件（4 字节 seqLen, 4 字节 dim, 再 seqLen×dim 个 float32）。
- 标准输出：识别出的文本（UTF-8）。

## 模型

与 [Qwen3-ASR-GGUF](https://github.com/HaujetZhao/Qwen3-ASR-GGUF) 的 Models Release 一致：将解压后的 `model` 目录（含 `qwen3_asr_encoder_*.onnx` 与 `qwen3_asr_llm.q4_k.gguf`）放在 DataForgeLite 同目录下即可。
