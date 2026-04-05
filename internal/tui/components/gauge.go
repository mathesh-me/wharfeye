package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Gauge renders a percentage bar with label.
func Gauge(label string, percent float64, width int, color lipgloss.Color) string {
	if width < 10 {
		width = 10
	}

	labelStr := fmt.Sprintf("%s %.0f%%", label, percent)
	barWidth := width - len(labelStr) - 3 // 3 for " [" and "]"
	if barWidth < 5 {
		barWidth = 5
	}

	filled := int(percent / 100.0 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", barWidth-filled))

	return fmt.Sprintf("%s [%s]", labelStr, bar)
}

// MiniGauge renders a compact percentage bar without label.
func MiniGauge(percent float64, width int, color lipgloss.Color) string {
	if width < 3 {
		width = 3
	}

	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	return filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", width-filled))
}
