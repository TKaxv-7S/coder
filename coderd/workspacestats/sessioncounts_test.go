package workspacestats_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/workspacestats"
)

func TestSessionCountsFromProto(t *testing.T) {
	t.Parallel()

	t.Run("MapTakesPrecedence", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // Deprecated fields are set to verify precedence.
		st := &agentproto.Stats{
			SessionCounts:      map[string]int64{"Cursor": 2, "ssh": 1},
			SessionCountVscode: 9,
		}
		require.Equal(t, map[string]int64{"Cursor": 2, "ssh": 1}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("DropsNonPositiveEntries", func(t *testing.T) {
		t.Parallel()
		st := &agentproto.Stats{
			SessionCounts: map[string]int64{"vscode": 1, "reconnecting_pty": 0, "bogus": -1},
		}
		require.Equal(t, map[string]int64{"vscode": 1}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("OldAgentFallback", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // Deprecated fields simulate an old agent.
		st := &agentproto.Stats{
			SessionCountVscode:    3,
			SessionCountJetbrains: 1,
			SessionCountSsh:       2,
		}
		require.Equal(t, map[string]int64{
			"vscode":    3,
			"jetbrains": 1,
			"ssh":       2,
		}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("AllZeroMapFallsBackToLegacyFields", func(t *testing.T) {
		t.Parallel()
		// A new agent with no active sessions sends zero-valued map entries.
		// The legacy fields are zero too, so the result is empty either way.
		st := &agentproto.Stats{
			SessionCounts: map[string]int64{"ssh": 0, "reconnecting_pty": 0},
		}
		require.Empty(t, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, workspacestats.SessionCountsFromProto(&agentproto.Stats{}))
	})
}

func TestClearSessionCounts(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // Deprecated fields simulate an old agent.
	st := &agentproto.Stats{
		SessionCounts:      map[string]int64{"vscode": 1},
		SessionCountVscode: 1,
		SessionCountSsh:    2,
	}
	workspacestats.ClearSessionCounts(st)
	require.Empty(t, workspacestats.SessionCountsFromProto(st))
}
