package fakellm_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
)

func openAIResponsesClient(t *testing.T, baseURL string) fantasy.LanguageModel {
	t.Helper()
	provider, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(baseURL),
		fantasyopenai.WithUseResponsesAPI(),
	)
	require.NoError(t, err)
	model, err := provider.LanguageModel(context.Background(), "gpt-4o")
	require.NoError(t, err)
	return model
}

func TestServer_OpenAIResponses_Blocking_Text(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`{"text": "42 angels, roughly speaking."}`))
	model := openAIResponsesClient(t, srv.URL)

	resp, err := model.Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("how many angels?")},
	})
	require.NoError(t, err)
	require.Equal(t, "42 angels, roughly speaking.", resp.Content.Text())
}

func TestServer_OpenAIResponses_Streaming_Text(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`{"text": "42 angels, roughly speaking."}`))
	model := openAIResponsesClient(t, srv.URL)

	stream, err := model.Stream(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("how many angels?")},
	})
	require.NoError(t, err)

	var text string
	var sawFinish bool
	for part := range stream {
		switch part.Type {
		case fantasy.StreamPartTypeTextDelta:
			text += part.Delta
		case fantasy.StreamPartTypeFinish:
			sawFinish = true
		}
	}
	require.Equal(t, "42 angels, roughly speaking.", text)
	require.True(t, sawFinish)
}

func TestServer_OpenAIResponses_ScriptedError(t *testing.T) {
	t.Parallel()

	srv := fakellm.NewServer(t, fakellm.MustParseString(`{"error": {"message": "rate limited"}}`))
	model := openAIResponsesClient(t, srv.URL)

	_, err := model.Generate(context.Background(), fantasy.Call{
		Prompt: fantasy.Prompt{fantasy.NewUserMessage("hi")},
	})
	require.Error(t, err)
}
