package cli

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

// syncGraphNode is a single unit (vertex) in the dependency graph.
type syncGraphNode struct {
	Unit   string `json:"unit"`
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}

// syncGraphEdge is a declared dependency (edge) from one unit to another.
// Satisfied reports whether the dependency's current status already equals
// the required status.
type syncGraphEdge struct {
	From           string `json:"from"`
	To             string `json:"to"`
	RequiredStatus string `json:"required_status"`
	CurrentStatus  string `json:"current_status"`
	Satisfied      bool   `json:"satisfied"`
}

// syncGraph is the whole dependency graph: units and the edges between
// them.
type syncGraph struct {
	Nodes []syncGraphNode `json:"nodes"`
	Edges []syncGraphEdge `json:"edges"`
}

// syncGraphTableRow is one row of the table output: a unit with its
// dependencies collapsed into a single column.
type syncGraphTableRow struct {
	Unit      string `table:"unit,default_sort"`
	Status    string `table:"status"`
	Ready     bool   `table:"ready"`
	DependsOn string `table:"depends on"`
}

// buildSyncGraph folds the unit event log into a dependency graph. Nodes
// are ordered by first appearance. An edge is satisfied when the current
// status of its target equals the required status. A node is ready when it
// is pending and all of its edges are satisfied, mirroring the Manager's
// readiness semantics.
func buildSyncGraph(events []unit.Event) syncGraph {
	statuses := make(map[unit.ID]unit.Status)
	order := make([]unit.ID, 0)
	seen := make(map[unit.ID]bool)
	ensure := func(u unit.ID) {
		if !seen[u] {
			seen[u] = true
			order = append(order, u)
		}
	}

	type rawEdge struct {
		from     unit.ID
		to       unit.ID
		required unit.Status
	}
	var rawEdges []rawEdge

	for _, ev := range events {
		switch ev.Kind {
		case unit.EventStatusChange:
			ensure(ev.Unit)
			statuses[ev.Unit] = ev.To
		case unit.EventDependencyAdded:
			ensure(ev.Unit)
			ensure(ev.DependsOn)
			rawEdges = append(rawEdges, rawEdge{
				from:     ev.Unit,
				to:       ev.DependsOn,
				required: ev.RequiredStatus,
			})
		}
	}

	// A node starts ready only if it is pending; any unsatisfied edge then
	// makes it not ready.
	ready := make(map[unit.ID]bool, len(order))
	for _, u := range order {
		ready[u] = statuses[u] == unit.StatusPending
	}

	edges := make([]syncGraphEdge, 0, len(rawEdges))
	for _, re := range rawEdges {
		current := statuses[re.to]
		satisfied := current == re.required
		if !satisfied {
			ready[re.from] = false
		}
		edges = append(edges, syncGraphEdge{
			From:           string(re.from),
			To:             string(re.to),
			RequiredStatus: string(re.required),
			CurrentStatus:  string(current),
			Satisfied:      satisfied,
		})
	}

	nodes := make([]syncGraphNode, 0, len(order))
	for _, u := range order {
		nodes = append(nodes, syncGraphNode{
			Unit:   string(u),
			Status: string(statuses[u]),
			Ready:  ready[u],
		})
	}

	return syncGraph{Nodes: nodes, Edges: edges}
}

// syncGraphTableRows flattens the graph into table rows, one per unit,
// with dependencies joined into the "depends on" column.
func syncGraphTableRows(g syncGraph) []syncGraphTableRow {
	deps := make(map[string][]string, len(g.Nodes))
	for _, e := range g.Edges {
		mark := "✗"
		if e.Satisfied {
			mark = "✓"
		}
		deps[e.From] = append(deps[e.From], fmt.Sprintf("%s (%s) %s", e.To, e.RequiredStatus, mark))
	}

	rows := make([]syncGraphTableRow, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		rows = append(rows, syncGraphTableRow{
			Unit:      n.Unit,
			Status:    n.Status,
			Ready:     n.Ready,
			DependsOn: strings.Join(deps[n.Unit], ", "),
		})
	}
	return rows
}

// statusStyle returns the color style for a unit status. Completed units
// are green, running (started) units are yellow, and everything else
// (pending, unregistered) is dim gray. cliui.Color yields a colorless
// profile in tests and on non-TTY output, so this is safe for goldens.
func statusStyle(status string) pretty.Style {
	switch unit.Status(status) {
	case unit.StatusComplete:
		return pretty.Style{pretty.FgColor(cliui.Color("2"))}
	case unit.StatusStarted:
		return pretty.Style{pretty.FgColor(cliui.Color("3"))}
	default:
		return pretty.Style{pretty.FgColor(cliui.Color("8"))}
	}
}

// renderSyncGraphASCII renders the dependency graph as a colorized text
// summary: a units section followed by a dependencies section. The status
// marker (●) and status label are colored by state.
func renderSyncGraphASCII(g syncGraph) string {
	if len(g.Nodes) == 0 {
		return "No units found"
	}

	var sb strings.Builder

	unitWidth := 0
	for _, n := range g.Nodes {
		if len(n.Unit) > unitWidth {
			unitWidth = len(n.Unit)
		}
	}

	_, _ = sb.WriteString("Units:\n")
	for _, n := range g.Nodes {
		style := statusStyle(n.Status)
		marker := pretty.Sprint(style, "●")
		status := n.Status
		if status == "" {
			status = "unregistered"
		}
		status = pretty.Sprint(style, status)
		readiness := ""
		if n.Ready {
			readiness = " (ready)"
		}
		_, _ = sb.WriteString(fmt.Sprintf("  %s %-*s  %s%s\n", marker, unitWidth, n.Unit, status, readiness))
	}

	if len(g.Edges) > 0 {
		_, _ = sb.WriteString("\nDependencies:\n")
		for _, e := range g.Edges {
			mark := pretty.Sprint(statusStyle(""), "✗")
			if e.Satisfied {
				mark = pretty.Sprint(statusStyle(string(unit.StatusComplete)), "✓")
			}
			_, _ = sb.WriteString(fmt.Sprintf("  %s → %s  (requires %s) %s\n", e.From, e.To, e.RequiredStatus, mark))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// renderSyncGraphDOT renders the dependency graph in Graphviz DOT format.
// Nodes are colored by status using named Graphviz fill colors so the
// output can be piped to `dot` for visualization.
func renderSyncGraphDOT(g syncGraph) string {
	var sb strings.Builder
	_, _ = sb.WriteString("digraph sync {\n")
	_, _ = sb.WriteString("  rankdir=LR;\n")
	_, _ = sb.WriteString("  node [shape=box style=filled];\n")

	for _, n := range g.Nodes {
		label := n.Unit
		if n.Ready {
			label += " (ready)"
		}
		_, _ = sb.WriteString(fmt.Sprintf("  %q [label=%q fillcolor=%q];\n", n.Unit, label, dotFillColor(n.Status)))
	}

	for _, e := range g.Edges {
		style := "dashed"
		if e.Satisfied {
			style = "solid"
		}
		_, _ = sb.WriteString(fmt.Sprintf("  %q -> %q [label=%q style=%s];\n", e.From, e.To, e.RequiredStatus, style))
	}

	_, _ = sb.WriteString("}")
	return sb.String()
}

// dotFillColor maps a unit status to a Graphviz named color.
func dotFillColor(status string) string {
	switch unit.Status(status) {
	case unit.StatusComplete:
		return "palegreen"
	case unit.StatusStarted:
		return "lightyellow"
	default:
		return "lightgray"
	}
}
