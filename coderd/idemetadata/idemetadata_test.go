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
		{name: "HyphenatedKnownName", input: "vscode-insiders", want: "vscode_insiders"},
		{name: "UnknownHyphensFolded", input: "my-future-ide", want: "my_future_ide"},
		{name: "UnknownLowercased", input: "Cursor Nightly", want: "cursor nightly"},
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

func TestFamily(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "VSCode", input: "vscode", want: idemetadata.AppNameVSCode},
		{name: "VSCodeFork", input: "cursor", want: idemetadata.AppNameVSCode},
		{name: "VSCodeInsidersHyphenated", input: "vscode-insiders", want: idemetadata.AppNameVSCode},
		{name: "VSCodium", input: "codium", want: idemetadata.AppNameVSCode},
		{name: "CaseInsensitive", input: "JetBrains", want: idemetadata.AppNameJetBrains},
		{name: "Alias", input: "reconnecting-pty", want: idemetadata.AppNameReconnectingPTY},
		{name: "Other", input: "other", want: idemetadata.AppNameUnknown},
		{name: "UnknownName", input: "SomeFutureIDE", want: idemetadata.AppNameUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, idemetadata.Family(tc.input))
		})
	}
}
