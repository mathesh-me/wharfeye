package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

var securityCmd = &cobra.Command{
	Use:   "security [container-name]",
	Short: "Run security scan and print report",
	Long: `Analyze container configurations for security issues and print a report.

Optionally pass a container name to see detailed security findings with
specific hardening procedures to reach a 100/100 security score.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSecurity,
}

func init() {
	securityCmd.Flags().StringVar(&formatFlag, "format", "table", "output format: table, json")
	rootCmd.AddCommand(securityCmd)
}

func runSecurity(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := detectClient(ctx)
	if err != nil {
		return fmt.Errorf("detecting runtime: %w", err)
	}

	scanner := engine.NewScanner(client)

	// Single container mode with hardening advice
	if len(args) == 1 {
		fmt.Printf("\033[33mRunning security scan for %s...\033[0m\n", args[0])
		return runSecuritySingle(ctx, client, scanner, args[0])
	}

	fmt.Printf("\033[33mRunning security scan across all containers...\033[0m\n")
	report, err := scanner.ScanFleet(ctx)
	if err != nil {
		return fmt.Errorf("scanning fleet: %w", err)
	}

	if formatFlag == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	return printSecurityTable(report)
}

func runSecuritySingle(ctx context.Context, client runtime.Client, scanner *engine.Scanner, nameOrID string) error {
	containers, err := client.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}

	var target *models.Container
	for i, c := range containers {
		if c.Name == nameOrID || c.ID == nameOrID || strings.HasPrefix(c.ID, nameOrID) {
			target = &containers[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("container %q not found", nameOrID)
	}

	report, err := scanner.ScanContainer(ctx, *target)
	if err != nil {
		return fmt.Errorf("scanning container: %w", err)
	}

	if formatFlag == "json" {
		hardening := engine.GetContainerHardening(report)
		output := struct {
			Report    *models.ContainerSecurityReport `json:"report"`
			Hardening []engine.HardeningAdvice        `json:"hardening"`
		}{report, hardening}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	return printContainerSecurity(report)
}

func printContainerSecurity(report *models.ContainerSecurityReport) error {
	fmt.Printf("Container: %s (score: %s)\n", report.ContainerName, scoreColor(report.Score))
	fmt.Printf("Image:     %s\n\n", report.Image)

	passed := 0
	failed := 0
	for _, c := range report.Checks {
		if c.Passed {
			passed++
		} else {
			failed++
		}
	}
	fmt.Printf("Checks: %d passed | %d failed\n\n", passed, failed)

	// Show all checks - passed and failed
	for _, c := range report.Checks {
		if c.Passed {
			fmt.Printf("  [PASS] [%s] %s\n", c.ID, c.Name)
		}
	}
	if passed > 0 {
		fmt.Println()
	}

	// Show failed checks with hardening advice
	hardening := engine.GetContainerHardening(report)
	if len(hardening) == 0 {
		fmt.Println("All checks passed - this container is fully hardened!")
		return nil
	}

	fmt.Println("Failed checks with hardening procedures:")
	fmt.Println()
	for _, h := range hardening {
		fmt.Printf("  [%s] %s %s\n", h.CheckID, severityBadge(models.Severity(h.Severity)), h.CheckName)
		fmt.Printf("    Fix (docker run): %s\n", h.RunFix)
		if h.DockerFix != "" {
			fmt.Printf("    Fix (Dockerfile): %s\n", h.DockerFix)
		}
		fmt.Printf("    Why: %s\n", h.Explanation)
		if h.Reference != "" {
			fmt.Printf("    Ref: %s\n", h.Reference)
		}
		fmt.Println()
	}

	fmt.Printf("Apply all fixes above to reach 100/100.\n")
	fmt.Println()
	fmt.Println("References:")
	fmt.Println("  - CIS Docker Benchmark v1.6.0: https://www.cisecurity.org/benchmark/docker")
	fmt.Println("  - NIST SP 800-190 (Container Security): https://csrc.nist.gov/pubs/sp/800/190/final")
	fmt.Println("  - Docker Security Best Practices: https://docs.docker.com/engine/security/")
	return nil
}

func printSecurityTable(report *models.FleetSecurityReport) error {
	// Fleet summary
	fmt.Printf("Fleet Security Score: %s\n\n", scoreColor(report.FleetScore))
	fmt.Printf("Summary: %d containers | %d critical | %d high | %d medium | %d low\n",
		report.Summary.TotalContainers,
		report.Summary.CriticalIssues,
		report.Summary.HighIssues,
		report.Summary.MediumIssues,
		report.Summary.LowIssues,
	)
	fmt.Printf("Checks: %d passed | %d failed\n\n",
		report.Summary.PassedChecks,
		report.Summary.FailedChecks,
	)

	// Per-container table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONTAINER\tIMAGE\tSCORE\tCRITICAL\tHIGH\tMEDIUM\tLOW")

	for _, cr := range report.Containers {
		critical, high, medium, low := countSeverities(cr.Checks)
		fmt.Fprintf(w, "%s\t%s\t%d/100\t%d\t%d\t%d\t%d\n",
			truncate(cr.ContainerName, 25),
			truncate(cr.Image, 25),
			cr.Score,
			critical, high, medium, low,
		)
	}
	w.Flush()

	// Security score bar chart with color-coded bars
	entries := make([]barEntry, 0, len(report.Containers))
	for _, cr := range report.Containers {
		tag := fmt.Sprintf("%d/100", cr.Score)
		barColor := colorBrRed
		if cr.Score >= 80 {
			barColor = colorBrGreen
		} else if cr.Score >= 50 {
			barColor = colorBrYellow
		}
		entries = append(entries, barEntry{
			Label: cr.ContainerName,
			Value: float64(cr.Score),
			Max:   100,
			Tag:   tag,
			Color: barColor,
		})
	}
	renderBarChart("Security Scores", entries, 30)

	// Show failed checks for each container
	for _, cr := range report.Containers {
		failed := failedChecks(cr.Checks)
		if len(failed) == 0 {
			continue
		}

		fmt.Printf("\n%s (score: %d/100):\n", cr.ContainerName, cr.Score)
		for _, c := range failed {
			fmt.Printf("  [%s] %s %s - %s\n",
				c.ID, severityBadge(c.Severity), c.Name, c.Details)
		}
	}

	return nil
}

func countSeverities(checks []models.SecurityCheck) (critical, high, medium, low int) {
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

func failedChecks(checks []models.SecurityCheck) []models.SecurityCheck {
	var failed []models.SecurityCheck
	for _, c := range checks {
		if !c.Passed {
			failed = append(failed, c)
		}
	}
	return failed
}

func scoreColor(score int) string {
	switch {
	case score >= 80:
		return fmt.Sprintf("%d/100 (Good)", score)
	case score >= 60:
		return fmt.Sprintf("%d/100 (Fair)", score)
	case score >= 40:
		return fmt.Sprintf("%d/100 (Poor)", score)
	default:
		return fmt.Sprintf("%d/100 (Critical)", score)
	}
}

func severityBadge(s models.Severity) string {
	switch s {
	case models.SeverityCritical:
		return "CRIT"
	case models.SeverityHigh:
		return "HIGH"
	case models.SeverityMedium:
		return "MED "
	case models.SeverityLow:
		return "LOW "
	default:
		return "    "
	}
}
