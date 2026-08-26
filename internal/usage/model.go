package usage

import "time"

type Window struct {
	Provider         string     `json:"provider"`
	Name             string     `json:"name"`
	PercentUsed      float64    `json:"percent_used"`
	PercentRemaining float64    `json:"percent_remaining"`
	ResetAt          *time.Time `json:"reset_at,omitempty"`
	Source           string     `json:"source"`
}

type Snapshot struct {
	CollectedAt time.Time  `json:"collected_at"`
	Windows     []Window   `json:"windows"`
	History     []Snapshot `json:"-"`
}

type State string

const (
	StateGreen    State = "green"
	StateDraining State = "draining"
	StateCritical State = "critical"
)

type Advice struct {
	State       State      `json:"state"`
	Action      string     `json:"action"`
	Message     string     `json:"message"`
	WorstWindow *Window    `json:"worst_window,omitempty"`
	DrainRate   *DrainRate `json:"drain_rate,omitempty"`
	Snapshot    Snapshot   `json:"snapshot"`
}
