package chatadvisor_test

// This file is a migration spike: it re-implements a couple of
// representative chatadvisor tests using aibridge/fakellm instead of
// chattest.FakeModel + the package-local streamFromParts/textMessage
// helpers, to see how well the JSONL script format fits real usage.
//
// Findings, see the fakellm design thread for full context:
//   - Single-turn "the model just says X" tests (the overwhelming
//     majority of chatadvisor's StreamFn usage) convert essentially
//     1:1, and the test body gets *shorter*: no streamFromParts
//     boilerplate, no manual fantasy.StreamPart sequencing.
//   - fakellm.NewModel currently returns *fakellm.Model, not
//     fantasy.LanguageModel directly, which is fine here since
//     RuntimeConfig.Model accepts the interface.

import (
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
)

func TestAdvisorRunAdvice_FakeLLM(t *testing.T) {
	t.Parallel()

	const (
		question        = "What is the smallest safe change?"
		maxOutputTokens = int64(321)
	)

	model := fakellm.NewModel(fakellm.MustParseString(`
		{"text": "Take the smallest safe change."}
	`))

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model:           model,
		MaxUsesPerRun:   2,
		MaxOutputTokens: maxOutputTokens,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(t.Context(), question, nil, nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "Take the smallest safe change.", result.Advice)
	require.Equal(t, "fakellm/fakellm", result.AdvisorModel)
	require.Equal(t, 1, result.RemainingUses)
	require.EqualValues(t, 1, model.Calls())
}

func TestAdvisorToolSuccess_FakeLLM(t *testing.T) {
	t.Parallel()

	model := fakellm.NewModel(fakellm.MustParseString(`
		{"text": "Use the smaller diff."}
	`))

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model:           model,
		MaxUsesPerRun:   2,
		MaxOutputTokens: 128,
	})
	require.NoError(t, err)

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "What's the safest next step?"})
	require.False(t, resp.IsError)
}
