package models

// Priority represents recommendation priority level.
type Priority string

const (
	PriorityHigh   Priority = "HIGH"
	PriorityMedium Priority = "MEDIUM"
	PriorityLow    Priority = "LOW"
)

// PriorityScore returns numeric priority for sorting.
func (p Priority) Score() int {
	switch p {
	case PriorityHigh:
		return 3
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 1
	default:
		return 0
	}
}

// Recommendation represents a performance recommendation.
type Recommendation struct {
	ID            string   `json:"id"`
	ContainerID   string   `json:"container_id,omitempty"`
	ContainerName string   `json:"container_name,omitempty"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Priority      Priority `json:"priority"`
	Category      string   `json:"category"`
	Impact        string   `json:"impact"`
	Action        string   `json:"action"`
	Reference     string   `json:"reference,omitempty"`
}

// AdvisorReport holds all recommendations.
type AdvisorReport struct {
	Recommendations []Recommendation `json:"recommendations"`
	HighCount       int              `json:"high_count"`
	MediumCount     int              `json:"medium_count"`
	LowCount        int              `json:"low_count"`
}
