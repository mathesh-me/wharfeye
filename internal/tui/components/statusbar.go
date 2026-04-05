package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/engine"
)

var (
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)
)

// StatusBar renders the bottom status bar with host aggregates and key hints.
func StatusBar(host engine.HostSummary, width int, activeTab int) string {
	// Runtime info
	runtimeIcon := runtimeEmoji(host.RuntimeName)
	runtimeInfo := fmt.Sprintf("%s %s %s", runtimeIcon, host.RuntimeName, host.RuntimeVersion)

	// Resource summary
	resources := fmt.Sprintf("Avg CPU: %.1f%% | Avg Mem: %s | Running: %d/%d",
		host.AvgCPUPercent,
		formatBytesShort(host.AvgMemoryUsage),
		host.RunningContainers,
		host.TotalContainers,
	)

	// Key hints
	keys := fmt.Sprintf("[%s] Dashboard  [%s] Security  [%s] Advisor  [%s] Detail  [%s] Scan  [%s] Search  [%s] Quit",
		statusKeyStyle.Render("1"),
		statusKeyStyle.Render("2"),
		statusKeyStyle.Render("3"),
		statusKeyStyle.Render("Enter"),
		statusKeyStyle.Render("s"),
		statusKeyStyle.Render("/"),
		statusKeyStyle.Render("q"),
	)

	left := statusBarStyle.Render(runtimeInfo + "  " + resources)
	right := statusBarStyle.Render(keys)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	return left + statusBarStyle.Render(fmt.Sprintf("%*s", gap, "")) + right
}

func runtimeEmoji(name string) string {
	switch name {
	case "Docker":
		return "[D]"
	case "Podman":
		return "[P]"
	case "containerd":
		return "[C]"
	default:
		return "[?]"
	}
}
