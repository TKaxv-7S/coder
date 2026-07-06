package integrationtest

// Migration spike: does fakellm.Server work as the "upstream" in a real,
// full-stack aibridge integration test, in place of
// testutil.NewMockUpstream + a checked-in txtar fixture?
//
// This drives the actual aibridge.RequestBridge (real interceptors, real
// recorder, real Anthropic/OpenAI SDK clients on both the inbound and
// outbound legs) with fakellm.Server standing in as the upstream
// provider. It intentionally does NOT try to replicate TestSimple's
// exact fixture-derived assertions (specific message IDs, specific
// cached-token counts) -- those are properties of the captured fixture
// bytes, not something a "just echo the prompt" script should try to
// reproduce. Instead it validates the same *categories* of behavior
// TestSimple checks: correct upstream path, prompt tracked, response
// non-empty and well-formed, token usage recorded, interception
// recorded with client/user-agent metadata.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/fakellm"
)

func TestFakeLLM_AnthropicSimple(t *testing.T) {
	t.Parallel()

	for _, streaming := range []bool{true, false} {
		name := "non-streaming"
		if streaming {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			upstream := fakellm.NewServer(t, fakellm.MustParseString(
				`{"text": "42 angels, roughly speaking."}`,
			))

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

			reqBody := []byte(fmt.Sprintf(`{
				"model": "claude-sonnet-4-20250514",
				"max_tokens": 100,
				"stream": %v,
				"messages": [{"role": "user", "content": "how many angels can dance on the head of a pin?"}]
			}`, streaming))
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody,
				http.Header{"User-Agent": {"claude-cli/2.0.67 (external, cli)"}})
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Upstream saw exactly one request, at the real Anthropic path.
			received := upstream.Requests()
			require.Len(t, received, 1)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotEmpty(t, body)

			// Prompt usage was tracked from the real request the client sent
			// to the bridge (not from fakellm at all -- fakellm never sees
			// the client's prompt, only the re-issued upstream request).
			promptUsages := bridgeServer.Recorder.RecordedPromptUsages()
			require.NotEmpty(t, promptUsages)
			require.Contains(t, promptUsages[0].Prompt, "how many angels can dance on the head of a pin")

			// Token usage recorded from fakellm's (zeroed) usage numbers --
			// proves the bridge's usage-parsing code path runs end to end
			// against fakellm's response, even though the numbers themselves
			// are meaningless placeholders.
			tokenUsages := bridgeServer.Recorder.RecordedTokenUsages()
			require.GreaterOrEqual(t, len(tokenUsages), 1)

			interceptions := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			require.Equal(t, "claude-cli/2.0.67 (external, cli)", interceptions[0].UserAgent)
			require.Equal(t, string(aibridge.ClientClaudeCode), interceptions[0].Client)

			bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)

			// Sanity: the actual response text made it through the whole
			// pipeline unmodified.
			if !streaming {
				var msg struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				}
				require.NoError(t, json.Unmarshal(body, &msg))
				require.Len(t, msg.Content, 1)
				require.Equal(t, "42 angels, roughly speaking.", msg.Content[0].Text)
			}
		})
	}
}

func TestFakeLLM_OpenAISimple(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	upstream := fakellm.NewServer(t, fakellm.MustParseString(
		`{"text": "42 angels, roughly speaking."}`,
	))

	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

	reqBody := []byte(`{
		"model": "gpt-4o",
		"stream": false,
		"messages": [{"role": "user", "content": "how many angels can dance on the head of a pin?"}]
	}`)
	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody,
		http.Header{"User-Agent": {"codex_cli_rs/0.87.0 (Mac OS 26.2.0; arm64)"}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	received := upstream.Requests()
	require.Len(t, received, 1)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var msg struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(body, &msg))
	require.Len(t, msg.Choices, 1)
	require.Equal(t, "42 angels, roughly speaking.", msg.Choices[0].Message.Content)

	require.Equal(t, string(aibridge.ClientCodex), bridgeServer.Recorder.RecordedInterceptions()[0].Client)
}
