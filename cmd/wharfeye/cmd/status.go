package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

var formatFlag string

var statusCmd = &cobra.Command{
	Use:   "status [container-name]",
	Short: "Show container status overview",
	Long: `Auto-detect runtime and display all containers with their resource usage.

Optionally pass a container name or ID to see detailed info for a single container
including CPU/memory usage, port mappings, mounts, and configuration.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&formatFlag, "format", "table", "output format: table, json")
	rootCmd.AddCommand(statusCmd)
}

// statusOutput holds the complete status data for JSON output.
type statusOutput struct {
	Runtime    *runtime.RuntimeInfo `json:"runtime"`
	Summary    engine.HostSummary   `json:"summary"`
	Containers []containerStatus    `json:"containers"`
}

type containerStatus struct {
	models.Container
	CPU    float64 `json:"cpu_percent"`
	Memory uint64  `json:"memory_bytes"`
	MemPct float64 `json:"memory_percent"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := detectClient(ctx)
	if err != nil {
		return fmt.Errorf("detecting runtime: %w", err)
	}

	// Single container mode
	if len(args) == 1 {
		return runStatusSingle(ctx, client, args[0])
	}

	eng := engine.New(client, engine.DefaultConfig())

	snapshot, err := eng.CollectOnce(ctx)
	if err != nil {
		return fmt.Errorf("collecting status: %w", err)
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

	output := statusOutput{
		Runtime:    info,
		Summary:    snapshot.Host,
		Containers: statuses,
	}

	if formatFlag == "json" {
		return printStatusJSON(output)
	}

	printStatusTable(output)

	// Filter to running containers for sampling
	var running []models.Container
	for _, c := range snapshot.Containers {
		if c.IsRunning() {
			running = append(running, c)
		}
	}

	if len(running) > 0 {
		// Collect a few quick samples for lightweight charts.
		sampleCount := 8
		fmt.Printf("\n\033[33mSampling fleet metrics - %d snapshots across %d running containers...\033[0m", sampleCount, len(running))
		samples := collectSamples(ctx, client, running, sampleCount, 0)
		printFleetSparklines(samples, snapshot.Containers)
	}

	return nil
}

func runStatusSingle(ctx context.Context, client runtime.Client, nameOrID string) error {
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

	detail, err := client.InspectContainer(ctx, target.ID)
	if err != nil {
		if target.IsRunning() {
			return fmt.Errorf("inspecting container: %w", err)
		}
		detail = containerDetailFromSummary(*target)
	} else {
		detail = mergeContainerDetail(detail, *target)
	}

	if formatFlag == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(detail)
	}

	// Compact info
	fmt.Printf("%s  %s  %s  %s\n", detail.Name, detail.ShortID(), detail.Image, detail.Status)
	fmt.Printf("  Runtime: %s  Created: %s\n", detail.Runtime, detail.Created.Format("2006-01-02 15:04:05"))

	// Live stats if running
	if target.IsRunning() {
		stats, _ := client.ContainerStats(ctx, target.ID)
		if stats != nil {
			fmt.Printf("  CPU: %.1f%%  Memory: %s / %s (%.1f%%)  PIDs: %d\n",
				stats.CPU.Percent,
				formatBytes(stats.Memory.Usage), formatBytes(stats.Memory.Limit),
				stats.Memory.Percent, stats.PIDs)
			fmt.Printf("  Net: %s rx / %s tx  Block: %s read / %s write\n",
				formatBytes(stats.NetworkIO.RxBytes), formatBytes(stats.NetworkIO.TxBytes),
				formatBytes(stats.BlockIO.ReadBytes), formatBytes(stats.BlockIO.WriteBytes))
		}

		// Collect a few quick samples and keep the progress visible.
		fmt.Printf("\n  \033[33mSampling container metrics - 6 snapshots for %s...\033[0m", target.Name)
		var cpuVals, memVals, rxVals, txVals []float64
		for i := 0; i < 6; i++ {
			s, err := client.ContainerStats(ctx, target.ID)
			if err != nil {
				fmt.Print(".")
				continue
			}
			cpuVals = append(cpuVals, s.CPU.Percent)
			memVals = append(memVals, float64(s.Memory.Usage))
			rxVals = append(rxVals, float64(s.NetworkIO.RxBytes))
			txVals = append(txVals, float64(s.NetworkIO.TxBytes))
			fmt.Print(".")
		}
		fmt.Println()

		if len(cpuVals) > 0 {
			w := 50
			netMax := maxVal(rxVals)
			if m := maxVal(txVals); m > netMax {
				netMax = m
			}
			if netMax == 0 {
				netMax = 1
			}

			fmt.Println()
			renderTrendBox("CPU", colorBrCyan, cpuVals, "%", w, fmt.Sprintf("Avg: %.1f%%  Peak: %.1f%%", avg(cpuVals), maxVal(cpuVals)))
			fmt.Println()
			renderTrendBox("Memory", colorBrGreen, memVals, "bytes", w, fmt.Sprintf("Avg: %s  Peak: %s", formatBytes(uint64(avg(memVals))), formatBytes(uint64(maxVal(memVals)))))
			fmt.Println()
			renderSparkBox("Network I/O", colorBrMagenta, []sparkBoxLine{
				{Label: "RX", Values: rxVals, Max: netMax, Color: colorBrMagenta, Suffix: formatBytes(uint64(rxVals[len(rxVals)-1]))},
				{Label: "TX", Values: txVals, Max: netMax, Color: colorMagenta, Suffix: formatBytes(uint64(txVals[len(txVals)-1]))},
			}, w)
			fmt.Println()
		}
	}

	return nil
}

func containerDetailFromSummary(container models.Container) *models.ContainerDetail {
	return &models.ContainerDetail{
		Container: container,
	}
}

func mergeContainerDetail(detail *models.ContainerDetail, summary models.Container) *models.ContainerDetail {
	if detail == nil {
		return containerDetailFromSummary(summary)
	}

	if detail.ID == "" {
		detail.ID = summary.ID
	}
	if detail.Name == "" {
		detail.Name = summary.Name
	}
	if detail.Image == "" {
		detail.Image = summary.Image
	}
	if detail.ImageID == "" {
		detail.ImageID = summary.ImageID
	}
	if detail.Status == "" || detail.Status == detail.State {
		detail.Status = summary.Status
	}
	if detail.State == "" {
		detail.State = summary.State
	}
	if detail.Created.IsZero() {
		detail.Created = summary.Created
	}
	if len(detail.Ports) == 0 {
		detail.Ports = summary.Ports
	}
	if detail.Labels == nil {
		detail.Labels = summary.Labels
	}
	if len(detail.Mounts) == 0 {
		detail.Mounts = summary.Mounts
	}
	if detail.Runtime == "" {
		detail.Runtime = summary.Runtime
	}

	return detail
}

func printStatusJSON(output statusOutput) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// fleetSample holds one snapshot of fleet-level metrics.
type fleetSample struct {
	TotalCPU float64
	TotalMem float64
	TotalRx  float64
	TotalTx  float64
	PerCtr   map[string]containerSample
}

type containerSample struct {
	CPU float64
	Mem float64
}

func collectSamples(ctx context.Context, client runtime.Client, running []models.Container, count int, interval time.Duration) []fleetSample {
	var samples []fleetSample

	for i := 0; i < count; i++ {
		if i > 0 && interval > 0 {
			time.Sleep(interval)
		}

		// Collect stats for all containers in parallel
		type result struct {
			name  string
			stats *models.ContainerStats
		}
		ch := make(chan result, len(running))
		for _, c := range running {
			go func(ctr models.Container) {
				stats, err := client.ContainerStats(ctx, ctr.ID)
				if err != nil {
					ch <- result{name: ctr.Name}
					return
				}
				ch <- result{name: ctr.Name, stats: stats}
			}(c)
		}

		sample := fleetSample{PerCtr: make(map[string]containerSample)}
		for range running {
			r := <-ch
			if r.stats == nil {
				continue
			}
			sample.TotalCPU += r.stats.CPU.Percent
			sample.TotalMem += float64(r.stats.Memory.Usage)
			sample.TotalRx += float64(r.stats.NetworkIO.RxBytes)
			sample.TotalTx += float64(r.stats.NetworkIO.TxBytes)
			sample.PerCtr[r.name] = containerSample{
				CPU: r.stats.CPU.Percent,
				Mem: float64(r.stats.Memory.Usage),
			}
		}
		samples = append(samples, sample)
		fmt.Print(".")
	}
	fmt.Println()

	return samples
}

func printFleetSparklines(samples []fleetSample, containers []models.Container) {
	if len(samples) == 0 {
		return
	}

	cpuVals := make([]float64, len(samples))
	memVals := make([]float64, len(samples))
	rxVals := make([]float64, len(samples))
	txVals := make([]float64, len(samples))

	for i, s := range samples {
		cpuVals[i] = s.TotalCPU
		memVals[i] = s.TotalMem
		rxVals[i] = s.TotalRx
		txVals[i] = s.TotalTx
	}

	w := 50
	last := samples[len(samples)-1]
	rxRecent := sampleDeltas(rxVals)
	txRecent := sampleDeltas(txVals)

	fmt.Println()
	renderTrendBox("Fleet CPU", colorBrCyan, cpuVals, "%", w, fmt.Sprintf("Avg: %.1f%%  Peak: %.1f%%", avg(cpuVals), maxVal(cpuVals)))
	fmt.Println()

	renderTrendBox("Fleet Memory", colorBrGreen, memVals, "bytes", w, fmt.Sprintf("Avg: %s  Peak: %s", formatBytes(uint64(avg(memVals))), formatBytes(uint64(maxVal(memVals)))))
	fmt.Println()

	renderSeriesTrendBox("Fleet Network I/O", colorBrMagenta, []trendSeriesLine{
		{Label: "RX", Values: rxRecent, Color: colorBrMagenta, Suffix: formatBytes(uint64(last.TotalRx))},
		{Label: "TX", Values: txRecent, Color: colorMagenta, Suffix: formatBytes(uint64(last.TotalTx))},
	}, w)
	fmt.Println()

	// Per-container CPU on a common 0-100% scale so rows are directly comparable.
	var ctrLines []metricBoxLine
	for _, c := range containers {
		if !c.IsRunning() {
			continue
		}
		lastCPU := 0.0
		peakCPU := 0.0
		for _, s := range samples {
			v := s.PerCtr[c.Name].CPU
			lastCPU = v
			if v > peakCPU {
				peakCPU = v
			}
		}
		name := c.Name
		if len(name) > 18 {
			name = name[:16] + ".."
		}
		ctrLines = append(ctrLines, metricBoxLine{
			Label:  name,
			Value:  lastCPU,
			Max:    100,
			Color:  colorBrYellow,
			Suffix: fmt.Sprintf("%4.1f%%", lastCPU),
		})
		if peakCPU > 0 {
			ctrLines[len(ctrLines)-1].Suffix = fmt.Sprintf("%4.1f%% p%-4.1f", lastCPU, peakCPU)
		}
	}
	renderMetricBox("Per-Container CPU", colorBrYellow, ctrLines, w)
	fmt.Println()
}

func maxVal(vals []float64) float64 {
	m := 0.0
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func minVal(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func sampleDeltas(vals []float64) []float64 {
	if len(vals) == 0 {
		return nil
	}
	if len(vals) == 1 {
		return []float64{0}
	}
	out := make([]float64, len(vals))
	for i := 1; i < len(vals); i++ {
		delta := vals[i] - vals[i-1]
		if delta < 0 {
			delta = 0
		}
		out[i] = delta
	}
	out[0] = out[1]
	return out
}

func printStatusTable(output statusOutput) {
	fmt.Printf("%s %s | Containers: %d (%d running, %d stopped)\n",
		output.Runtime.Name,
		output.Runtime.Version,
		output.Summary.TotalContainers,
		output.Summary.RunningContainers,
		output.Summary.StoppedContainers,
	)
	fmt.Printf("Avg CPU: %.1f%% | Avg Mem: %s | Net I/O: %s rx / %s tx | Block I/O: %s read / %s write\n\n",
		output.Summary.AvgCPUPercent,
		formatBytes(output.Summary.AvgMemoryUsage),
		formatBytes(output.Summary.TotalNetRx),
		formatBytes(output.Summary.TotalNetTx),
		formatBytes(output.Summary.TotalBlockRead),
		formatBytes(output.Summary.TotalBlockWrite),
	)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE\tSTATUS\tCPU\tMEM\tPORTS\tRUNTIME")

	for _, cs := range output.Containers {
		cpu := "-"
		mem := "-"
		if cs.Container.IsRunning() {
			cpu = fmt.Sprintf("[%s] %.1f%%", cpuBar(cs.CPU, 10), cs.CPU)
			mem = fmt.Sprintf("[%s] %s", memBar(cs.MemPct, 10), formatBytes(cs.Memory))
		}

		ports := formatPorts(cs.Container.Ports)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			cs.Container.Name,
			truncate(cs.Container.Image, 30),
			cs.Container.Status,
			cpu,
			mem,
			ports,
			cs.Container.Runtime,
		)
	}

	w.Flush()
}

// cpuBar renders an ASCII bar for CPU percentage.
func cpuBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("#", filled) + strings.Repeat(".", width-filled)
}

// memBar renders an ASCII bar for memory percentage.
func memBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("#", filled) + strings.Repeat(".", width-filled)
}

func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatPorts(ports []models.Port) string {
	if len(ports) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.HostPort > 0 {
			parts = append(parts, p.String())
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
