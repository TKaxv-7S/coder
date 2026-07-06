package chatd

// Migration spike: real chatd structured-output tests (title generation,
// turn-status-label generation) reimplemented with fakellm's object
// steps instead of chattest.FakeModel.GenerateObjectFn, to validate the
// new step kind against actual production code rather than just the
// fakellm package's own unit tests.

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func Test_generateManualTitle_UsesTimeout_FakeLLM(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("refresh chat title"),
		),
	}

	model := fakellm.NewModel(fakellm.MustParseString(`{"object": {"value": {"title": "Refresh title"}}}`))

	title, _, err := generateManualTitle(context.Background(), messages, model)
	require.NoError(t, err)
	require.Equal(t, "Refresh title", title)

	// Assertions on the outgoing call happen after the fact, against the
	// captured ObjectCall, instead of inline inside GenerateObjectFn.
	calls := model.ObjectCalls()
	require.Len(t, calls, 1)
	require.Equal(t, "propose_title", calls[0].Call.SchemaName)
	require.Len(t, calls[0].Call.Prompt, 2)

	// The deadline assertion from the original test *can* be expressed
	// now: fakellm captures the context.Context each call arrived with,
	// so a test can inspect its Deadline()/Err() after the fact instead
	// of needing a live closure like chattest.FakeModel's GenerateObjectFn.
	deadline, ok := calls[0].Ctx.Deadline()
	require.True(t, ok, "manual title generation should set a deadline")
	require.WithinDuration(t, time.Now().Add(30*time.Second), deadline, 2*time.Second)
}

func Test_generateManualTitle_ReturnsUsageForEmptyNormalizedTitle_FakeLLM(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("refresh chat title"),
		),
	}

	model := fakellm.NewModel(fakellm.MustParseString(
		`{"object": {"value": {"title": "\"\""}, "usage": {"input_tokens": 11, "output_tokens": 7, "total_tokens": 18}}}`,
	))

	_, usage, err := generateManualTitle(context.Background(), messages, model)
	require.ErrorContains(t, err, "generated title was empty")
	require.Equal(t, int64(11), usage.InputTokens)
	require.Equal(t, int64(7), usage.OutputTokens)
	require.Equal(t, int64(18), usage.TotalTokens)
}

func TestGenerateStructuredTurnStatusLabel_FakeLLM(t *testing.T) {
	t.Parallel()

	t.Run("returns compact label", func(t *testing.T) {
		t.Parallel()

		model := fakellm.NewModel(fakellm.MustParseString(`{"object": {"value": {"label": "Submitted PR"}}}`))

		label, err := generateStructuredTurnStatusLabel(t.Context(), model, turnStatusLabelPrompt, "done")
		require.NoError(t, err)
		require.Equal(t, "Submitted PR", label)

		calls := model.ObjectCalls()
		require.Len(t, calls, 1)
		require.Equal(t, "propose_turn_status_label", calls[0].Call.SchemaName)
	})

	t.Run("rejects narrative label", func(t *testing.T) {
		t.Parallel()

		model := fakellm.NewModel(fakellm.MustParseString(`{"object": {"value": {"label": "Agent identified failing tests"}}}`))

		_, err := generateStructuredTurnStatusLabel(t.Context(), model, turnStatusLabelPrompt, "done")
		require.ErrorContains(t, err, "generated turn status label was invalid")
	})
}

var _ fantasy.LanguageModel = (*fakellm.Model)(nil)
