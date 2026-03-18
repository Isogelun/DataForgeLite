# DataForgeLite

<div align="center">

**一站式歌唱合成数据集制作工具**

专为歌唱合成（如DiffSinger）数据集制作而设计的跨平台桌面应用程序

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

[快速开始](#快速开始) | [功能特性](#功能特性) | [开发指南](#开发指南) | [文档](#文档)

</div>

## 项目简介

DataForgeLite 是一款专为歌唱合成（如DiffSinger）数据集制作而设计的桌面应用程序。它整合了从音频处理到数据集生成的完整工作流程，使用 Go 语言开发，通过 Fyne 框架实现跨平台 GUI 界面。

### 核心优势

- **环境独立性**：内置 ONNX 推理引擎，无需配置 Python 环境
- **一站式解决方案**：整合音频处理、标注管理、数据集构建全流程
- **跨平台支持**：支持 Windows、macOS、Linux
- **用户友好**：直观的图形界面，替代复杂的命令行操作
- **专业专注**：专门针对歌唱合成数据集制作优化

## 功能特性

### 已实现功能

- ✅ 项目管理系统
  - 项目创建和配置
  - 多项目支持和状态管理
  - 工作空间管理

- ✅ 数据模型设计
  - 完整的项目数据模型
  - 音频文件数据模型
  - 数据集和标注模型
  - 质量评估指标

### 开发中功能

- 🚧 音频处理模块
  - 音频文件导入和格式转换
  - 音频切片和质量检查
  - 音频元数据提取

- 🚧 标注管理系统
  - 多语言字典支持
  - 音素标注验证
  - ONNX 强制对齐集成

- 🚧 数据集构建
  - 声学模型数据集构建
  - 唱法模型数据集扩展
  - 音高推断和 MIDI 生成

- 🚧 质量控制
  - 音素覆盖分析
  - 音域统计
  - 数据完整性检查

### ASR（语音识别，纯 Go + GGUF）

- ✅ **仅使用 GGUF 后端、无 Python**
  - 编码器：Go + ONNX（`qwen3_asr_encoder_frontend/backend.int4.onnx`）
  - 解码器：子进程调用 `llama_asr_decode`（需自行用 llama.cpp 编译，见 `cmd/llama_asr_decode/README.md`）
- 模型目录：在 **exe 同目录或当前目录**（或该目录下的 `model/` 子目录）放入 [Qwen3-ASR-GGUF](https://github.com/HaujetZhao/Qwen3-ASR-GGUF) 的 encoder ONNX 与 `qwen3_asr_llm.q4_k.gguf`
- 将编译好的 `llama_asr_decode.exe` 放在 **DataForgeLite.exe 同目录或当前目录**（或上级目录）即可

## 项目结构

```
DataForgeLite/
├── cmd/
│   └── dataforgether/
│       └── main.go                 # 应用程序入口
├── internal/                      # 内部包（待实现）
│   ├── annotation/                 # 标注管理模块
│   ├── data/                      # 数据处理模块
│   ├── dataset/                    # 数据集构建模块
│   ├── exporter/                   # 数据导出模块
│   ├── processor/                  # 音频处理模块
│   ├── project/                    # 项目管理模块
│   └── ui/                         # 用户界面模块
├── pkg/                           # 公共包
│   ├── models/                     # 数据模型
│   │   ├── audio.go               # 音频文件模型
│   │   ├── dataset.go             # 数据集模型
│   │   └── project.go             # 项目模型
│   └── utils/                      # 工具函数（待实现）
├── assets/                        # 资源文件
│   ├── dictionaries/             # 字典文件
│   ├── scripts/                  # Python 脚本
│   └── models/                   # 预训练模型
├── build/                         # 构建脚本
│   └── build.go                   # 跨平台构建脚本
├── docs/                         # 文档
├── tests/                        # 测试文件
├── main.go                       # 示例代码
├── go.mod                        # Go 模块定义
├── Makefile                      # 构建命令
└── README.md                     # 项目说明
```

## 技术栈

### 核心技术

- **编程语言**：Go 1.21+
- **GUI 框架**：Fyne v2.4.5
- **推理引擎**：ONNX Runtime（计划集成）
- **音频处理**：go-audio（计划集成）

### 主要依赖

```go
require (
    fyne.io/fyne/v2 v2.4.5              // 跨平台 GUI 框架
    github.com/xuri/excelize/v2 v2.8.0  // Excel 文件处理
    github.com/jszwec/csvutil v1.6.0   // CSV 文件处理
    github.com/tealeg/xlsx/v3 v3.3.6   // XLSX 文件处理
    github.com/disintegration/imaging v1.6.2  // 图像处理
    github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646  // 图像缩放
    github.com/gonum/plot v0.0.0-20231212051148-a0550b7137e4  // 数据可视化
)
```

## 快速开始

### 系统要求

- **操作系统**：Windows 10/11, macOS 10.15+, Ubuntu 18.04+
- **Go 版本**：1.21 或更高版本
- **内存**：8GB RAM（推荐 16GB）
- **存储**：10GB 可用空间

### 开发环境设置

```bash
# 1. 克隆仓库
git clone https://github.com/your-org/DataForgeLite.git
cd DataForgeLite

# 2. 安装依赖
go mod download

# 3. 设置开发环境
make setup-dev

# 4. 运行开发版本
make run
```

### 构建项目

```bash
# 构建开发版本（当前平台）
make build-dev

# 构建所有平台版本
make build

# 运行测试
make test

# 代码格式化
make fmt

# 代码检查
make lint
```

### Makefile 命令说明

| 命令 | 说明 |
|------|------|
| `make help` | 显示所有可用命令 |
| `make build` | 构建所有平台的发布版本 |
| `make build-dev` | 构建开发版本（当前平台） |
| `make test` | 运行测试 |
| `make clean` | 清理构建文件 |
| `make fmt` | 格式化代码 |
| `make lint` | 代码检查 |
| `make setup-dev` | 设置开发环境 |
| `make run` | 运行开发版本 |
| `make deps-update` | 更新依赖 |
| `make package` | 打包发布版本 |

## 开发指南

### 项目架构

项目采用分层模块化架构：

```
┌─────────────────────────────────────────────────────────────┐
│                    GUI Layer (Fyne)                        │
├─────────────────────────────────────────────────────────────┤
│                 Application Logic Layer                     │
├─────────────────────────────────────────────────────────────┤
│  Project │  Audio  │  Annotation │  Dataset  │  Utils    │
│  Manager │ Processor│  Manager    │  Builder  │  Module   │
├─────────────────────────────────────────────────────────────┤
│            Data Processing Layer (ONNX Runtime)            │
├─────────────────────────────────────────────────────────────┤
│              Storage & Configuration Layer                 │
└─────────────────────────────────────────────────────────────┘
```

### 核心数据模型

#### 项目模型（Project）

```go
type Project struct {
    ID          string        // 项目唯一标识
    Name        string        // 项目名称
    Description string        // 项目描述
    Type        ProjectType   // 项目类型：声学/唱法
    Language    Language      // 项目语言：中文/日文/英文
    Config      ProjectConfig // 项目配置
    Status      ProjectStatus // 项目状态
    Workspace   string        // 工作空间路径
    Progress    float64       // 进度（0-100）
    CreatedAt   time.Time     // 创建时间
    UpdatedAt   time.Time     // 更新时间
}
```

#### 音频文件模型（AudioFile）

```go
type AudioFile struct {
    ID            string                 // 文件唯一标识
    ProjectID     string                 // 所属项目ID
    FilePath      string                 // 原始文件路径
    ProcessedPath string                 // 处理后文件路径
    Metadata      *AudioMetadata         // 音频元数据
    Status        AudioStatus            // 处理状态
    Validation    *AudioValidationResult // 验证结果
    Quality       *AudioQualityMetrics  // 质量指标
    Annotations   []*Annotation          // 标注信息
    CreatedAt     time.Time              // 创建时间
    UpdatedAt     time.Time              // 更新时间
}
```

#### 数据集模型（Dataset）

```go
type Dataset struct {
    ID             string              // 数据集唯一标识
    ProjectID      string              // 所属项目ID
    Name           string              // 数据集名称
    Type           DatasetType         // 数据集类型
    Status         DatasetStatus       // 数据集状态
    Path           string              // 数据集路径
    AudioFiles     []*AudioFile        // 音频文件列表
    Transcriptions []*TranscriptionEntry // 转录条目
    Statistics     *DatasetStatistics  // 统计信息
    QualityMetrics *DatasetQualityMetrics // 质量指标
    CreatedAt      time.Time           // 创建时间
    UpdatedAt      time.Time           // 更新时间
}
```

### 开发规范

#### 代码风格

- 遵循 Go 官方代码规范
- 使用 `go fmt` 格式化代码
- 使用 `golangci-lint` 进行代码检查
- 添加必要的注释和文档

#### Git 提交规范

```
<type>(<scope>): <subject>

<body>

<footer>
```

类型（type）：
- `feat`: 新功能
- `fix`: 修复 bug
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具链相关

#### 测试规范

- 每个模块都需要单元测试
- 测试覆盖率 >80%
- 使用 table-driven test 模式
- 添加集成测试和端到端测试

## 文档

- [产品文档](docs/产品文档.md) - 产品概述和功能介绍
- [开发文档](docs/开发文档.md) - 技术架构和开发指南
- [开发计划](docs/开发计划.md) - 项目规划和里程碑

## 贡献指南

我们欢迎社区贡献！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'feat: add some amazing feature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

详细的贡献指南请参考 [CONTRIBUTING.md](CONTRIBUTING.md)（待创建）

## 路线图

### 第一阶段：基础架构（进行中）

- [x] 项目结构和数据模型
- [x] 构建系统
- [ ] Fyne UI 框架集成
- [ ] 项目管理模块
- [ ] 音频处理基础

### 第二阶段：音频处理与标注（计划中）

- [ ] 音频处理模块
- [ ] 字典管理系统
- [ ] 标注验证系统
- [ ] ONNX 强制对齐集成
- [ ] 质量控制系统

### 第三阶段：数据集构建（计划中）

- [ ] 声学模型数据集构建
- [ ] ONNX 唱法模型集成
- [ ] ONNX 音高推断
- [ ] 批量处理系统

### 第四阶段：优化与发布（计划中）

- [ ] 性能优化
- [ ] 稳定性改进
- [ ] 用户体验优化
- [ ] 全面测试和发布

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详细信息。

## 致谢

- [DiffSinger](https://github.com/openvpi/DiffSinger) - 优秀的歌唱合成框架
- [MakeDiffSinger](https://github.com/openvpi/MakeDiffSinger) - 数据集制作流程参考
- [Fyne](https://fyne.io/) - 跨平台 GUI 框架
- [HFA](https://github.com/qiuqiao/HFA) - 强制对齐工具

## 联系方式

- **GitHub Issues**: [问题报告和功能请求](https://github.com/your-org/DataForgeLite/issues)
- **GitHub Discussions**: [讨论和问题解答](https://github.com/your-org/DataForgeLite/discussions)

---

<div align="center">

**如果这个项目对您有帮助，请给我们一个 ⭐️**

Made with ❤️ by the DataForgeLite Team

</div>
