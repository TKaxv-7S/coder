package unit

import "time"

// Phase is a derived stage of a unit's lifecycle. Phases are not stored
// by the Manager; they are reconstructed from the event log.
type Phase string

const (
	// PhaseBlocked means the unit is registered but at least one of its
	// dependencies is not at its required status.
	PhaseBlocked Phase = "blocked"
	// PhaseReady means the unit is registered and every declared
	// dependency is at its required status.
	PhaseReady Phase = "ready"
	// PhaseRunning means the unit has started.
	PhaseRunning Phase = "running"
	// PhaseDone means the unit has completed.
	PhaseDone Phase = "done"
)

// Interval is a derived span of a unit's lifecycle.
type Interval struct {
	Unit  ID
	Phase Phase
	Start time.Time
	// End is the zero time if the interval is still open.
	End time.Time
}

// DeriveIntervals replays events in Seq order and returns per-unit
// lifecycle intervals. Readiness is reconstructed from status_change and
// dependency_added events, mirroring the Manager's readiness semantics
// (recalculateReadinessUnsafe): a pending unit is ready when every
// declared dependency's current status equals its required status. This
// captures blocked-to-ready flips caused by other units' status changes
// and ready-to-blocked flips caused by dependency edges added after
// registration.
func DeriveIntervals(events []Event) map[ID][]Interval {
	type edge struct {
		dependsOn      ID
		requiredStatus Status
	}
	statuses := make(map[ID]Status)
	deps := make(map[ID][]edge)
	// dependents holds reverse edges so a status change can be folded
	// into the phase of every unit that depends on the changed unit.
	dependents := make(map[ID][]ID)
	intervals := make(map[ID][]Interval)

	// phaseOf derives the current phase of a registered unit from the
	// replayed state. The second return is false for units that have
	// not registered yet (for example, a dependency that only appears
	// on the depends-on side of an edge).
	phaseOf := func(u ID) (Phase, bool) {
		switch statuses[u] {
		case StatusNotRegistered:
			return "", false
		case StatusStarted:
			return PhaseRunning, true
		case StatusComplete:
			return PhaseDone, true
		}
		for _, e := range deps[u] {
			if statuses[e.dependsOn] != e.requiredStatus {
				return PhaseBlocked, true
			}
		}
		return PhaseReady, true
	}

	// setPhase closes the unit's open interval and opens a new one if
	// the phase changed.
	setPhase := func(u ID, phase Phase, at time.Time) {
		ivs := intervals[u]
		if len(ivs) > 0 {
			last := &ivs[len(ivs)-1]
			if last.Phase == phase {
				return
			}
			last.End = at
		}
		intervals[u] = append(ivs, Interval{Unit: u, Phase: phase, Start: at})
	}

	for _, ev := range events {
		affected := []ID{ev.Unit}
		switch ev.Kind {
		case EventStatusChange:
			statuses[ev.Unit] = ev.To
			affected = append(affected, dependents[ev.Unit]...)
		case EventDependencyAdded:
			deps[ev.Unit] = append(deps[ev.Unit], edge{
				dependsOn:      ev.DependsOn,
				requiredStatus: ev.RequiredStatus,
			})
			dependents[ev.DependsOn] = append(dependents[ev.DependsOn], ev.Unit)
		}
		for _, u := range affected {
			if phase, ok := phaseOf(u); ok {
				setPhase(u, phase, ev.Time)
			}
		}
	}

	return intervals
}
