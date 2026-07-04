package chatd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
)

// structuredOutputOverlayPrompt instructs the model how to finish a
// structured output turn.
const structuredOutputOverlayPrompt = `<structured_output>
This turn requires a structured final answer.
- Use your normal tools first as needed to gather information and complete the task.
- When the work is done, call the ` + structuredoutput.ToolName + ` tool exactly once with the final answer in its "output" argument. The output must satisfy the tool's JSON schema.
- Never emit the final answer as plain text; only the ` + structuredoutput.ToolName + ` tool result counts.
- Call ` + structuredoutput.ToolName + ` alone, never batched with other tool calls.
- If the tool returns a validation error, fix the listed fields and call it again.
</structured_output>`

// activeTurnResponseFormat returns the active turn's structured
// output request from the latest visible user message. It must
// receive the full message list (not compaction-filtered prompt
// rows) so the trigger message survives compaction. The last
// response-format part wins.
func activeTurnResponseFormat(
	ctx context.Context,
	logger slog.Logger,
	messages []database.ChatMessage,
) *structuredoutput.Request {
	// Only user-authored (user-visible) messages carry the
	// response-format part. Skip hidden model-visibility user rows
	// (e.g. injected context) like activeTurnAPIKeyIDFromMessages
	// does.
	idx := lastMessageIndex(messages, func(message database.ChatMessage) bool {
		return message.Role == database.ChatMessageRoleUser && isUserVisibleChatMessage(message)
	})
	if idx == -1 {
		return nil
	}
	message := messages[idx]
	parts, err := chatprompt.ParseContent(message)
	if err != nil {
		logger.Warn(ctx, "failed to parse user message for response format",
			slog.F("message_id", message.ID),
			slog.Error(err),
		)
		return nil
	}
	var format *codersdk.ChatResponseFormat
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeResponseFormat && part.ResponseFormat != nil {
			format = part.ResponseFormat
		}
	}
	if format == nil {
		return nil
	}
	request, verr := structuredoutput.NewRequest(format)
	if verr != nil {
		// The HTTP layer validates before persisting, so a
		// persisted-but-invalid format indicates a version skew
		// or manual edit. Degrade to a normal turn rather than
		// failing generation forever.
		logger.Warn(ctx, "persisted response format is invalid; ignoring",
			slog.F("message_id", message.ID),
			slog.F("field", verr.Field),
			slog.F("detail", verr.Detail),
		)
		return nil
	}
	return request
}
