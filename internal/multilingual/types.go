// Package multilingual 提供多语言文本处理功能
// 支持中文、英文、日文的语言检测和文本转换
package multilingual

// Language 语言类型
type Language string

const (
	// LanguageChinese 中文
	LanguageChinese Language = "zh"
	// LanguageEnglish 英文
	LanguageEnglish Language = "en"
	// LanguageJapanese 日文
	LanguageJapanese Language = "ja"
	// LanguageUnknown 未知语言
	LanguageUnknown Language = "unknown"
)

// TextRecord 文本记录
type TextRecord struct {
	// ID 唯一标识符
	ID string
	// OriginalText 原始文本
	OriginalText string
	// DetectedLanguage 检测到的语言
	DetectedLanguage Language
	// ConvertedText 转换后的文本
	ConvertedText string
}

// CharStats 字符统计信息
type CharStats struct {
	// HanCount 汉字数量
	HanCount int
	// HiraganaCount 平假名数量
	HiraganaCount int
	// KatakanaCount 片假名数量
	KatakanaCount int
	// LatinCount 拉丁字母数量
	LatinCount int
	// TotalCount 总字符数
	TotalCount int
}
