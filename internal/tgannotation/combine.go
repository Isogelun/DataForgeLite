package tgannotation

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

type CombineConfig struct {
	WavsDir   string // 输入 WAV 目录
	TgDir     string // 输入 TextGrid 目录，空则同 WavsDir
	OutDir    string // 输出父目录，下面自动创建 wavs/ 和 TextGrid/
	Suffix    string // 正则后缀，默认 `_\d+`
	WavSubtype string
	Overwrite bool
}

func Combine(cfg CombineConfig) error {
	if cfg.Suffix == "" {
		cfg.Suffix = `_\d+`
	}
	if cfg.WavSubtype == "" {
		cfg.WavSubtype = "PCM_16"
	}
	tgDir := cfg.TgDir
	if tgDir == "" {
		tgDir = cfg.WavsDir
	}
	wavsOutDir := filepath.Join(cfg.OutDir, "wavs")
	tgOutDir := filepath.Join(cfg.OutDir, "TextGrid")
	if err := os.MkdirAll(wavsOutDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(tgOutDir, 0755); err != nil {
		return err
	}

	// 按 stem 分组
	suffixRe := regexp.MustCompile(cfg.Suffix + `$`)
	entries, err := os.ReadDir(tgDir)
	if err != nil {
		return err
	}
	groups := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".TextGrid") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		base := suffixRe.ReplaceAllString(stem, "")
		groups[base] = append(groups[base], filepath.Join(tgDir, e.Name()))
	}

	// 按 key 排序
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		files := groups[name]
		// 自然排序
		sort.Slice(files, func(i, j int) bool { return files[i] < files[j] })

		outTg := filepath.Join(tgOutDir, name+".TextGrid")
		outWav := filepath.Join(wavsOutDir, name+".wav")
		if !cfg.Overwrite {
			if _, err := os.Stat(outTg); err == nil {
				return fmt.Errorf("文件已存在: %s", outTg)
			}
			if _, err := os.Stat(outWav); err == nil {
				return fmt.Errorf("文件已存在: %s", outWav)
			}
		}

		tg := &TextGrid{}
		sentencesTier := &Tier{Name: "sentences"}
		wordsTier := &Tier{Name: "words"}
		phonesTier := &Tier{Name: "phones"}

		var allSamples []int
		var sr int
		sentenceStart := 0.0

		for _, tgFile := range files {
			wavFile := filepath.Join(cfg.WavsDir, strings.TrimSuffix(filepath.Base(tgFile), ".TextGrid")+".wav")
			samples, sr_, err := loadWavSamples(wavFile)
			if err != nil {
				return fmt.Errorf("读取 WAV 失败 %s: %w", wavFile, err)
			}
			if sr == 0 {
				sr = sr_
			} else if sr_ != sr {
				return fmt.Errorf("采样率不一致: %s (%d != %d)", wavFile, sr_, sr)
			}

			sentenceEnd := float64(len(samples))/float64(sr) + sentenceStart
			allSamples = append(allSamples, samples...)

			stem := strings.TrimSuffix(filepath.Base(wavFile), ".wav")
			sentencesTier.Intervals = append(sentencesTier.Intervals, Interval{
				Xmin: sentenceStart, Xmax: sentenceEnd, Mark: stem,
			})

			seg, err := ReadTextGrid(tgFile)
			if err != nil {
				return fmt.Errorf("读取 TextGrid 失败 %s: %w", tgFile, err)
			}

			// words tier = seg.Tiers[0], phones tier = seg.Tiers[1]
			if len(seg.Tiers) >= 1 {
				tier := seg.Tiers[0]
				start := sentenceStart
				for j, iv := range tier.Intervals {
					var end float64
					if j == len(tier.Intervals)-1 {
						end = sentenceEnd
					} else {
						end = start + (iv.Xmax - iv.Xmin)
					}
					wordsTier.Intervals = append(wordsTier.Intervals, Interval{Xmin: start, Xmax: end, Mark: iv.Mark})
					start = end
				}
			}
			if len(seg.Tiers) >= 2 {
				tier := seg.Tiers[1]
				start := sentenceStart
				for j, iv := range tier.Intervals {
					var end float64
					if j == len(tier.Intervals)-1 {
						end = sentenceEnd
					} else {
						end = start + (iv.Xmax - iv.Xmin)
					}
					phonesTier.Intervals = append(phonesTier.Intervals, Interval{Xmin: start, Xmax: end, Mark: iv.Mark})
					start = end
				}
			}
			sentenceStart = sentenceEnd
		}

		tg.Xmin = 0
		tg.Xmax = sentenceStart
		sentencesTier.Xmin = 0; sentencesTier.Xmax = sentenceStart
		wordsTier.Xmin = 0; wordsTier.Xmax = sentenceStart
		phonesTier.Xmin = 0; phonesTier.Xmax = sentenceStart
		tg.Tiers = []*Tier{sentencesTier, wordsTier, phonesTier}

		if err := WriteTextGrid(outTg, tg); err != nil {
			return err
		}
		if err := writeWavSamples(outWav, allSamples, sr, cfg.WavSubtype); err != nil {
			return err
		}
	}
	return nil
}

func loadWavSamples(path string) ([]int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	dec := wav.NewDecoder(f)
	dec.ReadInfo()
	if !dec.IsValidFile() {
		return nil, 0, fmt.Errorf("无效 WAV: %s", path)
	}
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}
	return buf.Data, int(dec.SampleRate), nil
}

func writeWavSamples(path string, samples []int, sr int, subtype string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bitDepth := subtypeBitDepth(subtype)
	enc := wav.NewEncoder(f, sr, bitDepth, 1, 1)
	buf := &audio.IntBuffer{
		Data:   samples,
		Format: &audio.Format{NumChannels: 1, SampleRate: sr},
	}
	if err := enc.Write(buf); err != nil {
		return err
	}
	return enc.Close()
}

func subtypeBitDepth(subtype string) int {
	switch subtype {
	case "PCM_24":
		return 24
	case "PCM_32":
		return 32
	default:
		return 16
	}
}

// clamp float to avoid unused import
var _ = math.Pi
