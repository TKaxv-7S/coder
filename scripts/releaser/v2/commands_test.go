package v2 //nolint:testpackage // Tests unexported release helpers.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_channelFor(t *testing.T) {
	t.Parallel()

	rc, err := parseVersion("v2.35.0-rc.1")
	require.NoError(t, err)
	release, err := parseVersion("v2.34.2")
	require.NoError(t, err)

	// A release candidate is always the rc channel, even if the stable
	// flag is somehow set.
	assert.Equal(t, "rc", channelFor(rc, false))
	assert.Equal(t, "rc", channelFor(rc, true))
	assert.Equal(t, "stable", channelFor(release, true))
	assert.Equal(t, "mainline", channelFor(release, false))
}

func Test_currentBranch(t *testing.T) {
	t.Parallel()

	t.Run("OnBranch", func(t *testing.T) {
		t.Parallel()
		mock := &mockExecutor{
			RunOutputFunc: func(_ string, args ...string) (string, error) {
				if len(args) >= 1 && args[0] == "branch" {
					return "release/2.34", nil
				}
				return "", nil
			},
		}
		branch, err := currentBranch(mock)
		require.NoError(t, err)
		assert.Equal(t, "release/2.34", branch)
	})

	t.Run("DetachedAncestorOfMain", func(t *testing.T) {
		t.Parallel()
		// Empty branch (detached HEAD); merge-base --is-ancestor
		// succeeds, so we treat it as main.
		mock := &mockExecutor{
			RunOutputFunc: func(_ string, _ ...string) (string, error) { return "", nil },
			RunFunc:       func(_ string, _ ...string) error { return nil },
		}
		branch, err := currentBranch(mock)
		require.NoError(t, err)
		assert.Equal(t, "main", branch)
	})

	t.Run("DetachedNotAncestorOfMain", func(t *testing.T) {
		t.Parallel()
		mock := &mockExecutor{
			RunOutputFunc: func(_ string, _ ...string) (string, error) { return "", nil },
			RunFunc:       func(_ string, _ ...string) error { return assert.AnError },
		}
		_, err := currentBranch(mock)
		require.Error(t, err)
	})
}

func Test_triggerReleaseWorkflow(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{}
	err := triggerReleaseWorkflow(mock, "v2.35.0-rc.0", "rc", "the release notes")
	require.NoError(t, err)

	require.Len(t, mock.MutationCalls, 1)
	call := mock.MutationCalls[0]
	assert.Contains(t, call, "gh workflow run release.yaml")
	assert.Contains(t, call, "--repo coder/coder")
	assert.Contains(t, call, "--ref v2.35.0-rc.0")
	assert.Contains(t, call, "-f dry_run=false")
	assert.Contains(t, call, "-f release_channel=rc")
	assert.Contains(t, call, "-f release_notes=the release notes")
}
