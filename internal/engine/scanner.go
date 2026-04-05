package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// Scanner performs security analysis on containers.
type Scanner struct {
	client runtime.Client
}

// NewScanner creates a new security scanner.
func NewScanner(client runtime.Client) *Scanner {
	return &Scanner{client: client}
}

// ScanFleet performs security analysis on all containers.
func (s *Scanner) ScanFleet(ctx context.Context) (*models.FleetSecurityReport, error) {
	containers, err := s.client.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	reports := make([]models.ContainerSecurityReport, 0, len(containers))
	var summary models.SecuritySummary
	summary.TotalContainers = len(containers)

	totalScore := 0

	for _, ctr := range containers {
		report, err := s.ScanContainer(ctx, ctr)
		if err != nil {
			slog.Warn("scanning container", "name", ctr.Name, "error", err)
			continue
		}

		for _, c := range report.Checks {
			if c.Passed {
				summary.PassedChecks++
			} else {
				summary.FailedChecks++
				switch c.Severity {
				case models.SeverityCritical:
					summary.CriticalIssues++
				case models.SeverityHigh:
					summary.HighIssues++
				case models.SeverityMedium:
					summary.MediumIssues++
				case models.SeverityLow:
					summary.LowIssues++
				}
			}
		}

		totalScore += report.Score
		reports = append(reports, *report)
	}

	fleetScore := 0
	if len(reports) > 0 {
		fleetScore = totalScore / len(reports)
	}

	return &models.FleetSecurityReport{
		Containers: reports,
		FleetScore: fleetScore,
		Summary:    summary,
	}, nil
}

// ScanContainer runs all security checks on a single container.
func (s *Scanner) ScanContainer(ctx context.Context, ctr models.Container) (*models.ContainerSecurityReport, error) {
	detail, err := s.client.InspectContainer(ctx, ctr.ID)
	if err != nil {
		return nil, fmt.Errorf("inspecting container %s: %w", ctr.Name, err)
	}

	checks := runSecurityChecks(detail)
	score := models.CalculateScore(checks)

	report := &models.ContainerSecurityReport{
		ContainerID:   ctr.ID,
		ContainerName: ctr.Name,
		Image:         ctr.Image,
		Checks:        checks,
		Score:         score,
	}

	return report, nil
}

func runSecurityChecks(detail *models.ContainerDetail) []models.SecurityCheck {
	checks := make([]models.SecurityCheck, 0, 20)

	checks = append(checks, checkRunningAsRoot(detail))
	checks = append(checks, checkPrivileged(detail))
	checks = append(checks, checkAllCapabilities(detail))
	checks = append(checks, checkHostNetwork(detail))
	checks = append(checks, checkHostPID(detail))
	checks = append(checks, checkWritableRootFS(detail))
	checks = append(checks, checkNoHealthCheck(detail))
	checks = append(checks, checkNoResourceLimits(detail))
	checks = append(checks, checkSensitiveMount(detail))
	checks = append(checks, checkNoSecurityOptions(detail))
	checks = append(checks, checkExposedPorts(detail))
	checks = append(checks, checkLatestTag(detail))
	checks = append(checks, checkNoRestartPolicy(detail))
	checks = append(checks, checkExcessiveCapabilities(detail))
	checks = append(checks, checkNoUserNamespace(detail))
	checks = append(checks, checkHostIPC(detail))
	checks = append(checks, checkNoPIDLimit(detail))
	checks = append(checks, checkSecretsInEnv(detail))
	checks = append(checks, checkSeccompDisabled(detail))
	checks = append(checks, checkPrivilegedPorts(detail))

	return checks
}

func checkRunningAsRoot(d *models.ContainerDetail) models.SecurityCheck {
	passed := d.Config.User != "" && d.Config.User != "root" && d.Config.User != "0"
	details := ""
	if !passed {
		user := d.Config.User
		if user == "" {
			user = "root (default)"
		}
		details = fmt.Sprintf("Container runs as user: %s", user)
	}
	return models.SecurityCheck{
		ID:          "SEC-001",
		Name:        "Running as root",
		Description: "Container runs as UID 0",
		Severity:    models.SeverityHigh,
		Passed:      passed,
		Details:     details,
	}
}

func checkPrivileged(d *models.ContainerDetail) models.SecurityCheck {
	passed := !d.HostConfig.Privileged
	details := ""
	if !passed {
		details = "--privileged flag enabled"
	}
	return models.SecurityCheck{
		ID:          "SEC-002",
		Name:        "Privileged mode",
		Description: "--privileged flag enabled",
		Severity:    models.SeverityCritical,
		Passed:      passed,
		Details:     details,
	}
}

func checkAllCapabilities(d *models.ContainerDetail) models.SecurityCheck {
	hasAll := false
	for _, cap := range d.HostConfig.CapAdd {
		if strings.ToUpper(cap) == "ALL" {
			hasAll = true
			break
		}
	}
	details := ""
	if hasAll {
		details = "--cap-add=ALL detected"
	}
	return models.SecurityCheck{
		ID:          "SEC-003",
		Name:        "All capabilities",
		Description: "--cap-add=ALL detected",
		Severity:    models.SeverityCritical,
		Passed:      !hasAll,
		Details:     details,
	}
}

func checkHostNetwork(d *models.ContainerDetail) models.SecurityCheck {
	isHost := d.NetworkMode == "host"
	details := ""
	if isHost {
		details = "Container uses --network=host"
	}
	return models.SecurityCheck{
		ID:          "SEC-004",
		Name:        "Host network",
		Description: "--network=host mode",
		Severity:    models.SeverityHigh,
		Passed:      !isHost,
		Details:     details,
	}
}

func checkHostPID(d *models.ContainerDetail) models.SecurityCheck {
	isHost := d.HostConfig.PidMode == "host"
	details := ""
	if isHost {
		details = "Container uses --pid=host"
	}
	return models.SecurityCheck{
		ID:          "SEC-005",
		Name:        "Host PID namespace",
		Description: "--pid=host enabled",
		Severity:    models.SeverityHigh,
		Passed:      !isHost,
		Details:     details,
	}
}

func checkWritableRootFS(d *models.ContainerDetail) models.SecurityCheck {
	passed := d.HostConfig.ReadonlyRootfs
	details := ""
	if !passed {
		details = "Root filesystem is writable (ReadonlyRootfs not set)"
	}
	return models.SecurityCheck{
		ID:          "SEC-006",
		Name:        "Writable root filesystem",
		Description: "ReadonlyRootfs not set",
		Severity:    models.SeverityMedium,
		Passed:      passed,
		Details:     details,
	}
}

func checkNoHealthCheck(d *models.ContainerDetail) models.SecurityCheck {
	passed := d.Config.Healthcheck != nil
	details := ""
	if !passed {
		details = "No HEALTHCHECK defined"
	}
	return models.SecurityCheck{
		ID:          "SEC-007",
		Name:        "No health check",
		Description: "No HEALTHCHECK defined",
		Severity:    models.SeverityLow,
		Passed:      passed,
		Details:     details,
	}
}

func checkNoResourceLimits(d *models.ContainerDetail) models.SecurityCheck {
	hasMemLimit := d.HostConfig.MemoryLimit > 0
	hasCPULimit := d.HostConfig.HasCPULimit()
	passed := hasMemLimit && hasCPULimit
	details := ""
	if !passed {
		missing := []string{}
		if !hasMemLimit {
			missing = append(missing, "memory")
		}
		if !hasCPULimit {
			missing = append(missing, "CPU")
		}
		details = fmt.Sprintf("No %s limits set", strings.Join(missing, "/"))
	}
	return models.SecurityCheck{
		ID:          "SEC-008",
		Name:        "No resource limits",
		Description: "No CPU/memory limits set",
		Severity:    models.SeverityMedium,
		Passed:      passed,
		Details:     details,
	}
}

func checkSensitiveMount(d *models.ContainerDetail) models.SecurityCheck {
	sensitivePaths := []string{
		"/var/run/docker.sock",
		"/run/docker.sock",
		"/etc/shadow",
		"/etc/passwd",
		"/run/podman/podman.sock",
		"/run/containerd/containerd.sock",
	}

	found := []string{}
	for _, m := range d.Mounts {
		for _, sp := range sensitivePaths {
			if m.Source == sp || m.Destination == sp {
				found = append(found, m.Source)
			}
		}
	}

	passed := len(found) == 0
	details := ""
	if !passed {
		details = fmt.Sprintf("Sensitive paths mounted: %s", strings.Join(found, ", "))
	}
	return models.SecurityCheck{
		ID:          "SEC-009",
		Name:        "Sensitive mount",
		Description: "Docker socket or sensitive paths mounted",
		Severity:    models.SeverityHigh,
		Passed:      passed,
		Details:     details,
	}
}

func checkNoSecurityOptions(d *models.ContainerDetail) models.SecurityCheck {
	passed := len(d.HostConfig.SecurityOpt) > 0
	details := ""
	if !passed {
		details = "No AppArmor/Seccomp profile configured"
	}
	return models.SecurityCheck{
		ID:          "SEC-010",
		Name:        "No security options",
		Description: "No AppArmor/Seccomp profile",
		Severity:    models.SeverityMedium,
		Passed:      passed,
		Details:     details,
	}
}

func checkExposedPorts(d *models.ContainerDetail) models.SecurityCheck {
	exposed := []string{}
	for _, p := range d.Ports {
		if p.HostPort > 0 && (p.IP == "" || p.IP == "0.0.0.0" || p.IP == "::") {
			exposed = append(exposed, fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
		}
	}

	passed := len(exposed) == 0
	details := ""
	if !passed {
		details = fmt.Sprintf("Ports bound to all interfaces: %s", strings.Join(exposed, ", "))
	}
	return models.SecurityCheck{
		ID:          "SEC-011",
		Name:        "Exposed ports on 0.0.0.0",
		Description: "Ports bound to all interfaces",
		Severity:    models.SeverityMedium,
		Passed:      passed,
		Details:     details,
	}
}

func checkLatestTag(d *models.ContainerDetail) models.SecurityCheck {
	image := d.Image
	hasLatest := strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":")
	details := ""
	if hasLatest {
		details = fmt.Sprintf("Image uses :latest or no tag: %s", image)
	}
	return models.SecurityCheck{
		ID:          "SEC-012",
		Name:        "Using :latest tag",
		Description: "No pinned image version",
		Severity:    models.SeverityLow,
		Passed:      !hasLatest,
		Details:     details,
	}
}

func checkNoRestartPolicy(d *models.ContainerDetail) models.SecurityCheck {
	policy := d.HostConfig.RestartPolicy
	passed := policy != "" && policy != "no"
	details := ""
	if !passed {
		details = "No restart policy set - container won't auto-recover"
	}
	return models.SecurityCheck{
		ID:          "SEC-013",
		Name:        "No restart policy",
		Description: "Container won't auto-recover",
		Severity:    models.SeverityLow,
		Passed:      passed,
		Details:     details,
	}
}

func checkExcessiveCapabilities(d *models.ContainerDetail) models.SecurityCheck {
	// Check for common overly-permissive capabilities
	dangerousCaps := map[string]bool{
		"SYS_ADMIN":  true,
		"SYS_PTRACE": true,
		"NET_ADMIN":  true,
		"SYS_RAWIO":  true,
		"DAC_OVERRIDE": true,
	}

	found := []string{}
	for _, cap := range d.HostConfig.CapAdd {
		capUpper := strings.ToUpper(cap)
		if capUpper == "ALL" {
			continue // handled by SEC-003
		}
		if dangerousCaps[capUpper] {
			found = append(found, capUpper)
		}
	}

	passed := len(found) == 0
	details := ""
	if !passed {
		details = fmt.Sprintf("Unnecessary capabilities: %s", strings.Join(found, ", "))
	}
	return models.SecurityCheck{
		ID:          "SEC-014",
		Name:        "Excessive capabilities",
		Description: "Unnecessary capabilities added",
		Severity:    models.SeverityMedium,
		Passed:      passed,
		Details:     details,
	}
}

func checkNoUserNamespace(d *models.ContainerDetail) models.SecurityCheck {
	// This check is a best-effort heuristic: if the container runs as root
	// without user namespace remapping, it's a concern
	// In practice, user namespace remap is a daemon-level setting
	passed := d.Config.User != "" && d.Config.User != "root" && d.Config.User != "0"
	details := ""
	if !passed {
		details = "User namespace remapping not enabled; container runs as host root"
	}
	return models.SecurityCheck{
		ID:          "SEC-015",
		Name:        "No user namespace remap",
		Description: "User namespace remapping not enabled",
		Severity:    models.SeverityLow,
		Passed:      passed,
		Details:     details,
	}
}

// SEC-016: Host IPC namespace
func checkHostIPC(d *models.ContainerDetail) models.SecurityCheck {
	passed := d.HostConfig.IpcMode != "host"
	details := ""
	if !passed {
		details = "Container shares host IPC namespace - can access host shared memory"
	}
	return models.SecurityCheck{
		ID:          "SEC-016",
		Name:        "Host IPC namespace",
		Description: "Container uses --ipc=host",
		Severity:    models.SeverityHigh,
		Passed:      passed,
		Details:     details,
	}
}

// SEC-017: No PID limit
func checkNoPIDLimit(d *models.ContainerDetail) models.SecurityCheck {
	passed := d.HostConfig.PidsLimit > 0
	details := ""
	if !passed {
		details = "No PID limit set - container can fork-bomb the host"
	}
	return models.SecurityCheck{
		ID:          "SEC-017",
		Name:        "No PID limit",
		Description: "No --pids-limit set",
		Severity:    models.SeverityMedium,
		Passed:      passed,
		Details:     details,
	}
}

// SEC-018: Secrets in environment variables
func checkSecretsInEnv(d *models.ContainerDetail) models.SecurityCheck {
	sensitiveKeys := []string{
		"PASSWORD", "SECRET", "TOKEN", "API_KEY", "APIKEY",
		"ACCESS_KEY", "PRIVATE_KEY", "CREDENTIAL",
	}

	var found []string
	for _, env := range d.Config.Env {
		key := env
		if idx := strings.Index(env, "="); idx > 0 {
			key = env[:idx]
		}
		upper := strings.ToUpper(key)
		for _, s := range sensitiveKeys {
			if strings.Contains(upper, s) {
				found = append(found, key)
				break
			}
		}
	}

	passed := len(found) == 0
	details := ""
	if !passed {
		details = fmt.Sprintf("Possible secrets in env vars: %s", strings.Join(found, ", "))
	}
	return models.SecurityCheck{
		ID:          "SEC-018",
		Name:        "Secrets in environment",
		Description: "Credentials may be exposed via environment variables",
		Severity:    models.SeverityHigh,
		Passed:      passed,
		Details:     details,
	}
}

// SEC-019: Seccomp explicitly disabled
func checkSeccompDisabled(d *models.ContainerDetail) models.SecurityCheck {
	disabled := false
	for _, opt := range d.HostConfig.SecurityOpt {
		if strings.Contains(opt, "seccomp=unconfined") || strings.Contains(opt, "seccomp:unconfined") {
			disabled = true
			break
		}
	}

	return models.SecurityCheck{
		ID:          "SEC-019",
		Name:        "Seccomp disabled",
		Description: "Default seccomp profile explicitly disabled",
		Severity:    models.SeverityHigh,
		Passed:      !disabled,
		Details: func() string {
			if disabled {
				return "Running with seccomp=unconfined removes syscall filtering"
			}
			return ""
		}(),
	}
}

// SEC-020: Privileged port mapping
func checkPrivilegedPorts(d *models.ContainerDetail) models.SecurityCheck {
	var privPorts []string
	for _, p := range d.Ports {
		if p.HostPort > 0 && p.HostPort < 1024 {
			privPorts = append(privPorts, fmt.Sprintf("%d", p.HostPort))
		}
	}

	passed := len(privPorts) == 0
	details := ""
	if !passed {
		details = fmt.Sprintf("Privileged host ports mapped: %s", strings.Join(privPorts, ", "))
	}
	return models.SecurityCheck{
		ID:          "SEC-020",
		Name:        "Privileged port mapping",
		Description: "Container maps to host ports below 1024",
		Severity:    models.SeverityLow,
		Passed:      passed,
		Details:     details,
	}
}

