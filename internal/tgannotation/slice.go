package tgannotation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SliceConfig struct {
	InDir                string // 输入父目录，下面含 wavs/ 和 TextGrid/
	OutDir               string // 输出父目录，下面自动创建 wavs/ 和 TextGrid/
	PreserveSentenceNames bool
	Digits               int
	WavSubtype           string
	Overwrite            bool
}

func Slice(cfg SliceConfig) error {
	if cfg.Digits == 0 {
		cfg.Digits = 3
	}
	if cfg.WavSubtype == "" {
		cfg.WavSubtype = "PCM_16"
	}
	wavsInDir := filepath.Join(cfg.InDir, "wavs")
	tgInDir := filepath.Join(cfg.InDir, "TextGrid")
	wavsOutDir := filepath.Join(cfg.OutDir, "wavs")
	tgOutDir := filepath.Join(cfg.OutDir, "TextGrid")
	if err := os.MkdirAll(wavsOutDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(tgOutDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(tgInDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".TextGrid") {
			continue
		}
		tgPath := filepath.Join(tgInDir, e.Name())
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		wavPath := filepath.Join(wavsInDir, stem+".wav")

		tg, err := ReadTextGrid(tgPath)
		if err != nil {
			return fmt.Errorf("读取 TextGrid 失败 %s: %w", tgPath, err)
		}
		if len(tg.Tiers) < 3 {
			return fmt.Errorf("TextGrid 层级不足（需要 3 层）: %s", tgPath)
		}

		samples, sr, err := loadWavSamples(wavPath)
		if err != nil {
			return fmt.Errorf("读取 WAV 失败 %s: %w", wavPath, err)
		}

		sentencesTier := tg.Tiers[0]
		wordsTier := tg.Tiers[1]
		phonesTier := tg.Tiers[2]

		idx := 0
		for _, sentence := range sentencesTier.Intervals {
			if sentence.Mark == "" {
				continue
			}

			sentTg := &TextGrid{Xmin: 0, Xmax: sentence.Xmax - sentence.Xmin}
			wTier := &Tier{Name: "words", Xmin: 0, Xmax: sentTg.Xmax}
			pTier := &Tier{Name: "phones", Xmin: 0, Xmax: sentTg.Xmax}

			for _, w := range wordsTier.Intervals {
				lo := max64(sentence.Xmin, w.Xmin)
				hi := min64(sentence.Xmax, w.Xmax)
				if lo >= hi {
					continue
				}
				wTier.Intervals = append(wTier.Intervals, Interval{
					Xmin: lo - sentence.Xmin, Xmax: hi - sentence.Xmin, Mark: w.Mark,
				})
			}
			for _, p := range phonesTier.Intervals {
				lo := max64(sentence.Xmin, p.Xmin)
				hi := min64(sentence.Xmax, p.Xmax)
				if lo >= hi {
					continue
				}
				pTier.Intervals = append(pTier.Intervals, Interval{
					Xmin: lo - sentence.Xmin, Xmax: hi - sentence.Xmin, Mark: p.Mark,
				})
			}
			sentTg.Tiers = []*Tier{wTier, pTier}

			var outStem string
			if cfg.PreserveSentenceNames {
				outStem = sentence.Mark
			} else {
				outStem = fmt.Sprintf("%s_%0*d", stem, cfg.Digits, idx)
			}
			outTg := filepath.Join(tgOutDir, outStem+".TextGrid")
			outWav := filepath.Join(wavsOutDir, outStem+".wav")

			if !cfg.Overwrite {
				if _, err := os.Stat(outTg); err == nil {
					return fmt.Errorf("文件已存在: %s", outTg)
				}
				if _, err := os.Stat(outWav); err == nil {
					return fmt.Errorf("文件已存在: %s", outWav)
				}
			}

			if err := WriteTextGrid(outTg, sentTg); err != nil {
				return err
			}

			start := int(sentence.Xmin * float64(sr))
			end := min(len(samples), int(sentence.Xmax*float64(sr))+1)
			if start > len(samples) {
				start = len(samples)
			}
			if err := writeWavSamples(outWav, samples[start:end], sr, cfg.WavSubtype); err != nil {
				return err
			}
			idx++
		}
	}
	return nil
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
