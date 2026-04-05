# Advisor Rules

12 performance recommendations based on observed container metrics and configuration.

| ID | Recommendation | Trigger |
|----|---------------|---------|
| PERF-001 | Set memory limit | No limit + usage > 512MB |
| PERF-002 | Reduce memory limit | Peak usage < 30% of limit |
| PERF-003 | Increase memory limit | Usage consistently > 85% of limit |
| PERF-004 | Set CPU limit | No limit + CPU spikes > 50% |
| PERF-005 | Container restart loop | > 3 restarts |
| PERF-006 | Zombie container | 0% CPU for extended period |
| PERF-007 | Excessive logging | Log output > 100MB |
| PERF-008 | Image layer bloat | Image > 1GB |
| PERF-009 | Dangling images cleanup | > 5 untagged images |
| PERF-010 | Orphaned volumes | Volumes not attached to any container |
| PERF-011 | Add health check | No healthcheck on long-running container |
| PERF-012 | Network mode optimization | 3+ containers on default bridge network |
