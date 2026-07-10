package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

// laneColors is the palette used to give each timeline lane (unit) its own
// color, cycled by lane index. cliui.Color yields a colorless profile in
// tests and on non-TTY output, so colored lanes do not affect golden
// files.
var laneColors = []string{"1", "2", "3", "4", "5", "6", "9", "10", "11", "12", "13", "14"}

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

// timelineEdge is one declared dependency of a unit.
type timelineEdge struct {
	dependsOn      unit.ID
	requiredStatus unit.Status
}

// timelineRenderer renders the unit event log as a git-log-style ASCII
// graph. Each unit gets a lane in order of first appearance; every event
// prints a "*" row in its unit's lane with an elapsed-time label. When a
// status change satisfies another unit's dependencies, a connector row is
// drawn from the satisfying lane to the dependent lane, followed by a
// derived "ready" row. Lanes disappear once a unit completes.
//
// The renderer is incremental: renderEvents may be called repeatedly with
// a growing event log and only renders rows for events it has not seen
// yet, which is how --watch streams new rows as they occur.
type timelineRenderer struct {
	lanes      []unit.ID
	laneOf     map[unit.ID]int
	active     map[unit.ID]bool
	statuses   map[unit.ID]unit.Status
	deps       map[unit.ID][]timelineEdge
	dependents map[unit.ID][]unit.ID

	// base is the time of the first event seen; elapsed labels are
	// relative to it.
	base    time.Time
	started bool
	// lastSeq is the highest event sequence number consumed so far.
	lastSeq uint64
}

func newTimelineRenderer() *timelineRenderer {
	return &timelineRenderer{
		laneOf:     make(map[unit.ID]int),
		active:     make(map[unit.ID]bool),
		statuses:   make(map[unit.ID]unit.Status),
		deps:       make(map[unit.ID][]timelineEdge),
		dependents: make(map[unit.ID][]unit.ID),
	}
}

// renderTimeline renders a complete event log in one shot.
func renderTimeline(events []unit.Event) string {
	if len(events) == 0 {
		return "No events found"
	}
	return strings.TrimRight(newTimelineRenderer().renderEvents(events), "\n")
}

// renderEvents consumes events with a sequence number greater than any
// previously consumed and returns the newly produced graph rows,
// newline-terminated. Events must be ordered by Seq.
func (r *timelineRenderer) renderEvents(events []unit.Event) string {
	var sb strings.Builder
	for _, ev := range events {
		if ev.Seq <= r.lastSeq {
			continue
		}
		r.lastSeq = ev.Seq
		if !r.started {
			r.base = ev.Time
			r.started = true
		}
		r.ensureLane(ev.Unit)

		switch ev.Kind {
		case unit.EventStatusChange:
			// Snapshot dependent readiness before applying the status
			// change so blocked-to-ready flips can be detected.
			wasReady := make(map[unit.ID]bool, len(r.dependents[ev.Unit]))
			for _, dep := range r.dependents[ev.Unit] {
				wasReady[dep] = r.isReady(dep)
			}

			r.statuses[ev.Unit] = ev.To
			r.writeRow(&sb, r.glyphRow(ev.Unit, "*"), ev.Time, ev.Unit, syncEventDescription(ev))
			if ev.To == unit.StatusComplete {
				r.active[ev.Unit] = false
			}

			for _, dep := range r.dependents[ev.Unit] {
				if !wasReady[dep] && r.isReady(dep) {
					_, _ = sb.WriteString(r.connectorRow(ev.Unit, dep) + "\n")
					r.writeRow(&sb, r.glyphRow(dep, "*"), ev.Time, dep, "ready (dependency satisfied)")
				}
			}
		case unit.EventDependencyAdded:
			r.deps[ev.Unit] = append(r.deps[ev.Unit], timelineEdge{
				dependsOn:      ev.DependsOn,
				requiredStatus: ev.RequiredStatus,
			})
			r.dependents[ev.DependsOn] = append(r.dependents[ev.DependsOn], ev.Unit)
			r.writeRow(&sb, r.glyphRow(ev.Unit, "*"), ev.Time, ev.Unit, syncEventDescription(ev))
		}
	}
	return sb.String()
}

func (r *timelineRenderer) ensureLane(u unit.ID) {
	if _, ok := r.laneOf[u]; ok {
		return
	}
	r.laneOf[u] = len(r.lanes)
	r.lanes = append(r.lanes, u)
	r.active[u] = true
}

// isReady mirrors the Manager's readiness semantics: a pending unit is
// ready when every dependency's current status equals its required
// status.
func (r *timelineRenderer) isReady(u unit.ID) bool {
	if r.statuses[u] != unit.StatusPending {
		return false
	}
	for _, e := range r.deps[u] {
		if r.statuses[e.dependsOn] != e.requiredStatus {
			return false
		}
	}
	return true
}

// laneStyle returns the color style for the lane at the given index.
func laneStyle(lane int) pretty.Style {
	return pretty.Style{pretty.FgColor(cliui.Color(laneColors[lane%len(laneColors)]))}
}

// colorizeLane colors s with the lane's color. In tests and on non-TTY
// output this returns s unchanged.
func colorizeLane(lane int, s string) string {
	return pretty.Sprint(laneStyle(lane), s)
}

// glyphRow renders one graph row: mark in the target unit's lane, "|" in
// other active lanes, and spaces elsewhere. Each lane's glyph is drawn in
// that lane's color.
func (r *timelineRenderer) glyphRow(target unit.ID, mark string) string {
	glyphs := make([]string, len(r.lanes))
	for i, u := range r.lanes {
		switch {
		case u == target:
			glyphs[i] = colorizeLane(i, mark)
		case r.active[u]:
			glyphs[i] = colorizeLane(i, "|")
		default:
			glyphs[i] = " "
		}
	}
	return strings.TrimRight(strings.Join(glyphs, " "), " ")
}

// connectorRow draws the dependency-satisfaction edge from lane "from"
// toward lane "to". Each lane's glyph is drawn in that lane's color.
func (r *timelineRenderer) connectorRow(from, to unit.ID) string {
	glyphs := make([]string, len(r.lanes))
	for i, u := range r.lanes {
		switch {
		case u == from && r.laneOf[from] < r.laneOf[to]:
			glyphs[i] = colorizeLane(i, `\`)
		case u == from:
			glyphs[i] = colorizeLane(i, "/")
		case r.active[u]:
			glyphs[i] = colorizeLane(i, "|")
		default:
			glyphs[i] = " "
		}
	}
	return strings.TrimRight(strings.Join(glyphs, " "), " ")
}

func (r *timelineRenderer) writeRow(sb *strings.Builder, graph string, at time.Time, u unit.ID, description string) {
	name := colorizeLane(r.laneOf[u], string(u))
	_, _ = sb.WriteString(fmt.Sprintf("%s  [%s]  %s  %s\n", graph, syncElapsed(r.base, at), name, description))
}
