package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/models"
)

var (
	priorityHighStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	priorityMedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220")).
				Bold(true)

	priorityLowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	categoryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)
)

// AdvisorView renders the recommendations tab.
func AdvisorView(report *models.AdvisorReport, selected int, width, height int) string {
	if report == nil {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("Analyzing... (metrics collection in progress)")
	}

	if len(report.Recommendations) == 0 {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("No recommendations - your containers look good!")
	}

	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("── Performance Recommendations ──"))
	sb.WriteString("\n\n")

	// Summary
	sb.WriteString(fmt.Sprintf("  %s %d high  %s %d medium  %s %d low\n\n",
		priorityHighStyle.Render("●"), report.HighCount,
		priorityMedStyle.Render("●"), report.MediumCount,
		priorityLowStyle.Render("●"), report.LowCount,
	))

	// Recommendation list
	for i, r := range report.Recommendations {
		prefix := "  "
		if i == selected {
			prefix = "▸ "
		}

		prio := renderPriority(r.Priority)
		container := r.ContainerName
		if container == "" {
			container = "(fleet)"
		}

		line := fmt.Sprintf("%s[%s] %s  %-20s  %s",
			prefix, r.ID, prio, truncAdv(container, 20), r.Title)

		if i == selected {
			sb.WriteString(selectedStyle.Width(width).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// Detail panel for selected recommendation
	if selected >= 0 && selected < len(report.Recommendations) {
		r := report.Recommendations[selected]
		sb.WriteString("\n")
		sb.WriteString(sectionStyle.Render(fmt.Sprintf("[%s] %s", r.ID, r.Title)))
		sb.WriteString("\n")

		if r.ContainerName != "" {
			sb.WriteString(fmt.Sprintf("  %s %s\n",
				labelStyle.Render("Container:"), valueStyle.Render(r.ContainerName)))
		}
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("Category:"), categoryStyle.Render(r.Category)))
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("Description:"), valueStyle.Render(r.Description)))
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("Impact:"), valueStyle.Render(r.Impact)))
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("Action:"), valueStyle.Render(r.Action)))
	}

	return sb.String()
}

func renderPriority(p models.Priority) string {
	switch p {
	case models.PriorityHigh:
		return priorityHighStyle.Render("HIGH")
	case models.PriorityMedium:
		return priorityMedStyle.Render("MED ")
	case models.PriorityLow:
		return priorityLowStyle.Render("LOW ")
	default:
		return "    "
	}
}

func truncAdv(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
