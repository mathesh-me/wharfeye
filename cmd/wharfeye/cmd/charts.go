package cmd

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

type barEntry struct {
	Label string
	Value float64
	Max   float64
	Tag   string
	Color string
}

const (
	colorReset   = "\033[0m"
	colorCyan    = "\033[36m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorMagenta = "\033[35m"
	colorRed     = "\033[31m"
	colorDim     = "\033[2m"
	colorBold    = "\033[1m"

	colorBrCyan    = "\033[96m"
	colorBrGreen   = "\033[92m"
	colorBrYellow  = "\033[93m"
	colorBrMagenta = "\033[95m"
	colorBrRed     = "\033[91m"
)

const (
	boxH  = "\u2500"
	boxV  = "\u2502"
	boxTL = "\u256D"
	boxTR = "\u256E"
	boxBL = "\u2570"
	boxBR = "\u256F"
	boxMR = "\u251C"
	boxML = "\u2524"
)

var sparkBlocks = []rune{' ', '\u2581', '\u2582', '\u2583', '\u2584', '\u2585', '\u2586', '\u2587', '\u2588'}
var ansiSeqRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visibleWidth(s string) int {
	return len(ansiSeqRE.ReplaceAllString(s, ""))
}

func padVisibleRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if n := visibleWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func printBoxRow(yPad, inner int, content string) {
	d := colorDim
	r := colorReset
	fmt.Printf("  %*s %s%s%s %s%s%s\n", yPad, "", d, boxV, r, padVisibleRight(content, inner-1), d, boxV+r)
}

func colorDimmed(color string) string {
	switch color {
	case colorBrCyan:
		return colorCyan
	case colorBrGreen:
		return colorGreen
	case colorBrYellow:
		return colorYellow
	case colorBrMagenta:
		return colorMagenta
	case colorBrRed:
		return colorRed
	default:
		return colorDim
	}
}

// brailleDotBit maps [subRow][subCol] to the braille bit index (0-7).
// Braille cell layout (2 wide x 4 tall):
//
//	col0  col1
//	dot1  dot4   -> bits 0, 3
//	dot2  dot5   -> bits 1, 4
//	dot3  dot6   -> bits 2, 5
//	dot7  dot8   -> bits 6, 7
var brailleDotBit = [4][2]uint{{0, 3}, {1, 4}, {2, 5}, {6, 7}}

// brailleInterpolate fills colPix (pixel-column -> pixel-row) for values
// mapped onto a pixel grid of pixW x pixH.
func brailleInterpolate(values []float64, pixW, pixH int, colPix []int) {
	if len(values) == 0 {
		return
	}
	minV, maxV := values[0], values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	span := maxV - minV

	xs := make([]int, len(values))
	ys := make([]int, len(values))
	for i, v := range values {
		px := 0
		if len(values) > 1 {
			px = int(math.Round(float64(i) * float64(pixW-1) / float64(len(values)-1)))
		}
		level := float64(pixH-1) / 2
		if span > 0 {
			level = ((v - minV) / span) * float64(pixH-1)
		}
		xs[i] = px
		ys[i] = pixH - 1 - int(math.Round(level))
	}

	for i := 0; i < len(values)-1; i++ {
		x0, y0 := xs[i], ys[i]
		x1, y1 := xs[i+1], ys[i+1]
		for px := x0; px <= x1; px++ {
			t := 0.0
			if x1 > x0 {
				t = float64(px-x0) / float64(x1-x0)
			}
			py := int(math.Round(float64(y0) + t*float64(y1-y0)))
			if py < 0 {
				py = 0
			}
			if py >= pixH {
				py = pixH - 1
			}
			colPix[px] = py
		}
	}
	if n := len(values); n > 0 {
		colPix[xs[n-1]] = ys[n-1]
	}
}

func localLinePlot(values []float64, width, height int, color string, _ string) []string {
	if width <= 0 || height <= 0 {
		return nil
	}

	pixW := width * 2
	pixH := height * 4

	colPix := make([]int, pixW)
	for i := range colPix {
		colPix[i] = -1
	}
	brailleInterpolate(values, pixW, pixH, colPix)

	out := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		b.Grow(width * 8)
		for col := 0; col < width; col++ {
			bits := uint(0)
			for subRow := 0; subRow < 4; subRow++ {
				for subCol := 0; subCol < 2; subCol++ {
					pixRow := row*4 + subRow
					pixCol := col*2 + subCol
					if pixCol < pixW && colPix[pixCol] == pixRow {
						bits |= 1 << brailleDotBit[subRow][subCol]
					}
				}
			}
			if bits > 0 {
				b.WriteString(color)
				b.WriteRune('\u2800' + rune(bits))
				b.WriteString(colorReset)
			} else {
				b.WriteString(colorDim)
				b.WriteRune('·')
				b.WriteString(colorReset)
			}
		}
		out[row] = b.String()
	}
	return out
}

type plotSeries struct {
	Values []float64
	Color  string
}

func localMultiLinePlot(series []plotSeries, width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}

	pixW := width * 2
	pixH := height * 4

	// Build a shared pixel grid using global min/max so series share the same scale
	minV, maxV := 0.0, 0.0
	haveValue := false
	for _, s := range series {
		for _, v := range s.Values {
			if !haveValue {
				minV, maxV = v, v
				haveValue = true
				continue
			}
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}

	// colPix[seriesIdx][pixCol] = pixRow, -1 if no data
	colPix := make([][]int, len(series))
	for idx, s := range series {
		colPix[idx] = make([]int, pixW)
		for c := range colPix[idx] {
			colPix[idx][c] = -1
		}
		if !haveValue || len(s.Values) == 0 {
			continue
		}
		span := maxV - minV
		// Re-scale values using global min/max
		scaled := make([]float64, len(s.Values))
		for i, v := range s.Values {
			if span > 0 {
				scaled[i] = (v - minV) / span
			} else {
				scaled[i] = 0.5
			}
		}
		// Map scaled [0,1] to pixel rows
		xs := make([]int, len(scaled))
		ys := make([]int, len(scaled))
		for i, sv := range scaled {
			px := 0
			if len(scaled) > 1 {
				px = int(math.Round(float64(i) * float64(pixW-1) / float64(len(scaled)-1)))
			}
			xs[i] = px
			ys[i] = pixH - 1 - int(math.Round(sv*float64(pixH-1)))
		}
		for i := 0; i < len(scaled)-1; i++ {
			x0, y0 := xs[i], ys[i]
			x1, y1 := xs[i+1], ys[i+1]
			for px := x0; px <= x1; px++ {
				t := 0.0
				if x1 > x0 {
					t = float64(px-x0) / float64(x1-x0)
				}
				py := int(math.Round(float64(y0) + t*float64(y1-y0)))
				if py < 0 {
					py = 0
				}
				if py >= pixH {
					py = pixH - 1
				}
				colPix[idx][px] = py
			}
		}
		if n := len(scaled); n > 0 {
			colPix[idx][xs[n-1]] = ys[n-1]
		}
	}

	out := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		b.Grow(width * 8)
		for col := 0; col < width; col++ {
			// Try each series; use the first one that has any dots in this cell
			drawn := false
			for idx, s := range series {
				bits := uint(0)
				for subRow := 0; subRow < 4; subRow++ {
					for subCol := 0; subCol < 2; subCol++ {
						pixRow := row*4 + subRow
						pixCol := col*2 + subCol
						if pixCol < pixW && colPix[idx][pixCol] == pixRow {
							bits |= 1 << brailleDotBit[subRow][subCol]
						}
					}
				}
				if bits > 0 {
					b.WriteString(s.Color)
					b.WriteRune('\u2800' + rune(bits))
					b.WriteString(colorReset)
					drawn = true
					break
				}
			}
			if !drawn {
				b.WriteString(colorDim)
				b.WriteRune('·')
				b.WriteString(colorReset)
			}
		}
		out[row] = b.String()
	}
	return out
}

// renderBarChart draws a framed horizontal bar chart.
func renderBarChart(title string, entries []barEntry, barWidth int) {
	if len(entries) == 0 {
		return
	}

	maxLabel := 0
	maxTag := 0
	for _, e := range entries {
		if len(e.Label) > maxLabel {
			maxLabel = len(e.Label)
		}
		if len(e.Tag) > maxTag {
			maxTag = len(e.Tag)
		}
	}
	if maxLabel > 20 {
		maxLabel = 20
	}

	// inner = space + label + space + bar + space + tag(padded) + space
	// tag is padded to maxTag+1, plus one trailing space before the right border
	inner := 1 + maxLabel + 1 + barWidth + 1 + (maxTag + 1) + 1
	if titleLen := len(title) + 2; titleLen > inner {
		inner = titleLen
	}
	d := colorDim
	r := colorReset

	fmt.Println()
	fmt.Printf("  %s%s%s%s%s\n", d, boxTL, strings.Repeat(boxH, inner), boxTR, r)
	fmt.Printf("  %s%s%s %s%s%s%s%s%s%s\n", d, boxV, r, colorBold, title, r, strings.Repeat(" ", inner-len(title)-1), d, boxV, r)
	fmt.Printf("  %s%s%s%s%s\n", d, boxMR, strings.Repeat(boxH, inner), boxML, r)

	for _, e := range entries {
		label := e.Label
		if len(label) > maxLabel {
			label = label[:maxLabel-2] + ".."
		}
		ratio := 0.0
		if e.Max > 0 {
			ratio = e.Value / e.Max
		}
		if ratio > 1 {
			ratio = 1
		}
		filled := int(math.Round(ratio * float64(barWidth)))
		c := e.Color
		if c == "" {
			c = colorCyan
		}
		bar := c + strings.Repeat("\u2588", filled) + r + d + strings.Repeat("\u2591", barWidth-filled) + r
		fmt.Printf("  %s%s%s %-*s %s %-*s %s%s%s\n", d, boxV, r, maxLabel, label, bar, maxTag+1, e.Tag, d, boxV, r)
	}
	fmt.Printf("  %s%s%s%s%s\n", d, boxBL, strings.Repeat(boxH, inner), boxBR, r)
	fmt.Println()
}

// coloredSparkline renders a sparkline with optional ANSI color.
func coloredSparkline(values []float64, max float64, width int, color string) string {
	if len(values) == 0 {
		return strings.Repeat(" ", width)
	}
	result := make([]rune, width)
	for i := 0; i < width; i++ {
		idx := i * len(values) / width
		if idx >= len(values) {
			idx = len(values) - 1
		}
		v := values[idx]
		if max <= 0 {
			result[i] = ' '
			continue
		}
		level := int(v / max * 8)
		if level < 0 {
			level = 0
		}
		if level > 8 {
			level = 8
		}
		result[i] = sparkBlocks[level]
	}
	s := string(result)
	if color != "" {
		return color + s + colorReset
	}
	return s
}

func renderTrendBox(title, color string, values []float64, unit string, width int, stats string) {
	const graphH = 3
	const yPad = 10
	const graphRightPad = 2
	d := colorDim
	r := colorReset
	inner := width

	current := "n/a"
	if len(values) > 0 {
		current = fmtChartVal(values[len(values)-1], unit)
	}

	minText, maxText := "n/a", "n/a"
	if len(values) > 0 {
		minText = fmtChartVal(minVal(values), unit)
		maxText = fmtChartVal(maxVal(values), unit)
	}

	labelW := len("Max " + maxText)
	if minLabelW := len("Min " + minText); minLabelW > labelW {
		labelW = minLabelW
	}
	if labelW < 10 {
		labelW = 10
	}

	graphW := inner - labelW - 2 - graphRightPad
	if graphW < 18 {
		graphW = 18
	}
	graphRows := localLinePlot(values, graphW, graphH, color, colorDim)

	summary := truncate(fmt.Sprintf("Now %s  %s", current, stats), inner-1)

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxTL, strings.Repeat(boxH, inner), boxTR, r)
	printBoxRow(yPad, inner, colorBold+color+title+r)
	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxMR, strings.Repeat(boxH, inner), boxML, r)
	printBoxRow(yPad, inner, fmt.Sprintf("%-*s %s%s", labelW, "Max "+maxText, graphRows[0], strings.Repeat(" ", graphRightPad)))
	for i := 1; i < graphH-1; i++ {
		printBoxRow(yPad, inner, fmt.Sprintf("%-*s %s%s", labelW, "", graphRows[i], strings.Repeat(" ", graphRightPad)))
	}
	printBoxRow(yPad, inner, fmt.Sprintf("%-*s %s%s", labelW, "Min "+minText, graphRows[graphH-1], strings.Repeat(" ", graphRightPad)))
	printBoxRow(yPad, inner, summary)
	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxBL, strings.Repeat(boxH, inner), boxBR, r)
}

type trendSeriesLine struct {
	Label  string
	Values []float64
	Color  string
	Suffix string
}

func renderSeriesTrendBox(title string, titleColor string, lines []trendSeriesLine, width int) {
	const graphH = 3
	const yPad = 10
	const graphRightPad = 2
	d := colorDim
	r := colorReset

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxTL, strings.Repeat(boxH, width), boxTR, r)
	printBoxRow(yPad, width, colorBold+titleColor+title+r)
	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxMR, strings.Repeat(boxH, width), boxML, r)

	labelW := 10
	var allVals []float64
	for _, l := range lines {
		allVals = append(allVals, l.Values...)
	}
	maxText := "n/a"
	minText := "n/a"
	if len(allVals) > 0 {
		maxText = fmtChartVal(maxVal(allVals), "bytes")
		minText = fmtChartVal(minVal(allVals), "bytes")
	}
	if maxLabelW := len("Max " + maxText); maxLabelW > labelW {
		labelW = maxLabelW
	}
	if minLabelW := len("Min " + minText); minLabelW > labelW {
		labelW = minLabelW
	}

	graphW := width - labelW - 2 - graphRightPad
	if graphW < 18 {
		graphW = 18
	}
	seriesPlots := make([]plotSeries, 0, len(lines))
	for _, l := range lines {
		seriesPlots = append(seriesPlots, plotSeries{Values: l.Values, Color: l.Color})
	}
	graphRows := localMultiLinePlot(seriesPlots, graphW, graphH)

	var summaryParts []string
	for _, l := range lines {
		summaryParts = append(summaryParts, fmt.Sprintf("%s%s%s %s", l.Color, l.Label, colorReset, l.Suffix))
	}
	summary := strings.Join(summaryParts, "   ")

	printBoxRow(yPad, width, fmt.Sprintf("%-*s %s%s", labelW, "Max "+maxText, graphRows[0], strings.Repeat(" ", graphRightPad)))
	for i := 1; i < graphH-1; i++ {
		printBoxRow(yPad, width, fmt.Sprintf("%-*s %s%s", labelW, "", graphRows[i], strings.Repeat(" ", graphRightPad)))
	}
	printBoxRow(yPad, width, fmt.Sprintf("%-*s %s%s", labelW, "Min "+minText, graphRows[graphH-1], strings.Repeat(" ", graphRightPad)))
	printBoxRow(yPad, width, summary)

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxBL, strings.Repeat(boxH, width), boxBR, r)
}

type metricBoxLine struct {
	Label  string
	Value  float64
	Max    float64
	Color  string
	Suffix string
}

func renderMetricBox(title string, titleColor string, lines []metricBoxLine, width int) {
	const yPad = 10
	d := colorDim
	r := colorReset

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxTL, strings.Repeat(boxH, width), boxTR, r)
	printBoxRow(yPad, width, colorBold+titleColor+title+r)
	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxMR, strings.Repeat(boxH, width), boxML, r)

	labelW := 12
	suffixW := 6
	for _, l := range lines {
		if len(l.Label) > labelW {
			labelW = len(l.Label)
		}
		if len(l.Suffix) > suffixW {
			suffixW = len(l.Suffix)
		}
	}
	if labelW > 18 {
		labelW = 18
	}
	if suffixW > 14 {
		suffixW = 14
	}

	barW := width - labelW - suffixW - 3
	if barW < 12 {
		barW = 12
	}

	for _, l := range lines {
		label := l.Label
		if len(label) > labelW {
			label = label[:labelW-2] + ".."
		}
		ratio := 0.0
		if l.Max > 0 {
			ratio = l.Value / l.Max
		}
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		filled := int(math.Round(ratio * float64(barW)))
		track := make([]rune, barW)
		for i := range track {
			track[i] = '·'
		}
		if filled > 0 {
			for i := 0; i < filled-1 && i < barW; i++ {
				track[i] = '─'
			}
			if filled-1 < barW {
				track[filled-1] = '█'
			}
		}
		var bar strings.Builder
		bar.Grow(barW * 8)
		for _, ch := range track {
			switch ch {
			case '█':
				bar.WriteString(l.Color)
				bar.WriteRune(ch)
				bar.WriteString(r)
			case '─':
				bar.WriteString(colorDimmed(l.Color))
				bar.WriteRune(ch)
				bar.WriteString(r)
			default:
				bar.WriteString(colorDim)
				bar.WriteRune(ch)
				bar.WriteString(r)
			}
		}
		suffix := l.Suffix
		if len(suffix) > suffixW {
			suffix = truncate(suffix, suffixW)
		}
		printBoxRow(yPad, width, fmt.Sprintf("%-*s %s %*s", labelW, label, bar.String(), suffixW, suffix))
	}

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxBL, strings.Repeat(boxH, width), boxBR, r)
}

// renderSparkBox draws a framed box containing labeled sparklines.
func renderSparkBox(title string, titleColor string, lines []sparkBoxLine, width int) {
	const yPad = 10
	d := colorDim
	r := colorReset

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxTL, strings.Repeat(boxH, width), boxTR, r)
	printBoxRow(yPad, width, colorBold+titleColor+title+r)
	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxMR, strings.Repeat(boxH, width), boxML, r)

	labelW := 2
	suffixW := 0
	for _, l := range lines {
		if len(l.Label) > labelW {
			labelW = len(l.Label)
		}
		if len(l.Suffix) > suffixW {
			suffixW = len(l.Suffix)
		}
	}
	if suffixW < 8 {
		suffixW = 8
	}

	for _, l := range lines {
		sparkW := width - labelW - suffixW - 4
		if sparkW < 10 {
			sparkW = 10
		}
		printBoxRow(yPad, width, fmt.Sprintf("%s%-*s%s %s %-*s",
			l.Color, labelW, l.Label, r,
			coloredSparkline(l.Values, l.Max, sparkW, l.Color),
			suffixW, l.Suffix))
	}

	fmt.Printf("  %*s %s%s%s%s%s\n", yPad, "", d, boxBL, strings.Repeat(boxH, width), boxBR, r)
}

type sparkBoxLine struct {
	Label  string
	Values []float64
	Max    float64
	Color  string
	Suffix string
}

func fmtChartVal(v float64, unit string) string {
	switch unit {
	case "%":
		return fmt.Sprintf("%.0f%%", v)
	case "bytes":
		return formatBytes(uint64(v))
	default:
		if v >= 1000000 {
			return fmt.Sprintf("%.1fM", v/1000000)
		}
		if v >= 1000 {
			return fmt.Sprintf("%.1fK", v/1000)
		}
		return fmt.Sprintf("%.0f", v)
	}
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	t := 0.0
	for _, v := range values {
		t += v
	}
	return t / float64(len(values))
}
