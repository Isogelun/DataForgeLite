package multilingual

import (
	"regexp"
	"strings"
)

// EnglishProcessor 英文处理器
type EnglishProcessor struct {
	punctuationPattern *regexp.Regexp
	contractionPattern *regexp.Regexp
	spacePattern       *regexp.Regexp
}

// NewEnglishProcessor 创建新的英文处理器
func NewEnglishProcessor() *EnglishProcessor {
	return &EnglishProcessor{
		// 匹配常见标点符号
		punctuationPattern: regexp.MustCompile(`[.,!?;:"'(){}\[\]<>—–…&@#$%^*+=\\|/]`),
		// 匹配缩写（如 don't, I'm, won't）
		contractionPattern: regexp.MustCompile(`'([stremd]|ll|ve|re|nt)\b`),
		// 匹配多个空白字符
		spacePattern: regexp.MustCompile(`\s+`),
	}
}

// Process 处理英文文本（去除标点符号）
// 参数:
//   text: 英文文本
// 返回:
//   string: 处理后的文本（小写、无标点）
//   error: 执行错误
func (p *EnglishProcessor) Process(text string) (string, error) {
	// 1. 转换为小写
	result := strings.ToLower(text)

	// 2. 处理缩写：将 don't → dont, I'm → Im 等
	result = p.contractionPattern.ReplaceAllString(result, "$1")

	// 3. 移除标点符号
	result = p.punctuationPattern.ReplaceAllString(result, "")

	// 4. 合并多余空格
	result = p.spacePattern.ReplaceAllString(result, " ")

	// 5. 修剪首尾空格
	result = strings.TrimSpace(result)

	return result, nil
}

// GetLanguage 获取处理器支持的语言
func (p *EnglishProcessor) GetLanguage() Language {
	return LanguageEnglish
}
