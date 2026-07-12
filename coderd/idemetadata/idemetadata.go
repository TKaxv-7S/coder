// Package idemetadata is the single source of truth for metadata about IDEs
// and other session types that report workspace usage. App names are stored
// as reported throughout the stats pipeline; well-known spellings are
// canonicalized at ingestion and grouped into families at API and metrics
// boundaries. It is a leaf package so both agent and coderd code can import
// it.
package idemetadata

import (
	"strings"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// MaxAppNameLength is the maximum length of an app name in runes. Longer
// names are truncated before storage.
const MaxAppNameLength = 64

// Canonical app names for Coder's built-in session types. These are the
// keys used by the agent and by the workspace usage API, and the values
// the template usage rollup aggregates into dedicated columns.
const (
	AppNameVSCode          = "vscode"
	AppNameJetBrains       = "jetbrains"
	AppNameSSH             = "ssh"
	AppNameReconnectingPTY = "reconnecting_pty"
	AppNameUnknown         = "unknown"
	// AppNameOther aggregates session counts beyond the per-report entry cap.
	AppNameOther = "other"
)

// families maps canonical (lowercase) app names to their family, keeping
// metric-label cardinality bounded while arbitrary app names flow through
// the pipeline. A family is named after its canonical app name. Names are
// stored as reported and grouped at read time, so extending this map needs
// no migration and applies retroactively to stored data.
var families = map[string]string{
	AppNameVSCode:          AppNameVSCode,
	"vscode_insiders":      AppNameVSCode,
	"cursor":               AppNameVSCode,
	"windsurf":             AppNameVSCode,
	"positron":             AppNameVSCode,
	"vscodium":             AppNameVSCode,
	"codium":               AppNameVSCode,
	"antigravity":          AppNameVSCode,
	"trae":                 AppNameVSCode,
	"kiro":                 AppNameVSCode,
	"devin":                AppNameVSCode,
	AppNameJetBrains:       AppNameJetBrains,
	AppNameSSH:             AppNameSSH,
	AppNameReconnectingPTY: AppNameReconnectingPTY,
	AppNameUnknown:         AppNameUnknown,
	AppNameOther:           AppNameUnknown,
}

// Family returns the family for the given app name. Matching is
// case-insensitive and alias-aware; unknown names map to AppNameUnknown.
func Family(appName string) string {
	if family, ok := families[canonicalKey(appName)]; ok {
		return family
	}
	return AppNameUnknown
}

// Normalize prepares a client-supplied app name for storage: it strips null
// bytes (which Postgres TEXT rejects), truncates to MaxAppNameLength runes,
// and canonicalizes well-known names case-insensitively. Unrecognized names
// are preserved as-is, including casing, and grouped at display time via
// Family. An empty result becomes AppNameUnknown so a bad name never
// invalidates the surrounding report.
func Normalize(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	appName = utilstrings.Truncate(appName, MaxAppNameLength)
	if appName == "" {
		return AppNameUnknown
	}
	if key := canonicalKey(appName); families[key] != "" {
		return key
	}
	return appName
}

// canonicalKey lowercases the app name and folds hyphens to underscores so
// spellings like "reconnecting-pty" (codersdk.UsageAppNameReconnectingPty)
// and "vscode-insiders" match their canonical form.
func canonicalKey(appName string) string {
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}
