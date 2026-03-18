package hubertfa

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Phoneme represents a single phoneme with time boundaries.
type Phoneme struct {
	Start float64
	End   float64
	Text  string
}

func NewPhoneme(start, end float64, text string) (Phoneme, error) {
	start = math.Max(0, start)
	if start >= end {
		return Phoneme{}, fmt.Errorf("phoneme invalid: text=%s start=%f end=%f", text, start, end)
	}
	return Phoneme{Start: start, End: end, Text: text}, nil
}

// Word represents a word composed of phonemes.
type Word struct {
	Start    float64
	End      float64
	Text     string
	Phonemes []Phoneme
}

func NewWord(start, end float64, text string, initPhoneme bool) (*Word, error) {
	start = math.Max(0, start)
	if start >= end {
		return nil, fmt.Errorf("word invalid: text=%s start=%f end=%f", text, start, end)
	}
	w := &Word{Start: start, End: end, Text: text}
	if initPhoneme {
		ph, err := NewPhoneme(start, end, text)
		if err != nil {
			return nil, err
		}
		w.Phonemes = append(w.Phonemes, ph)
	}
	return w, nil
}

func (w *Word) AddPhoneme(ph Phoneme, log *[]string) {
	if ph.Start == ph.End {
		msg := fmt.Sprintf("WARNING: %s phoneme length is 0", ph.Text)
		if log != nil {
			*log = append(*log, msg)
		}
		return
	}
	if ph.Start >= w.Start && ph.End <= w.End {
		w.Phonemes = append(w.Phonemes, ph)
	} else {
		msg := fmt.Sprintf("WARNING: %s: phoneme boundary exceeds word", ph.Text)
		if log != nil {
			*log = append(*log, msg)
		}
	}
}

func (w *Word) AppendPhoneme(ph Phoneme, log *[]string) {
	if ph.Start == ph.End {
		msg := fmt.Sprintf("WARNING: %s phoneme length is 0", ph.Text)
		if log != nil {
			*log = append(*log, msg)
		}
		return
	}
	if len(w.Phonemes) == 0 {
		if ph.Start == w.Start {
			w.Phonemes = append(w.Phonemes, ph)
			w.End = ph.End
		} else if log != nil {
			*log = append(*log, fmt.Sprintf("WARNING: %s: phoneme left boundary exceeds word", ph.Text))
		}
	} else {
		last := w.Phonemes[len(w.Phonemes)-1]
		if ph.Start == last.End {
			w.Phonemes = append(w.Phonemes, ph)
			w.End = ph.End
		} else if log != nil {
			*log = append(*log, fmt.Sprintf("WARNING: %s: phoneme append failed", ph.Text))
		}
	}
}

func (w *Word) MoveStart(newStart float64, log *[]string) {
	if len(w.Phonemes) > 0 && newStart >= 0 && newStart < w.Phonemes[0].End {
		w.Start = newStart
		w.Phonemes[0].Start = newStart
	} else if log != nil {
		*log = append(*log, fmt.Sprintf("WARNING: %s: cannot adjust word start", w.Text))
	}
}

func (w *Word) MoveEnd(newEnd float64, log *[]string) {
	if len(w.Phonemes) > 0 && newEnd > w.Phonemes[len(w.Phonemes)-1].Start {
		w.End = newEnd
		w.Phonemes[len(w.Phonemes)-1].End = newEnd
	} else if log != nil {
		*log = append(*log, fmt.Sprintf("WARNING: %s: cannot adjust word end", w.Text))
	}
}

// WordList is a list of Words with validation and manipulation.
type WordList struct {
	Words []*Word
	Log   []string
}

func NewWordList() *WordList {
	return &WordList{}
}

func (wl *WordList) Len() int { return len(wl.Words) }

func (wl *WordList) Get(i int) *Word { return wl.Words[i] }

func (wl *WordList) PhonemeTexts() []string {
	var result []string
	for _, w := range wl.Words {
		for _, ph := range w.Phonemes {
			result = append(result, ph.Text)
		}
	}
	return result
}

func (wl *WordList) Intervals() [][2]float64 {
	result := make([][2]float64, len(wl.Words))
	for i, w := range wl.Words {
		result[i] = [2]float64{w.Start, w.End}
	}
	return result
}

func (wl *WordList) overlappingWords(nw *Word) []*Word {
	var result []*Word
	for _, w := range wl.Words {
		if !(nw.End <= w.Start || nw.Start >= w.End) {
			result = append(result, w)
		}
	}
	return result
}

func (wl *WordList) Append(w *Word) {
	if len(w.Phonemes) == 0 {
		wl.Log = append(wl.Log, fmt.Sprintf("WARNING: %s: empty phonemes, invalid word", w.Text))
		return
	}
	if len(wl.Words) == 0 {
		wl.Words = append(wl.Words, w)
		return
	}
	if len(wl.overlappingWords(w)) == 0 {
		wl.Words = append(wl.Words, w)
	} else {
		wl.Log = append(wl.Log, fmt.Sprintf("WARNING: %s: interval overlap, cannot add word", w.Text))
	}
}

func removeOverlapping(raw, remove [2]float64) [][2]float64 {
	rStart, rEnd := raw[0], raw[1]
	mStart, mEnd := remove[0], remove[1]
	overlapStart := math.Max(rStart, mStart)
	overlapEnd := math.Min(rEnd, mEnd)
	if overlapStart >= overlapEnd {
		return [][2]float64{raw}
	}
	var result [][2]float64
	if rStart < overlapStart {
		result = append(result, [2]float64{rStart, overlapStart})
	}
	if overlapEnd < rEnd {
		result = append(result, [2]float64{overlapEnd, rEnd})
	}
	return result
}

func (wl *WordList) AddAP(newWord *Word, minDur float64) {
	if len(newWord.Phonemes) == 0 {
		wl.Log = append(wl.Log, fmt.Sprintf("WARNING: %s phonemes empty", newWord.Text))
		return
	}
	if len(wl.Words) == 0 {
		wl.Append(newWord)
		return
	}
	overlapping := wl.overlappingWords(newWord)
	if len(overlapping) == 0 {
		wl.Append(newWord)
		wl.sortByStart()
		return
	}
	apIntervals := [][2]float64{{newWord.Start, newWord.End}}
	for _, w := range wl.Words {
		var temp [][2]float64
		for _, ap := range apIntervals {
			temp = append(temp, removeOverlapping(ap, [2]float64{w.Start, w.End})...)
		}
		apIntervals = temp
	}
	for _, ap := range apIntervals {
		if ap[1]-ap[0] >= minDur {
			w, err := NewWord(ap[0], ap[1], newWord.Text, true)
			if err != nil {
				wl.Log = append(wl.Log, fmt.Sprintf("ERROR: %v", err))
				continue
			}
			wl.Append(w)
		}
	}
	wl.sortByStart()
}

func (wl *WordList) sortByStart() {
	sort.Slice(wl.Words, func(i, j int) bool {
		return wl.Words[i].Start < wl.Words[j].Start
	})
}

func (wl *WordList) FillSmallGaps(wavLength float64, gapLength float64) {
	if len(wl.Words) == 0 {
		return
	}
	if wl.Words[0].Start < 0 {
		wl.Words[0].Start = 0
	}
	first := wl.Words[0]
	if first.Start > 0 && math.Abs(first.Start) < gapLength && gapLength < (first.End-first.Start) {
		first.MoveStart(0, &wl.Log)
	}
	last := wl.Words[len(wl.Words)-1]
	if last.End >= wavLength-gapLength {
		last.MoveEnd(wavLength, &wl.Log)
	}
	for i := 1; i < len(wl.Words); i++ {
		gap := wl.Words[i].Start - wl.Words[i-1].End
		if gap > 0 && gap <= gapLength {
			wl.Words[i-1].MoveEnd(wl.Words[i].Start, &wl.Log)
		}
	}
}

func (wl *WordList) AddSP(wavLength float64) {
	addPhone := "SP"
	result := NewWordList()
	result.Log = wl.Log

	if wl.Words[0].Start > 0 {
		w, err := NewWord(0, wl.Words[0].Start, addPhone, true)
		if err == nil {
			result.Append(w)
		} else {
			wl.Log = append(wl.Log, fmt.Sprintf("ERROR: %v", err))
		}
	}
	result.Append(wl.Words[0])
	for i := 1; i < len(wl.Words); i++ {
		word := wl.Words[i]
		lastEnd := result.Words[len(result.Words)-1].End
		if word.Start > lastEnd {
			w, err := NewWord(lastEnd, word.Start, addPhone, true)
			if err == nil {
				result.Append(w)
			} else {
				wl.Log = append(wl.Log, fmt.Sprintf("ERROR: %v", err))
			}
		}
		result.Append(word)
	}
	lastWord := wl.Words[len(wl.Words)-1]
	if lastWord.End < wavLength {
		w, err := NewWord(lastWord.End, wavLength, addPhone, true)
		if err == nil {
			result.Append(w)
		} else {
			wl.Log = append(wl.Log, fmt.Sprintf("ERROR: %v", err))
		}
	}
	wl.Words = result.Words
	wl.Log = result.Log
}

func (wl *WordList) ClearLanguagePrefix() {
	for _, w := range wl.Words {
		for i := range w.Phonemes {
			parts := strings.SplitN(w.Phonemes[i].Text, "/", 2)
			if len(parts) == 2 {
				w.Phonemes[i].Text = parts[1]
			}
		}
	}
}

func (wl *WordList) LogString() string {
	return strings.Join(wl.Log, "\n")
}
