package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/coder/coder/v2/agent/unit"
)

// syncEventDescription renders a unit event as a short human-readable
// phrase, shared by the sync status history and sync timeline views.
func syncEventDescription(ev unit.Event) string {
	switch ev.Kind {
	case unit.EventStatusChange:
		if ev.From == unit.StatusNotRegistered {
			return fmt.Sprintf("registered (%s)", ev.To)
		}
		return fmt.Sprintf("%s → %s", ev.From, ev.To)
	case unit.EventDependencyAdded:
		return fmt.Sprintf("depends on %s (%s)", ev.DependsOn, ev.RequiredStatus)
	}
	return string(ev.Kind)
}

// syncElapsed formats the offset of t from base as a compact elapsed
// label, for example "+2.3s".
func syncElapsed(base, t time.Time) string {
	return fmt.Sprintf("+%.1fs", t.Sub(base).Seconds())
}

// renderTimeline renders the unit event log as a git-log-style ASCII
// graph. Each unit gets a lane in order of first appearance; every event
// prints a "*" row in its unit's lane with an elapsed-time label. When a
// status change satisfies another unit's dependencies, a connector row is
// drawn from the satisfying lane to the dependent lane, followed by a
// derived "ready" row. Lanes disappear once a unit completes.
func renderTimeline(events []unit.Event) string {
	if len(events) == 0 {
		return "No events found"
	}

	type edge struct {
		dependsOn      unit.ID
		requiredStatus unit.Status
	}
	var (
		lanes      []unit.ID
		laneOf     = make(map[unit.ID]int)
		active     = make(map[unit.ID]bool)
		statuses   = make(map[unit.ID]unit.Status)
		deps       = make(map[unit.ID][]edge)
		dependents = make(map[unit.ID][]unit.ID)
	)

	ensureLane := func(u unit.ID) {
		if _, ok := laneOf[u]; ok {
			return
		}
		laneOf[u] = len(lanes)
		lanes = append(lanes, u)
		active[u] = true
	}

	// isReady mirrors the Manager's readiness semantics: a pending unit
	// is ready when every dependency's current status equals its
	// required status.
	isReady := func(u unit.ID) bool {
		if statuses[u] != unit.StatusPending {
			return false
		}
		for _, e := range deps[u] {
			if statuses[e.dependsOn] != e.requiredStatus {
				return false
			}
		}
		return true
	}

	// glyphRow renders one graph row: mark in the target unit's lane,
	// "|" in other active lanes, and spaces elsewhere.
	glyphRow := func(target unit.ID, mark string) string {
		glyphs := make([]string, len(lanes))
		for i, u := range lanes {
			switch {
			case u == target:
				glyphs[i] = mark
			case active[u]:
				glyphs[i] = "|"
			default:
				glyphs[i] = " "
			}
		}
		return strings.TrimRight(strings.Join(glyphs, " "), " ")
	}

	// connectorRow draws the dependency-satisfaction edge from lane
	// "from" toward lane "to".
	connectorRow := func(from, to unit.ID) string {
		glyphs := make([]string, len(lanes))
		for i, u := range lanes {
			switch {
			case u == from && laneOf[from] < laneOf[to]:
				glyphs[i] = `\`
			case u == from:
				glyphs[i] = "/"
			case active[u]:
				glyphs[i] = "|"
			default:
				glyphs[i] = " "
			}
		}
		return strings.TrimRight(strings.Join(glyphs, " "), " ")
	}

	base := events[0].Time
	var sb strings.Builder
	writeRow := func(graph string, at time.Time, u unit.ID, description string) {
		_, _ = sb.WriteString(fmt.Sprintf("%s  [%s]  %s  %s\n", graph, syncElapsed(base, at), u, description))
	}

	for _, ev := range events {
		ensureLane(ev.Unit)

		switch ev.Kind {
		case unit.EventStatusChange:
			// Snapshot dependent readiness before applying the status
			// change so blocked-to-ready flips can be detected.
			wasReady := make(map[unit.ID]bool, len(dependents[ev.Unit]))
			for _, dep := range dependents[ev.Unit] {
				wasReady[dep] = isReady(dep)
			}

			statuses[ev.Unit] = ev.To
			writeRow(glyphRow(ev.Unit, "*"), ev.Time, ev.Unit, syncEventDescription(ev))
			if ev.To == unit.StatusComplete {
				active[ev.Unit] = false
			}

			for _, dep := range dependents[ev.Unit] {
				if !wasReady[dep] && isReady(dep) {
					_, _ = sb.WriteString(connectorRow(ev.Unit, dep) + "\n")
					writeRow(glyphRow(dep, "*"), ev.Time, dep, "ready (dependency satisfied)")
				}
			}
		case unit.EventDependencyAdded:
			deps[ev.Unit] = append(deps[ev.Unit], edge{
				dependsOn:      ev.DependsOn,
				requiredStatus: ev.RequiredStatus,
			})
			dependents[ev.DependsOn] = append(dependents[ev.DependsOn], ev.Unit)
			writeRow(glyphRow(ev.Unit, "*"), ev.Time, ev.Unit, syncEventDescription(ev))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
