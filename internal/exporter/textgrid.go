package exporter

import (
	"fmt"
	"strings"

	"DataForgeLite/internal/tgannotation"
)

// Interval 表示 TextGrid 中的一个时间区间
type Interval struct {
	// Xmin 区间起始时间（秒）
	Xmin float64
	// Xmax 区间结束时间（秒）
	Xmax float64
	// Text 区间文本（音素或词语）
	Text string
}

// Tier 表示 TextGrid 中的一个层级
type Tier struct {
	// Name 层级名称（如 "phones", "words", "ph"）
	Name string
	// Class 层级类型（如 "IntervalTier", "TextTier"）
	Class string
	// Xmin 层级起始时间
	Xmin float64
	// Xmax 层级结束时间
	Xmax float64
	// Intervals 区间列表
	Intervals []Interval
}

// TextGrid 表示整个 TextGrid 文件的结构
type TextGrid struct {
	// Xmin 文件起始时间（秒）
	Xmin float64
	// Xmax 文件结束时间（秒）
	Xmax float64
	// Tiers 层级列表
	Tiers []Tier
}

// PhonemeData 表示从 TextGrid 提取的音素数据
type PhonemeData struct {
	// PhSeq 音素序列
	PhSeq []string
	// PhDur 音素时长序列（秒）
	PhDur []float64
}

// TextGridParser TextGrid 文件解析器（使用 tgannotation 的 TextGrid 解析实现，避免格式差异导致的解析问题）
type TextGridParser struct{}

func NewTextGridParser() *TextGridParser { return &TextGridParser{} }

// Parse 解析 Praat TextGrid（.TextGrid）
func (p *TextGridParser) Parse(filePath string) (*TextGrid, error) {
	tg, err := tgannotation.ReadTextGrid(filePath)
	if err != nil {
		return nil, NewParseError(fmt.Sprintf("解析 TextGrid 文件失败: %s", filePath), err)
	}

	out := &TextGrid{Xmin: tg.Xmin, Xmax: tg.Xmax, Tiers: make([]Tier, 0, len(tg.Tiers))}
	for _, t := range tg.Tiers {
		tt := Tier{
			Name:      t.Name,
			Class:     "IntervalTier",
			Xmin:      t.Xmin,
			Xmax:      t.Xmax,
			Intervals: make([]Interval, 0, len(t.Intervals)),
		}
		for _, iv := range t.Intervals {
			tt.Intervals = append(tt.Intervals, Interval{
				Xmin: iv.Xmin,
				Xmax: iv.Xmax,
				Text: iv.Mark,
			})
		}
		out.Tiers = append(out.Tiers, tt)
	}
	return out, nil
}

// ExtractPhonemes 从 TextGrid 中提取指定层级的音素数据
func (p *TextGridParser) ExtractPhonemes(tg *TextGrid, tierName string) (*PhonemeData, error) {
	// 查找指定名称的层级
	var targetTier *Tier
	for i := range tg.Tiers {
		if strings.EqualFold(tg.Tiers[i].Name, tierName) {
			targetTier = &tg.Tiers[i]
			break
		}
	}

	if targetTier == nil {
		return nil, NewParseError(fmt.Sprintf("未找到名称为 '%s' 的层级（可用层级: %s）", tierName, strings.Join(tg.GetTierNames(), ", ")), nil)
	}

	data := &PhonemeData{
		PhSeq: make([]string, 0),
		PhDur: make([]float64, 0),
	}

	// 提取音素和时长
	for _, interval := range targetTier.Intervals {
		text := strings.TrimSpace(interval.Text)
		if text == "" {
			// TextGrid 中空标注通常表示静音；为了与常见训练管线兼容，统一映射为 SP
			text = "SP"
		}
		data.PhSeq = append(data.PhSeq, text)
		duration := interval.Xmax - interval.Xmin
		if duration < 0 {
			duration = 0
		}
		data.PhDur = append(data.PhDur, duration)
	}

	return data, nil
}

// GetTierNames 获取所有层级名称
func (tg *TextGrid) GetTierNames() []string {
	names := make([]string, 0, len(tg.Tiers))
	for _, tier := range tg.Tiers {
		names = append(names, tier.Name)
	}
	return names
}

// GetTier 根据名称获取层级
func (tg *TextGrid) GetTier(name string) *Tier {
	for i := range tg.Tiers {
		if tg.Tiers[i].Name == name {
			return &tg.Tiers[i]
		}
	}
	return nil
}

// String 返回 TextGrid 的字符串表示
func (tg *TextGrid) String() string {
	return fmt.Sprintf("TextGrid{Xmin: %.3f, Xmax: %.3f, Tiers: %d}", tg.Xmin, tg.Xmax, len(tg.Tiers))
}

// String 返回 PhonemeData 的字符串表示
func (pd *PhonemeData) String() string {
	return fmt.Sprintf("PhonemeData{PhSeq: %v, PhDur: %v}", pd.PhSeq, pd.PhDur)
}
