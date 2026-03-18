package multilingual

import (
	"strings"
	"unicode"
)

// JapaneseProcessor 日文处理器
type JapaneseProcessor struct {
	dictionary *JapaneseDictionary
	kanaMap    map[rune]string
}

// NewJapaneseProcessor 创建新的日文处理器
// 参数:
//
//	dictPath: 日文罗马音词典路径（可选）
func NewJapaneseProcessor(dictPath string) *JapaneseProcessor {
	processor := &JapaneseProcessor{
		dictionary: NewJapaneseDictionary(),
		kanaMap:    buildKanaMap(),
	}

	// 加载默认词典
	processor.dictionary.LoadDefault()

	// 如果提供了词典路径，尝试加载
	if dictPath != "" {
		processor.dictionary.Load(dictPath)
	}

	return processor
}

// buildKanaMap 构建平假名/片假名到罗马音的映射表
func buildKanaMap() map[rune]string {
	kanaMap := make(map[rune]string)

	// 平假名基本音
	hiragana := map[rune]string{
		'あ': "a", 'い': "i", 'う': "u", 'え': "e", 'お': "o",
		'か': "ka", 'き': "ki", 'く': "ku", 'け': "ke", 'こ': "ko",
		'さ': "sa", 'し': "shi", 'す': "su", 'せ': "se", 'そ': "so",
		'た': "ta", 'ち': "chi", 'つ': "tsu", 'て': "te", 'と': "to",
		'な': "na", 'に': "ni", 'ぬ': "nu", 'ね': "ne", 'の': "no",
		'は': "ha", 'ひ': "hi", 'ふ': "fu", 'へ': "he", 'ほ': "ho",
		'ま': "ma", 'み': "mi", 'む': "mu", 'め': "me", 'も': "mo",
		'や': "ya", 'ゆ': "yu", 'よ': "yo",
		'ら': "ra", 'り': "ri", 'る': "ru", 'れ': "re", 'ろ': "ro",
		'わ': "wa", 'を': "wo", 'ん': "n",
		'が': "ga", 'ぎ': "gi", 'ぐ': "gu", 'げ': "ge", 'ご': "go",
		'ざ': "za", 'じ': "ji", 'ず': "zu", 'ぜ': "ze", 'ぞ': "zo",
		'だ': "da", 'ぢ': "ji", 'づ': "zu", 'で': "de", 'ど': "do",
		'ば': "ba", 'び': "bi", 'ぶ': "bu", 'べ': "be", 'ぼ': "bo",
		'ぱ': "pa", 'ぴ': "pi", 'ぷ': "pu", 'ぺ': "pe", 'ぽ': "po",
	}

	// 片假名基本音（与平假名相同）
	katakana := map[rune]string{
		'ア': "a", 'イ': "i", 'ウ': "u", 'エ': "e", 'オ': "o",
		'カ': "ka", 'キ': "ki", 'ク': "ku", 'ケ': "ke", 'コ': "ko",
		'サ': "sa", 'シ': "shi", 'ス': "su", 'セ': "se", 'ソ': "so",
		'タ': "ta", 'チ': "chi", 'ツ': "tsu", 'テ': "te", 'ト': "to",
		'ナ': "na", 'ニ': "ni", 'ヌ': "nu", 'ネ': "ne", 'ノ': "no",
		'ハ': "ha", 'ヒ': "hi", 'フ': "fu", 'ヘ': "he", 'ホ': "ho",
		'マ': "ma", 'ミ': "mi", 'ム': "mu", 'メ': "me", 'モ': "mo",
		'ヤ': "ya", 'ユ': "yu", 'ヨ': "yo",
		'ラ': "ra", 'リ': "ri", 'ル': "ru", 'レ': "re", 'ロ': "ro",
		'ワ': "wa", 'ヲ': "wo", 'ン': "n",
		'ガ': "ga", 'ギ': "gi", 'グ': "gu", 'ゲ': "ge", 'ゴ': "go",
		'ザ': "za", 'ジ': "ji", 'ズ': "zu", 'ゼ': "ze", 'ゾ': "zo",
		'ダ': "da", 'ヂ': "ji", 'ヅ': "zu", 'デ': "de", 'ド': "do",
		'バ': "ba", 'ビ': "bi", 'ブ': "bu", 'ベ': "be", 'ボ': "bo",
		'パ': "pa", 'ピ': "pi", 'プ': "pu", 'ペ': "pe", 'ポ': "po",
	}

	// 合并所有映射
	for k, v := range hiragana {
		kanaMap[k] = v
	}
	for k, v := range katakana {
		kanaMap[k] = v
	}

	return kanaMap
}

// Process 处理日文文本（转罗马音）
// 参数:
//
//	text: 日文文本
//
// 返回:
//
//	string: 罗马音
//	error: 执行错误
func (p *JapaneseProcessor) Process(text string) (string, error) {
	var result []string

	// 首先尝试匹配日文汉字（查词典）
	remaining := text
	for len(remaining) > 0 {
		// 尝试匹配最长的汉字词
		matched := false
		for i := min(len(remaining), 10); i > 0; i-- {
			substr := remaining[:i]
			romaji := p.dictionary.GetRomaji(substr)
			if romaji != "" {
				result = append(result, romaji)
				remaining = remaining[i:]
				matched = true
				break
			}
		}

		if !matched {
			// 处理单个字符
			r := rune(remaining[0])
			remaining = remaining[1:]

			if isKana(r) {
				// 假名：直接转换
				if romaji, exists := p.kanaMap[r]; exists {
					result = append(result, romaji)
				} else {
					result = append(result, string(r))
				}
			} else if unicode.IsSpace(r) {
				// 空白字符
				result = append(result, " ")
			} else {
				// 其他字符（保留）
				result = append(result, string(r))
			}
		}
	}

	// 拼接结果
	output := strings.Join(result, "")

	// 应用特殊规则（促音双写、长音等）
	output = p.applySpecialRules(output)

	// 合并多余空格
	output = strings.Join(strings.Fields(output), " ")

	return strings.ToLower(output), nil
}

// applySpecialRules 应用特殊发音规则
func (p *JapaneseProcessor) applySpecialRules(romaji string) string {
	// 促音规则：在辅音前双写（如 gakkou）
	// 这里简化处理，实际应用中需要更复杂的逻辑

	// 长音规则：おう → oo, えい → ei
	// 这里简化处理

	return romaji
}

// GetLanguage 获取处理器支持的语言
func (p *JapaneseProcessor) GetLanguage() Language {
	return LanguageJapanese
}

// isKana 判断是否为假名字符
func isKana(r rune) bool {
	return isHiragana(r) || isKatakana(r)
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
