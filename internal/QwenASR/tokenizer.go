package asrinfer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Special token IDs for Qwen3-ASR
const (
	AudioStartTokenID = 151669
	AudioEndTokenID   = 151670
	AudioPadTokenID   = 151676
	ImStartTokenID    = 151644
	ImEndTokenID      = 151645
	EndOfTextTokenID  = 151643
	ASRTextTokenID    = 151704
)

// EOS token IDs
var EOSTokenIDs = []int{EndOfTextTokenID, ImEndTokenID}

// BPETokenizer implements a byte-pair encoding tokenizer (decode only).
// For ASR we only need decode: token ID -> text.
type BPETokenizer struct {
	IdToToken map[int]string
	TokenToId map[string]int
}

// LoadTokenizer loads vocab.json from the model directory.
func LoadTokenizer(modelDir string) (*BPETokenizer, error) {
	vocabPath := filepath.Join(modelDir, "vocab.json")
	data, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("read vocab.json: %w", err)
	}
	var vocab map[string]int
	if err := json.Unmarshal(data, &vocab); err != nil {
		return nil, fmt.Errorf("parse vocab.json: %w", err)
	}

	idToToken := make(map[int]string, len(vocab))
	for token, id := range vocab {
		idToToken[id] = token
	}

	return &BPETokenizer{
		IdToToken: idToToken,
		TokenToId: vocab,
	}, nil
}

// Decode converts a sequence of token IDs to text.
func (t *BPETokenizer) Decode(ids []int) string {
	var rawBytes []byte
	for _, id := range ids {
		if isSpecialTokenID(id) {
			continue
		}
		token, ok := t.IdToToken[id]
		if !ok {
			continue
		}
		for _, r := range token {
			if b, ok := gpt2UnicodeToByte[r]; ok {
				rawBytes = append(rawBytes, b)
			}
			// unknown runes are skipped
		}
	}
	return strings.TrimSpace(string(rawBytes))
}

// gpt2UnicodeToByte is the reverse of GPT2's bytes_to_unicode() mapping.
// It maps each Unicode character used in BPE vocab back to its original byte value.
var gpt2UnicodeToByte = func() map[rune]byte {
	// Build bytes_to_unicode: same logic as Python's GPT2 tokenizer
	bs := make([]int, 0, 256)
	for b := int('!'); b <= int('~'); b++ {
		bs = append(bs, b)
	}
	for b := 0x00A1; b <= 0x00AC; b++ {
		bs = append(bs, b)
	}
	for b := 0x00AE; b <= 0x00FF; b++ {
		bs = append(bs, b)
	}
	cs := make([]int, len(bs))
	copy(cs, bs)
	n := 0
	for b := 0; b < 256; b++ {
		found := false
		for _, x := range bs {
			if x == b {
				found = true
				break
			}
		}
		if !found {
			bs = append(bs, b)
			cs = append(cs, 256+n)
			n++
		}
	}
	m := make(map[rune]byte, 256)
	for i, b := range bs {
		m[rune(cs[i])] = byte(b)
	}
	return m
}()

// DecodeSkipSpecial decodes only non-special tokens, returning just the text content.
func (t *BPETokenizer) DecodeSkipSpecial(ids []int) string {
	var filtered []int
	for _, id := range ids {
		if !isSpecialTokenID(id) {
			filtered = append(filtered, id)
		}
	}
	return t.Decode(filtered)
}

// IsEOS checks if a token ID is an end-of-sequence token.
func IsEOS(id int) bool {
	for _, eos := range EOSTokenIDs {
		if id == eos {
			return true
		}
	}
	return false
}

func isSpecialTokenID(id int) bool {
	return id >= 151643
}

func decodeByteLevelTokens(s string) string {
	// Replace byte-level tokens like <0xE4> with actual bytes
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+6 <= len(s) && s[i:i+3] == "<0x" && s[i+5] == '>' {
			hexStr := s[i+3 : i+5]
			var b byte
			if _, err := fmt.Sscanf(hexStr, "%02X", &b); err == nil {
				result.WriteByte(b)
				i += 6
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// BuildPromptTokens constructs the initial prompt for ASR inference.
// Format: <|im_start|>system\nYou are a helpful assistant.<|im_end|>\n<|im_start|>user\n<|audio_start|><|audio_pad|>...<|audio_end|><|im_end|>\n<|im_start|>assistant\n<|asr_text|>
func BuildPromptTokens(t *BPETokenizer, audioLen int) []int {
	var ids []int

	// <|im_start|>system\nYou are a helpful assistant.<|im_end|>\n
	ids = append(ids, ImStartTokenID)
	ids = append(ids, encodeText(t, "system\nYou are a helpful assistant.")...)
	ids = append(ids, ImEndTokenID)
	ids = append(ids, encodeText(t, "\n")...)

	// <|im_start|>user\n<|audio_start|>
	ids = append(ids, ImStartTokenID)
	ids = append(ids, encodeText(t, "user\n")...)
	ids = append(ids, AudioStartTokenID)

	// <|audio_pad|> repeated audioLen times
	for i := 0; i < audioLen; i++ {
		ids = append(ids, AudioPadTokenID)
	}

	// <|audio_end|><|im_end|>\n
	ids = append(ids, AudioEndTokenID)
	ids = append(ids, ImEndTokenID)
	ids = append(ids, encodeText(t, "\n")...)

	// <|im_start|>assistant\n<|asr_text|>
	ids = append(ids, ImStartTokenID)
	ids = append(ids, encodeText(t, "assistant\n")...)
	ids = append(ids, ASRTextTokenID)

	return ids
}

// encodeText does a simple character-level encoding using the vocabulary.
// For the prompt template we only need basic ASCII characters.
func encodeText(t *BPETokenizer, text string) []int {
	var ids []int
	for _, ch := range text {
		s := string(ch)
		if id, ok := t.TokenToId[s]; ok {
			ids = append(ids, id)
		} else {
			// Try with Ġ prefix for space
			if ch == ' ' {
				if id, ok := t.TokenToId["Ġ"]; ok {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}
