package hubertfa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteTextGrid writes a Praat TextGrid file for the given word list.
func WriteTextGrid(outputDir, baseName string, wavLength float64, words *WordList) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create TextGrid dir: %w", err)
	}
	tgPath := filepath.Join(outputDir, baseName+".TextGrid")

	var wordIntervals []tgInterval
	var phoneIntervals []tgInterval

	for _, w := range words.Words {
		wordIntervals = append(wordIntervals, tgInterval{
			XMin: w.Start,
			XMax: w.End,
			Text: w.Text,
		})
		for _, ph := range w.Phonemes {
			start := ph.Start
			if start < 0 {
				start = 0
			}
			phoneIntervals = append(phoneIntervals, tgInterval{
				XMin: start,
				XMax: ph.End,
				Text: ph.Text,
			})
		}
	}

	var sb strings.Builder
	sb.WriteString("File type = \"ooTextFile\"\n")
	sb.WriteString("Object class = \"TextGrid\"\n\n")
	sb.WriteString(fmt.Sprintf("xmin = 0\nxmax = %f\n", wavLength))
	sb.WriteString("tiers? <exists>\nsize = 2\nitem []:\n")

	writeTier(&sb, 1, "words", wavLength, wordIntervals)
	writeTier(&sb, 2, "phones", wavLength, phoneIntervals)

	return os.WriteFile(tgPath, []byte(sb.String()), 0644)
}

type tgInterval struct {
	XMin float64
	XMax float64
	Text string
}

func writeTier(sb *strings.Builder, index int, name string, xmax float64, intervals []tgInterval) {
	sb.WriteString(fmt.Sprintf("    item [%d]:\n", index))
	sb.WriteString("        class = \"IntervalTier\"\n")
	sb.WriteString(fmt.Sprintf("        name = \"%s\"\n", name))
	sb.WriteString("        xmin = 0\n")
	sb.WriteString(fmt.Sprintf("        xmax = %f\n", xmax))
	sb.WriteString(fmt.Sprintf("        intervals: size = %d\n", len(intervals)))
	for i, iv := range intervals {
		sb.WriteString(fmt.Sprintf("        intervals [%d]:\n", i+1))
		sb.WriteString(fmt.Sprintf("            xmin = %f\n", iv.XMin))
		sb.WriteString(fmt.Sprintf("            xmax = %f\n", iv.XMax))
		sb.WriteString(fmt.Sprintf("            text = \"%s\"\n", escapeTextGrid(iv.Text)))
	}
}

func escapeTextGrid(s string) string {
	return strings.ReplaceAll(s, "\"", "\"\"")
}
