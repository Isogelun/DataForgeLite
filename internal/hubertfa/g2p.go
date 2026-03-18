package hubertfa

import (
	"fmt"
	"os"
	"strings"
)

// G2PResult holds the output of a G2P conversion.
type G2PResult struct {
	PhSeq           []string // phoneme sequence with language prefix
	WordSeq         []string // word sequence
	PhIdxToWordIdx  []int    // maps each phoneme index to its word index (-1 for SP)
}

// G2P is the interface for grapheme-to-phoneme conversion.
type G2P interface {
	Convert(text string) (*G2PResult, error)
}

// PhonemeG2P treats each space-separated token as a phoneme.
type PhonemeG2P struct {
	Language string // empty string means no prefix
}

func NewPhonemeG2P(language string) *PhonemeG2P {
	return &PhonemeG2P{Language: language}
}

func (g *PhonemeG2P) Convert(text string) (*G2PResult, error) {
	tokens := strings.Fields(text)
	var wordSeq []string
	for _, t := range tokens {
		if t != "SP" {
			wordSeq = append(wordSeq, t)
		}
	}

	phSeq := []string{"SP"}
	phIdx := []int{-1}
	wordIdx := 0
	for _, word := range wordSeq {
		phSeq = append(phSeq, word)
		phIdx = append(phIdx, wordIdx)
		phSeq = append(phSeq, "SP")
		phIdx = append(phIdx, -1)
		wordIdx++
	}

	addLanguagePrefix(phSeq, g.Language)
	return &G2PResult{PhSeq: phSeq, WordSeq: wordSeq, PhIdxToWordIdx: phIdx}, nil
}

// DictionaryG2P uses a tab-separated dictionary file.
type DictionaryG2P struct {
	Language   string
	Dictionary map[string][]string
}

func NewDictionaryG2P(language, dictPath string) (*DictionaryG2P, error) {
	data, err := os.ReadFile(dictPath)
	if err != nil {
		return nil, fmt.Errorf("read dictionary: %w", err)
	}
	dict := make(map[string][]string)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		word := strings.TrimSpace(parts[0])
		phones := strings.Fields(strings.TrimSpace(parts[1]))
		dict[word] = phones
	}
	return &DictionaryG2P{Language: language, Dictionary: dict}, nil
}

func (g *DictionaryG2P) Convert(text string) (*G2PResult, error) {
	rawWords := strings.Fields(text)
	var wordSeq []string
	phSeq := []string{"SP"}
	phIdx := []int{-1}
	wordSeqIdx := 0

	for _, word := range rawWords {
		phones, ok := g.Dictionary[word]
		if !ok {
			// skip unknown words
			continue
		}
		wordSeq = append(wordSeq, word)
		for i, ph := range phones {
			if (i == 0 || i == len(phones)-1) && ph == "SP" {
				continue
			}
			phSeq = append(phSeq, ph)
			phIdx = append(phIdx, wordSeqIdx)
		}
		if phSeq[len(phSeq)-1] != "SP" {
			phSeq = append(phSeq, "SP")
			phIdx = append(phIdx, -1)
		}
		wordSeqIdx++
	}

	addLanguagePrefix(phSeq, g.Language)
	return &G2PResult{PhSeq: phSeq, WordSeq: wordSeq, PhIdxToWordIdx: phIdx}, nil
}

// NewG2P creates a G2P instance by name.
func NewG2P(g2pType, language, dictPath string, vocab *VocabConfig, modelDir string) (G2P, error) {
	switch g2pType {
	case "dictionary":
		dp := dictPath
		if dp == "" {
			var err error
			dp, err = DictionaryPath(vocab, modelDir, language)
			if err != nil {
				return nil, err
			}
		}
		return NewDictionaryG2P(language, dp)
	case "phoneme":
		return NewPhonemeG2P(language), nil
	default:
		return nil, fmt.Errorf("unsupported g2p type: %s", g2pType)
	}
}

func addLanguagePrefix(phSeq []string, language string) {
	if language == "" {
		return
	}
	for i, ph := range phSeq {
		if ph != "SP" {
			phSeq[i] = language + "/" + ph
		}
	}
}
