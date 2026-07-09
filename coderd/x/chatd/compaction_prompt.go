package chatd

import (
	"context"
	"fmt"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
)

// sameCompactionProviderIdentity reports whether the chat model and the
// compaction override model are backed by the same provider instance.
// Legacy configs without an AIProviderID compare as different (fail
// closed), so cross-provider stripping applies.
func sameCompactionProviderIdentity(chatConfig, overrideConfig database.ChatModelConfig) bool {
	return chatConfig.AIProviderID.Valid && overrideConfig.AIProviderID.Valid &&
		chatConfig.AIProviderID.UUID == overrideConfig.AIProviderID.UUID
}

// sanitizeCompactionPrompt adapts the chat prompt for a compaction model
// that differs from the chat model. The prompt was built for the chat
// model, so provider-executed tool history and file parts the compaction
// provider rejects must not replay to it. The input messages are never
// mutated; the assistant generation keeps using the original prompt.
func sanitizeCompactionPrompt(
	ctx context.Context,
	logger slog.Logger,
	prompt []fantasy.Message,
	compactionModel fantasy.LanguageModel,
	chatConfig database.ChatModelConfig,
	overrideConfig database.ChatModelConfig,
) []fantasy.Message {
	messages := prompt
	if !sameCompactionProviderIdentity(chatConfig, overrideConfig) {
		messages = stripProviderExecutedToolParts(ctx, logger, messages)
	}
	messages = replaceUnsupportedFileParts(ctx, logger, messages, func(mediaType string) bool {
		return chatprovider.AcceptsFilePartMediaType(
			compactionModel.Provider(),
			compactionModel.Model(),
			mediaType,
		)
	})
	sanitized, stats := chatsanitize.SanitizeAnthropicProviderToolHistory(
		compactionModel.Provider(),
		messages,
	)
	chatsanitize.LogAnthropicProviderToolSanitization(
		ctx,
		logger,
		"compaction_prompt",
		compactionModel.Provider(),
		compactionModel.Model(),
		stats,
	)
	return sanitized
}

// stripProviderExecutedToolParts removes provider-executed tool calls and
// results from a copy of messages. Provider-executed blocks produced by
// one provider can be rejected when replayed to another, and compaction
// only needs the conversation text. Messages emptied by stripping are
// dropped.
func stripProviderExecutedToolParts(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
) []fantasy.Message {
	removed := 0
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		parts := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			switch typed := part.(type) {
			case fantasy.ToolCallPart:
				if typed.ProviderExecuted {
					removed++
					continue
				}
			case fantasy.ToolResultPart:
				if typed.ProviderExecuted {
					removed++
					continue
				}
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 && len(msg.Content) > 0 {
			continue
		}
		msg.Content = parts
		out = append(out, msg)
	}
	if removed > 0 {
		logger.Debug(ctx, "stripped provider-executed tool history from compaction prompt",
			slog.F("removed_parts", removed),
		)
	}
	return out
}

// replaceUnsupportedFileParts swaps file parts the compaction model does
// not accept for short text placeholders in a copy of messages, so the
// summary notes the attachment existed instead of silently losing it.
func replaceUnsupportedFileParts(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
	acceptsFilePart func(mediaType string) bool,
) []fantasy.Message {
	replaced := 0
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		parts := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			filePart, ok := part.(fantasy.FilePart)
			if !ok || acceptsFilePart(filePart.MediaType) {
				parts = append(parts, part)
				continue
			}
			replaced++
			parts = append(parts, fantasy.TextPart{
				Text: fmt.Sprintf(
					"[Attachment %q (%s) omitted: not supported by the compaction model]",
					filePart.Filename,
					filePart.MediaType,
				),
				ProviderOptions: filePart.ProviderOptions,
			})
		}
		msg.Content = parts
		out = append(out, msg)
	}
	if replaced > 0 {
		logger.Debug(ctx, "replaced unsupported file parts in compaction prompt",
			slog.F("replaced_parts", replaced),
		)
	}
	return out
}
