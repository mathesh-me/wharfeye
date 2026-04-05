package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// Advisor analyzes collected metrics and generates performance recommendations.
type Advisor struct {
	client    runtime.Client
	collector *Collector
}

// NewAdvisor creates a new performance advisor.
func NewAdvisor(client runtime.Client, collector *Collector) *Advisor {
	return &Advisor{
		client:    client,
		collector: collector,
	}
}

// Analyze generates recommendations based on current state and metric history.
func (a *Advisor) Analyze(ctx context.Context) (*models.AdvisorReport, error) {
	containers, err := a.client.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var recs []models.Recommendation

	// Per-container analysis
	for _, ctr := range containers {
		detail, err := a.client.InspectContainer(ctx, ctr.ID)
		if err != nil {
			slog.Warn("inspecting container for advisor", "name", ctr.Name, "error", err)
			continue
		}

		// Non-running containers: only check restart loop
		if !ctr.IsRunning() {
			if r := checkRestartLoop(ctr, detail); r != nil {
				recs = append(recs, *r)
			}
			continue
		}

		stats, err := a.client.ContainerStats(ctx, ctr.ID)
		if err != nil {
			slog.Warn("getting stats for advisor", "name", ctr.Name, "error", err)
			continue
		}

		// Get historical stats from collector
		var cpuHistory []float64
		var memHistory []float64
		if a.collector != nil {
			for _, snap := range a.collector.History() {
				if s, ok := snap.Stats[ctr.ID]; ok {
					cpuHistory = append(cpuHistory, s.CPU.Percent)
					memHistory = append(memHistory, float64(s.Memory.Usage))
				}
			}
		}

		recs = append(recs, a.analyzeContainer(ctr, detail, stats, cpuHistory, memHistory)...)
	}

	// Fleet-level analysis
	recs = append(recs, a.analyzeFleet(ctx)...)

	// Sort by priority (high first)
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].Priority.Score() > recs[j].Priority.Score()
	})

	report := &models.AdvisorReport{
		Recommendations: recs,
	}
	for _, r := range recs {
		switch r.Priority {
		case models.PriorityHigh:
			report.HighCount++
		case models.PriorityMedium:
			report.MediumCount++
		case models.PriorityLow:
			report.LowCount++
		}
	}

	return report, nil
}

// analyzeContainer runs all per-container rules.
func (a *Advisor) analyzeContainer(ctr models.Container, detail *models.ContainerDetail,
	stats *models.ContainerStats, cpuHistory, memHistory []float64) []models.Recommendation {

	var recs []models.Recommendation

	if r := checkSetMemoryLimit(ctr, detail, stats); r != nil {
		recs = append(recs, *r)
	}
	if r := checkReduceMemoryLimit(ctr, detail, stats, memHistory); r != nil {
		recs = append(recs, *r)
	}
	if r := checkIncreaseMemoryLimit(ctr, detail, stats, memHistory); r != nil {
		recs = append(recs, *r)
	}
	if r := checkSetCPULimit(ctr, detail, stats, cpuHistory); r != nil {
		recs = append(recs, *r)
	}
	if r := checkRestartLoop(ctr, detail); r != nil {
		recs = append(recs, *r)
	}
	if r := checkZombieContainer(ctr, stats, cpuHistory); r != nil {
		recs = append(recs, *r)
	}
	if r := checkExcessiveLogging(ctr, detail); r != nil {
		recs = append(recs, *r)
	}
	if r := checkAddHealthCheck(ctr, detail); r != nil {
		recs = append(recs, *r)
	}

	return recs
}

// analyzeFleet runs fleet-level rules.
func (a *Advisor) analyzeFleet(ctx context.Context) []models.Recommendation {
	var recs []models.Recommendation

	if r := a.checkImageBloat(ctx); r != nil {
		recs = append(recs, r...)
	}
	if r := a.checkDanglingImages(ctx); r != nil {
		recs = append(recs, *r)
	}
	if r := a.checkOrphanedVolumes(ctx); r != nil {
		recs = append(recs, *r)
	}
	if r := a.checkNetworkOptimization(ctx); r != nil {
		recs = append(recs, *r)
	}

	return recs
}

// PERF-001: Set memory limit
func checkSetMemoryLimit(ctr models.Container, detail *models.ContainerDetail, stats *models.ContainerStats) *models.Recommendation {
	if detail.HostConfig.MemoryLimit > 0 {
		return nil
	}
	const threshold = 512 * 1024 * 1024 // 512MB
	if stats.Memory.Usage <= uint64(threshold) {
		return nil
	}

	return &models.Recommendation{
		ID:            "PERF-001",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Set memory limit",
		Description:   fmt.Sprintf("Container %q uses %s memory without a limit set", ctr.Name, formatMB(stats.Memory.Usage)),
		Priority:      models.PriorityHigh,
		Category:      "resources",
		Impact:        "Prevents unbounded memory growth that could affect other containers",
		Action:        fmt.Sprintf("Add --memory=%dM to container configuration", stats.Memory.Usage/(1024*1024)*2),
		Reference:     "https://docs.docker.com/reference/cli/docker/container/run/#memory",
	}
}

// PERF-002: Reduce memory limit
func checkReduceMemoryLimit(ctr models.Container, detail *models.ContainerDetail, stats *models.ContainerStats, memHistory []float64) *models.Recommendation {
	if detail.HostConfig.MemoryLimit <= 0 {
		return nil
	}

	limit := float64(detail.HostConfig.MemoryLimit)

	if len(memHistory) < 10 {
		// Not enough history, check current
		if stats.Memory.Percent >= 30 {
			return nil
		}
	} else {
		peak := 0.0
		for _, v := range memHistory {
			if v > peak {
				peak = v
			}
		}
		if peak/limit*100 >= 30 {
			return nil
		}
	}

	return &models.Recommendation{
		ID:            "PERF-002",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Reduce memory limit",
		Description:   fmt.Sprintf("Container %q peak memory usage is below 30%% of its %s limit", ctr.Name, formatMB(uint64(limit))),
		Priority:      models.PriorityLow,
		Category:      "resources",
		Impact:        "Frees memory for other containers",
		Action:        "Consider reducing the memory limit to match actual usage patterns",
		Reference:     "https://docs.docker.com/reference/cli/docker/container/run/#memory",
	}
}

// PERF-003: Increase memory limit
func checkIncreaseMemoryLimit(ctr models.Container, detail *models.ContainerDetail, stats *models.ContainerStats, memHistory []float64) *models.Recommendation {
	if detail.HostConfig.MemoryLimit <= 0 {
		return nil
	}

	if len(memHistory) < 5 {
		if stats.Memory.Percent <= 85 {
			return nil
		}
	} else {
		limit := float64(detail.HostConfig.MemoryLimit)
		highCount := 0
		for _, v := range memHistory {
			if v/limit*100 > 85 {
				highCount++
			}
		}
		if float64(highCount)/float64(len(memHistory)) < 0.5 {
			return nil
		}
	}

	return &models.Recommendation{
		ID:            "PERF-003",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Increase memory limit",
		Description:   fmt.Sprintf("Container %q consistently uses >85%% of its memory limit", ctr.Name),
		Priority:      models.PriorityHigh,
		Category:      "resources",
		Impact:        "Prevents OOM kills and performance degradation",
		Action:        fmt.Sprintf("Increase memory limit from %s", formatMB(uint64(detail.HostConfig.MemoryLimit))),
	}
}

// PERF-004: Set CPU limit
func checkSetCPULimit(ctr models.Container, detail *models.ContainerDetail, stats *models.ContainerStats, cpuHistory []float64) *models.Recommendation {
	if detail.HostConfig.HasCPULimit() {
		return nil
	}

	hasSpike := false
	for _, v := range cpuHistory {
		if v > 50 {
			hasSpike = true
			break
		}
	}
	// Fallback: check current CPU stats (for one-shot mode with no history)
	if !hasSpike && stats != nil && stats.CPU.Percent > 50 {
		hasSpike = true
	}

	if !hasSpike {
		return nil
	}

	return &models.Recommendation{
		ID:            "PERF-004",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Set CPU limit",
		Description:   fmt.Sprintf("Container %q has CPU spikes >50%% without a CPU limit", ctr.Name),
		Priority:      models.PriorityMedium,
		Category:      "resources",
		Impact:        "Prevents a single container from starving others of CPU",
		Action:        "Add --cpus or --cpu-quota to limit CPU usage",
		Reference:     "https://docs.docker.com/reference/cli/docker/container/run/#cpus",
	}
}

// PERF-005: Container restart loop
func checkRestartLoop(ctr models.Container, detail *models.ContainerDetail) *models.Recommendation {
	if detail.RestartCount <= 3 {
		return nil
	}

	return &models.Recommendation{
		ID:            "PERF-005",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Container restart loop",
		Description:   fmt.Sprintf("Container %q has restarted %d times", ctr.Name, detail.RestartCount),
		Priority:      models.PriorityHigh,
		Category:      "stability",
		Impact:        "Frequent restarts indicate a crash loop, wasting resources",
		Action:        "Check container logs (docker logs <name>) and fix the root cause of crashes",
		Reference:     "https://docs.docker.com/reference/cli/docker/container/logs/",
	}
}

// PERF-006: Zombie container
func checkZombieContainer(ctr models.Container, stats *models.ContainerStats, cpuHistory []float64) *models.Recommendation {
	allZero := true

	if len(cpuHistory) >= 15 {
		// With sufficient history, check all samples
		for _, v := range cpuHistory {
			if v > 0.1 {
				allZero = false
				break
			}
		}
	} else {
		// Fallback: in one-shot mode, check current stats + container has been running > 5 min
		if stats == nil || stats.CPU.Percent > 0.1 {
			return nil
		}
		if ctr.Created.IsZero() || time.Since(ctr.Created) < 5*time.Minute {
			return nil
		}
	}

	if !allZero {
		return nil
	}

	return &models.Recommendation{
		ID:            "PERF-006",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Zombie container",
		Description:   fmt.Sprintf("Container %q is running but has 0%% CPU usage for an extended period", ctr.Name),
		Priority:      models.PriorityMedium,
		Category:      "resources",
		Impact:        "Container is consuming memory but doing no work",
		Action:        "Verify this container is still needed; consider stopping it",
	}
}

// PERF-007: Excessive logging
func checkExcessiveLogging(ctr models.Container, detail *models.ContainerDetail) *models.Recommendation {
	if detail.LogPath == "" {
		return nil
	}

	info, err := os.Stat(detail.LogPath)
	if err != nil {
		return nil
	}

	const logThreshold = 100 * 1024 * 1024 // 100MB
	if info.Size() < logThreshold {
		return nil
	}

	return &models.Recommendation{
		ID:            "PERF-007",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Excessive logging",
		Description:   fmt.Sprintf("Container %q log file is %s", ctr.Name, formatMB(uint64(info.Size()))),
		Priority:      models.PriorityMedium,
		Category:      "resources",
		Impact:        "Large log files consume disk space and slow down log queries",
		Action:        "Configure log rotation with --log-opt max-size=10m --log-opt max-file=3",
		Reference:     "https://docs.docker.com/engine/logging/configure/",
	}
}

// PERF-008: Image layer bloat
func (a *Advisor) checkImageBloat(ctx context.Context) []models.Recommendation {
	images, err := a.client.ListImages(ctx)
	if err != nil {
		return nil
	}

	var recs []models.Recommendation
	const oneGB = 1024 * 1024 * 1024

	for _, img := range images {
		if img.Size > oneGB {
			name := "unnamed"
			if len(img.Tags) > 0 {
				name = img.Tags[0]
			}
			recs = append(recs, models.Recommendation{
				ID:          "PERF-008",
				Title:       "Image layer bloat",
				Description: fmt.Sprintf("Image %q is %s - consider using multi-stage builds", name, formatMB(uint64(img.Size))),
				Priority:    models.PriorityMedium,
				Category:    "images",
				Impact:      "Large images increase pull time and disk usage",
				Action:      "Use multi-stage builds, minimize layers, and use slim/alpine base images",
			})
		}
	}

	return recs
}

// PERF-009: Dangling images
func (a *Advisor) checkDanglingImages(ctx context.Context) *models.Recommendation {
	images, err := a.client.ListImages(ctx)
	if err != nil {
		return nil
	}

	dangling := 0
	for _, img := range images {
		if len(img.Tags) == 0 || (len(img.Tags) == 1 && img.Tags[0] == "<none>:<none>") {
			dangling++
		}
	}

	if dangling <= 5 {
		return nil
	}

	return &models.Recommendation{
		ID:          "PERF-009",
		Title:       "Dangling images cleanup",
		Description: fmt.Sprintf("%d dangling images consuming disk space", dangling),
		Priority:    models.PriorityLow,
		Category:    "images",
		Impact:      "Frees disk space",
		Action:      "Run 'docker image prune' to remove dangling images",
	}
}

// PERF-010: Orphaned volumes
func (a *Advisor) checkOrphanedVolumes(ctx context.Context) *models.Recommendation {
	volumes, err := a.client.ListVolumes(ctx)
	if err != nil {
		return nil
	}

	orphaned := 0
	for _, v := range volumes {
		if !v.InUse {
			orphaned++
		}
	}

	if orphaned == 0 {
		return nil
	}

	return &models.Recommendation{
		ID:          "PERF-010",
		Title:       "Orphaned volumes",
		Description: fmt.Sprintf("%d volumes not attached to any container", orphaned),
		Priority:    models.PriorityLow,
		Category:    "storage",
		Impact:      "Frees disk space used by unused volumes",
		Action:      "Run 'docker volume prune' to remove orphaned volumes",
	}
}

// checkAddHealthCheck (PERF-011)
func checkAddHealthCheck(ctr models.Container, detail *models.ContainerDetail) *models.Recommendation {
	if detail.Config.Healthcheck != nil {
		return nil
	}

	// Only recommend for long-running containers (those in "running" state)
	if !ctr.IsRunning() {
		return nil
	}

	return &models.Recommendation{
		ID:            "PERF-011",
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Title:         "Add health check",
		Description:   fmt.Sprintf("Container %q has no HEALTHCHECK defined", ctr.Name),
		Priority:      models.PriorityLow,
		Category:      "reliability",
		Impact:        "Health checks enable automatic restart of unhealthy containers",
		Action:        "Add a HEALTHCHECK instruction to your Dockerfile or use --health-cmd",
		Reference:     "https://docs.docker.com/reference/dockerfile/#healthcheck",
	}
}

func formatMB(b uint64) string {
	const (
		mb = 1024 * 1024
		gb = 1024 * mb
	)
	if b >= gb {
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	}
	return fmt.Sprintf("%d MB", b/mb)
}

// PERF-012: Network mode optimization
// that could benefit from a custom network for better DNS-based service discovery.
func (a *Advisor) checkNetworkOptimization(ctx context.Context) *models.Recommendation {
	containers, err := a.client.ListContainers(ctx)
	if err != nil {
		return nil
	}

	bridgeCount := 0
	for _, ctr := range containers {
		if !ctr.IsRunning() {
			continue
		}
		detail, err := a.client.InspectContainer(ctx, ctr.ID)
		if err != nil {
			continue
		}
		if detail.NetworkMode == "default" || detail.NetworkMode == "bridge" {
			bridgeCount++
		}
	}

	if bridgeCount < 3 {
		return nil
	}

	return &models.Recommendation{
		ID:          "PERF-012",
		Title:       "Network mode optimization",
		Description: fmt.Sprintf("%d containers use the default bridge network", bridgeCount),
		Priority:    models.PriorityLow,
		Category:    "networking",
		Impact:      "Custom networks provide automatic DNS resolution between containers and better isolation",
		Action:      "Create a custom bridge network and attach related containers for DNS-based service discovery",
	}
}
