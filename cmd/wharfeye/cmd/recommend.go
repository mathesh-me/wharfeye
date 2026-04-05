package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
)

var recommendCmd = &cobra.Command{
	Use:   "recommend [container-name]",
	Short: "Show performance recommendations",
	Long: `Analyze container metrics and configurations to generate performance recommendations.

Optionally pass a container name to see recommendations for just that container.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRecommend,
}

func init() {
	recommendCmd.Flags().StringVar(&formatFlag, "format", "table", "output format: table, json")
	rootCmd.AddCommand(recommendCmd)
}

func runRecommend(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := detectClient(ctx)
	if err != nil {
		return fmt.Errorf("detecting runtime: %w", err)
	}

	advisor := engine.NewAdvisor(client, nil)

	if len(args) == 1 {
		fmt.Printf("\033[33mAnalyzing performance for %s...\033[0m\n", args[0])
	} else {
		fmt.Printf("\033[33mAnalyzing performance across all containers...\033[0m\n")
	}
	report, err := advisor.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("analyzing: %w", err)
	}

	// Filter to single container if specified
	if len(args) == 1 {
		report = filterRecommendations(report, args[0])
	}

	if formatFlag == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	return printRecommendTable(report)
}

func printRecommendTable(report *models.AdvisorReport) error {
	if len(report.Recommendations) == 0 {
		fmt.Println("No recommendations - your containers look good!")
		return nil
	}

	fmt.Printf("Recommendations: %d high | %d medium | %d low\n\n",
		report.HighCount, report.MediumCount, report.LowCount)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPRIORITY\tCONTAINER\tTITLE")

	for _, r := range report.Recommendations {
		container := r.ContainerName
		if container == "" {
			container = "(fleet)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			r.ID,
			priorityBadge(r.Priority),
			truncate(container, 20),
			r.Title,
		)
	}
	w.Flush()

	// Detailed recommendations
	fmt.Println()
	for _, r := range report.Recommendations {
		fmt.Printf("[%s] %s %s\n", r.ID, priorityBadge(r.Priority), r.Title)
		if r.ContainerName != "" {
			fmt.Printf("  Container: %s\n", r.ContainerName)
		}
		fmt.Printf("  %s\n", r.Description)
		fmt.Printf("  Impact: %s\n", r.Impact)
		fmt.Printf("  Action: %s\n", r.Action)
		if r.Reference != "" {
			fmt.Printf("  Ref: %s\n", r.Reference)
		}
		fmt.Println()
	}

	return nil
}

func filterRecommendations(report *models.AdvisorReport, name string) *models.AdvisorReport {
	filtered := &models.AdvisorReport{}
	for _, r := range report.Recommendations {
		if r.ContainerName == name || r.ContainerID == name {
			filtered.Recommendations = append(filtered.Recommendations, r)
			switch r.Priority {
			case models.PriorityHigh:
				filtered.HighCount++
			case models.PriorityMedium:
				filtered.MediumCount++
			case models.PriorityLow:
				filtered.LowCount++
			}
		}
	}
	return filtered
}

func priorityBadge(p models.Priority) string {
	switch p {
	case models.PriorityHigh:
		return "HIGH"
	case models.PriorityMedium:
		return "MED "
	case models.PriorityLow:
		return "LOW "
	default:
		return "    "
	}
}
