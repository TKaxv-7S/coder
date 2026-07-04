package chatd_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const structuredE2ESchema = `{
	"type": "object",
	"properties": {"answer": {"type": "string"}},
	"required": ["answer"],
	"additionalProperties": false
}`

// structuredOutputE2E bundles the shared fixture of the structured
// output e2e tests: a database, a fake OpenAI-compatible provider,
// and an active chatd server.
type structuredOutputE2E struct {
	ctx    context.Context
	db     database.Store
	user   database.User
	org    database.Organization
	model  database.ChatModelConfig
	server *chatd.Server
}

// newStructuredOutputE2E starts the fixture. streamFn handles the
// provider's streamed generation calls; non-streaming (title)
// requests are answered automatically.
func newStructuredOutputE2E(t *testing.T, streamFn func(req *chattest.OpenAIRequest) chattest.OpenAIResponse) *structuredOutputE2E {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return streamFn(req)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	return &structuredOutputE2E{
		ctx:    testutil.Context(t, testutil.WaitLong),
		db:     db,
		user:   user,
		org:    org,
		model:  model,
		server: newActiveTestServer(t, db, ps),
	}
}

// createChat starts a turn whose trigger user message carries the
// test schema as its structured output request.
func (e *structuredOutputE2E) createChat(t *testing.T, title string, mutate ...func(*chatd.CreateOptions)) database.Chat {
	t.Helper()
	opts := chatd.CreateOptions{
		OrganizationID: e.org.ID,
		OwnerID:        e.user.ID,
		APIKeyID:       testAPIKeyID(t, e.db, e.user.ID),
		Title:          title,
		ModelConfigID:  e.model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("answer with structure"),
			codersdk.ChatMessageResponseFormat(codersdk.ChatResponseFormat{
				Schema: json.RawMessage(structuredE2ESchema),
			}),
		},
	}
	for _, m := range mutate {
		m(&opts)
	}
	chat, err := e.server.CreateChat(e.ctx, opts)
	require.NoError(t, err)
	return chat
}

func TestActiveServer_StructuredOutput(t *testing.T) {
	t.Parallel()

	t.Run("finalizer success finishes turn", func(t *testing.T) {
		t.Parallel()

		var streamedCallCount atomic.Int32
		var rawBodiesMu sync.Mutex
		var rawBodies []string
		e2e := newStructuredOutputE2E(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			rawBodiesMu.Lock()
			rawBodies = append(rawBodies, string(req.RawBody))
			rawBodiesMu.Unlock()
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"42"}}`),
			)
		})
		chat := e2e.createChat(t, "structured-output-success")

		chatResult := waitForChatStatus(e2e.ctx, t, e2e.db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chatResult.WorkerID.Valid)
		require.Equal(t, int32(1), streamedCallCount.Load(),
			"successful finalizer result should stop the turn after one model call")

		parts := chatToolParts(e2e.ctx, t, e2e.db, chat.ID)
		call := requireToolCallPart(t, parts, structuredoutput.ToolName)
		require.JSONEq(t, `{"output":{"answer":"42"}}`, string(call.Args))
		result := requireToolResultPart(t, parts, structuredoutput.ToolName)
		require.False(t, result.IsError)
		require.JSONEq(t, `{"answer":"42"}`, string(result.Result))

		rawBodiesMu.Lock()
		bodies := append([]string(nil), rawBodies...)
		rawBodiesMu.Unlock()
		require.Len(t, bodies, 1)
		// Required tool choice is set on structured output steps.
		require.Contains(t, bodies[0], `"tool_choice":"required"`)
		// The finalizer tool definition carries the caller schema.
		require.Contains(t, bodies[0], structuredoutput.ToolName)
		// The response-format part must never reach the provider.
		require.NotContains(t, bodies[0], "response-format")
		require.NotContains(t, bodies[0], "response_format")
	})

	t.Run("invalid args retry then success", func(t *testing.T) {
		t.Parallel()

		var streamedCallCount atomic.Int32
		e2e := newStructuredOutputE2E(t, func(_ *chattest.OpenAIRequest) chattest.OpenAIResponse {
			switch streamedCallCount.Add(1) {
			case 1:
				// Missing required "answer" property.
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{}}`),
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"fixed"}}`),
				)
			}
		})
		chat := e2e.createChat(t, "structured-output-retry")

		waitForChatStatus(e2e.ctx, t, e2e.db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load(),
			"validation failure should retry within the same turn")

		parts := chatToolParts(e2e.ctx, t, e2e.db, chat.ID)
		var sawError, sawSuccess bool
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != structuredoutput.ToolName {
				continue
			}
			if part.IsError {
				sawError = true
				require.Contains(t, string(part.Result), "does not satisfy the required schema")
			} else {
				sawSuccess = true
				require.JSONEq(t, `{"answer":"fixed"}`, string(part.Result))
			}
		}
		require.True(t, sawError, "first invalid finalizer call should produce an error tool result")
		require.True(t, sawSuccess, "second finalizer call should produce the validated result")
	})

	t.Run("dynamic tool pauses then finalizes", func(t *testing.T) {
		t.Parallel()

		var streamedCallCount atomic.Int32
		e2e := newStructuredOutputE2E(t, func(_ *chattest.OpenAIRequest) chattest.OpenAIResponse {
			switch streamedCallCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"data"}`),
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"from dynamic"}}`),
				)
			}
		})
		dynamicToolsJSON := dynamicToolJSON(t, "my_dynamic_tool")
		chat := e2e.createChat(t, "structured-output-dynamic", func(opts *chatd.CreateOptions) {
			opts.DynamicTools = dynamicToolsJSON
		})

		// 1. The dynamic tool call pauses the turn as usual.
		var chatResult database.Chat
		testutil.Eventually(e2e.ctx, t, func(ctx context.Context) bool {
			got, getErr := e2e.db.GetChatByID(ctx, chat.ID)
			if getErr != nil {
				return false
			}
			chatResult = got
			return got.Status == database.ChatStatusRequiresAction || got.Status == database.ChatStatusError
		}, testutil.IntervalFast)
		require.Equal(t, database.ChatStatusRequiresAction, chatResult.Status,
			"expected requires_action, got %s (last_error=%q)",
			chatResult.Status, chatLastErrorMessage(chatResult.LastError))

		call := requireToolCallPart(t, chatToolParts(e2e.ctx, t, e2e.db, chat.ID), "my_dynamic_tool")

		// 2. Submitting results resumes the turn, which then
		// finalizes with structured output.
		err := e2e.server.SubmitToolResults(e2e.ctx, chatd.SubmitToolResultsOptions{
			ChatID:        chat.ID,
			UserID:        e2e.user.ID,
			ModelConfigID: chatResult.LastModelConfigID,
			Results: []codersdk.ToolResult{{
				ToolCallID: call.ToolCallID,
				Output:     json.RawMessage(`{"result":"dynamic data"}`),
			}},
			DynamicTools: dynamicToolsJSON,
		})
		require.NoError(t, err)

		waitForChatStatus(e2e.ctx, t, e2e.db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load())
		result := requireToolResultPart(t, chatToolParts(e2e.ctx, t, e2e.db, chat.ID), structuredoutput.ToolName)
		require.False(t, result.IsError)
		require.JSONEq(t, `{"answer":"from dynamic"}`, string(result.Result))
	})

	t.Run("finalizer batched with another tool is rejected then retried", func(t *testing.T) {
		t.Parallel()

		var streamedCallCount atomic.Int32
		e2e := newStructuredOutputE2E(t, func(_ *chattest.OpenAIRequest) chattest.OpenAIResponse {
			switch streamedCallCount.Add(1) {
			case 1:
				// The finalizer is exclusive: batching it with another
				// tool call must fail both with policy errors. Both
				// calls share one chunk with distinct indexes so the
				// stream parser keeps them separate.
				batched := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/x"}`)
				finalizerCall := chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"early"}}`).Choices[0].ToolCalls[0]
				finalizerCall.Index = 1
				batched.Choices[0].ToolCalls = append(batched.Choices[0].ToolCalls, finalizerCall)
				return chattest.OpenAIStreamingResponse(batched)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk(structuredoutput.ToolName, `{"output":{"answer":"alone"}}`),
				)
			}
		})
		chat := e2e.createChat(t, "structured-output-exclusive")

		waitForChatStatus(e2e.ctx, t, e2e.db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load())

		parts := chatToolParts(e2e.ctx, t, e2e.db, chat.ID)
		var policyError, success bool
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != structuredoutput.ToolName {
				continue
			}
			if part.IsError && strings.Contains(string(part.Result), "must be called alone") {
				policyError = true
			}
			if !part.IsError {
				success = true
				require.JSONEq(t, `{"answer":"alone"}`, string(part.Result))
			}
		}
		require.True(t, policyError, "batched finalizer call should produce an exclusivity policy error")
		require.True(t, success, "retried lone finalizer call should succeed")
	})

	t.Run("oversized valid output is not truncated", func(t *testing.T) {
		t.Parallel()

		// Larger than the 16KiB tool-result truncation floor that a
		// small context window produces.
		bigAnswer := strings.Repeat("x", 40_000)
		args, err := json.Marshal(map[string]any{"output": map[string]any{"answer": bigAnswer}})
		require.NoError(t, err)
		e2e := newStructuredOutputE2E(t, func(_ *chattest.OpenAIRequest) chattest.OpenAIResponse {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(structuredoutput.ToolName, string(args)),
			)
		})
		// A small context window keeps the per-result truncation
		// budget at its floor; the schema-valid finalizer result must
		// still persist intact because truncation would corrupt the
		// canonical JSON while reporting success.
		smallContextModel := dbgen.ChatModelConfig(t, e2e.db, database.ChatModelConfig{
			Provider:     "openai-compat",
			ContextLimit: 4096,
		})
		chat := e2e.createChat(t, "structured-output-oversized", func(opts *chatd.CreateOptions) {
			opts.ModelConfigID = smallContextModel.ID
		})

		waitForChatStatus(e2e.ctx, t, e2e.db, chat.ID, database.ChatStatusWaiting)

		result := requireToolResultPart(t, chatToolParts(e2e.ctx, t, e2e.db, chat.ID), structuredoutput.ToolName)
		require.False(t, result.IsError)
		var decoded map[string]string
		require.NoError(t, json.Unmarshal(result.Result, &decoded),
			"persisted structured output result must remain valid JSON")
		require.Equal(t, map[string]string{"answer": bigAnswer}, decoded,
			"persisted result must be the unwrapped, untruncated output value")
	})

	t.Run("persistent text only responses fail fast", func(t *testing.T) {
		t.Parallel()

		var streamedCallCount atomic.Int32
		e2e := newStructuredOutputE2E(t, func(_ *chattest.OpenAIRequest) chattest.OpenAIResponse {
			// Always answer in plain text, as a model or proxy that
			// ignores required tool choice would.
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("plain text answer")...)
		})
		chat := e2e.createChat(t, "structured-output-text-storm")

		chatResult := waitForChatStatus(e2e.ctx, t, e2e.db, chat.ID, database.ChatStatusError)
		payload := requireChatLastErrorPayload(t, chatResult.LastError)
		require.Equal(t, codersdk.ChatErrorKindStructuredOutput, payload.Kind)
		// maxStructuredOutputTextOnlySteps bounds the provider-call
		// storm to a handful of attempts instead of the full step
		// budget.
		require.Equal(t, int32(5), streamedCallCount.Load(),
			"the turn must fail after a short streak of text-only completions")
	})
}
