package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

var exportFlag string

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate full report (metrics + security + recommendations)",
	Long:  "Collect metrics, run security scan, and generate performance recommendations in a single report.",
	RunE:  runReport,
}

func init() {
	reportCmd.Flags().StringVar(&formatFlag, "format", "table", "output format: table, json")
	reportCmd.Flags().StringVar(&exportFlag, "export", "", "export format: html, csv (writes to wharfeye-report.<ext>)")
	rootCmd.AddCommand(reportCmd)
}

type fullReport struct {
	GeneratedAt     time.Time                    `json:"generated_at"`
	Runtime         *runtime.RuntimeInfo         `json:"runtime"`
	Host            engine.HostSummary           `json:"host"`
	Containers      []containerStatus            `json:"containers"`
	Security        *models.FleetSecurityReport  `json:"security"`
	Recommendations *models.AdvisorReport        `json:"recommendations"`
}

func runReport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := detectClient(ctx)
	if err != nil {
		return fmt.Errorf("detecting runtime: %w", err)
	}

	eng := engine.New(client, engine.DefaultConfig())

	// Collect metrics
	snapshot, err := eng.CollectOnce(ctx)
	if err != nil {
		return fmt.Errorf("collecting metrics: %w", err)
	}

	info := &runtime.RuntimeInfo{
		Name:    snapshot.Host.RuntimeName,
		Version: snapshot.Host.RuntimeVersion,
	}

	statuses := make([]containerStatus, 0, len(snapshot.Containers))
	for _, c := range snapshot.Containers {
		cs := containerStatus{Container: c}
		if s, ok := snapshot.Stats[c.ID]; ok {
			cs.CPU = s.CPU.Percent
			cs.Memory = s.Memory.Usage
			cs.MemPct = s.Memory.Percent
		}
		statuses = append(statuses, cs)
	}

	// Security scan
	scanner := engine.NewScanner(client)
	secReport, err := scanner.ScanFleet(ctx)
	if err != nil {
		return fmt.Errorf("security scan: %w", err)
	}

	// Advisor analysis
	advisor := engine.NewAdvisor(client, nil)
	advReport, err := advisor.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("advisor analysis: %w", err)
	}

	report := fullReport{
		GeneratedAt:     time.Now(),
		Runtime:         info,
		Host:            snapshot.Host,
		Containers:      statuses,
		Security:        secReport,
		Recommendations: advReport,
	}

	switch exportFlag {
	case "html":
		return exportHTML(report)
	case "csv":
		return exportCSV(report)
	}

	if formatFlag == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	return printReportTable(report)
}

func printReportTable(r fullReport) error {
	fmt.Printf("WharfEye Report - %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("%s %s | Containers: %d (%d running) | CPU: %.1f%% | Mem: %s\n\n",
		r.Runtime.Name, r.Runtime.Version,
		r.Host.TotalContainers, r.Host.RunningContainers,
		r.Host.TotalCPUPercent, formatBytes(r.Host.TotalMemoryUsage),
	)

	// Container table
	fmt.Println("=== Containers ===")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE\tSTATUS\tCPU\tMEM")
	for _, cs := range r.Containers {
		cpu, mem := "-", "-"
		if cs.Container.IsRunning() {
			cpu = fmt.Sprintf("%.1f%%", cs.CPU)
			mem = formatBytes(cs.Memory)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			cs.Container.Name, truncate(cs.Container.Image, 30),
			cs.Container.Status, cpu, mem,
		)
	}
	w.Flush()

	// Security summary
	fmt.Printf("\n=== Security (Fleet Score: %s) ===\n", scoreColor(r.Security.FleetScore))
	fmt.Printf("Critical: %d | High: %d | Medium: %d | Low: %d\n",
		r.Security.Summary.CriticalIssues, r.Security.Summary.HighIssues,
		r.Security.Summary.MediumIssues, r.Security.Summary.LowIssues,
	)

	for _, cr := range r.Security.Containers {
		failed := failedChecks(cr.Checks)
		if len(failed) == 0 {
			continue
		}
		fmt.Printf("  %s (score: %d/100):\n", cr.ContainerName, cr.Score)
		for _, c := range failed {
			fmt.Printf("    [%s] %s %s\n", c.ID, severityBadge(c.Severity), c.Name)
		}
	}

	// Recommendations
	fmt.Printf("\n=== Recommendations (%d high | %d medium | %d low) ===\n",
		r.Recommendations.HighCount, r.Recommendations.MediumCount, r.Recommendations.LowCount,
	)
	for _, rec := range r.Recommendations.Recommendations {
		container := rec.ContainerName
		if container == "" {
			container = "(fleet)"
		}
		fmt.Printf("  [%s] %s  %s - %s\n", rec.ID, priorityBadge(rec.Priority), container, rec.Title)
		fmt.Printf("    %s\n", rec.Description)
		fmt.Printf("    Action: %s\n\n", rec.Action)
	}

	if len(r.Recommendations.Recommendations) == 0 {
		fmt.Println("  No recommendations - your containers look good!")
	}

	return nil
}

func exportHTML(r fullReport) error {
	const filename = "wharfeye-report.html"

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"fmtBytes": formatBytes,
		"fmtPct":   func(v float64) string { return fmt.Sprintf("%.1f%%", v) },
		"scoreLabel": func(s int) string {
			switch {
			case s >= 80:
				return "good"
			case s >= 50:
				return "warn"
			default:
				return "bad"
			}
		},
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating %s: %w", filename, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, r); err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	fmt.Printf("Report exported to %s\n", filename)
	return nil
}

func exportCSV(r fullReport) error {
	const filename = "wharfeye-report.csv"

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating %s: %w", filename, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header
	w.Write([]string{
		"container_name", "image", "status", "runtime",
		"cpu_percent", "memory_bytes", "memory_percent",
		"security_score", "critical", "high", "medium", "low",
		"ports", "restart_count",
	})

	// Build security score map
	secScores := make(map[string]models.ContainerSecurityReport)
	for _, cr := range r.Security.Containers {
		secScores[cr.ContainerID] = cr
	}

	for _, cs := range r.Containers {
		sec := secScores[cs.Container.ID]
		crit, high, med, low := countSeverities(sec.Checks)

		ports := formatPorts(cs.Container.Ports)

		w.Write([]string{
			cs.Container.Name,
			cs.Container.Image,
			cs.Container.Status,
			cs.Container.Runtime,
			strconv.FormatFloat(cs.CPU, 'f', 2, 64),
			strconv.FormatUint(cs.Memory, 10),
			strconv.FormatFloat(cs.MemPct, 'f', 2, 64),
			strconv.Itoa(sec.Score),
			strconv.Itoa(crit),
			strconv.Itoa(high),
			strconv.Itoa(med),
			strconv.Itoa(low),
			ports,
			"0",
		})
	}

	fmt.Printf("Report exported to %s\n", filename)
	return nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>WharfEye Report - {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</title>
<style>
:root { --bg: #0f172a; --card: #1e293b; --border: #334155; --text: #e2e8f0; --muted: #94a3b8; --blue: #3b82f6; --green: #22c55e; --yellow: #eab308; --red: #ef4444; --orange: #f97316; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: system-ui, sans-serif; background: var(--bg); color: var(--text); padding: 2rem; line-height: 1.6; }
h1 { color: var(--blue); margin-bottom: 0.5rem; }
h2 { color: var(--text); margin: 2rem 0 1rem; border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }
.meta { color: var(--muted); margin-bottom: 2rem; }
.summary { display: flex; gap: 1.5rem; flex-wrap: wrap; margin-bottom: 1.5rem; }
.stat-card { background: var(--card); border: 1px solid var(--border); border-radius: 0.5rem; padding: 1rem 1.5rem; min-width: 140px; }
.stat-card .label { font-size: 0.75rem; color: var(--muted); text-transform: uppercase; }
.stat-card .value { font-size: 1.5rem; font-weight: 700; }
table { width: 100%; border-collapse: collapse; margin-bottom: 1rem; }
th, td { text-align: left; padding: 0.5rem 1rem; border-bottom: 1px solid var(--border); }
th { color: var(--muted); font-size: 0.75rem; text-transform: uppercase; }
.badge { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 1rem; font-size: 0.7rem; font-weight: 600; }
.badge.critical { background: rgba(239,68,68,0.15); color: var(--red); }
.badge.high { background: rgba(249,115,22,0.15); color: var(--orange); }
.badge.medium { background: rgba(234,179,8,0.15); color: var(--yellow); }
.badge.low { background: rgba(148,163,184,0.15); color: var(--muted); }
.score { font-weight: 700; font-size: 1.25rem; }
.score.good { color: var(--green); }
.score.warn { color: var(--yellow); }
.score.bad { color: var(--red); }
.rec { background: var(--card); border: 1px solid var(--border); border-radius: 0.5rem; padding: 1rem; margin-bottom: 0.75rem; }
.rec-title { font-weight: 600; }
.rec-meta { font-size: 0.85rem; color: var(--muted); margin-top: 0.5rem; }
footer { margin-top: 3rem; text-align: center; color: var(--muted); font-size: 0.8rem; }
</style>
</head>
<body>
<h1>WharfEye Report</h1>
<div class="meta">{{.Runtime.Name}} {{.Runtime.Version}} - Generated {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</div>

<div class="summary">
  <div class="stat-card"><div class="label">Containers</div><div class="value">{{.Host.RunningContainers}} / {{.Host.TotalContainers}}</div></div>
  <div class="stat-card"><div class="label">CPU</div><div class="value">{{fmtPct .Host.TotalCPUPercent}}</div></div>
  <div class="stat-card"><div class="label">Memory</div><div class="value">{{fmtBytes .Host.TotalMemoryUsage}}</div></div>
  <div class="stat-card"><div class="label">Images</div><div class="value">{{.Host.ImageCount}}</div></div>
  <div class="stat-card"><div class="label">Volumes</div><div class="value">{{.Host.VolumeCount}}</div></div>
</div>

<h2>Containers</h2>
<table>
<tr><th>Name</th><th>Image</th><th>Status</th><th>CPU</th><th>Memory</th></tr>
{{range .Containers}}
<tr>
  <td>{{.Container.Name}}</td>
  <td>{{.Container.Image}}</td>
  <td>{{.Container.Status}}</td>
  <td>{{if .Container.IsRunning}}{{fmtPct .CPU}}{{else}}-{{end}}</td>
  <td>{{if .Container.IsRunning}}{{fmtBytes .Memory}}{{else}}-{{end}}</td>
</tr>
{{end}}
</table>

<h2>Security <span class="score {{scoreLabel .Security.FleetScore}}">{{.Security.FleetScore}}/100</span></h2>
<div class="summary">
  <span class="badge critical">{{.Security.Summary.CriticalIssues}} critical</span>
  <span class="badge high">{{.Security.Summary.HighIssues}} high</span>
  <span class="badge medium">{{.Security.Summary.MediumIssues}} medium</span>
  <span class="badge low">{{.Security.Summary.LowIssues}} low</span>
</div>
<table>
<tr><th>Container</th><th>Image</th><th>Score</th><th>Failed Checks</th></tr>
{{range .Security.Containers}}
<tr>
  <td>{{.ContainerName}}</td>
  <td>{{.Image}}</td>
  <td><span class="score {{scoreLabel .Score}}">{{.Score}}</span></td>
  <td>{{range .Checks}}{{if not .Passed}}<span class="badge {{.Severity | printf "%s" | html}}">{{.Severity}}</span> {{.Name}}<br>{{end}}{{end}}</td>
</tr>
{{end}}
</table>

<h2>Recommendations</h2>
{{if .Recommendations.Recommendations}}
<div class="summary">
  <span class="badge high">{{.Recommendations.HighCount}} high</span>
  <span class="badge medium">{{.Recommendations.MediumCount}} medium</span>
  <span class="badge low">{{.Recommendations.LowCount}} low</span>
</div>
{{range .Recommendations.Recommendations}}
<div class="rec">
  <div><span class="badge {{.Priority | printf "%s" | html}}">{{.Priority}}</span> <span class="rec-title">[{{.ID}}] {{.Title}}</span></div>
  {{if .ContainerName}}<div style="color: var(--blue); font-size: 0.85rem;">{{.ContainerName}}</div>{{end}}
  <div style="margin-top: 0.25rem;">{{.Description}}</div>
  <div class="rec-meta"><strong>Impact:</strong> {{.Impact}}<br><strong>Action:</strong> {{.Action}}</div>
</div>
{{end}}
{{else}}
<p>No recommendations - your containers look good!</p>
{{end}}

<footer>Generated by WharfEye - Container Intelligence Dashboard</footer>
</body>
</html>`
