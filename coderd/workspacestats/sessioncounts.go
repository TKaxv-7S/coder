package workspacestats

import (
	"sort"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// maxSessionCountEntries bounds the number of distinct app names accepted
// per stats report. Counts beyond the cap are aggregated under
// idemetadata.AppNameOther so a malicious or buggy agent cannot fan out
// child-table rows. Legitimate agents report a handful of entries.
const maxSessionCountEntries = 64

// SessionCountsFromProto returns the per-app session counts reported by an
// agent. The deprecated fixed fields are only populated by agents from
// before the session_counts map was introduced and are converted here.
// Entries with non-positive counts are dropped, and reports with more than
// maxSessionCountEntries distinct names have the overflow aggregated under
// idemetadata.AppNameOther.
func SessionCountsFromProto(st *agentproto.Stats) map[string]int64 {
	counts := make(map[string]int64, len(st.GetSessionCounts()))
	for app, count := range st.GetSessionCounts() {
		if count <= 0 {
			continue
		}
		counts[app] = count
	}
	if len(counts) > 0 {
		return capSessionCounts(counts)
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

// capSessionCounts keeps at most maxSessionCountEntries named entries,
// preferring well-known names and then lexicographic order for determinism,
// and sums the remainder into idemetadata.AppNameOther.
func capSessionCounts(counts map[string]int64) map[string]int64 {
	if len(counts) <= maxSessionCountEntries {
		return counts
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		iKnown := idemetadata.Lookup(names[i]).Family != idemetadata.FamilyUnknown
		jKnown := idemetadata.Lookup(names[j]).Family != idemetadata.FamilyUnknown
		if iKnown != jKnown {
			return iKnown
		}
		return names[i] < names[j]
	})
	capped := make(map[string]int64, maxSessionCountEntries+1)
	var other int64
	for i, name := range names {
		if i < maxSessionCountEntries {
			capped[name] = counts[name]
			continue
		}
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
