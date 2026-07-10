package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

// statusChange builds a status_change event.
func statusChange(seq uint64, u unit.ID, from, to unit.Status) unit.Event {
	return unit.Event{
		Seq:  seq,
		Kind: unit.EventStatusChange,
		Unit: u,
		From: from,
		To:   to,
	}
}

// dependencyAdded builds a dependency_added event.
func dependencyAdded(seq uint64, u, dependsOn unit.ID, required unit.Status) unit.Event {
	return unit.Event{
		Seq:            seq,
		Kind:           unit.EventDependencyAdded,
		Unit:           u,
		DependsOn:      dependsOn,
		RequiredStatus: required,
	}
}

func TestBuildSyncGraph(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		g := buildSyncGraph(nil)
		require.Empty(t, g.Nodes)
		require.Empty(t, g.Edges)
	})

	t.Run("nodes_ordered_by_first_appearance", func(t *testing.T) {
		t.Parallel()
		events := []unit.Event{
			statusChange(1, "b", unit.StatusNotRegistered, unit.StatusPending),
			statusChange(2, "a", unit.StatusNotRegistered, unit.StatusPending),
			statusChange(3, "b", unit.StatusPending, unit.StatusStarted),
		}
		g := buildSyncGraph(events)
		require.Len(t, g.Nodes, 2)
		require.Equal(t, "b", g.Nodes[0].Unit)
		require.Equal(t, "a", g.Nodes[1].Unit)
		require.Equal(t, string(unit.StatusStarted), g.Nodes[0].Status)
	})

	t.Run("dependency_added_creates_both_nodes", func(t *testing.T) {
		t.Parallel()
		events := []unit.Event{
			dependencyAdded(1, "app", "db", unit.StatusComplete),
		}
		g := buildSyncGraph(events)
		require.Len(t, g.Nodes, 2)
		require.Equal(t, "app", g.Nodes[0].Unit)
		require.Equal(t, "db", g.Nodes[1].Unit)
		require.Len(t, g.Edges, 1)
		require.Equal(t, "app", g.Edges[0].From)
		require.Equal(t, "db", g.Edges[0].To)
		require.False(t, g.Edges[0].Satisfied)
	})

	t.Run("edge_satisfied_when_status_matches", func(t *testing.T) {
		t.Parallel()
		events := []unit.Event{
			statusChange(1, "app", unit.StatusNotRegistered, unit.StatusPending),
			dependencyAdded(2, "app", "db", unit.StatusComplete),
			statusChange(3, "db", unit.StatusNotRegistered, unit.StatusPending),
			statusChange(4, "db", unit.StatusPending, unit.StatusStarted),
			statusChange(5, "db", unit.StatusStarted, unit.StatusComplete),
		}
		g := buildSyncGraph(events)
		require.Len(t, g.Edges, 1)
		require.True(t, g.Edges[0].Satisfied)
		require.Equal(t, string(unit.StatusComplete), g.Edges[0].CurrentStatus)

		// app is pending with a satisfied dependency, so it is ready.
		app := nodeByUnit(t, g, "app")
		require.True(t, app.Ready)
	})

	t.Run("node_not_ready_with_unsatisfied_edge", func(t *testing.T) {
		t.Parallel()
		events := []unit.Event{
			statusChange(1, "app", unit.StatusNotRegistered, unit.StatusPending),
			dependencyAdded(2, "app", "db", unit.StatusComplete),
			statusChange(3, "db", unit.StatusNotRegistered, unit.StatusPending),
		}
		g := buildSyncGraph(events)
		app := nodeByUnit(t, g, "app")
		require.False(t, app.Ready)
	})

	t.Run("started_node_not_ready", func(t *testing.T) {
		t.Parallel()
		events := []unit.Event{
			statusChange(1, "solo", unit.StatusNotRegistered, unit.StatusPending),
			statusChange(2, "solo", unit.StatusPending, unit.StatusStarted),
		}
		g := buildSyncGraph(events)
		solo := nodeByUnit(t, g, "solo")
		require.False(t, solo.Ready)
	})
}

func nodeByUnit(t *testing.T, g syncGraph, name string) syncGraphNode {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Unit == name {
			return n
		}
	}
	t.Fatalf("node %q not found", name)
	return syncGraphNode{}
}

func TestRenderSyncGraphASCII(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "No units found", renderSyncGraphASCII(syncGraph{}))
	})

	t.Run("units_and_edges", func(t *testing.T) {
		t.Parallel()
		events := []unit.Event{
			statusChange(1, "app", unit.StatusNotRegistered, unit.StatusPending),
			dependencyAdded(2, "app", "db", unit.StatusComplete),
			statusChange(3, "db", unit.StatusNotRegistered, unit.StatusPending),
			statusChange(4, "db", unit.StatusPending, unit.StatusComplete),
		}
		out := renderSyncGraphASCII(buildSyncGraph(events))
		require.Contains(t, out, "Units:")
		require.Contains(t, out, "Dependencies:")
		require.Contains(t, out, "app → db")
		require.Contains(t, out, "requires completed")
		// app is pending with a satisfied dependency.
		require.Contains(t, out, "(ready)")
		// Satisfied edge is marked with a check.
		require.Contains(t, out, "✓")
	})
}

func TestRenderSyncGraphDOT(t *testing.T) {
	t.Parallel()

	events := []unit.Event{
		statusChange(1, "app", unit.StatusNotRegistered, unit.StatusPending),
		dependencyAdded(2, "app", "db", unit.StatusComplete),
		statusChange(3, "db", unit.StatusNotRegistered, unit.StatusPending),
		statusChange(4, "db", unit.StatusPending, unit.StatusComplete),
	}
	out := renderSyncGraphDOT(buildSyncGraph(events))
	require.True(t, strings.HasPrefix(out, "digraph sync {"))
	require.Contains(t, out, `"app" -> "db"`)
	require.Contains(t, out, "fillcolor=\"palegreen\"") // db completed
	require.Contains(t, out, "style=solid")             // satisfied edge
	require.True(t, strings.HasSuffix(out, "}"))
}

func TestSyncGraphTableRows(t *testing.T) {
	t.Parallel()

	events := []unit.Event{
		statusChange(1, "app", unit.StatusNotRegistered, unit.StatusPending),
		dependencyAdded(2, "app", "db", unit.StatusComplete),
		statusChange(3, "db", unit.StatusNotRegistered, unit.StatusPending),
	}
	rows := syncGraphTableRows(buildSyncGraph(events))
	require.Len(t, rows, 2)
	require.Equal(t, "app", rows[0].Unit)
	require.Contains(t, rows[0].DependsOn, "db (completed)")
	require.Empty(t, rows[1].DependsOn)
}
