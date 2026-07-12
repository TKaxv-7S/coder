package workspacestats

import (
	"maps"
	"slices"
	"strings"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// maxSessionCountEntries bounds the number of distinct app names accepted
// per stats report so a malicious or buggy agent cannot fan out child-table
// rows. Overflow is aggregated under idemetadata.AppNameOther.
const maxSessionCountEntries = 64

// SessionCountsFromProto returns the per-app session counts reported by an
// agent, with names normalized and non-positive counts dropped. The
// deprecated fixed fields are converted for agents from before the
// session_counts map (API < 2.11).
func SessionCountsFromProto(st *agentproto.Stats) map[string]int64 {
	counts := make(map[string]int64, len(st.GetSessionCounts()))
	for app, count := range st.GetSessionCounts() {
		if count > 0 {
			counts[idemetadata.Normalize(app)] += count
		}
	}
	if len(counts) > 0 {
		return capSessionCounts(counts)
	}
	//nolint:staticcheck // Deprecated fields are read for old-agent compatibility.
	legacy := [...]struct {
		app   string
		count int64
	}{
		{idemetadata.AppNameVSCode, st.SessionCountVscode},
		{idemetadata.AppNameJetBrains, st.SessionCountJetbrains},
		{idemetadata.AppNameReconnectingPTY, st.SessionCountReconnectingPty},
		{idemetadata.AppNameSSH, st.SessionCountSsh},
	}
	for _, l := range legacy {
		if l.count > 0 {
			counts[l.app] = l.count
		}
	}
	return counts
}

// HasSessionCounts reports whether the stats contain any active session,
// without building the counts map.
func HasSessionCounts(st *agentproto.Stats) bool {
	for _, count := range st.GetSessionCounts() {
		if count > 0 {
			return true
		}
	}
	//nolint:staticcheck // Deprecated fields are read for old-agent compatibility.
	return st.SessionCountVscode > 0 || st.SessionCountJetbrains > 0 ||
		st.SessionCountReconnectingPty > 0 || st.SessionCountSsh > 0
}

// capSessionCounts keeps at most maxSessionCountEntries named entries,
// preferring well-known names and then lexicographic order for determinism,
// and sums the remainder into idemetadata.AppNameOther.
func capSessionCounts(counts map[string]int64) map[string]int64 {
	if len(counts) <= maxSessionCountEntries {
		return counts
	}
	names := slices.Collect(maps.Keys(counts))
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = idemetadata.Family(name) != idemetadata.AppNameUnknown
	}
	slices.SortFunc(names, func(a, b string) int {
		if known[a] != known[b] {
			if known[a] {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	capped := make(map[string]int64, maxSessionCountEntries+1)
	for _, name := range names[:maxSessionCountEntries] {
		capped[name] = counts[name]
	}
	var other int64
	for _, name := range names[maxSessionCountEntries:] {
		other += counts[name]
	}
	if other > 0 {
		capped[idemetadata.AppNameOther] += other
	}
	return capped
}

// ClearSessionCounts zeroes all session counts on the given stats, including
// the deprecated fixed fields sent by older agents.
func ClearSessionCounts(st *agentproto.Stats) {
	st.SessionCounts = nil
	//nolint:staticcheck // Deprecated fields are cleared for old-agent compatibility.
	st.SessionCountSsh, st.SessionCountJetbrains, st.SessionCountVscode, st.SessionCountReconnectingPty = 0, 0, 0, 0
}
