package structuredoutput_test

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyschema "charm.land/fantasy/schema"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
)

const validSchema = `{
	"type": "object",
	"properties": {
		"title": {"type": "string"},
		"tags": {"type": "array", "items": {"type": "string"}},
		"score": {"type": "integer", "minimum": 0}
	},
	"required": ["title", "score"],
	"additionalProperties": false
}`

func schemaFormat(schema string) *codersdk.ChatResponseFormat {
	return &codersdk.ChatResponseFormat{Schema: json.RawMessage(schema)}
}

func TestNewRequest(t *testing.T) {
	t.Parallel()

	t.Run("NilForNoRequest", func(t *testing.T) {
		t.Parallel()
		req, verr := structuredoutput.NewRequest(nil)
		require.Nil(t, verr)
		require.Nil(t, req)
	})

	t.Run("CarriesDescription", func(t *testing.T) {
		t.Parallel()
		format := schemaFormat(validSchema)
		format.Description = "a report"
		req, verr := structuredoutput.NewRequest(format)
		require.Nil(t, verr)
		require.Equal(t, "a report", req.Description)
	})

	t.Run("SchemaValidation", func(t *testing.T) {
		t.Parallel()
		fragmentRefSchema := `{
			"type": "object",
			"properties": {"node": {"$ref": "#/$defs/node"}},
			"$defs": {"node": {"type": "object", "properties": {"name": {"type": "string"}}}}
		}`
		// Exceeds maxSchemaBytes (16 KiB).
		hugeSchema := `{"type":"object","description":"` + strings.Repeat("x", 16*1024) + `"}`

		cases := []struct {
			name   string
			schema string
			// wantDetail is a substring of the expected validation
			// error detail; empty means the schema must be accepted.
			wantDetail string
		}{
			{name: "Valid", schema: validSchema},
			{name: "FragmentLocalRef", schema: fragmentRefSchema},
			{name: "Missing", schema: "", wantDetail: "is required"},
			{name: "TooLarge", schema: hugeSchema, wantDetail: "maximum size"},
			{name: "NotJSON", schema: `{"type": "object"`, wantDetail: "must be a JSON object"},
			{name: "RootArray", schema: `{"type":"array","items":{"type":"string"}}`, wantDetail: `"type":"object"`},
			{name: "RootString", schema: `{"type":"string"}`, wantDetail: `"type":"object"`},
			{name: "RootTypeOmitted", schema: `{"properties":{"a":{"type":"string"}}}`, wantDetail: `"type":"object"`},
			{name: "RootBoolean", schema: `true`, wantDetail: "must be a JSON object"},
			{name: "RemoteHTTPRef", schema: `{"type":"object","properties":{"a":{"$ref":"https://example.com/schema.json"}}}`, wantDetail: "fragment-local"},
			{name: "RemoteFileRef", schema: `{"type":"object","properties":{"a":{"$ref":"file:///etc/passwd"}}}`, wantDetail: "fragment-local"},
			{name: "RemoteDynamicRef", schema: `{"type":"object","properties":{"a":{"$dynamicRef":"https://example.com/x"}}}`, wantDetail: "fragment-local"},
			{name: "RelativeDocumentRef", schema: `{"type":"object","allOf":[{"$ref":"other.json#/foo"}]}`, wantDetail: "fragment-local"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				req, verr := structuredoutput.NewRequest(schemaFormat(tc.schema))
				if tc.wantDetail == "" {
					require.Nil(t, verr)
					require.NotNil(t, req)
					return
				}
				require.NotNil(t, verr)
				require.Equal(t, "response_format.schema", verr.Field)
				require.Contains(t, verr.Detail, tc.wantDetail)
			})
		}
	})
}

func TestTool(t *testing.T) {
	t.Parallel()

	newRequest := func(t *testing.T, schema string) *structuredoutput.Request {
		t.Helper()
		req, verr := structuredoutput.NewRequest(schemaFormat(schema))
		require.Nil(t, verr)
		require.NotNil(t, req)
		return req
	}

	t.Run("Info", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		info := tool.Info()
		require.Equal(t, structuredoutput.ToolName, info.Name)
		require.Equal(t, []string{"output"}, info.Required)
		require.False(t, info.Parallel)

		// The caller schema is wrapped under the "output" property.
		outputSchema, ok := info.Parameters["output"].(map[string]any)
		require.True(t, ok, "output parameter should be the caller schema")
		require.Equal(t, "object", outputSchema["type"])
		var want map[string]any
		require.NoError(t, json.Unmarshal([]byte(validSchema), &want))
		require.Equal(t, want, outputSchema)

		// Each Info call returns a deep copy: mutations by consumers
		// (e.g. chatloop's schema.Normalize) must not leak into
		// later calls.
		outputSchema["type"] = "mutated"
		fresh, ok := tool.Info().Parameters["output"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "object", fresh["type"])
	})

	t.Run("ExemptFromResultTruncation", func(t *testing.T) {
		t.Parallel()
		// The successful result is canonical validated JSON; chatloop
		// must never truncate it (truncation would corrupt the payload
		// while persisting it as a success).
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		exempter, ok := tool.(chatloop.ResultTruncationExempter)
		require.True(t, ok, "finalizer must implement chatloop.ResultTruncationExempter")
		require.True(t, exempter.ExemptFromResultTruncation())
	})

	t.Run("InfoIncludesDescription", func(t *testing.T) {
		t.Parallel()
		format := schemaFormat(validSchema)
		format.Description = "a quarterly report"
		req, verr := structuredoutput.NewRequest(format)
		require.Nil(t, verr)
		info := structuredoutput.Tool(req).Info()
		require.Contains(t, info.Description, "Output description: a quarterly report")
	})

	t.Run("RunValidCanonicalizes", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		resp, err := tool.Run(t.Context(), fantasy.ToolCall{
			ID:   "call_1",
			Name: structuredoutput.ToolName,
			// Non-canonical spacing and key order.
			Input: "{\"output\": {\n\t\"score\": 3,   \"title\": \"hi\"\n}}",
		})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.JSONEq(t, `{"score":3,"title":"hi"}`, resp.Content)
		// Canonical encoding is compact.
		require.NotContains(t, resp.Content, "\n")
	})

	t.Run("RunInvalidJSONArgs", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		resp, err := tool.Run(t.Context(), fantasy.ToolCall{Input: `not json`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "invalid arguments")
	})

	t.Run("RunMissingOutput", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		resp, err := tool.Run(t.Context(), fantasy.ToolCall{Input: `{"result": {}}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, `missing required "output" argument`)
	})

	t.Run("RunSchemaMismatch", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		for _, input := range []string{
			`{"output": {"title": "hi"}}`,                            // missing required score
			`{"output": {"title": "hi", "score": -1}}`,               // minimum violation
			`{"output": {"title": "hi", "score": 1, "extra": true}}`, // additionalProperties
			`{"output": {"title": 42, "score": 1}}`,                  // wrong type
			`{"output": ["not", "an", "object"]}`,                    // wrong root kind
		} {
			resp, err := tool.Run(t.Context(), fantasy.ToolCall{Input: input})
			require.NoError(t, err)
			require.True(t, resp.IsError, "input %s should fail validation", input)
			require.Contains(t, resp.Content, "does not satisfy the required schema")
			require.Contains(t, resp.Content, structuredoutput.ToolName)
		}
	})

	// buildToolDefinitions in chatloop runs schema.Normalize on the
	// wrapped tool schema before sending it to the provider. Guard
	// that normalizing a nested caller schema keeps its semantics
	// (type arrays become anyOf, bare arrays gain items) and that
	// the in-place mutation does not corrupt the tool's validation.
	t.Run("SchemaNormalizeRoundTrip", func(t *testing.T) {
		t.Parallel()
		nested := `{
			"type": "object",
			"properties": {
				"name": {"type": ["string", "null"]},
				"list": {"type": "array"},
				"child": {
					"type": "object",
					"properties": {"deep": {"type": ["integer", "string"]}},
					"additionalProperties": false
				}
			},
			"required": ["name"]
		}`
		tool := structuredoutput.Tool(newRequest(t, nested))
		info := tool.Info()

		// Mirror the wrapping and normalization the chatloop applies
		// before sending the tool definition to the provider.
		inputSchema := map[string]any{
			"type":       "object",
			"properties": info.Parameters,
			"required":   info.Required,
		}
		fantasyschema.Normalize(inputSchema)

		// The normalized tool-input schema validates the whole
		// {"output": ...} argument object. Build a finalizer for it
		// and nest the tool args under "output" once so its Run
		// validates them against the normalized schema.
		normalized, err := json.Marshal(inputSchema)
		require.NoError(t, err)
		envelopeTool := structuredoutput.Tool(newRequest(t, string(normalized)))

		validArgs := `{"output": {"name": null, "list": [1, "x"], "child": {"deep": "y"}}}`
		invalidArgs := `{"output": {"child": {"deep": true}}}`
		resp, err := envelopeTool.Run(t.Context(), fantasy.ToolCall{Input: `{"output": ` + validArgs + `}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, "normalized schema must accept what the original accepts")
		resp, err = envelopeTool.Run(t.Context(), fantasy.ToolCall{Input: `{"output": ` + invalidArgs + `}`})
		require.NoError(t, err)
		require.True(t, resp.IsError, "normalized schema must reject what the original rejects")

		// The Normalize mutation above must not corrupt the tool's
		// own validation: Run validates against the compiled
		// schema, and each Info call deep-copies the parameter map.
		resp, runErr := tool.Run(t.Context(), fantasy.ToolCall{Input: validArgs})
		require.NoError(t, runErr)
		require.False(t, resp.IsError)
		resp, runErr = tool.Run(t.Context(), fantasy.ToolCall{Input: invalidArgs})
		require.NoError(t, runErr)
		require.True(t, resp.IsError)
	})
}
