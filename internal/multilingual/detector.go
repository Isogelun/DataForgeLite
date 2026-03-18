package multilingual

import (
	"unicode"
)

// Unicode 范围常量
const (
	// 汉字范围
	HanStart = 0x4e00
	HanEnd   = 0x9fff

	// 平假名范围
	HiraganaStart = 0x3040
	HiraganaEnd   = 0x309f

	// 片假名范围
	KatakanaStart = 0x30a0
	KatakanaEnd   = 0x30ff

	// 拉丁字母范围
	LatinStart = 0x0041
	LatinEnd   = 0x007a

	// 判定阈值
	ChineseThreshold  = 0.8 // 汉字占比≥80% 判定为中文
	JapaneseThreshold = 0.3 // 假名占比≥30% 判定为日文
	EnglishThreshold  = 0.8 // 拉丁字母占比≥80% 判定为英文
)

// LanguageDetector 语言检测器接口
type LanguageDetector interface {
	// Detect 检测文本语言
	Detect(text string) Language
	// GetStats 获取字符统计信息
	GetStats(text string) *CharStats
}

// DefaultLanguageDetector 默认语言检测器实现
type DefaultLanguageDetector struct{}

// NewLanguageDetector 创建新的语言检测器
func NewLanguageDetector() LanguageDetector {
	return &DefaultLanguageDetector{}
}

// Detect 检测文本的语言类型
// 参数:
//   text: 待检测的文本
// 返回:
//   Language: 检测到的语言类型
func (d *DefaultLanguageDetector) Detect(text string) Language {
	stats := d.GetStats(text)

	if stats.TotalCount == 0 {
		return LanguageUnknown
	}

	// 计算各类字符占比
	hanRatio := float64(stats.HanCount) / float64(stats.TotalCount)
	kanaRatio := float64(stats.HiraganaCount+stats.KatakanaCount) / float64(stats.TotalCount)
	latinRatio := float64(stats.LatinCount) / float64(stats.TotalCount)

	// 判定规则：
	// 1. 假名占比≥30% → 日文（优先判定，因为日文可能包含汉字）
	// 2. 汉字占比≥80% → 中文
	// 3. 拉丁字母占比≥80% → 英文
	// 4. 否则 → 未知

	if kanaRatio >= JapaneseThreshold {
		return LanguageJapanese
	}

	if hanRatio >= ChineseThreshold {
		return LanguageChinese
	}

	if latinRatio >= EnglishThreshold {
		return LanguageEnglish
	}

	return LanguageUnknown
}

// GetStats 获取文本的字符统计信息
// 参数:
//   text: 待统计的文本
// 返回:
//   *CharStats: 字符统计信息
func (d *DefaultLanguageDetector) GetStats(text string) *CharStats {
	stats := &CharStats{}

	for _, r := range text {
		// 跳过空白字符
		if unicode.IsSpace(r) {
			continue
		}

		stats.TotalCount++

		// 判断字符类型
		if isHan(r) {
			stats.HanCount++
		} else if isHiragana(r) {
			stats.HiraganaCount++
		} else if isKatakana(r) {
			stats.KatakanaCount++
		} else if isLatin(r) {
			stats.LatinCount++
		}
	}

	return stats
}

// isHan 判断是否为汉字
func isHan(r rune) bool {
	return r >= HanStart && r <= HanEnd
}

// isHiragana 判断是否为平假名
func isHiragana(r rune) bool {
	return r >= HiraganaStart && r <= HiraganaEnd
}

// isKatakana 判断是否为片假名
func isKatakana(r rune) bool {
	return r >= KatakanaStart && r <= KatakanaEnd
}

// isLatin 判断是否为拉丁字母
func isLatin(r rune) bool {
	return (r >= LatinStart && r <= LatinEnd) || unicode.Is(unicode.Latin, r)
}
