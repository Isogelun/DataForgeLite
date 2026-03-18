package hubertfa

import (
	"fmt"
	"math"
	"os"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// LoadWAV reads a WAV file and returns mono float32 samples normalised to [-1, 1]
// along with the original sample rate.
func LoadWAV(path string) ([]float32, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open wav: %w", err)
	}
	defer f.Close()

	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, 0, fmt.Errorf("invalid wav file: %s", path)
	}

	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, fmt.Errorf("decode wav: %w", err)
	}

	sr := int(dec.SampleRate)
	nCh := int(dec.NumChans)
	bitDepth := int(dec.BitDepth)
	samples := buf.Data

	// Mix to mono
	mono := mixToMono(samples, nCh)

	// Normalise to [-1, 1]
	scale := float32(1.0 / math.Pow(2, float64(bitDepth-1)))
	out := make([]float32, len(mono))
	for i, s := range mono {
		out[i] = float32(s) * scale
	}
	_ = audio.Format{} // keep import
	return out, sr, nil
}

func mixToMono(samples []int, nCh int) []int {
	if nCh == 1 {
		return samples
	}
	n := len(samples) / nCh
	mono := make([]int, n)
	for i := 0; i < n; i++ {
		sum := 0
		for c := 0; c < nCh; c++ {
			sum += samples[i*nCh+c]
		}
		mono[i] = sum / nCh
	}
	return mono
}

// Resample resamples audio from srcRate to dstRate using linear interpolation.
// For FA inference the quality is adequate since the ONNX model is robust.
func Resample(samples []float32, srcRate, dstRate int) []float32 {
	if srcRate == dstRate {
		return samples
	}
	ratio := float64(srcRate) / float64(dstRate)
	outLen := int(float64(len(samples)) / ratio)
	out := make([]float32, outLen)
	for i := 0; i < outLen; i++ {
		srcIdx := float64(i) * ratio
		idx := int(srcIdx)
		frac := float32(srcIdx - float64(idx))
		if idx+1 < len(samples) {
			out[i] = samples[idx]*(1-frac) + samples[idx+1]*frac
		} else if idx < len(samples) {
			out[i] = samples[idx]
		}
	}
	return out
}

// LoadAndResample loads a WAV file and resamples to the target sample rate.
func LoadAndResample(path string, targetSR int) ([]float32, error) {
	samples, sr, err := LoadWAV(path)
	if err != nil {
		return nil, err
	}
	return Resample(samples, sr, targetSR), nil
}

// PadWav prepends padSamples zeros to wav data.
func PadWav(wav []float32, padSamples int) []float32 {
	if padSamples <= 0 {
		return wav
	}
	out := make([]float32, padSamples+len(wav))
	copy(out[padSamples:], wav)
	return out
}
