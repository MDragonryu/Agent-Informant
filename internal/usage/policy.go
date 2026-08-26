package usage

import "fmt"

type Policy struct {
	DrainingRemaining float64
	CriticalRemaining float64
}

func DefaultPolicy() Policy {
	return Policy{DrainingRemaining: 25, CriticalRemaining: 10}
}

func (p Policy) Evaluate(snapshot Snapshot) (Advice, error) {
	if len(snapshot.Windows) == 0 {
		return Advice{}, fmt.Errorf("no usable usage windows found")
	}
	if p.CriticalRemaining < 0 || p.DrainingRemaining < 0 || p.CriticalRemaining > p.DrainingRemaining || p.DrainingRemaining > 100 {
		return Advice{}, fmt.Errorf("invalid thresholds: require 0 <= critical <= draining <= 100")
	}

	worst := snapshot.Windows[0]
	for _, w := range snapshot.Windows[1:] {
		if w.PercentRemaining < worst.PercentRemaining {
			worst = w
		}
	}

	advice := Advice{Snapshot: snapshot, WorstWindow: &worst}
	switch {
	case worst.PercentRemaining <= p.CriticalRemaining:
		advice.State = StateCritical
		advice.Action = "checkpoint-and-stop"
		advice.Message = "Stop implementation at the nearest safe point. Make the current state coherent, record a handoff/checkpoint, and do not start new work."
	case worst.PercentRemaining <= p.DrainingRemaining:
		advice.State = StateDraining
		advice.Action = "finish-current-work"
		advice.Message = "Finish the current coherent unit of work, avoid substantial new work or delegation, then checkpoint before continuing later."
	default:
		advice.State = StateGreen
		advice.Action = "continue"
		advice.Message = "Usage headroom is sufficient for normal operation."
	}
	return advice, nil
}

func ExitCode(state State) int {
	switch state {
	case StateDraining:
		return 10
	case StateCritical:
		return 20
	default:
		return 0
	}
}
