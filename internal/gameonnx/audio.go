package gameonnx

import (
	"fmt"
	"os"

	"github.com/go-audio/wav"
)

func readWavMonoFloat32(path string) (samples []float32, sampleRate int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, 0, fmt.Errorf("invalid wav file")
	}
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}

	sampleRate = buf.Format.SampleRate
	ch := buf.Format.NumChannels
	if ch <= 0 {
		return nil, 0, fmt.Errorf("invalid channels=%d", ch)
	}

	maxInt := float32(float64(int64(1)<<(uint(buf.SourceBitDepth)-1)) - 1)
	if maxInt <= 0 {
		maxInt = 32767
	}

	nFrames := len(buf.Data) / ch
	out := make([]float32, nFrames)
	for i := 0; i < nFrames; i++ {
		var sum float32
		for c := 0; c < ch; c++ {
			sum += float32(buf.Data[i*ch+c]) / maxInt
		}
		out[i] = sum / float32(ch)
	}
	return out, sampleRate, nil
}
