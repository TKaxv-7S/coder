// Package idemetadata is the single source of truth for metadata about IDEs
// and other session types that report workspace usage. App names are stored
// raw throughout the stats pipeline; this package canonicalizes well-known
// legacy spellings at ingestion and supplies the family and display-name
// grouping applied at API and metrics boundaries. It is a leaf package so
// both agent and coderd code can import it.
package idemetadata

import "strings"

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

// Families group related session types for display and for metric labels,
// keeping label cardinality bounded while arbitrary app names flow through
// the pipeline.
const (
	FamilyVSCode          = "vscode"
	FamilyJetBrains       = "jetbrains"
	FamilySSH             = "ssh"
	FamilyReconnectingPTY = "reconnecting_pty"
	FamilyUnknown         = "unknown"
)

// IDEInfo describes how a session type is grouped and displayed.
type IDEInfo struct {
	Family      string
	DisplayName string
}

// known maps canonical (lowercase) app names to their metadata.
var known = map[string]IDEInfo{
	AppNameVSCode:          {Family: FamilyVSCode, DisplayName: "VS Code"},
	"vscode_insiders":      {Family: FamilyVSCode, DisplayName: "VS Code Insiders"},
	"cursor":               {Family: FamilyVSCode, DisplayName: "Cursor"},
	"windsurf":             {Family: FamilyVSCode, DisplayName: "Windsurf"},
	"positron":             {Family: FamilyVSCode, DisplayName: "Positron"},
	"vscodium":             {Family: FamilyVSCode, DisplayName: "VSCodium"},
	AppNameJetBrains:       {Family: FamilyJetBrains, DisplayName: "JetBrains"},
	AppNameSSH:             {Family: FamilySSH, DisplayName: "SSH"},
	AppNameReconnectingPTY: {Family: FamilyReconnectingPTY, DisplayName: "Web Terminal"},
	AppNameUnknown:         {Family: FamilyUnknown, DisplayName: "Unknown"},
	AppNameOther:           {Family: FamilyUnknown, DisplayName: "Other"},
}

// aliases maps well-known legacy spellings (lowercase) to canonical app
// names so historical client values keep aggregating with their canonical
// counterparts.
var aliases = map[string]string{
	// codersdk.UsageAppNameReconnectingPty uses a hyphen.
	"reconnecting-pty": AppNameReconnectingPTY,
}

// Lookup returns metadata for the given app name. Matching is
// case-insensitive and alias-aware; unknown names fall back to family
// "unknown" with the raw name as the display name.
func Lookup(appName string) IDEInfo {
	if info, ok := known[canonicalKey(appName)]; ok {
		return info
	}
	return IDEInfo{Family: FamilyUnknown, DisplayName: appName}
}

// Normalize prepares a client-supplied app name for storage: it strips null
// bytes (which Postgres TEXT rejects), truncates to MaxAppNameLength runes,
// and canonicalizes well-known names and legacy spellings
// case-insensitively. Unrecognized names are preserved as-is, including
// casing; they are grouped at display time via Lookup. An empty result
// becomes "unknown" so a bad name never invalidates the surrounding report.
func Normalize(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	if runes := []rune(appName); len(runes) > MaxAppNameLength {
		appName = string(runes[:MaxAppNameLength])
	}
	if appName == "" {
		return AppNameUnknown
	}
	if canonical, ok := canonicalName(appName); ok {
		return canonical
	}
	return appName
}

// canonicalName resolves well-known names and aliases to their canonical
// form, reporting whether the name was recognized.
func canonicalName(appName string) (string, bool) {
	key := canonicalKey(appName)
	if _, ok := known[key]; ok {
		return key, true
	}
	return appName, false
}

// canonicalKey lowercases and resolves aliases to produce the map key for
// a raw app name.
func canonicalKey(appName string) string {
	key := strings.ToLower(appName)
	if alias, ok := aliases[key]; ok {
		return alias
	}
	return key
}
