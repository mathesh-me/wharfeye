package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/models"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255"))

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
)

// ContainerRow represents a single row in the container table.
type ContainerRow struct {
	Container models.Container
	CPU       float64
	Memory    uint64
	MemPct    float64
	NetRx     uint64
	NetTx     uint64
	CPUHist   []float64
}

// RenderTable renders the container table with the given rows, selected index,
// and terminal width.
func RenderTable(rows []ContainerRow, selected int, width int) string {
	if width < 80 {
		width = 80
	}

	// Column widths
	nameW := 20
	imageW := 25
	cpuW := 8
	memW := 10
	sparkW := 8
	statusW := 15
	portsW := width - nameW - imageW - cpuW - memW - sparkW - statusW - 14 // padding
	if portsW < 5 {
		portsW = 5
	}

	// Header
	header := fmt.Sprintf("  %-*s  %-*s  %*s  %*s  %-*s  %-*s  %-*s",
		nameW, "NAME",
		imageW, "IMAGE",
		cpuW, "CPU",
		memW, "MEM",
		sparkW, "CPU HIST",
		statusW, "STATUS",
		portsW, "PORTS",
	)

	var sb strings.Builder
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")

	for i, row := range rows {
		name := truncateStr(row.Container.Name, nameW)
		image := truncateStr(row.Container.Image, imageW)

		cpu := "-"
		mem := "-"
		spark := strings.Repeat(" ", sparkW)

		if row.Container.IsRunning() {
			cpu = fmt.Sprintf("%.1f%%", row.CPU)
			mem = formatBytesShort(row.Memory)
			if len(row.CPUHist) > 0 {
				spark = Sparkline(row.CPUHist, sparkW)
			}
		}

		status := truncateStr(row.Container.Status, statusW)
		ports := truncateStr(formatPortsShort(row.Container.Ports), portsW)

		line := fmt.Sprintf("  %-*s  %-*s  %*s  %*s  %-*s  %-*s  %-*s",
			nameW, name,
			imageW, image,
			cpuW, cpu,
			memW, mem,
			sparkW, spark,
			statusW, status,
			portsW, ports,
		)

		if i == selected {
			sb.WriteString(selectedStyle.Width(width).Render(line))
		} else if row.Container.IsRunning() {
			sb.WriteString(runningStyle.Render(line))
		} else {
			sb.WriteString(stoppedStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatBytesShort(b uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fG", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.0fM", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.0fK", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatPortsShort(ports []models.Port) string {
	if len(ports) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.HostPort > 0 {
			parts = append(parts, fmt.Sprintf("%d->%d", p.HostPort, p.ContainerPort))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}
