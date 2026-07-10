package agentssh

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractMagicSessionType(t *testing.T) {
	t.Parallel()

	envWith := func(value string) []string {
		return []string{
			"FOO=bar",
			fmt.Sprintf("%s=%s", MagicSessionTypeEnvironmentVariable, value),
			"BAZ=qux",
		}
	}

	for _, tc := range []struct {
		name string
		env  []string
		want MagicSessionType
	}{
		{name: "NoEnvDefaultsToSSH", env: []string{"FOO=bar"}, want: MagicSessionTypeSSH},
		{name: "EmptyValueDefaultsToSSH", env: envWith(""), want: MagicSessionTypeSSH},
		{name: "VSCode", env: envWith("vscode"), want: MagicSessionTypeVSCode},
		{name: "JetBrainsLegacyCasing", env: envWith("JetBrains"), want: MagicSessionTypeJetBrains},
		{name: "UnknownPreservedRaw", env: envWith("Cursor Nightly"), want: MagicSessionType("Cursor Nightly")},
		{name: "LastInstanceWins", env: append(envWith("vscode"), MagicSessionTypeEnvironmentVariable+"=cursor"), want: MagicSessionType("cursor")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sessionType, filteredEnv := extractMagicSessionType(tc.env)
			require.Equal(t, tc.want, sessionType)
			for _, kv := range filteredEnv {
				require.NotContains(t, kv, MagicSessionTypeEnvironmentVariable+"=", "env should be stripped")
			}
		})
	}
}

func TestConnCounts(t *testing.T) {
	t.Parallel()

	s := &Server{}

	// Counters are created dynamically per session type, including types
	// unknown to this build of the agent.
	s.getOrCreateConnCounter(MagicSessionTypeSSH).Add(1)
	s.getOrCreateConnCounter(MagicSessionTypeVSCode).Add(1)
	s.getOrCreateConnCounter(MagicSessionType("Cursor")).Add(2)
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
		"Cursor": 2,
	}, s.ConnStats())

	// The same counter instance is reused per type.
	s.getOrCreateConnCounter(MagicSessionType("Cursor")).Add(-2)
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
		"Cursor": 0,
	}, s.ConnStats())
}
