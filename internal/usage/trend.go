package usage

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	trendLookback = 30 * time.Minute
	trendMinSpan  = 20 * time.Second
)

type DrainRate struct {
	Provider                   string  `json:"provider"`
	Window                     string  `json:"window"`
	Samples                    int     `json:"samples"`
	ObservedSeconds            float64 `json:"observed_seconds"`
	PercentPerHour             float64 `json:"percent_per_hour"`
	EstimatedMinutesToCritical *float64 `json:"estimated_minutes_to_critical,omitempty"`
	EstimatedMinutesToExhaust  *float64 `json:"estimated_minutes_to_exhaust,omitempty"`
	Level                      string  `json:"level"`
}

func AnalyzeDrainRate(history []Snapshot, current Window, criticalRemaining float64, now time.Time) *DrainRate {
	cutoff := now.Add(-trendLookback)
	type point struct {
		time      time.Time
		remaining float64
		resetAt   *time.Time
	}
	points := make([]point, 0)
	for _, snapshot := range history {
		if snapshot.CollectedAt.Before(cutoff) || snapshot.CollectedAt.After(now.Add(time.Minute)) {
			continue
		}
		for _, w := range snapshot.Windows {
			if !strings.EqualFold(w.Provider, current.Provider) || !strings.EqualFold(w.Name, current.Name) {
				continue
			}
			if !sameResetCycle(w.ResetAt, current.ResetAt) {
				continue
			}
			points = append(points, point{time: snapshot.CollectedAt, remaining: w.PercentRemaining, resetAt: w.ResetAt})
		}
	}
	if len(points) < 2 {
		return nil
	}

	first := points[0]
	last := points[len(points)-1]
	span := last.time.Sub(first.time)
	if span < trendMinSpan {
		return nil
	}
	delta := first.remaining - last.remaining
	if delta <= 0.05 { // ignore tiny noise and non-draining samples
		return nil
	}
	rate := delta / span.Hours()
	if rate <= 0 || math.IsInf(rate, 0) || math.IsNaN(rate) {
		return nil
	}

	result := &DrainRate{
		Provider:        current.Provider,
		Window:          current.Name,
		Samples:         len(points),
		ObservedSeconds: span.Seconds(),
		PercentPerHour:  rate,
		Level:           drainRateLevel(rate),
	}
	toExhaust := current.PercentRemaining / rate * 60
	result.EstimatedMinutesToExhaust = &toExhaust
	if current.PercentRemaining > criticalRemaining {
		toCritical := (current.PercentRemaining - criticalRemaining) / rate * 60
		result.EstimatedMinutesToCritical = &toCritical
	} else {
		zero := 0.0
		result.EstimatedMinutesToCritical = &zero
	}
	return result
}

func sameResetCycle(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return math.Abs(a.Sub(*b).Seconds()) < 2
}

func drainRateLevel(rate float64) string {
	switch {
	case rate >= 30:
		return "extreme"
	case rate >= 10:
		return "fast"
	case rate >= 3:
		return "elevated"
	default:
		return "slow"
	}
}

func (r *DrainRate) Summary() string {
	if r == nil {
		return ""
	}
	text := fmt.Sprintf("Drain rate is %s at %.1f%%/hour", r.Level, r.PercentPerHour)
	if r.EstimatedMinutesToExhaust != nil {
		text += fmt.Sprintf("; projected exhaustion in about %.0f minutes", *r.EstimatedMinutesToExhaust)
	}
	return text + "."
}
