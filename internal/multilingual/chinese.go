package multilingual

import (
	"strings"
	"unicode"
)

// ChineseProcessor 中文处理器
type ChineseProcessor struct {
	dictionary *PinyinDictionary
	toneMap    map[rune]rune
}

// NewChineseProcessor 创建新的中文处理器
func NewChineseProcessor() *ChineseProcessor {
	return &ChineseProcessor{
		dictionary: NewPinyinDictionary(),
		toneMap: map[rune]rune{
			'ā': 'a', 'á': 'a', 'ǎ': 'a', 'à': 'a',
			'ē': 'e', 'é': 'e', 'ě': 'e', 'è': 'e',
			'ī': 'i', 'í': 'i', 'ǐ': 'i', 'ì': 'i',
			'ō': 'o', 'ó': 'o', 'ǒ': 'o', 'ò': 'o',
			'ū': 'u', 'ú': 'u', 'ǔ': 'u', 'ù': 'u',
			'ǖ': 'ü', 'ǘ': 'ü', 'ǚ': 'ü', 'ǜ': 'ü',
			'ń': 'n', 'ň': 'n', 'ǹ': 'n',
			'ḿ': 'm',
		},
	}
}

// Process 处理中文文本（转无声调拼音）
// 参数:
//   text: 中文文本
// 返回:
//   string: 无声调拼音（空格分隔）
//   error: 执行错误
func (p *ChineseProcessor) Process(text string) (string, error) {
	var result []string

	for _, r := range text {
		if isHan(r) {
			// 汉字：查询拼音
			pinyin := p.dictionary.GetPinyin(r)
			if pinyin != "" {
				// 移除声调
				pinyin = p.removeTone(pinyin)
				result = append(result, pinyin)
			} else {
				// 未知汉字，保留原字符
				result = append(result, string(r))
			}
		} else if unicode.IsSpace(r) {
			// 空白字符：保留
			result = append(result, " ")
		} else {
			// 非汉字字符：保留原样
			result = append(result, string(r))
		}
	}

	// 拼接结果，合并多余空格
	output := strings.Join(result, "")
	output = strings.Join(strings.Fields(output), " ")

	return output, nil
}

// GetLanguage 获取处理器支持的语言
func (p *ChineseProcessor) GetLanguage() Language {
	return LanguageChinese
}

// removeTone 移除拼音中的声调符号
func (p *ChineseProcessor) removeTone(pinyin string) string {
	var result strings.Builder
	for _, r := range pinyin {
		if tone, exists := p.toneMap[r]; exists {
			result.WriteRune(tone)
		} else {
			result.WriteRune(r)
		}
	}
	return strings.ToLower(result.String())
}

