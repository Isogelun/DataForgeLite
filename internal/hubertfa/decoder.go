package hubertfa

import (
	"math"
)

// ---- math helpers ----

func sigmoid(x float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(x))))
}

func softmaxCols(mat [][]float32) {
	if len(mat) == 0 || len(mat[0]) == 0 {
		return
	}
	T := len(mat[0])
	V := len(mat)
	for t := 0; t < T; t++ {
		mx := float32(-math.MaxFloat32)
		for v := 0; v < V; v++ {
			if mat[v][t] > mx {
				mx = mat[v][t]
			}
		}
		sum := float32(0)
		for v := 0; v < V; v++ {
			mat[v][t] = float32(math.Exp(float64(mat[v][t] - mx)))
			sum += mat[v][t]
		}
		for v := 0; v < V; v++ {
			mat[v][t] /= sum
		}
	}
}

func logSoftmaxCols(mat [][]float32) [][]float32 {
	V := len(mat)
	T := len(mat[0])
	out := make([][]float32, V)
	for v := 0; v < V; v++ {
		out[v] = make([]float32, T)
	}
	for t := 0; t < T; t++ {
		mx := float32(-math.MaxFloat32)
		for v := 0; v < V; v++ {
			if mat[v][t] > mx {
				mx = mat[v][t]
			}
		}
		logSum := float32(0)
		for v := 0; v < V; v++ {
			logSum += float32(math.Exp(float64(mat[v][t] - mx)))
		}
		logSum = float32(math.Log(float64(logSum)))
		for v := 0; v < V; v++ {
			out[v][t] = mat[v][t] - mx - logSum
		}
	}
	return out
}

func clipF32(x, lo, hi float32) float32 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// ---- AlignmentDecoder ----

type AlignmentDecoder struct {
	Vocab       *VocabConfig
	SampleRate  int
	HopSize     int
	FrameLength float64
}

func NewAlignmentDecoder(vocab *VocabConfig, sampleRate, hopSize int) *AlignmentDecoder {
	return &AlignmentDecoder{
		Vocab:       vocab,
		SampleRate:  sampleRate,
		HopSize:     hopSize,
		FrameLength: float64(hopSize) / float64(sampleRate),
	}
}

// DecodeResult holds alignment decoder output.
type DecodeResult struct {
	Words      *WordList
	Confidence float64
}

// Decode runs the forced-alignment Viterbi decoder.
// phFrameLogits: [vocabSize][T], phEdgeLogits: [T]
func (d *AlignmentDecoder) Decode(
	phFrameLogits [][]float32,
	phEdgeLogits []float32,
	wavLength float64,
	phSeq []string,
	wordSeq []string,
	phIdxToWordIdx []int,
) *DecodeResult {
	phSeqID := make([]int, len(phSeq))
	for i, ph := range phSeq {
		phSeqID[i] = d.Vocab.Vocab[ph]
	}

	if wordSeq == nil {
		wordSeq = phSeq
		phIdxToWordIdx = make([]int, len(phSeq))
		for i := range phIdxToWordIdx {
			phIdxToWordIdx[i] = i
		}
	}

	numFrames := int(float64(wavLength)*float64(d.SampleRate)+0.5) / d.HopSize
	vocabSize := len(phFrameLogits)
	if numFrames > len(phFrameLogits[0]) {
		numFrames = len(phFrameLogits[0])
	}

	// Truncate to numFrames
	truncLogits := make([][]float32, vocabSize)
	for v := 0; v < vocabSize; v++ {
		truncLogits[v] = phFrameLogits[v][:numFrames]
	}
	truncEdge := phEdgeLogits[:numFrames]

	// Build mask
	phMask := make([]float32, vocabSize)
	for i := range phMask {
		phMask[i] = 1e9
	}
	phMask[0] = 0
	for _, id := range phSeqID {
		phMask[id] = 0
	}

	// Adjusted logits
	adjusted := make([][]float32, vocabSize)
	for v := 0; v < vocabSize; v++ {
		adjusted[v] = make([]float32, numFrames)
		for t := 0; t < numFrames; t++ {
			adjusted[v][t] = truncLogits[v][t] - phMask[v]
		}
	}

	// Softmax and logSoftmax on adjusted
	phProbLog := logSoftmaxCols(adjusted)
	softmaxCols(adjusted)
	phFramePred := adjusted

	// Edge prediction
	edgePred := make([]float32, numFrames)
	for t := range edgePred {
		edgePred[t] = clipF32(sigmoid(truncEdge[t]), 0, 1)
	}

	T := numFrames
	S := len(phSeqID)

	// probLog for sequence: [S][T]
	probLog := make([][]float32, S)
	for s := 0; s < S; s++ {
		probLog[s] = phProbLog[phSeqID[s]]
	}

	// edgeDiff
	edgeDiff := make([]float32, T)
	for t := 0; t < T-1; t++ {
		edgeDiff[t] = edgePred[t+1] - edgePred[t]
	}
	edgeDiff[T-1] = 0

	// edgeProb (smoothed)
	edgeProb := make([]float32, T)
	edgeProb[0] = clipF32(edgePred[0], 0, 1)
	for t := 1; t < T; t++ {
		edgeProb[t] = clipF32(edgePred[t]+edgePred[t-1], 0, 1)
	}

	// Forward pass
	phIdxSeq, phTimeInt, frameConfidence := d.decode(phSeqID, probLog, edgeProb, S, T)

	totalConfidence := 0.0
	for _, c := range frameConfidence {
		totalConfidence += math.Log(float64(c) + 1e-6)
	}
	if len(frameConfidence) > 0 {
		totalConfidence = math.Exp(totalConfidence / float64(len(frameConfidence)) / 3)
	}

	// Postprocess: fractional time adjustments
	phTimeFrac := make([]float64, len(phTimeInt))
	for i, t := range phTimeInt {
		f := float64(edgeDiff[t]) / 2
		if f < -0.5 {
			f = -0.5
		}
		if f > 0.5 {
			f = 0.5
		}
		phTimeFrac[i] = f
	}

	// phTimePred: start of each phoneme + end sentinel
	phTimePred := make([]float64, len(phTimeInt)+1)
	for i, t := range phTimeInt {
		phTimePred[i] = d.FrameLength * (float64(t) + phTimeFrac[i])
		if phTimePred[i] < 0 {
			phTimePred[i] = 0
		}
	}
	phTimePred[len(phTimeInt)] = d.FrameLength * float64(T)

	// Build intervals [start, end]
	phIntervals := make([][2]float64, len(phTimeInt))
	for i := range phTimeInt {
		phIntervals[i] = [2]float64{phTimePred[i], phTimePred[i+1]}
	}

	// Build word list
	words := NewWordList()
	var currentWord *Word
	wordIdxLast := -1

	for i, phIdx := range phIdxSeq {
		phText := phSeq[phIdx]
		if phText == "SP" {
			continue
		}
		ph, err := NewPhoneme(phIntervals[i][0], phIntervals[i][1], phText)
		if err != nil {
			continue
		}
		wIdx := phIdxToWordIdx[phIdx]
		if wIdx == wordIdxLast && currentWord != nil {
			currentWord.AppendPhoneme(ph, &words.Log)
		} else {
			currentWord, err = NewWord(phIntervals[i][0], phIntervals[i][1], wordSeq[wIdx], false)
			if err != nil {
				continue
			}
			currentWord.AddPhoneme(ph, &words.Log)
			words.Append(currentWord)
			wordIdxLast = wIdx
		}
	}

	_ = phFramePred // used by softmaxCols above
	return &DecodeResult{Words: words, Confidence: totalConfidence}
}

func (d *AlignmentDecoder) decode(
	phSeqID []int,
	probLog [][]float32, // [S][T]
	edgeProb []float32, // [T]
	S, T int,
) (phIdxSeq []int, phTimeInt []int, frameConfidence []float32) {
	const negInf = float32(-1e30)

	// dp[s][t]
	dp := make([][]float32, S)
	backtrack := make([][]int8, S)
	for s := 0; s < S; s++ {
		dp[s] = make([]float32, T)
		backtrack[s] = make([]int8, T)
		for t := 0; t < T; t++ {
			dp[s][t] = negInf
			backtrack[s][t] = -1
		}
	}

	currPhMaxProbLog := make([]float32, S)
	for i := range currPhMaxProbLog {
		currPhMaxProbLog[i] = negInf
	}

	// Init
	dp[0][0] = probLog[0][0]
	currPhMaxProbLog[0] = probLog[0][0]
	if phSeqID[0] == 0 && S > 1 {
		dp[1][0] = probLog[1][0]
		currPhMaxProbLog[1] = probLog[1][0]
	}

	edgeProbLog := make([]float32, T)
	notEdgeProbLog := make([]float32, T)
	for t := 0; t < T; t++ {
		edgeProbLog[t] = float32(math.Log(float64(edgeProb[t]) + 1e-6))
		notEdgeProbLog[t] = float32(math.Log(float64(1-edgeProb[t]) + 1e-6))
	}

	prob3PadLen := 2
	if S < 2 {
		prob3PadLen = 1
	}

	// Forward
	for t := 1; t < T; t++ {
		for s := S - 1; s >= 0; s-- {
			// Type 1: stay
			prob1 := dp[s][t-1] + probLog[s][t] + notEdgeProbLog[t]

			// Type 2: move to next phoneme
			prob2 := negInf
			if s > 0 {
				prob2 = dp[s-1][t-1] + probLog[s-1][t] + edgeProbLog[t] + currPhMaxProbLog[s-1]*float32(T)/float32(S)
			}

			// Type 3: skip
			prob3 := negInf
			if s >= prob3PadLen {
				srcS := s - prob3PadLen
				if srcS < S-1 && phSeqID[srcS] != 0 {
					prob3 = negInf
				} else {
					prob3 = dp[srcS][t-1] + probLog[srcS][t] + edgeProbLog[t] + currPhMaxProbLog[srcS]*float32(T)/float32(S)
				}
			}

			best := prob1
			bestType := int8(0)
			if prob2 > best {
				best = prob2
				bestType = 1
			}
			if prob3 > best {
				best = prob3
				bestType = 2
			}

			dp[s][t] = best
			backtrack[s][t] = bestType

			if bestType == 0 {
				if probLog[s][t] > currPhMaxProbLog[s] {
					currPhMaxProbLog[s] = probLog[s][t]
				}
			} else {
				currPhMaxProbLog[s] = probLog[s][t]
			}
		}
		// Reset silent phonemes
		for s := 0; s < S; s++ {
			if phSeqID[s] == 0 {
				currPhMaxProbLog[s] = 0
			}
		}
	}

	// Backward
	s := S - 1
	if S > 1 && phSeqID[S-1] == 0 && dp[S-2][T-1] > dp[S-1][T-1] {
		s = S - 2
	}

	for t := T - 1; t >= 0; t-- {
		frameConfidence = append(frameConfidence, dp[s][t])
		if backtrack[s][t] != 0 || t == 0 {
			phIdxSeq = append(phIdxSeq, s)
			phTimeInt = append(phTimeInt, t)
			switch backtrack[s][t] {
			case 1:
				s--
			case 2:
				s -= prob3PadLen
			}
		}
	}

	// Reverse
	reverseInts(phIdxSeq)
	reverseInts(phTimeInt)
	reverseF32(frameConfidence)

	// Compute frame confidence as exp of diffs
	if len(frameConfidence) > 0 {
		diffs := make([]float32, len(frameConfidence))
		diffs[0] = float32(math.Exp(float64(frameConfidence[0])))
		for i := 1; i < len(frameConfidence); i++ {
			diffs[i] = float32(math.Exp(float64(frameConfidence[i] - frameConfidence[i-1])))
		}
		frameConfidence = diffs
	}

	return
}

func reverseInts(a []int) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func reverseF32(a []float32) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

// ---- NonLexicalDecoder ----

type NonLexicalDecoder struct {
	Vocab       *VocabConfig
	ClassNames  []string // e.g. ["None", "AP", "EP"]
	SampleRate  int
	HopSize     int
	FrameLength float64
}

func NewNonLexicalDecoder(vocab *VocabConfig, sampleRate, hopSize int) *NonLexicalDecoder {
	classNames := []string{"None"}
	classNames = append(classNames, vocab.NonLexicalPhonemes...)
	return &NonLexicalDecoder{
		Vocab:       vocab,
		ClassNames:  classNames,
		SampleRate:  sampleRate,
		HopSize:     hopSize,
		FrameLength: float64(hopSize) / float64(sampleRate),
	}
}

// Decode processes cvnt_logits and returns non-lexical word lists per phoneme type.
// cvntLogits: [classes][T]
func (d *NonLexicalDecoder) Decode(
	cvntLogits [][]float32,
	wavLength float64,
	nonLexicalPhonemes []string,
) []*WordList {
	numFrames := len(cvntLogits[0])
	if wavLength > 0 {
		nf := int(float64(wavLength)*float64(d.SampleRate)+0.5) / d.HopSize
		if nf < numFrames {
			numFrames = nf
		}
	}

	// Softmax along class axis for each frame
	classes := len(cvntLogits)
	probs := make([][]float32, classes)
	for c := 0; c < classes; c++ {
		probs[c] = make([]float32, numFrames)
	}
	for t := 0; t < numFrames; t++ {
		mx := float32(-math.MaxFloat32)
		for c := 0; c < classes; c++ {
			if cvntLogits[c][t] > mx {
				mx = cvntLogits[c][t]
			}
		}
		sum := float32(0)
		for c := 0; c < classes; c++ {
			probs[c][t] = float32(math.Exp(float64(cvntLogits[c][t] - mx)))
			sum += probs[c][t]
		}
		for c := 0; c < classes; c++ {
			probs[c][t] /= sum
		}
	}

	var results []*WordList
	for _, ph := range nonLexicalPhonemes {
		idx := -1
		for i, cn := range d.ClassNames {
			if cn == ph {
				idx = i
				break
			}
		}
		if idx < 0 {
			results = append(results, NewWordList())
			continue
		}
		words := d.detectNonLexical(probs[idx][:numFrames], ph)
		results = append(results, words)
	}
	return results
}

func (d *NonLexicalDecoder) detectNonLexical(prob []float32, tag string) *WordList {
	const threshold = float32(0.5)
	const maxGap = 5
	const minFrames = 10

	wl := NewWordList()
	start := -1
	gapCount := 0

	for i := 0; i < len(prob); i++ {
		if prob[i] >= threshold {
			if start < 0 {
				start = i
			}
			gapCount = 0
		} else if start >= 0 {
			if gapCount < maxGap {
				gapCount++
			} else {
				end := i - gapCount - 1
				if end > start && (end-start) >= minFrames {
					w, err := NewWord(float64(start)*d.FrameLength, float64(end)*d.FrameLength, tag, true)
					if err == nil {
						wl.Words = append(wl.Words, w)
					}
				}
				start = -1
				gapCount = 0
			}
		}
	}
	if start >= 0 && (len(prob)-start) >= minFrames {
		w, err := NewWord(float64(start)*d.FrameLength, float64(len(prob)-1)*d.FrameLength, tag, true)
		if err == nil {
			wl.Words = append(wl.Words, w)
		}
	}
	return wl
}
