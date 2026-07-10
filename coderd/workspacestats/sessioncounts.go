package workspacestats

import (
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// SessionCountsFromProto returns the per-app session counts reported by an
// agent. The deprecated fixed fields are only populated by agents from
// before the session_counts map was introduced and are converted here.
// Entries with non-positive counts are dropped.
func SessionCountsFromProto(st *agentproto.Stats) map[string]int64 {
	counts := make(map[string]int64, len(st.GetSessionCounts()))
	for app, count := range st.GetSessionCounts() {
		if count <= 0 {
			continue
		}
		counts[app] = count
	}
	if len(counts) > 0 {
		return counts
	}
	// Old-agent fallback: convert the deprecated fixed fields.
	//nolint:staticcheck // Deprecated fields are read for old-agent compatibility.
	for app, count := range map[string]int64{
		idemetadata.AppNameVSCode:          st.SessionCountVscode,
		idemetadata.AppNameJetBrains:       st.SessionCountJetbrains,
		idemetadata.AppNameReconnectingPTY: st.SessionCountReconnectingPty,
		idemetadata.AppNameSSH:             st.SessionCountSsh,
	} {
		if count <= 0 {
			continue
		}
		counts[app] = count
	}
	return counts
}

// ClearSessionCounts zeroes all session counts on the given stats, including
// the deprecated fixed fields sent by older agents.
func ClearSessionCounts(st *agentproto.Stats) {
	st.SessionCounts = nil
	//nolint:staticcheck // Deprecated fields are cleared for old-agent compatibility.
	st.SessionCountSsh, st.SessionCountJetbrains, st.SessionCountVscode, st.SessionCountReconnectingPty = 0, 0, 0, 0
}
