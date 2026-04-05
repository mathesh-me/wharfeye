package engine

import (
	"context"
	"testing"

	"github.com/mathesh-me/wharfeye/internal/models"
)

func TestRunSecurityChecks_AllPassing(t *testing.T) {
	// A well-configured container should pass most checks
	detail := &models.ContainerDetail{
		Container: models.Container{
			ID:    "test-id",
			Name:  "secure-app",
			Image: "myapp:v1.2.3",
			Ports: []models.Port{
				{IP: "127.0.0.1", HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			},
		},
		Config: models.ContainerConfig{
			User: "appuser",
			Healthcheck: &models.Healthcheck{
				Test: []string{"CMD", "healthcheck"},
			},
		},
		HostConfig: models.HostConfig{
			Privileged:     false,
			ReadonlyRootfs: true,
			MemoryLimit:    512 * 1024 * 1024,
			CPUQuota:       50000,
			RestartPolicy:  "always",
			SecurityOpt:    []string{"seccomp=default"},
		},
		NetworkMode: "bridge",
	}

	checks := runSecurityChecks(detail)

	if len(checks) != 20 {
		t.Fatalf("expected 20 checks, got %d", len(checks))
	}

	// Count passed
	passed := 0
	for _, c := range checks {
		if c.Passed {
			passed++
		}
	}

	// With good config, most should pass
	if passed < 12 {
		t.Errorf("expected at least 12 passing checks for secure config, got %d", passed)
		for _, c := range checks {
			if !c.Passed {
				t.Logf("FAILED: %s - %s: %s", c.ID, c.Name, c.Details)
			}
		}
	}
}

func TestRunSecurityChecks_AllFailing(t *testing.T) {
	// A poorly configured container should fail many checks
	detail := &models.ContainerDetail{
		Container: models.Container{
			ID:    "test-id",
			Name:  "insecure-app",
			Image: "myapp",
			Ports: []models.Port{
				{IP: "0.0.0.0", HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			},
			Mounts: []models.Mount{
				{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"},
			},
		},
		Config: models.ContainerConfig{
			User: "",
		},
		HostConfig: models.HostConfig{
			Privileged:     true,
			ReadonlyRootfs: false,
			PidMode:        "host",
			CapAdd:         []string{"ALL"},
			MemoryLimit:    0,
			CPUQuota:       0,
			RestartPolicy:  "",
		},
		NetworkMode: "host",
	}

	checks := runSecurityChecks(detail)

	if len(checks) != 20 {
		t.Fatalf("expected 20 checks, got %d", len(checks))
	}

	failed := 0
	for _, c := range checks {
		if !c.Passed {
			failed++
		}
	}

	// Insecure config should fail most checks
	if failed < 10 {
		t.Errorf("expected at least 10 failing checks for insecure config, got %d", failed)
	}
}

func TestCheckRunningAsRoot(t *testing.T) {
	tests := []struct {
		name   string
		user   string
		passed bool
	}{
		{"empty user (root)", "", false},
		{"explicit root", "root", false},
		{"uid 0", "0", false},
		{"non-root user", "appuser", true},
		{"numeric non-root", "1000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &models.ContainerDetail{
				Config: models.ContainerConfig{User: tt.user},
			}
			check := checkRunningAsRoot(d)
			if check.Passed != tt.passed {
				t.Errorf("checkRunningAsRoot(user=%q) passed=%v, want %v", tt.user, check.Passed, tt.passed)
			}
			if check.ID != "SEC-001" {
				t.Errorf("expected ID SEC-001, got %s", check.ID)
			}
		})
	}
}

func TestCheckPrivileged(t *testing.T) {
	d := &models.ContainerDetail{
		HostConfig: models.HostConfig{Privileged: true},
	}
	check := checkPrivileged(d)
	if check.Passed {
		t.Error("privileged container should fail")
	}
	if check.Severity != models.SeverityCritical {
		t.Errorf("expected CRITICAL severity, got %s", check.Severity)
	}

	d.HostConfig.Privileged = false
	check = checkPrivileged(d)
	if !check.Passed {
		t.Error("non-privileged container should pass")
	}
}

func TestCheckAllCapabilities(t *testing.T) {
	d := &models.ContainerDetail{
		HostConfig: models.HostConfig{CapAdd: []string{"ALL"}},
	}
	check := checkAllCapabilities(d)
	if check.Passed {
		t.Error("--cap-add=ALL should fail")
	}

	d.HostConfig.CapAdd = []string{"NET_BIND_SERVICE"}
	check = checkAllCapabilities(d)
	if !check.Passed {
		t.Error("specific cap should pass SEC-003")
	}
}

func TestCheckHostNetwork(t *testing.T) {
	d := &models.ContainerDetail{NetworkMode: "host"}
	check := checkHostNetwork(d)
	if check.Passed {
		t.Error("host network should fail")
	}

	d.NetworkMode = "bridge"
	check = checkHostNetwork(d)
	if !check.Passed {
		t.Error("bridge network should pass")
	}
}

func TestCheckLatestTag(t *testing.T) {
	tests := []struct {
		image  string
		passed bool
	}{
		{"nginx:latest", false},
		{"nginx", false},
		{"nginx:1.25", true},
		{"myrepo/app:v2.1.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			d := &models.ContainerDetail{
				Container: models.Container{Image: tt.image},
			}
			check := checkLatestTag(d)
			if check.Passed != tt.passed {
				t.Errorf("checkLatestTag(%q) passed=%v, want %v", tt.image, check.Passed, tt.passed)
			}
		})
	}
}

func TestCheckSensitiveMount(t *testing.T) {
	d := &models.ContainerDetail{
		Container: models.Container{
			Mounts: []models.Mount{
				{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"},
			},
		},
	}
	check := checkSensitiveMount(d)
	if check.Passed {
		t.Error("docker socket mount should fail")
	}

	d.Mounts = []models.Mount{
		{Source: "/data", Destination: "/app/data"},
	}
	check = checkSensitiveMount(d)
	if !check.Passed {
		t.Error("normal mount should pass")
	}
}

func TestCheckNoResourceLimits(t *testing.T) {
	d := &models.ContainerDetail{
		HostConfig: models.HostConfig{
			MemoryLimit: 0,
			CPUQuota:    0,
		},
	}
	check := checkNoResourceLimits(d)
	if check.Passed {
		t.Error("no limits should fail")
	}

	d.HostConfig.MemoryLimit = 512 * 1024 * 1024
	d.HostConfig.CPUQuota = 50000
	check = checkNoResourceLimits(d)
	if !check.Passed {
		t.Error("with limits should pass")
	}

	// NanoCPUs should also count as CPU limit
	d.HostConfig.CPUQuota = 0
	d.HostConfig.NanoCPUs = 500000000 // --cpus=0.5
	check = checkNoResourceLimits(d)
	if !check.Passed {
		t.Error("with NanoCPUs limit should pass")
	}
}

func TestCheckExcessiveCapabilities(t *testing.T) {
	d := &models.ContainerDetail{
		HostConfig: models.HostConfig{
			CapAdd: []string{"SYS_ADMIN", "NET_ADMIN"},
		},
	}
	check := checkExcessiveCapabilities(d)
	if check.Passed {
		t.Error("dangerous caps should fail")
	}

	d.HostConfig.CapAdd = []string{"NET_BIND_SERVICE"}
	check = checkExcessiveCapabilities(d)
	if !check.Passed {
		t.Error("safe cap should pass")
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		name   string
		checks []models.SecurityCheck
		want   int
	}{
		{
			"all passed",
			[]models.SecurityCheck{
				{Severity: models.SeverityHigh, Passed: true},
				{Severity: models.SeverityMedium, Passed: true},
			},
			100,
		},
		{
			"all failed",
			[]models.SecurityCheck{
				{Severity: models.SeverityHigh, Passed: false},
				{Severity: models.SeverityMedium, Passed: false},
			},
			0,
		},
		{
			"mixed",
			[]models.SecurityCheck{
				{Severity: models.SeverityCritical, Passed: true},
				{Severity: models.SeverityHigh, Passed: false},
				{Severity: models.SeverityLow, Passed: true},
			},
			// max = 25+15+3 = 43, penalty = 15, score = 100 - 15*100/43 = 66
			66,
		},
		{
			"empty",
			nil,
			100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := models.CalculateScore(tt.checks)
			if got != tt.want {
				t.Errorf("CalculateScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScanFleet_WithMockClient(t *testing.T) {
	client := newMockClient()
	scanner := NewScanner(client)

	report, err := scanner.ScanFleet(context.Background())
	if err != nil {
		t.Fatalf("ScanFleet error: %v", err)
	}

	if report.Summary.TotalContainers != 3 {
		t.Errorf("expected 3 containers, got %d", report.Summary.TotalContainers)
	}

	if len(report.Containers) != 3 {
		t.Errorf("expected 3 container reports, got %d", len(report.Containers))
	}

	// Each container should have 20 checks
	for _, cr := range report.Containers {
		if len(cr.Checks) != 20 {
			t.Errorf("container %s: expected 20 checks, got %d", cr.ContainerName, len(cr.Checks))
		}
		if cr.Score < 0 || cr.Score > 100 {
			t.Errorf("container %s: score %d out of range", cr.ContainerName, cr.Score)
		}
	}

	if report.FleetScore < 0 || report.FleetScore > 100 {
		t.Errorf("fleet score %d out of range", report.FleetScore)
	}
}

func TestSeverityWeight(t *testing.T) {
	tests := []struct {
		sev    models.Severity
		weight int
	}{
		{models.SeverityCritical, 25},
		{models.SeverityHigh, 15},
		{models.SeverityMedium, 8},
		{models.SeverityLow, 3},
		{models.Severity("UNKNOWN"), 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.sev), func(t *testing.T) {
			if got := tt.sev.Weight(); got != tt.weight {
				t.Errorf("Weight() = %d, want %d", got, tt.weight)
			}
		})
	}
}
