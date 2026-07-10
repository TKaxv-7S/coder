package idemetadata_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/idemetadata"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "CanonicalPassthrough", input: "vscode", want: "vscode"},
		{name: "KnownNameCaseInsensitive", input: "JetBrains", want: "jetbrains"},
		{name: "LegacyAlias", input: "reconnecting-pty", want: "reconnecting_pty"},
		{name: "LegacyAliasCaseInsensitive", input: "Reconnecting-PTY", want: "reconnecting_pty"},
		{name: "UnknownPreservesCasing", input: "Cursor Nightly", want: "Cursor Nightly"},
		{name: "UnknownPreservesUnicode", input: "エディタ", want: "エディタ"},
		{name: "StripsNullBytes", input: "cur\x00sor", want: "cursor"},
		{name: "Empty", input: "", want: "unknown"},
		{name: "OnlyNullBytes", input: "\x00\x00", want: "unknown"},
		{
			name:  "TruncatesToMaxRunes",
			input: strings.Repeat("a", idemetadata.MaxAppNameLength+10),
			want:  strings.Repeat("a", idemetadata.MaxAppNameLength),
		},
		{
			// Rune-safe truncation must not split multibyte characters.
			name:  "TruncatesMultibyteSafely",
			input: strings.Repeat("あ", idemetadata.MaxAppNameLength+1),
			want:  strings.Repeat("あ", idemetadata.MaxAppNameLength),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, idemetadata.Normalize(tc.input))
		})
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		input           string
		wantFamily      string
		wantDisplayName string
	}{
		{name: "VSCode", input: "vscode", wantFamily: idemetadata.FamilyVSCode, wantDisplayName: "VS Code"},
		{name: "VSCodeFork", input: "cursor", wantFamily: idemetadata.FamilyVSCode, wantDisplayName: "Cursor"},
		{name: "CaseInsensitive", input: "JetBrains", wantFamily: idemetadata.FamilyJetBrains, wantDisplayName: "JetBrains"},
		{name: "Alias", input: "reconnecting-pty", wantFamily: idemetadata.FamilyReconnectingPTY, wantDisplayName: "Web Terminal"},
		{name: "UnknownFallsBackToRawName", input: "SomeFutureIDE", wantFamily: idemetadata.FamilyUnknown, wantDisplayName: "SomeFutureIDE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := idemetadata.Lookup(tc.input)
			require.Equal(t, tc.wantFamily, info.Family)
			require.Equal(t, tc.wantDisplayName, info.DisplayName)
		})
	}
}
