package runtime

import (
	"fmt"
	"log/slog"

	cgroupsv2 "github.com/containerd/cgroups/v3/cgroup2/stats"

	"github.com/mathesh-me/wharfeye/internal/models"
)

// parseCgroupStats extracts metrics from cgroup stats data into our common model.
func parseCgroupStats(stats *models.ContainerStats, data interface{}) {
	switch v := data.(type) {
	case *cgroupsv2.Metrics:
		parseCgroupV2(stats, v)
	default:
		slog.Debug("unknown cgroup metrics type", "type", fmt.Sprintf("%T", data))
	}
}

// parseCgroupV2 parses cgroup v2 metrics.
func parseCgroupV2(stats *models.ContainerStats, m *cgroupsv2.Metrics) {
	if m.CPU != nil {
		stats.CPU.System = m.CPU.SystemUsec * 1000 // convert to nanoseconds
		stats.CPU.User = m.CPU.UserUsec * 1000
		// CPU percent calculation requires two samples; set raw values for now
		totalUsage := m.CPU.UsageUsec * 1000
		_ = totalUsage // will be used by collector for delta calculation
	}

	if m.Memory != nil {
		stats.Memory.Usage = m.Memory.Usage
		stats.Memory.Limit = m.Memory.UsageLimit
		if m.Memory.UsageLimit > 0 && m.Memory.UsageLimit != ^uint64(0) {
			stats.Memory.Percent = float64(m.Memory.Usage) / float64(m.Memory.UsageLimit) * 100.0
		}
	}

	if m.Pids != nil {
		stats.PIDs = int64(m.Pids.Current)
	}

	// Network stats are not available through cgroup metrics
	// They need to be read from /proc/<pid>/net/dev
	// For now, leave as zero

	if m.Io != nil {
		for _, entry := range m.Io.Usage {
			stats.BlockIO.ReadBytes += entry.Rbytes
			stats.BlockIO.WriteBytes += entry.Wbytes
		}
	}
}
