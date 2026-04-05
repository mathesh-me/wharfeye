package components

import "strings"

// Sparkline characters from lowest to highest.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders a series of values as a sparkline string.
func Sparkline(values []float64, width int) string {
	if len(values) == 0 || width <= 0 {
		return strings.Repeat(" ", width)
	}

	// Take the last `width` values
	if len(values) > width {
		values = values[len(values)-width:]
	}

	// Find min/max
	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	var sb strings.Builder
	rng := maxVal - minVal
	for _, v := range values {
		idx := 0
		if rng > 0 {
			idx = int((v - minVal) / rng * float64(len(sparkBlocks)-1))
		}
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		if idx < 0 {
			idx = 0
		}
		sb.WriteRune(sparkBlocks[idx])
	}

	// Pad to width if needed
	for sb.Len() < width {
		sb.WriteRune(' ')
	}

	return sb.String()
}
