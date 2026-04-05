package models

// Severity represents a security finding severity level.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

// SeverityWeight returns the numeric weight for scoring.
func (s Severity) Weight() int {
	switch s {
	case SeverityCritical:
		return 25
	case SeverityHigh:
		return 15
	case SeverityMedium:
		return 8
	case SeverityLow:
		return 3
	default:
		return 0
	}
}

// SecurityCheck represents a single runtime security check result.
type SecurityCheck struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
	Passed      bool     `json:"passed"`
	Details     string   `json:"details,omitempty"`
}

// ContainerSecurityReport holds all security findings for a single container.
type ContainerSecurityReport struct {
	ContainerID   string          `json:"container_id"`
	ContainerName string          `json:"container_name"`
	Image         string          `json:"image"`
	Checks        []SecurityCheck `json:"checks"`
	Score         int             `json:"score"`
}

// FleetSecurityReport holds security results for all containers.
type FleetSecurityReport struct {
	Containers []ContainerSecurityReport `json:"containers"`
	FleetScore int                       `json:"fleet_score"`
	Summary    SecuritySummary           `json:"summary"`
}

// SecuritySummary holds aggregate counts.
type SecuritySummary struct {
	TotalContainers int `json:"total_containers"`
	CriticalIssues  int `json:"critical_issues"`
	HighIssues      int `json:"high_issues"`
	MediumIssues    int `json:"medium_issues"`
	LowIssues       int `json:"low_issues"`
	PassedChecks    int `json:"passed_checks"`
	FailedChecks    int `json:"failed_checks"`
}

// CalculateScore computes a security score (0-100) from check results.
// 100 = all passed, lower = more/worse failures.
func CalculateScore(checks []SecurityCheck) int {
	if len(checks) == 0 {
		return 100
	}

	maxPenalty := 0
	penalty := 0

	for _, c := range checks {
		w := c.Severity.Weight()
		maxPenalty += w
		if !c.Passed {
			penalty += w
		}
	}

	if maxPenalty == 0 {
		return 100
	}

	score := 100 - (penalty * 100 / maxPenalty)
	if score < 0 {
		score = 0
	}
	return score
}
