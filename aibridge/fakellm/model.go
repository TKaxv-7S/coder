package fakellm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
)

// Model is a fantasy.LanguageModel driven by a Script, with no HTTP/wire
// involvement at all — it replaces chattest.FakeModel's per-test-file
// hand-built fantasy.StreamResponse/textMessage helpers for the common
// "scripted echo/tool-call conversation" case.
//
// Because every scripted tool_call carries its own required Result,
// Model never needs a real tool to actually execute: a test can drive a
// full multi-turn tool-calling conversation by calling Generate/Stream,
// inspecting the returned ToolCallContent, fetching the matching
// canned result via ResultFor, and feeding a synthesized
// fantasy.ToolResultPart back in on the next call — all deterministic,
// no real tool dispatch required. (Tests that *do* want real tool
// dispatch to run should exercise chatloop/chatd end-to-end instead;
// Model's job is only to be the model.)
//
// Each of the four fantasy.LanguageModel methods is backed by its own
// sequence: an ordered run of calls, each either served by the script
// (the default) or intercepted by a Hook registered via
// SetGenerateHook/SetStreamHook/SetObjectHook/SetStreamObjectHook. This
// is the single mechanism for both "scripted content" and "imperative
// escape hatch" -- a scripted turn and a Hook are both just "what
// happens on call N," dispatched the same way.
type Model struct {
	ProviderName string
	ModelName    string

	script      *Script
	calls       atomic.Int64
	objectCalls atomic.Int64

	generate     *sequence[*fantasy.Response]
	stream       *sequence[fantasy.StreamResponse]
	object       *sequence[*fantasy.ObjectResponse]
	streamObject *sequence[fantasy.ObjectStreamResponse]

	mu          sync.Mutex
	results     map[string]json.RawMessage // tool call ID -> scripted result
	capturedGen []CapturedCall             // every call passed to Generate/Stream, in order
	capturedObj []CapturedObjectCall       // every call passed to GenerateObject/StreamObject, in order
}

// CapturedCall pairs a fantasy.Call with the context.Context it arrived
// with, so a test can assert on both after the fact -- e.g. ctx.Deadline()
// or ctx.Err() -- instead of asserting synchronously from inside a
// GenerateFn/StreamFn closure the way chattest.FakeModel requires.
type CapturedCall struct {
	Ctx  context.Context
	Call fantasy.Call
}

// CapturedObjectCall is CapturedCall for GenerateObject/StreamObject.
type CapturedObjectCall struct {
	Ctx  context.Context
	Call fantasy.ObjectCall
}

var _ fantasy.LanguageModel = (*Model)(nil)

// NewModel returns a Model driven by script.
func NewModel(script *Script) *Model {
	return &Model{
		ProviderName: "fakellm",
		ModelName:    "fakellm",
		script:       script,
		results:      make(map[string]json.RawMessage),
		generate:     newSequence[*fantasy.Response](),
		stream:       newSequence[fantasy.StreamResponse](),
		object:       newSequence[*fantasy.ObjectResponse](),
		streamObject: newSequence[fantasy.ObjectStreamResponse](),
	}
}

// SetGenerateHook arranges for the callIdx'th (0-based) Generate call to
// be intercepted by hook. See the Hook doc comment; this is the
// Generate-shaped instantiation, used for e.g. testing panic-recovery
// paths (a scripted turn has no, and shouldn't have, a way to express
// "this call panics," but a hook, being an arbitrary Go closure, can).
func (m *Model) SetGenerateHook(callIdx int64, hook Hook[*fantasy.Response]) {
	m.generate.set(callIdx, hook)
}

// SetStreamHook arranges for the callIdx'th (0-based) Stream call to be
// intercepted by hook instead of going straight to the scripted turn.
// This is fakellm's explicit escape hatch for tests that are about live
// cancellation/timing behavior rather than model content -- see the
// Hook doc comment. It is NOT part of the JSONL script format and never
// will be: scripts stay fully declarative; hooks are attached
// imperatively, per test, for the narrow cases that need them.
func (m *Model) SetStreamHook(callIdx int64, hook Hook[fantasy.StreamResponse]) {
	m.stream.set(callIdx, hook)
}

// SetObjectHook arranges for the callIdx'th (0-based) GenerateObject
// call to be intercepted by hook.
func (m *Model) SetObjectHook(callIdx int64, hook Hook[*fantasy.ObjectResponse]) {
	m.object.set(callIdx, hook)
}

// SetStreamObjectHook arranges for the callIdx'th (0-based) StreamObject
// call to be intercepted by hook.
func (m *Model) SetStreamObjectHook(callIdx int64, hook Hook[fantasy.ObjectStreamResponse]) {
	m.streamObject.set(callIdx, hook)
}

// ResultFor returns the scripted result for a tool call previously
// returned by Generate/Stream, keyed by the ToolCallID that was
// generated for it. Used by a test's own harness to build the
// fantasy.ToolResultPart it feeds back into the next Call, without
// needing any real tool to execute.
func (m *Model) ResultFor(toolCallID string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.results[toolCallID]
	return r, ok
}

// Calls returns the number of Turns consumed from the script so far via
// Generate/Stream. This can differ from the number of Generate/Stream
// invocations if a Hook chooses not to call next() (e.g.
// BlockUntilContextDone never consumes a turn) -- use GenerateCalls for
// the raw invocation count instead.
func (m *Model) Calls() int64 {
	return m.calls.Load()
}

// GenerateCalls returns every call passed to Generate/Stream so far, in
// order, paired with the context.Context each arrived with. Use this
// instead of chattest.FakeModel's pattern of asserting on the call from
// inside GenerateFn/StreamFn: fakellm's turns are declared up front, so
// assertions on what was sent -- including deadline/cancellation state
// -- happen after the fact against the captured calls.
func (m *Model) GenerateCalls() []CapturedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CapturedCall, len(m.capturedGen))
	copy(out, m.capturedGen)
	return out
}

// ObjectCalls returns every call passed to GenerateObject/StreamObject
// so far, in order, paired with its context.Context.
func (m *Model) ObjectCalls() []CapturedObjectCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CapturedObjectCall, len(m.capturedObj))
	copy(out, m.capturedObj)
	return out
}

func (m *Model) next() (Turn, int64, error) {
	idx := m.calls.Add(1) - 1
	if idx >= int64(len(m.script.Turns)) {
		return Turn{}, idx, xerrors.Errorf("fakellm: script exhausted: call %d made, but only %d turn(s) scripted", idx+1, len(m.script.Turns))
	}
	return m.script.Turns[idx], idx, nil
}

func (m *Model) nextObject() (ObjectTurn, int64, error) {
	idx := m.objectCalls.Add(1) - 1
	if idx >= int64(len(m.script.Objects)) {
		return ObjectTurn{}, idx, xerrors.Errorf("fakellm: script exhausted: object call %d made, but only %d object turn(s) scripted", idx+1, len(m.script.Objects))
	}
	return m.script.Objects[idx], idx, nil
}

func toolCallID(turnIdx int64, callIdx int) string {
	return fmt.Sprintf("fakellm-call-%d-%d", turnIdx, callIdx)
}

func (m *Model) recordResult(id string, result json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[id] = result
}

// Generate implements fantasy.LanguageModel, unless a Hook is set for
// this call index (see SetGenerateHook), in which case the hook
// controls behavior.
func (m *Model) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	m.mu.Lock()
	m.capturedGen = append(m.capturedGen, CapturedCall{Ctx: ctx, Call: call})
	m.mu.Unlock()

	return m.generate.call(ctx, func(context.Context) (*fantasy.Response, error) {
		return m.generateScripted()
	})
}

// generateScripted is Generate's default, hook-free behavior: consume
// the next scripted turn and translate it into a fantasy.Response.
func (m *Model) generateScripted() (*fantasy.Response, error) {
	turn, idx, err := m.next()
	if err != nil {
		return nil, err
	}
	if turn.Err != nil {
		return nil, turn.Err.error()
	}

	var content fantasy.ResponseContent
	for _, p := range turn.Parts {
		switch p.Kind {
		case PartText:
			content = append(content, fantasy.TextContent{Text: p.Text})
		case PartThink:
			content = append(content, fantasy.ReasoningContent{Text: p.Text})
		}
	}
	finish := fantasy.FinishReasonStop
	for i, tc := range turn.ToolCalls {
		id := toolCallID(idx, i)
		m.recordResult(id, tc.Result)
		content = append(content, fantasy.ToolCallContent{
			ToolCallID: id,
			ToolName:   tc.Name,
			Input:      string(tc.Args),
		})
		finish = fantasy.FinishReasonToolCalls
	}

	return &fantasy.Response{
		Content:      content,
		FinishReason: finish,
	}, nil
}

// Stream implements fantasy.LanguageModel by replaying the same content
// Generate would return as a single-shot stream (one delta per part/tool
// call, no artificial chunking or delay), unless a Hook is set for this
// call index (see SetStreamHook), in which case the hook controls
// delivery timing.
func (m *Model) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.capturedGen = append(m.capturedGen, CapturedCall{Ctx: ctx, Call: call})
	m.mu.Unlock()

	return m.stream.call(ctx, func(context.Context) (fantasy.StreamResponse, error) {
		return m.streamScripted()
	})
}

// streamScripted is Stream's default, hook-free behavior: consume the
// next scripted turn and replay it as a single-shot stream.
func (m *Model) streamScripted() (fantasy.StreamResponse, error) {
	turn, idx, err := m.next()
	if err != nil {
		return nil, err
	}

	return func(yield func(fantasy.StreamPart) bool) {
		if turn.Err != nil {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: turn.Err.error()})
			return
		}

		for _, p := range turn.Parts {
			var start, delta, end fantasy.StreamPartType
			switch p.Kind {
			case PartText:
				start, delta, end = fantasy.StreamPartTypeTextStart, fantasy.StreamPartTypeTextDelta, fantasy.StreamPartTypeTextEnd
			case PartThink:
				start, delta, end = fantasy.StreamPartTypeReasoningStart, fantasy.StreamPartTypeReasoningDelta, fantasy.StreamPartTypeReasoningEnd
			}
			if !yield(fantasy.StreamPart{Type: start}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: delta, Delta: p.Text}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: end}) {
				return
			}
		}

		finish := fantasy.FinishReasonStop
		for i, tc := range turn.ToolCalls {
			id := toolCallID(idx, i)
			m.recordResult(id, tc.Result)
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolInputStart, ID: id, ToolCallName: tc.Name}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolInputDelta, ID: id, ToolCallInput: string(tc.Args)}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolInputEnd, ID: id}) {
				return
			}
			if !yield(fantasy.StreamPart{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            id,
				ToolCallName:  tc.Name,
				ToolCallInput: string(tc.Args),
			}) {
				return
			}
			finish = fantasy.FinishReasonToolCalls
		}

		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: finish})
	}, nil
}

// GenerateObject implements fantasy.LanguageModel's structured-output
// path, used by chatd flows like title/turn-status-label generation,
// unless a Hook is set for this call index (see SetObjectHook).
func (m *Model) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	m.mu.Lock()
	m.capturedObj = append(m.capturedObj, CapturedObjectCall{Ctx: ctx, Call: call})
	m.mu.Unlock()

	return m.object.call(ctx, func(context.Context) (*fantasy.ObjectResponse, error) {
		return m.generateObjectScripted()
	})
}

// generateObjectScripted is GenerateObject's default, hook-free behavior.
func (m *Model) generateObjectScripted() (*fantasy.ObjectResponse, error) {
	turn, _, err := m.nextObject()
	if err != nil {
		return nil, err
	}
	if turn.Err != nil {
		return nil, turn.Err.error()
	}

	var obj any
	if err := json.Unmarshal(turn.Value, &obj); err != nil {
		return nil, xerrors.Errorf("fakellm: scripted object is not valid JSON: %w", err)
	}

	resp := &fantasy.ObjectResponse{
		Object:       obj,
		RawText:      string(turn.Value),
		FinishReason: fantasy.FinishReasonStop,
	}
	if turn.Usage != nil {
		resp.Usage = turn.Usage.toFantasy()
	}
	return resp, nil
}

// StreamObject implements fantasy.LanguageModel's streaming structured-
// output path. fakellm never chunks the object itself (no artificial
// deltas): it emits the whole scripted value as a single object part,
// then finishes -- unless a Hook is set for this call index (see
// SetStreamObjectHook).
func (m *Model) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	m.mu.Lock()
	m.capturedObj = append(m.capturedObj, CapturedObjectCall{Ctx: ctx, Call: call})
	m.mu.Unlock()

	return m.streamObject.call(ctx, func(context.Context) (fantasy.ObjectStreamResponse, error) {
		return m.streamObjectScripted()
	})
}

// streamObjectScripted is StreamObject's default, hook-free behavior.
func (m *Model) streamObjectScripted() (fantasy.ObjectStreamResponse, error) {
	turn, _, err := m.nextObject()
	if err != nil {
		return nil, err
	}

	return func(yield func(fantasy.ObjectStreamPart) bool) {
		if turn.Err != nil {
			yield(fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeError, Error: turn.Err.error()})
			return
		}

		var obj any
		if err := json.Unmarshal(turn.Value, &obj); err != nil {
			yield(fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeError, Error: xerrors.Errorf("fakellm: scripted object is not valid JSON: %w", err)})
			return
		}
		if !yield(fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeObject, Object: obj}) {
			return
		}

		finish := fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop}
		if turn.Usage != nil {
			finish.Usage = turn.Usage.toFantasy()
		}
		yield(finish)
	}, nil
}

func (m *Model) Provider() string { return m.ProviderName }
func (m *Model) Model() string    { return m.ModelName }
