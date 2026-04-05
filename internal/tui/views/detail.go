package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/tui/components"
)

// detailTitleStyle is an alias for titleStyle in detail context.
var detailTitleStyle = titleStyle

// DetailView renders a detailed view of a single container.
func DetailView(ctr models.Container, stats *models.ContainerStats,
	cpuHistory []float64, width, height int) string {

	var sb strings.Builder

	// Title
	sb.WriteString(detailTitleStyle.Render(fmt.Sprintf("── Container: %s ──", ctr.Name)))
	sb.WriteString("\n")

	// Info section
	sb.WriteString(sectionStyle.Render("Info"))
	sb.WriteString("\n")
	sb.WriteString(detailRow("ID", ctr.ShortID()))
	sb.WriteString(detailRow("Name", ctr.Name))
	sb.WriteString(detailRow("Image", ctr.Image))
	sb.WriteString(detailRow("Status", ctr.Status))
	sb.WriteString(detailRow("State", ctr.State))
	sb.WriteString(detailRow("Created", ctr.Created.Format("2006-01-02 15:04:05")))
	sb.WriteString(detailRow("Runtime", ctr.Runtime))

	// Ports
	if len(ctr.Ports) > 0 {
		ports := make([]string, 0, len(ctr.Ports))
		for _, p := range ctr.Ports {
			ports = append(ports, p.String())
		}
		sb.WriteString(detailRow("Ports", strings.Join(ports, ", ")))
	}

	// Mounts
	if len(ctr.Mounts) > 0 {
		for i, m := range ctr.Mounts {
			label := "Mount"
			if i > 0 {
				label = ""
			}
			sb.WriteString(detailRow(label, fmt.Sprintf("%s -> %s (%s)", m.Source, m.Destination, m.Mode)))
		}
	}

	// Stats section
	if stats != nil {
		sb.WriteString(sectionStyle.Render("Resources"))
		sb.WriteString("\n")

		// CPU gauge + sparkline
		cpuGauge := components.Gauge("CPU", stats.CPU.Percent, 40, lipgloss.Color("42"))
		sb.WriteString("  " + cpuGauge + "\n")

		if len(cpuHistory) > 0 {
			spark := components.Sparkline(cpuHistory, min(width-4, 60))
			sb.WriteString("  " + spark + "\n")
		}

		// Memory gauge
		memGauge := components.Gauge("MEM", stats.Memory.Percent, 40, lipgloss.Color("33"))
		sb.WriteString("  " + memGauge + "\n")
		sb.WriteString(detailRow("Memory", fmt.Sprintf("%s / %s",
			formatBytesDetail(stats.Memory.Usage),
			formatBytesDetail(stats.Memory.Limit))))

		// Network
		sb.WriteString(detailRow("Net RX", formatBytesDetail(stats.NetworkIO.RxBytes)))
		sb.WriteString(detailRow("Net TX", formatBytesDetail(stats.NetworkIO.TxBytes)))

		// Block IO
		sb.WriteString(detailRow("Disk Read", formatBytesDetail(stats.BlockIO.ReadBytes)))
		sb.WriteString(detailRow("Disk Write", formatBytesDetail(stats.BlockIO.WriteBytes)))

		// PIDs
		sb.WriteString(detailRow("PIDs", fmt.Sprintf("%d", stats.PIDs)))
	}

	// Labels
	if len(ctr.Labels) > 0 {
		sb.WriteString(sectionStyle.Render("Labels"))
		sb.WriteString("\n")
		for k, v := range ctr.Labels {
			sb.WriteString(detailRow(k, v))
		}
	}

	// Navigation hint
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1).
		Render("Press Esc or Backspace to return"))

	return sb.String()
}

// ContainerDetailFromSnapshot extracts container info from a snapshot by index.
func ContainerDetailFromSnapshot(snapshot *engine.Snapshot, filter string, index int) (models.Container, *models.ContainerStats) {
	rows := buildRows(snapshot, nil, filter)
	if index < 0 || index >= len(rows) {
		return models.Container{}, nil
	}

	row := rows[index]
	stats := snapshot.Stats[row.Container.ID]
	return row.Container, stats
}

func detailRow(label, value string) string {
	if label == "" {
		return fmt.Sprintf("  %s%s\n", labelStyle.Render(""), valueStyle.Render(value))
	}
	return fmt.Sprintf("  %s%s\n", labelStyle.Render(label+":"), valueStyle.Render(value))
}

func formatBytesDetail(b uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
