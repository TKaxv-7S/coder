package chatd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func claudeCodeTextMessage(t *testing.T, id int64, role database.ChatMessageRole, text string) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
	})
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func TestClaudeCodeTurnFromHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slog.Logger{}

	t.Run("SingleUserMessage", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			claudeCodeTextMessage(t, 1, database.ChatMessageRoleUser, "hello"),
		})
		require.NoError(t, err)
		require.True(t, turn.generate)
		require.Equal(t, "hello", turn.prompt)
		require.Empty(t, turn.reseed)
	})

	t.Run("TrailingUserRunJoined", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			claudeCodeTextMessage(t, 1, database.ChatMessageRoleUser, "first"),
			claudeCodeTextMessage(t, 2, database.ChatMessageRoleAssistant, "reply"),
			claudeCodeTextMessage(t, 3, database.ChatMessageRoleUser, "second"),
			claudeCodeTextMessage(t, 4, database.ChatMessageRoleUser, "third"),
		})
		require.NoError(t, err)
		require.True(t, turn.generate)
		require.Equal(t, "second\n\nthird", turn.prompt)
		// Everything before the trailing user run is reseed context.
		require.Len(t, turn.reseed, 2)
		require.Equal(t, "User", turn.reseed[0].Role)
		require.Equal(t, "first", turn.reseed[0].Text)
		require.Equal(t, "Assistant", turn.reseed[1].Role)
		require.Equal(t, "reply", turn.reseed[1].Text)
	})

	t.Run("HistoryEndsWithAssistant", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			claudeCodeTextMessage(t, 1, database.ChatMessageRoleUser, "hello"),
			claudeCodeTextMessage(t, 2, database.ChatMessageRoleAssistant, "done"),
		})
		require.NoError(t, err)
		// The turn's output is already committed; only FinishTurn
		// remains.
		require.False(t, turn.generate)
	})

	t.Run("EmptyHistory", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, nil)
		require.NoError(t, err)
		require.False(t, turn.generate)
	})

	t.Run("NonTextUserMessageErrors", func(t *testing.T) {
		t.Parallel()
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeFile, FileName: "img.png", MediaType: "image/png"},
		})
		require.NoError(t, err)
		_, err = claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			{
				ID:             1,
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no text content")
	})
}
