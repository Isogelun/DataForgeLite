package exporter

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Transcription 表示一条转录记录
type Transcription struct {
	// Name 文件名（不含扩展名）
	Name string
	// PhSeq 音素序列（空格分隔）
	PhSeq string
	// PhDur 音素时长序列（空格分隔，单位：秒）
	PhDur string
}

// DatasetRecord 表示一条数据集记录
type DatasetRecord struct {
	// Name 文件名（不含扩展名）
	Name string
	// PhSeq 音素序列
	PhSeq []string
	// PhDur 音素时长（秒）
	PhDur []float64
}

// CSVGenerator CSV 文件生成器
type CSVGenerator struct{}

// NewCSVGenerator 创建新的 CSV 生成器
func NewCSVGenerator() *CSVGenerator {
	return &CSVGenerator{}
}

// WriteCSV 将转录记录写入 CSV 文件
func (g *CSVGenerator) WriteCSV(filePath string, records []Transcription) error {
	// 确保输出目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return NewFileError(fmt.Sprintf("无法创建输出目录: %s", dir), err)
	}

	// 创建文件
	file, err := os.Create(filePath)
	if err != nil {
		return NewFileError(fmt.Sprintf("无法创建 CSV 文件: %s", filePath), err)
	}
	defer file.Close()

	// 创建 CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	header := []string{"name", "ph_seq", "ph_dur"}
	if err := writer.Write(header); err != nil {
		return NewFileError("写入 CSV 表头失败", err)
	}

	// 写入数据行
	for _, record := range records {
		row := []string{record.Name, record.PhSeq, record.PhDur}
		if err := writer.Write(row); err != nil {
			return NewFileError(fmt.Sprintf("写入 CSV 行失败: %s", record.Name), err)
		}
	}

	return nil
}

// Generate 从数据记录列表生成 CSV 文件
func (g *CSVGenerator) Generate(records []*DatasetRecord, outputPath string) error {
	// 转换为 Transcription 格式
	transcriptions := make([]Transcription, 0, len(records))
	for _, record := range records {
		transcriptions = append(transcriptions, Transcription{
			Name:  record.Name,
			PhSeq: strings.Join(record.PhSeq, " "),
			PhDur: g.formatDurations(record.PhDur),
		})
	}

	return g.WriteCSV(outputPath, transcriptions)
}

// formatDurations 将时长数组格式化为空格分隔的字符串
func (g *CSVGenerator) formatDurations(durations []float64) string {
	strs := make([]string, len(durations))
	for i, d := range durations {
		// TextGrid 的时间单位是秒；这里保留最多 10 位小数并去掉尾随 0，避免精度/科学计数法问题
		strs[i] = formatSeconds(d)
	}
	return strings.Join(strs, " ")
}

func formatSeconds(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	// 先固定 10 位，再 trim，保证稳定输出
	s := strconv.FormatFloat(sec, 'f', 10, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

// ReadCSV 从 CSV 文件读取转录记录
func (g *CSVGenerator) ReadCSV(filePath string) ([]Transcription, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, NewFileError(fmt.Sprintf("无法打开 CSV 文件: %s", filePath), err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, NewParseError(fmt.Sprintf("读取 CSV 文件失败: %s", filePath), err)
	}

	if len(records) < 1 {
		return nil, NewParseError("CSV 文件为空", nil)
	}

	// 跳过表头
	transcriptions := make([]Transcription, 0, len(records)-1)
	for i, record := range records {
		if i == 0 {
			// 验证表头
			if len(record) < 3 {
				return nil, NewParseError("CSV 表头格式无效", nil)
			}
			continue
		}

		if len(record) < 3 {
			return nil, NewParseError(fmt.Sprintf("CSV 第 %d 行格式无效", i+1), nil)
		}

		transcriptions = append(transcriptions, Transcription{
			Name:  record[0],
			PhSeq: record[1],
			PhDur: record[2],
		})
	}

	return transcriptions, nil
}

// ParseDurations 解析时长字符串为浮点数组
func ParseDurations(durStr string) ([]float64, error) {
	parts := strings.Fields(durStr)
	durations := make([]float64, 0, len(parts))
	for _, part := range parts {
		var d float64
		if _, err := fmt.Sscanf(part, "%f", &d); err != nil {
			return nil, NewParseError(fmt.Sprintf("无法解析时长: %s", part), err)
		}
		durations = append(durations, d)
	}
	return durations, nil
}

// ParsePhSeq 解析音素字符串为数组
func ParsePhSeq(phSeqStr string) []string {
	return strings.Fields(phSeqStr)
}

// String 返回 Transcription 的字符串表示
func (t *Transcription) String() string {
	return fmt.Sprintf("Transcription{Name: %q, PhSeq: %q, PhDur: %q}", t.Name, t.PhSeq, t.PhDur)
}

// String 返回 DatasetRecord 的字符串表示
func (r *DatasetRecord) String() string {
	return fmt.Sprintf("DatasetRecord{Name: %q, PhSeq: %v, PhDur: %v}", r.Name, r.PhSeq, r.PhDur)
}
