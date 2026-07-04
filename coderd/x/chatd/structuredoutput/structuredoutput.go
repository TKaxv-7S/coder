// Package structuredoutput implements server-validated structured
// final output for chat turns. A caller opts in by sending a
// response_format schema on a chat request; chatd runs the normal
// agent loop but injects a server-owned finalizer tool (ToolName)
// that validates the model's arguments against the caller's JSON
// schema, and the turn only finishes successfully once a validated
// result exists. The finalizer is an implementation detail: the API
// guarantee is server-validated output, not provider-native
// constrained decoding.
package structuredoutput

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/kaptinlin/jsonschema"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ToolName is the reserved name of the server-owned finalizer tool.
// Dynamic tools must never use this name; chatd rejects it at the
// HTTP layer and enforces builtin precedence at generation time.
const ToolName = "coder_structured_output"

// maxSchemaBytes caps the caller-provided JSON schema size.
const maxSchemaBytes = 16 * 1024

// outputProperty is the single top-level argument of the finalizer
// tool. The caller schema is nested under it because fantasy tool
// definitions treat ToolInfo.Parameters as a property map of an
// implicit root object schema.
const outputProperty = "output"

// Request is a normalized, validated structured output request
// active for one assistant turn.
type Request struct {
	Description string

	// schemaMap is the decoded schema object, parsed once during
	// NewRequest so tool definitions never reparse the raw bytes.
	schemaMap map[string]any
	compiled  *jsonschema.Schema
}

// NewRequest validates format and compiles its schema. It returns
// (nil, nil) when format is nil, i.e. when the turn has no
// structured output request. Validation errors carry field names so
// HTTP handlers can produce field-specific 400s.
func NewRequest(format *codersdk.ChatResponseFormat) (*Request, *codersdk.ValidationError) {
	if format == nil {
		return nil, nil
	}
	if len(format.Schema) == 0 {
		return nil, &codersdk.ValidationError{
			Field:  "response_format.schema",
			Detail: "is required.",
		}
	}
	if len(format.Schema) > maxSchemaBytes {
		return nil, &codersdk.ValidationError{
			Field:  "response_format.schema",
			Detail: fmt.Sprintf("exceeds the maximum size of %d bytes.", maxSchemaBytes),
		}
	}

	var root map[string]any
	if err := json.Unmarshal(format.Schema, &root); err != nil {
		return nil, &codersdk.ValidationError{
			Field:  "response_format.schema",
			Detail: "must be a JSON object.",
		}
	}
	if rootType, _ := root["type"].(string); rootType != "object" {
		return nil, &codersdk.ValidationError{
			Field:  "response_format.schema",
			Detail: `root must declare "type":"object"; wrap arrays or primitives in an object property.`,
		}
	}
	if refErr := validateFragmentOnlyRefs(root); refErr != nil {
		return nil, refErr
	}

	compiled, err := compileSchema(format.Schema)
	if err != nil {
		return nil, &codersdk.ValidationError{
			Field:  "response_format.schema",
			Detail: fmt.Sprintf("failed to compile: %v.", err),
		}
	}

	return &Request{
		Description: format.Description,
		schemaMap:   root,
		compiled:    compiled,
	}, nil
}

// compileSchema compiles schema bytes with a compiler that cannot
// load remote documents. Fragment-only $ref enforcement happens
// before compilation; stripping the loaders is defense in depth.
func compileSchema(schemaBytes []byte) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	clear(compiler.Loaders)
	return compiler.Compile(schemaBytes)
}

// refKeywords are schema keywords whose string values reference
// other schemas. Each must stay inside the caller's document.
var refKeywords = map[string]struct{}{
	"$ref":          {},
	"$dynamicRef":   {},
	"$recursiveRef": {},
}

// validateFragmentOnlyRefs walks the schema and rejects any
// reference value that does not start with "#". This keeps schema
// resolution local to the submitted document so no network or file
// lookups can be triggered.
func validateFragmentOnlyRefs(node any) *codersdk.ValidationError {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if _, isRef := refKeywords[key]; isRef {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return &codersdk.ValidationError{
						Field:  "response_format.schema",
						Detail: fmt.Sprintf(`%s values must be fragment-local (start with "#"); got %v.`, key, child),
					}
				}
				continue
			}
			if err := validateFragmentOnlyRefs(child); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := validateFragmentOnlyRefs(item); err != nil {
				return err
			}
		}
	}
	return nil
}

// Tool returns the server-owned finalizer fantasy.AgentTool for req.
func Tool(req *Request) fantasy.AgentTool {
	return &finalizerTool{req: req}
}

type finalizerTool struct {
	req  *Request
	opts fantasy.ProviderOptions
}

func (t *finalizerTool) Info() fantasy.ToolInfo {
	description := "Submit the final structured answer for this task. " +
		"Call this tool exactly once, alone, after all other work is done. " +
		"The output argument must satisfy the required JSON schema."
	if t.req.Description != "" {
		description += " Output description: " + t.req.Description
	}

	return fantasy.ToolInfo{
		Name:        ToolName,
		Description: description,
		// Deep-copied because chatloop's buildToolDefinitions runs
		// schema.Normalize on tool parameters, which mutates nested
		// maps in place. Sharing schemaMap would leak that mutation
		// into every later Info call on multi-step turns.
		Parameters: map[string]any{outputProperty: copyJSONObject(t.req.schemaMap)},
		Required:   []string{outputProperty},
		Parallel:   false,
	}
}

// copyJSONObject deep-copies a decoded JSON object (nested maps,
// slices, and scalars produced by encoding/json).
func copyJSONObject(obj map[string]any) map[string]any {
	copied := make(map[string]any, len(obj))
	for key, child := range obj {
		copied[key] = copyJSONAny(child)
	}
	return copied
}

func copyJSONAny(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return copyJSONObject(v)
	case []any:
		copied := make([]any, len(v))
		for i, child := range v {
			copied[i] = copyJSONAny(child)
		}
		return copied
	default:
		return v
	}
}

func (t *finalizerTool) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	canonical, err := t.req.validateOutput([]byte(call.Input))
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return fantasy.NewTextResponse(string(canonical)), nil
}

// ExemptFromResultTruncation implements chatloop's
// ResultTruncationExempter: a successful finalizer result is the
// canonical validated JSON of the output value, and truncating it
// would corrupt the payload while persisting it as a success. Error
// results are still truncated by chatloop.
func (*finalizerTool) ExemptFromResultTruncation() bool { return true }

func (t *finalizerTool) ProviderOptions() fantasy.ProviderOptions {
	return t.opts
}

func (t *finalizerTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.opts = opts
}

// validateOutput parses finalizer tool args, validates the "output"
// value against the compiled schema, and returns its canonical JSON
// encoding. Errors are stable, model-actionable strings surfaced as
// retryable tool errors.
func (r *Request) validateOutput(args []byte) (json.RawMessage, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(args, &parsed); err != nil {
		return nil, xerrors.New(`invalid arguments: expected a JSON object of the form {"output": <value matching the schema>}.`)
	}
	rawOutput, ok := parsed[outputProperty]
	if !ok {
		return nil, xerrors.New(`missing required "output" argument: pass the final answer as {"output": <value matching the schema>}.`)
	}

	var outputValue any
	if err := json.Unmarshal(rawOutput, &outputValue); err != nil {
		return nil, xerrors.New(`invalid "output" argument: not valid JSON.`)
	}
	result := r.compiled.Validate(outputValue)
	if !result.IsValid() {
		return nil, xerrors.Errorf(`"output" does not satisfy the required schema: %s. Fix the listed fields and call %s again.`, formatValidationErrors(result), ToolName)
	}

	// Re-marshal for a canonical encoding (stable whitespace,
	// escaped strings) independent of the model's formatting.
	canonical, err := json.Marshal(outputValue)
	if err != nil {
		return nil, xerrors.Errorf("encode validated output: %w", err)
	}
	return canonical, nil
}

// formatValidationErrors flattens an evaluation result into a
// compact, deterministic one-line summary for the model.
func formatValidationErrors(result *jsonschema.EvaluationResult) string {
	list := result.ToList(false)
	if list == nil {
		return "schema validation failed"
	}
	var sb strings.Builder
	appendErrors(&sb, *list)
	if sb.Len() == 0 {
		return "schema validation failed"
	}
	return sb.String()
}

func appendErrors(sb *strings.Builder, list jsonschema.List) {
	location := cmp.Or(list.InstanceLocation, "(root)")
	for _, key := range slices.Sorted(maps.Keys(list.Errors)) {
		if sb.Len() > 0 {
			_, _ = sb.WriteString("; ")
		}
		_, _ = fmt.Fprintf(sb, "%s: %s", location, list.Errors[key])
	}
	for _, detail := range list.Details {
		appendErrors(sb, detail)
	}
}
