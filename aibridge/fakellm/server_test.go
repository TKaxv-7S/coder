package fakellm_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
)

// These tests drive fakellm.Server through *real* fantasy provider
// clients (the same code chatd uses in production), proving genuine
// Anthropic/OpenAI wire compliance rather than just round-tripping
// through fakellm's own decoder.

func anthropicClient(t *testing.T, baseURL string) fantasy.LanguageModel {
	t.Helper()
	provider, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(baseURL),
	)
	require.NoError(t, err)
	model, err := provider.LanguageModel(context.Background(), "claude-sonnet-4-20250514")
	require.NoError(t, err)
	return model
}

func openAIClient(t *testing.T, baseURL string) fantasy.LanguageModel {
	t.Helper()
	provider, err := fantasyopenaicompat.New(
		fantasyopenaicompat.WithAPIKey("test-key"),
		fantasyopenaicompat.WithBaseURL(baseURL),
	)
	require.NoError(t, err)
	model, err := provider.LanguageModel(context.Background(), "gpt-4o")
	require.NoError(t, err)
	return model
}

func TestServer_Anthropic_Blocking_Text(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`{"text": "let me check that"}`))
	model := anthropicClient(t, srv.URL)

	resp, err := model.Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("does /tmp/foo exist?")},
	})
	require.NoError(t, err)
	require.Equal(t, "let me check that", resp.Content.Text())
	require.Equal(t, fantasy.FinishReasonStop, resp.FinishReason)
}

func TestServer_Anthropic_Streaming_ToolCall(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`
		{"text": "let me check that"}
		{"tool_call": {"name": "execute", "args": {"command": "ls -l"}, "result": {"ok": true}}}
	`))
	model := anthropicClient(t, srv.URL)

	stream, err := model.Stream(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("does /tmp/foo exist?")},
	})
	require.NoError(t, err)

	var text string
	var toolName, toolInput string
	var finish fantasy.FinishReason
	for part := range stream {
		switch part.Type {
		case fantasy.StreamPartTypeTextDelta:
			text += part.Delta
		case fantasy.StreamPartTypeToolCall:
			toolName = part.ToolCallName
			toolInput = part.ToolCallInput
		case fantasy.StreamPartTypeFinish:
			finish = part.FinishReason
		}
	}
	require.Equal(t, "let me check that", text)
	require.Equal(t, "execute", toolName)
	require.JSONEq(t, `{"command":"ls -l"}`, toolInput)
	require.Equal(t, fantasy.FinishReasonToolCalls, finish)
}

func TestServer_OpenAI_Blocking_Text(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`{"text": "nope it's not there"}`))
	model := openAIClient(t, srv.URL)

	resp, err := model.Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("does /tmp/foo exist?")},
	})
	require.NoError(t, err)
	require.Equal(t, "nope it's not there", resp.Content.Text())
}

func TestServer_OpenAI_Streaming_ToolCall(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`
		{"text": "should I create it?"}
		{"tool_call": {"name": "user_choice", "args": {"options": ["yes", "no"]}, "result": {"choice": "yes"}}}
	`))
	model := openAIClient(t, srv.URL)

	stream, err := model.Stream(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	var text string
	var sawToolCall bool
	var finish fantasy.FinishReason
	for part := range stream {
		switch part.Type {
		case fantasy.StreamPartTypeTextDelta:
			text += part.Delta
		case fantasy.StreamPartTypeToolCall:
			sawToolCall = true
			require.Equal(t, "user_choice", part.ToolCallName)
		case fantasy.StreamPartTypeFinish:
			finish = part.FinishReason
		}
	}
	require.Equal(t, "should I create it?", text)
	require.True(t, sawToolCall)
	require.Equal(t, fantasy.FinishReasonToolCalls, finish)
}

func TestServer_ScriptedError(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`{"error": {"message": "rate limited"}}`))
	model := anthropicClient(t, srv.URL)

	_, err := model.Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("hi")},
	})
	require.Error(t, err)
}

// TestServer_SharedTurnCounterAcrossProviders proves the two wire
// endpoints share one script timeline: whichever provider a request
// hits, it consumes "the next turn," matching Model's semantics.
func TestServer_SharedTurnCounterAcrossProviders(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`
		{"text": "first"}
		{"turn_end": true}
		{"text": "second"}
	`))

	anthropicResp, err := anthropicClient(t, srv.URL).Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	require.Equal(t, "first", anthropicResp.Content.Text())

	openAIResp, err := openAIClient(t, srv.URL).Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	require.Equal(t, "second", openAIResp.Content.Text())

	require.Len(t, srv.Requests(), 2)
}
