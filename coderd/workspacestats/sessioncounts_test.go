package workspacestats_test

import (
	"fmt"
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
			SessionCounts:      map[string]int64{"cursor": 2, "ssh": 1},
			SessionCountVscode: 9,
		}
		require.Equal(t, map[string]int64{"cursor": 2, "ssh": 1}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("NormalizesAndMergesNames", func(t *testing.T) {
		t.Parallel()
		st := &agentproto.Stats{
			SessionCounts: map[string]int64{"VSCode": 1, "vscode": 2, "Reconnecting-PTY": 1},
		}
		require.Equal(t, map[string]int64{
			"vscode":           3,
			"reconnecting_pty": 1,
		}, workspacestats.SessionCountsFromProto(st))
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

	t.Run("CapsEntriesAggregatingIntoOther", func(t *testing.T) {
		t.Parallel()
		sessionCounts := map[string]int64{"vscode": 5}
		for i := range 200 {
			sessionCounts[fmt.Sprintf("zz-ide-%03d", i)] = 1
		}
		got := workspacestats.SessionCountsFromProto(&agentproto.Stats{SessionCounts: sessionCounts})
		// 64 named entries (well-known first, then lexicographic) plus
		// "other" holding the 137 overflow counts.
		require.Len(t, got, 65)
		require.EqualValues(t, 5, got["vscode"])
		require.EqualValues(t, 1, got["zz-ide-000"])
		require.EqualValues(t, 137, got["other"])
		require.NotContains(t, got, "zz-ide-199")
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, workspacestats.SessionCountsFromProto(&agentproto.Stats{}))
	})
}

func TestHasSessionCounts(t *testing.T) {
	t.Parallel()

	require.False(t, workspacestats.HasSessionCounts(&agentproto.Stats{}))
	require.False(t, workspacestats.HasSessionCounts(&agentproto.Stats{
		SessionCounts: map[string]int64{"ssh": 0},
	}))
	require.True(t, workspacestats.HasSessionCounts(&agentproto.Stats{
		SessionCounts: map[string]int64{"ssh": 1},
	}))
	//nolint:staticcheck // Deprecated fields simulate an old agent.
	require.True(t, workspacestats.HasSessionCounts(&agentproto.Stats{SessionCountJetbrains: 1}))
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
