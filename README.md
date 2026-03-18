# DataForgeLite

<div align="center">

**一站式歌唱合成数据集制作工具**

专为 DiffSinger 等歌唱合成框架设计的 Windows 桌面应用

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![Platform](https://img.shields.io/badge/Platform-Windows-blue)
![License](https://img.shields.io/badge/License-MIT-green)

</div>

## 简介

DataForgeLite 是一款面向歌唱合成数据集制作的 Windows 桌面工具，整合了从原始音频到完整数据集的全流程：

- 音频预处理（响度归一化、重采样、单声道）
- VAD 智能切分
- ASR 语音识别（Qwen3-ASR，纯 Go ONNX 推理）
- HubertFA 强制对齐（音素级 TextGrid）
- GAME ONNX 音符推理（note_seq / note_dur）
- 数据集导出（DiffSinger 格式）

后端为 Go 编写的命令行程序，前端为 WPF（.NET 4.6.2）桌面客户端，**无需 Python 环境**。

---

## 功能

| 模块 | 说明 |
|------|------|
| 预处理 | LUFS 响度归一化、重采样到目标采样率、混音为单声道 |
| 切分 | 基于 VAD 的静音检测自动切片 |
| ASR | Qwen3-ASR ONNX 推理，支持中/英/日等多语言，支持 CPU / DirectML GPU |
| FA 对齐 | HubertFA ONNX 强制对齐，输出 TextGrid |
| GAME Align | GAME ONNX 音符边界推理，写入 note_seq / note_dur |
| 导出 | 生成 DiffSinger transcriptions CSV + wavs 目录 |
| 合并/切片 | 3-tier TextGrid 合并与拆分 |

---

## 依赖模型

### ASR 模型（Qwen3-ASR ONNX）

从以下仓库下载 ONNX 格式模型：

**[andrewleech/qwen3-asr-onnx](https://github.com/andrewleech/qwen3-asr-onnx)**

下载后将模型目录（含 `encoder.onnx`、`decoder_init.onnx`、`decoder_step.onnx`、`embed_tokens.bin`）放在 `DataForgeLite.exe` 同目录下，目录名包含 `qwen3` 和 `onnx` 即可自动识别，例如：

```
DataForgeLite.exe
qwen3-asr-1.7b-onnx/
  encoder.onnx
  decoder_init.onnx
  decoder_step.onnx
  embed_tokens.bin
  ...
```

### HubertFA 模型

将 HubertFA 模型目录放在 `DataForgeLite.exe` 同目录下，命名为 `hfa_model`：

```
hfa_model/
  model.onnx
  config.json
  vocab.json
  ...
```

### GAME ONNX 模型

将 GAME 模型目录放在 `DataForgeLite.exe` 同目录下，命名为 `Gameonnx`：

```
Gameonnx/
  encoder.onnx
  segmenter.onnx
  estimator.onnx
  dur2bd.onnx
  bd2dur.onnx
  config.json
```

### ONNX Runtime

- **CPU 推理**：使用标准版 `onnxruntime-win-x64-*.zip`，将 `onnxruntime.dll` 放在 `DataForgeLite.exe` 同目录
- **DirectML GPU 加速**：使用 `Microsoft.ML.OnnxRuntime.DirectML` NuGet 包中的 `onnxruntime.dll`，在 UI 顶部选择 `DML (GPU)` 并选择 GPU 设备

---

## 构建

### 前置要求

- Go 1.21+
- CGO 编译器（推荐 [llvm-mingw](https://github.com/mstorsjo/llvm-mingw)）
- .NET Framework 4.6.2（WPF 客户端）

### 编译 Go 后端

```bash
set CGO_ENABLED=1
go build -o DataForgeLite.exe ./cmd/
```

### 编译 WPF 客户端

用 Visual Studio 打开 `DataForgeLiteClient/DataForgeLiteClient.sln`，构建 Release 即可。构建时会自动：
- 编译 Go 后端
- 复制模型目录、ONNX Runtime DLL 到输出目录

---

## 使用

### 图形界面

运行 `DataForgeLiteClient.exe`，按页签顺序操作：

1. **项目概览** — 查看统计信息
2. **预处理/切分** — 音频归一化与 VAD 切片
3. **ASR** — 语音识别，生成 `.lab` 文件
4. **FA 对齐** — HubertFA 强制对齐，生成 TextGrid
5. **导出** — 生成 DiffSinger 数据集，可选运行 GAME Align

顶部导航栏可选择**推理设备**（CPU / DML GPU）。

### 命令行

```bash
# ASR
DataForgeLite.exe --asr --input ./wavs --output ./output --json

# FA 对齐
DataForgeLite.exe --fa --model-dir ./hfa_model --input ./wavs --output ./tg --json

# GAME 推理
DataForgeLite.exe --game --game-input-dir ./wavs --game-ort onnxruntime.dll --game-model-dir ./Gameonnx --json

# 预处理
DataForgeLite.exe --preprocess --preprocess-input ./raw --preprocess-output ./processed --target-lufs -18

# 切分
DataForgeLite.exe --split --split-input ./processed --split-output ./sliced
```

#### DirectML GPU 加速（命令行）

```bash
set ORT_DEVICE=dml
set ORT_DML_DEVICE_ID=0
DataForgeLite.exe --asr --input ./wavs --output ./output
```

---

## 项目结构

```
DataForgeLite/
├── cmd/                    # Go 后端入口
├── internal/
│   ├── QwenASR/            # Qwen3-ASR ONNX 推理
│   ├── hubertfa/           # HubertFA 强制对齐
│   ├── gameonnx/           # GAME ONNX 音符推理
│   ├── audiopreprocessor/  # 音频预处理
│   ├── audiosplitter/      # VAD 切分
│   ├── exporter/           # 数据集导出
│   ├── addphnum/           # 音素数量计算
│   ├── tgannotation/       # TextGrid 合并/切片
│   └── multilingual/       # 多语言支持
├── DataForgeLiteClient/    # WPF 桌面客户端
├── HubertFA/               # HubertFA Python 参考实现
├── docs/                   # 文档
└── tools/                  # 工具脚本
```

---

## 致谢

- [DiffSinger](https://github.com/openvpi/DiffSinger)
- [HubertFA](https://github.com/qiuqiao/HFA)
- [andrewleech/qwen3-asr-onnx](https://github.com/andrewleech/qwen3-asr-onnx)
- [GAME](https://github.com/openvpi/SOME)
- [onnxruntime_go](https://github.com/yalue/onnxruntime_go)

## 许可证

MIT
