package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathesh-me/wharfeye/internal/models"
)

var (
	criticalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	highStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	mediumStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))

	lowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	passedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	failedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	scoreGoodStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	scoreFairStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	scorePoorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	scoreCritStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

// SecurityView renders the security tab.
func SecurityView(report *models.FleetSecurityReport, selected int, width, height int) string {
	if report == nil {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("Press 's' to run security scan...")
	}

	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("── Security Posture ──"))
	sb.WriteString("\n\n")

	// Fleet score
	sb.WriteString(fmt.Sprintf("  Fleet Score: %s\n\n", renderScore(report.FleetScore)))

	// Summary bar
	sb.WriteString(fmt.Sprintf("  %s %d critical  %s %d high  %s %d medium  %s %d low  |  %s %d passed  %s %d failed\n\n",
		criticalStyle.Render("●"), report.Summary.CriticalIssues,
		highStyle.Render("●"), report.Summary.HighIssues,
		mediumStyle.Render("●"), report.Summary.MediumIssues,
		lowStyle.Render("●"), report.Summary.LowIssues,
		passedStyle.Render("✓"), report.Summary.PassedChecks,
		failedStyle.Render("✗"), report.Summary.FailedChecks,
	))

	// Per-container list
	headerLine := fmt.Sprintf("  %-25s  %-25s  %8s  %4s  %4s  %4s  %4s",
		"CONTAINER", "IMAGE", "SCORE", "CRIT", "HIGH", "MED", "LOW")
	sb.WriteString(headerStyle.Render(headerLine))
	sb.WriteString("\n")

	for i, cr := range report.Containers {
		critical, high, medium, low := countSevs(cr.Checks)

		line := fmt.Sprintf("  %-25s  %-25s  %s  %4d  %4d  %4d  %4d",
			truncSec(cr.ContainerName, 25),
			truncSec(cr.Image, 25),
			renderScoreCompact(cr.Score),
			critical, high, medium, low,
		)

		if i == selected {
			sb.WriteString(selectedStyle.Width(width).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// Show details for selected container
	if selected >= 0 && selected < len(report.Containers) {
		cr := report.Containers[selected]
		sb.WriteString("\n")
		sb.WriteString(sectionStyle.Render(fmt.Sprintf("Checks for %s", cr.ContainerName)))
		sb.WriteString("\n")

		for _, c := range cr.Checks {
			icon := passedStyle.Render("✓")
			if !c.Passed {
				icon = failedStyle.Render("✗")
			}

			sevStr := renderSeverity(c.Severity)
			details := ""
			if !c.Passed && c.Details != "" {
				details = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
					Render(" - " + c.Details)
			}

			sb.WriteString(fmt.Sprintf("  %s [%s] %s %s%s\n",
				icon, c.ID, sevStr, c.Name, details))
		}
	}

	return sb.String()
}

func renderScore(score int) string {
	text := fmt.Sprintf("%d/100", score)
	switch {
	case score >= 80:
		return scoreGoodStyle.Render(text)
	case score >= 60:
		return scoreFairStyle.Render(text)
	case score >= 40:
		return scorePoorStyle.Render(text)
	default:
		return scoreCritStyle.Render(text)
	}
}

func renderScoreCompact(score int) string {
	text := fmt.Sprintf("%3d", score)
	switch {
	case score >= 80:
		return scoreGoodStyle.Render(text)
	case score >= 60:
		return scoreFairStyle.Render(text)
	case score >= 40:
		return scorePoorStyle.Render(text)
	default:
		return scoreCritStyle.Render(text)
	}
}

func renderSeverity(s models.Severity) string {
	switch s {
	case models.SeverityCritical:
		return criticalStyle.Render("CRIT")
	case models.SeverityHigh:
		return highStyle.Render("HIGH")
	case models.SeverityMedium:
		return mediumStyle.Render("MED ")
	case models.SeverityLow:
		return lowStyle.Render("LOW ")
	default:
		return "    "
	}
}

func countSevs(checks []models.SecurityCheck) (critical, high, medium, low int) {
	for _, c := range checks {
		if c.Passed {
			continue
		}
		switch c.Severity {
		case models.SeverityCritical:
			critical++
		case models.SeverityHigh:
			high++
		case models.SeverityMedium:
			medium++
		case models.SeverityLow:
			low++
		}
	}
	return
}

func truncSec(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
