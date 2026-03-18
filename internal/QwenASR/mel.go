package asrinfer

import (
	"math"
	"math/cmplx"
)

// MelConfig holds Whisper-compatible Mel spectrogram parameters.
type MelConfig struct {
	SampleRate  int
	NFFt        int
	HopLength   int
	NumMelBins  int
	ChunkLength int // max audio seconds (30 for Whisper)
}

// DefaultMelConfig returns the Whisper / Qwen3-ASR default parameters.
func DefaultMelConfig() MelConfig {
	return MelConfig{
		SampleRate:  16000,
		NFFt:        400,
		HopLength:   160,
		NumMelBins:  128,
		ChunkLength: 30,
	}
}

// ComputeMelSpectrogram computes a log-Mel spectrogram from raw PCM float32 samples.
// Returns shape [numMelBins][T] where T = ceil(len(samples)/hopLength).
func ComputeMelSpectrogram(samples []float32, cfg MelConfig) [][]float32 {
	nFFT := cfg.NFFt
	hopLen := cfg.HopLength
	numMels := cfg.NumMelBins

	// Pad to center
	padLen := nFFT / 2
	padded := make([]float64, padLen+len(samples)+padLen)
	for i, s := range samples {
		padded[padLen+i] = float64(s)
	}

	// Number of frames
	numFrames := 1 + (len(padded)-nFFT)/hopLen

	// Hann window
	window := hannWindow(nFFT)

	// Mel filterbank
	melFB := melFilterbank(cfg.SampleRate, nFFT, numMels)

	// STFT -> Mel
	melSpec := make([][]float32, numMels)
	for i := range melSpec {
		melSpec[i] = make([]float32, numFrames)
	}

	freqBins := nFFT/2 + 1
	for frame := 0; frame < numFrames; frame++ {
		start := frame * hopLen
		// Windowed frame
		windowed := make([]float64, nFFT)
		for i := 0; i < nFFT; i++ {
			windowed[i] = padded[start+i] * window[i]
		}
		// FFT
		spectrum := rfft(windowed)
		// Power spectrum
		power := make([]float64, freqBins)
		for i := 0; i < freqBins; i++ {
			r := real(spectrum[i])
			im := imag(spectrum[i])
			power[i] = r*r + im*im
		}
		// Apply mel filterbank
		for m := 0; m < numMels; m++ {
			val := 0.0
			for f := 0; f < freqBins; f++ {
				val += melFB[m][f] * power[f]
			}
			if val < 1e-10 {
				val = 1e-10
			}
			melSpec[m][frame] = float32(math.Log10(val))
		}
	}

	// Normalize: Whisper-style clamp and scale
	maxVal := float32(-math.MaxFloat32)
	for m := 0; m < numMels; m++ {
		for t := 0; t < numFrames; t++ {
			if melSpec[m][t] > maxVal {
				maxVal = melSpec[m][t]
			}
		}
	}
	clampMin := maxVal - 8.0
	for m := 0; m < numMels; m++ {
		for t := 0; t < numFrames; t++ {
			if melSpec[m][t] < clampMin {
				melSpec[m][t] = clampMin
			}
			melSpec[m][t] = (melSpec[m][t] + 4.0) / 4.0
		}
	}

	return melSpec
}

func hannWindow(n int) []float64 {
	w := make([]float64, n)
	for i := 0; i < n; i++ {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n)))
	}
	return w
}

// nextPowerOf2 returns the smallest power of 2 >= n (e.g. 400 -> 512).
func nextPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// rfft computes the real FFT of a real-valued signal, returning n/2+1 complex values.
// Input is padded to the next power of 2 so that the radix-2 FFT does not index out of range.
func rfft(x []float64) []complex128 {
	n := len(x)
	n2 := nextPowerOf2(n)
	cx := make([]complex128, n2)
	for i := 0; i < n; i++ {
		cx[i] = complex(x[i], 0)
	}
	fft(cx)
	return cx[:n/2+1]
}

// fft computes an in-place radix-2 FFT (Cooley-Tukey).
func fft(a []complex128) {
	n := len(a)
	if n <= 1 {
		return
	}

	// Bit-reverse permutation
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		angle := -2.0 * math.Pi / float64(length)
		wn := cmplx.Exp(complex(0, angle))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for k := 0; k < length/2; k++ {
				u := a[i+k]
				v := w * a[i+k+length/2]
				a[i+k] = u + v
				a[i+k+length/2] = u - v
				w *= wn
			}
		}
	}
}

// melFilterbank creates a [numMels][nFFT/2+1] filterbank matrix.
func melFilterbank(sampleRate, nFFT, numMels int) [][]float64 {
	freqBins := nFFT/2 + 1
	fMax := float64(sampleRate) / 2.0

	// Mel scale boundaries
	melLow := hzToMel(0)
	melHigh := hzToMel(fMax)

	// numMels + 2 boundary points
	melPoints := make([]float64, numMels+2)
	for i := range melPoints {
		melPoints[i] = melLow + (melHigh-melLow)*float64(i)/float64(numMels+1)
	}

	hzPoints := make([]float64, numMels+2)
	for i := range hzPoints {
		hzPoints[i] = melToHz(melPoints[i])
	}

	binPoints := make([]float64, numMels+2)
	for i := range binPoints {
		binPoints[i] = hzPoints[i] * float64(nFFT) / float64(sampleRate)
	}

	fb := make([][]float64, numMels)
	for m := 0; m < numMels; m++ {
		fb[m] = make([]float64, freqBins)
		left := binPoints[m]
		center := binPoints[m+1]
		right := binPoints[m+2]
		for f := 0; f < freqBins; f++ {
			ff := float64(f)
			if ff >= left && ff <= center {
				if center-left > 0 {
					fb[m][f] = (ff - left) / (center - left)
				}
			} else if ff > center && ff <= right {
				if right-center > 0 {
					fb[m][f] = (right - ff) / (right - center)
				}
			}
		}
		// Slaney-style normalization
		enorm := 2.0 / (hzPoints[m+2] - hzPoints[m])
		for f := 0; f < freqBins; f++ {
			fb[m][f] *= enorm
		}
	}
	return fb
}

func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}
