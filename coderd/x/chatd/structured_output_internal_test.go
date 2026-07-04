package chatd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
)

const testResponseFormatSchema = `{
	"type": "object",
	"properties": {"answer": {"type": "string"}},
	"required": ["answer"]
}`

func testResponseFormat() codersdk.ChatResponseFormat {
	return codersdk.ChatResponseFormat{
		Schema:      json.RawMessage(testResponseFormatSchema),
		Description: "test answer",
	}
}

func userMessageWithFormat(t *testing.T, id int64, text string, format *codersdk.ChatResponseFormat) database.ChatMessage {
	t.Helper()
	parts := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
	if format != nil {
		parts = append(parts, codersdk.ChatMessageResponseFormat(*format))
	}
	return dbMessage(t, id, database.ChatMessageRoleUser, false, parts...)
}

func textAssistantMessage(t *testing.T, id int64, text string) database.ChatMessage {
	t.Helper()
	return dbMessage(t, id, database.ChatMessageRoleAssistant, false,
		codersdk.ChatMessageText(text),
	)
}

func finalizerCallMessage(t *testing.T, id int64, callID string, args string) database.ChatMessage {
	t.Helper()
	return dbMessage(t, id, database.ChatMessageRoleAssistant, false,
		codersdk.ChatMessageToolCall(callID, structuredoutput.ToolName, json.RawMessage(args)),
	)
}

func finalizerResultMessage(t *testing.T, id int64, callID string, result string, isError bool) database.ChatMessage {
	t.Helper()
	return dbMessage(t, id, database.ChatMessageRoleTool, false,
		codersdk.ChatMessageToolResult(callID, structuredoutput.ToolName, json.RawMessage(result), isError, false),
	)
}

func TestActiveTurnResponseFormat(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := slog.Make()
	format := testResponseFormat()

	t.Run("NoFormat", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "hello", nil),
		}
		require.Nil(t, activeTurnResponseFormat(ctx, logger, messages))
	})

	t.Run("ActiveTurnFormat", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "hello", &format),
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
		require.Equal(t, "test answer", req.Description)
	})

	t.Run("OlderTurnFormatIgnored", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured please", &format),
			textAssistantMessage(t, 2, "done"),
			userMessageWithFormat(t, 3, "now plain text", nil),
		}
		require.Nil(t, activeTurnResponseFormat(ctx, logger, messages))
	})

	t.Run("MultiTurnLatestWins", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "plain", nil),
			textAssistantMessage(t, 2, "ok"),
			userMessageWithFormat(t, 3, "structured", &format),
			textAssistantMessage(t, 4, "working on it"),
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
	})

	t.Run("SurvivesCompaction", func(t *testing.T) {
		t.Parallel()
		// Compaction marks older rows Compressed and inserts a
		// compressed summary user row; the trigger user message of
		// the active turn stays uncompressed.
		compressedSummary := userMessageWithFormat(t, 3, "summary", nil)
		compressedSummary.Visibility = database.ChatMessageVisibilityModel
		compressedSummary.Compressed = true
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "old", nil),
			textAssistantMessage(t, 2, "old answer"),
			compressedSummary,
			userMessageWithFormat(t, 4, "structured", &format),
			textAssistantMessage(t, 5, "step one"),
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
	})

	t.Run("SkipsModelVisibilityUserRows", func(t *testing.T) {
		t.Parallel()
		hidden := userMessageWithFormat(t, 2, "injected context", nil)
		hidden.Visibility = database.ChatMessageVisibilityModel
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			hidden,
		}
		req := activeTurnResponseFormat(ctx, logger, messages)
		require.NotNil(t, req)
	})

	t.Run("LastPartWinsWithinMessage", func(t *testing.T) {
		t.Parallel()
		second := testResponseFormat()
		second.Description = "second format"
		msg := dbMessage(t, 1, database.ChatMessageRoleUser, false,
			codersdk.ChatMessageText("hello"),
			codersdk.ChatMessageResponseFormat(format),
			codersdk.ChatMessageResponseFormat(second),
		)
		req := activeTurnResponseFormat(ctx, logger, []database.ChatMessage{msg})
		require.NotNil(t, req)
		require.Equal(t, "second format", req.Description)
	})

	t.Run("InvalidPersistedFormatIgnored", func(t *testing.T) {
		t.Parallel()
		invalid := codersdk.ChatResponseFormat{
			Schema: json.RawMessage(`{"type":"string"}`),
		}
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &invalid),
		}
		require.Nil(t, activeTurnResponseFormat(ctx, logger, messages))
	})
}

func TestDecideGenerationActionStructuredOutput(t *testing.T) {
	t.Parallel()

	format := testResponseFormat()
	// structuredDecision runs decideGenerationAction with the inputs
	// prepareGeneration derives for a structured output turn.
	structuredDecision := func(maxSteps int, messages []database.ChatMessage) (generationDecision, error) {
		return decideGenerationAction(generationDecisionInput{
			messages:                 messages,
			stopAfterTools:           map[string]struct{}{structuredoutput.ToolName: {}},
			structuredOutputRequired: true,
			maxSteps:                 maxSteps,
		})
	}

	t.Run("TextOnlyDoesNotFinish", func(t *testing.T) {
		t.Parallel()
		decision, err := structuredDecision(10, []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			textAssistantMessage(t, 2, "here is your answer as text"),
		})
		require.NoError(t, err)
		require.Equal(t, generationActionGenerateAssistant, decision.kind)
	})

	t.Run("TextOnlyFinishesWithoutStructuredOutput", func(t *testing.T) {
		t.Parallel()
		decision, err := decideGenerationAction(generationDecisionInput{
			messages: []database.ChatMessage{
				userMessageWithFormat(t, 1, "plain", nil),
				textAssistantMessage(t, 2, "answer"),
			},
			maxSteps: 10,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
		require.Equal(t, generationFinishReasonComplete, decision.finishReason)
	})

	t.Run("SuccessfulFinalizerFinishesTurn", func(t *testing.T) {
		t.Parallel()
		decision, err := structuredDecision(10, []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			finalizerCallMessage(t, 2, "call_1", `{"output":{"answer":"42"}}`),
			finalizerResultMessage(t, 3, "call_1", `{"answer":"42"}`, false),
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
		require.Equal(t, generationFinishReasonStopAfterTool, decision.finishReason)
	})

	t.Run("ErrorFinalizerResultRetries", func(t *testing.T) {
		t.Parallel()
		decision, err := structuredDecision(10, []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			finalizerCallMessage(t, 2, "call_1", `{"output":{}}`),
			finalizerResultMessage(t, 3, "call_1", `{"error":"missing answer"}`, true),
		})
		require.NoError(t, err)
		require.Equal(t, generationActionGenerateAssistant, decision.kind)
	})

	t.Run("MaxStepsWithoutFinalizerFailsTerminally", func(t *testing.T) {
		t.Parallel()
		_, err := structuredDecision(2, []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
			textAssistantMessage(t, 2, "text one"),
			textAssistantMessage(t, 3, "text two"),
		})
		require.Error(t, err)
		require.True(t, isTerminalGeneration(err))
		require.ErrorIs(t, err, errStructuredOutputNotProduced)
	})

	t.Run("TextOnlyStreakBelowCapRegenerates", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
		}
		for i := range maxStructuredOutputTextOnlySteps - 1 {
			messages = append(messages, textAssistantMessage(t, int64(2+i), "plain text"))
		}
		decision, err := structuredDecision(100, messages)
		require.NoError(t, err)
		require.Equal(t, generationActionGenerateAssistant, decision.kind)
	})

	t.Run("TextOnlyStreakAtCapFailsTerminally", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
		}
		for i := range maxStructuredOutputTextOnlySteps {
			messages = append(messages, textAssistantMessage(t, int64(2+i), "plain text"))
		}
		_, err := structuredDecision(100, messages)
		require.Error(t, err)
		require.True(t, isTerminalGeneration(err))
		require.ErrorIs(t, err, errStructuredOutputNotProduced)
	})

	t.Run("ToolCallResetsTextOnlyStreak", func(t *testing.T) {
		t.Parallel()
		// A failed finalizer attempt (or any other tool call) resets
		// the consecutive text-only streak so validation retries keep
		// their full budget.
		messages := []database.ChatMessage{
			userMessageWithFormat(t, 1, "structured", &format),
		}
		for i := range maxStructuredOutputTextOnlySteps {
			messages = append(messages, textAssistantMessage(t, int64(2+i), "plain text"))
		}
		next := int64(2 + maxStructuredOutputTextOnlySteps)
		messages = append(messages,
			finalizerCallMessage(t, next, "call_1", `{"output":{}}`),
			finalizerResultMessage(t, next+1, "call_1", `{"error":"missing answer"}`, true),
			textAssistantMessage(t, next+2, "still plain text"),
		)
		decision, err := structuredDecision(100, messages)
		require.NoError(t, err)
		require.Equal(t, generationActionGenerateAssistant, decision.kind)
	})

	t.Run("MaxStepsWithoutStructuredOutputFinishes", func(t *testing.T) {
		t.Parallel()
		decision, err := decideGenerationAction(generationDecisionInput{
			messages: []database.ChatMessage{
				userMessageWithFormat(t, 1, "plain", nil),
				dbMessage(t, 2, database.ChatMessageRoleAssistant, false,
					codersdk.ChatMessageToolCall("call_1", "read_file", json.RawMessage(`{}`)),
				),
				dbMessage(t, 3, database.ChatMessageRoleTool, false,
					codersdk.ChatMessageToolResult("call_1", "read_file", json.RawMessage(`{}`), false, false),
				),
			},
			maxSteps: 1,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
		require.Equal(t, generationFinishReasonMaxSteps, decision.finishReason)
	})
}
