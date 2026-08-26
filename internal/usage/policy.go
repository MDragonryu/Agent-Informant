package usage

import (
	"fmt"
	"time"
)

type Policy struct {
	DrainingRemaining   float64
	CriticalRemaining   float64
	RateDrainingMinutes float64
	RateCriticalMinutes float64
	Messages            Messages
}

type Messages struct {
	Green    string
	Draining string
	Critical string
}

func DefaultMessages() Messages {
	return Messages{
		Green:    "Usage headroom is sufficient for normal operation.",
		Draining: "Finish the current coherent unit of work, avoid substantial new work or delegation, then checkpoint before continuing later.",
		Critical: "Stop implementation at the nearest safe point. Make the current state coherent, record a handoff/checkpoint, and do not start new work.",
	}
}

func DefaultPolicy() Policy {
	return Policy{
		DrainingRemaining:   25,
		CriticalRemaining:   10,
		RateDrainingMinutes: 30,
		RateCriticalMinutes: 10,
		Messages:            DefaultMessages(),
	}
}

func (p Policy) Evaluate(snapshot Snapshot) (Advice, error) {
	return p.EvaluateWithHistory(snapshot, nil, time.Now().UTC())
}

func (p Policy) EvaluateWithHistory(snapshot Snapshot, history []Snapshot, now time.Time) (Advice, error) {
	if len(snapshot.Windows) == 0 {
		return Advice{}, fmt.Errorf("no usable usage windows found")
	}
	if p.CriticalRemaining < 0 || p.DrainingRemaining < 0 || p.CriticalRemaining > p.DrainingRemaining || p.DrainingRemaining > 100 {
		return Advice{}, fmt.Errorf("invalid thresholds: require 0 <= critical <= draining <= 100")
	}
	if p.RateCriticalMinutes < 0 || p.RateDrainingMinutes < 0 || p.RateCriticalMinutes > p.RateDrainingMinutes {
		return Advice{}, fmt.Errorf("invalid rate thresholds: require 0 <= rate critical <= rate draining")
	}

	messages := p.Messages
	defaults := DefaultMessages()
	if messages.Green == "" {
		messages.Green = defaults.Green
	}
	if messages.Draining == "" {
		messages.Draining = defaults.Draining
	}
	if messages.Critical == "" {
		messages.Critical = defaults.Critical
	}

	worst := snapshot.Windows[0]
	for _, w := range snapshot.Windows[1:] {
		if w.PercentRemaining < worst.PercentRemaining {
			worst = w
		}
	}

	advice := Advice{Snapshot: snapshot, WorstWindow: &worst}
	state := StateGreen
	switch {
	case worst.PercentRemaining <= p.CriticalRemaining:
		state = StateCritical
	case worst.PercentRemaining <= p.DrainingRemaining:
		state = StateDraining
	}

	if len(history) > 0 {
		advice.DrainRate = AnalyzeDrainRate(history, worst, p.CriticalRemaining, now)
		if advice.DrainRate != nil && advice.DrainRate.EstimatedMinutesToExhaust != nil {
			eta := *advice.DrainRate.EstimatedMinutesToExhaust
			switch {
			case p.RateCriticalMinutes > 0 && eta <= p.RateCriticalMinutes:
				state = StateCritical
			case p.RateDrainingMinutes > 0 && eta <= p.RateDrainingMinutes && state == StateGreen:
				state = StateDraining
			}
		}
	}

	advice.State = state
	switch state {
	case StateCritical:
		advice.Action = "checkpoint-and-stop"
		advice.Message = messages.Critical
	case StateDraining:
		advice.Action = "finish-current-work"
		advice.Message = messages.Draining
	default:
		advice.Action = "continue"
		advice.Message = messages.Green
	}
	if advice.DrainRate != nil {
		advice.Message += " " + advice.DrainRate.Summary()
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
