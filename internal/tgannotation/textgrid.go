package tgannotation

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Interval struct {
	Xmin float64
	Xmax float64
	Mark string
}

type Tier struct {
	Name      string
	Xmin      float64
	Xmax      float64
	Intervals []Interval
}

type TextGrid struct {
	Xmin  float64
	Xmax  float64
	Tiers []*Tier
}

func ReadTextGrid(path string) (*TextGrid, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return parseTextGrid(lines)
}

func parseTextGrid(lines []string) (*TextGrid, error) {
	tg := &TextGrid{}
	reFloat := regexp.MustCompile(`=\s*([0-9.eE+\-]+)`)
	reStr := regexp.MustCompile(`=\s*"(.*)"`)

	getFloat := func(line string) float64 {
		m := reFloat.FindStringSubmatch(line)
		if len(m) > 1 {
			v, _ := strconv.ParseFloat(m[1], 64)
			return v
		}
		return 0
	}
	getString := func(line string) string {
		m := reStr.FindStringSubmatch(line)
		if len(m) > 1 {
			return m[1]
		}
		return ""
	}

	i := 0
	for i < len(lines) {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "xmin") && tg.Xmin == 0 {
			tg.Xmin = getFloat(l)
		} else if strings.HasPrefix(l, "xmax") && tg.Xmax == 0 {
			tg.Xmax = getFloat(l)
		} else if strings.Contains(l, "item [") && strings.Contains(l, "]:") && !strings.Contains(l, "item []:") {
			tier := &Tier{}
			i++
			for i < len(lines) {
				l2 := strings.TrimSpace(lines[i])
				if strings.Contains(l2, "item [") && strings.Contains(l2, "]:") {
					break
				}
				if strings.HasPrefix(l2, "name") {
					tier.Name = getString(l2)
				} else if strings.HasPrefix(l2, "xmin") && tier.Xmin == 0 {
					tier.Xmin = getFloat(l2)
				} else if strings.HasPrefix(l2, "xmax") && tier.Xmax == 0 {
					tier.Xmax = getFloat(l2)
				} else if strings.HasPrefix(l2, "intervals [") {
					iv := Interval{}
					i++
					for i < len(lines) {
						l3 := strings.TrimSpace(lines[i])
						if strings.HasPrefix(l3, "intervals [") || strings.Contains(l3, "item [") {
							break
						}
						if strings.HasPrefix(l3, "xmin") {
							iv.Xmin = getFloat(l3)
						} else if strings.HasPrefix(l3, "xmax") {
							iv.Xmax = getFloat(l3)
						} else if strings.HasPrefix(l3, "text") {
							iv.Mark = getString(l3)
						}
						i++
					}
					tier.Intervals = append(tier.Intervals, iv)
					continue
				}
				i++
			}
			tg.Tiers = append(tg.Tiers, tier)
			continue
		}
		i++
	}
	return tg, nil
}

func WriteTextGrid(path string, tg *TextGrid) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "File type = \"ooTextFile\"\nObject class = \"TextGrid\"\n\n")
	fmt.Fprintf(w, "xmin = %g\nxmax = %g\n", tg.Xmin, tg.Xmax)
	fmt.Fprintf(w, "tiers? <exists>\nsize = %d\nitem []:\n", len(tg.Tiers))
	for ti, tier := range tg.Tiers {
		fmt.Fprintf(w, "    item [%d]:\n", ti+1)
		fmt.Fprintf(w, "        class = \"IntervalTier\"\n")
		fmt.Fprintf(w, "        name = \"%s\"\n", tier.Name)
		fmt.Fprintf(w, "        xmin = %g\n        xmax = %g\n", tier.Xmin, tier.Xmax)
		fmt.Fprintf(w, "        intervals: size = %d\n", len(tier.Intervals))
		for ii, iv := range tier.Intervals {
			fmt.Fprintf(w, "        intervals [%d]:\n", ii+1)
			fmt.Fprintf(w, "            xmin = %g\n            xmax = %g\n", iv.Xmin, iv.Xmax)
			fmt.Fprintf(w, "            text = \"%s\"\n", iv.Mark)
		}
	}
	return w.Flush()
}
