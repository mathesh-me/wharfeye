package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/tui/components"
)

var hostGaugeStyle = lipgloss.NewStyle().
	Padding(0, 1)

// DashboardView renders the main dashboard with container table and host gauges.
func DashboardView(snapshot *engine.Snapshot, cpuHistory map[string][]float64,
	selected int, filter string, width, height int) string {

	if snapshot == nil {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("Waiting for data...")
	}

	var sb strings.Builder

	// Title bar
	title := titleStyle.Render(fmt.Sprintf("── WharfEye ── %s %s ──",
		snapshot.Host.RuntimeName, snapshot.Host.RuntimeVersion))
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Host resource gauges (compact, single line)
	cpuGauge := components.Gauge("CPU", snapshot.Host.TotalCPUPercent, 30, lipgloss.Color("42"))
	memPct := 0.0
	if snapshot.Host.TotalMemoryLimit > 0 {
		memPct = float64(snapshot.Host.TotalMemoryUsage) / float64(snapshot.Host.TotalMemoryLimit) * 100
	}
	memGauge := components.Gauge("MEM", memPct, 30, lipgloss.Color("33"))

	gauges := hostGaugeStyle.Render(cpuGauge + "  " + memGauge)
	sb.WriteString(gauges)
	sb.WriteString("\n\n")

	// Filter containers
	rows := buildRows(snapshot, cpuHistory, filter)

	// Container table
	tableHeight := height - 8 // reserve space for title, gauges, status bar
	if tableHeight < 5 {
		tableHeight = 5
	}

	// Adjust selected to be within visible range
	if selected >= len(rows) {
		selected = len(rows) - 1
	}
	if selected < 0 {
		selected = 0
	}

	table := components.RenderTable(rows, selected, width)
	sb.WriteString(table)

	// Filter indicator
	if filter != "" {
		sb.WriteString(fmt.Sprintf("\n  Filter: %s (%d matches)", filter, len(rows)))
	}

	return sb.String()
}

// buildRows creates table rows from snapshot data, applying filter.
func buildRows(snapshot *engine.Snapshot, cpuHistory map[string][]float64, filter string) []components.ContainerRow {
	rows := make([]components.ContainerRow, 0, len(snapshot.Containers))
	filterLower := strings.ToLower(filter)

	for _, ctr := range snapshot.Containers {
		// Apply filter
		if filter != "" {
			nameMatch := strings.Contains(strings.ToLower(ctr.Name), filterLower)
			imageMatch := strings.Contains(strings.ToLower(ctr.Image), filterLower)
			if !nameMatch && !imageMatch {
				continue
			}
		}

		row := components.ContainerRow{
			Container: ctr,
		}

		if s, ok := snapshot.Stats[ctr.ID]; ok {
			row.CPU = s.CPU.Percent
			row.Memory = s.Memory.Usage
			row.MemPct = s.Memory.Percent
			row.NetRx = s.NetworkIO.RxBytes
			row.NetTx = s.NetworkIO.TxBytes
		}

		if hist, ok := cpuHistory[ctr.ID]; ok {
			row.CPUHist = hist
		}

		rows = append(rows, row)
	}

	return rows
}

// FilteredContainerCount returns the number of containers matching the filter.
func FilteredContainerCount(snapshot *engine.Snapshot, filter string) int {
	if snapshot == nil {
		return 0
	}
	if filter == "" {
		return len(snapshot.Containers)
	}
	count := 0
	filterLower := strings.ToLower(filter)
	for _, ctr := range snapshot.Containers {
		nameMatch := strings.Contains(strings.ToLower(ctr.Name), filterLower)
		imageMatch := strings.Contains(strings.ToLower(ctr.Image), filterLower)
		if nameMatch || imageMatch {
			count++
		}
	}
	return count
}
