package fakellm_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
)

func TestParse_Object(t *testing.T) {
	t.Parallel()

	t.Run("object steps go to their own timeline, independent of turns", func(t *testing.T) {
		t.Parallel()
		script := fakellm.MustParseString(`
			{"text": "before"}
			{"object": {"value": {"title": "Failed workspace logs"}}}
			{"text": "after"}
		`)
		// The text lines are NOT split into two turns by the object line
		// in between -- object calls live on a separate timeline, so
		// they don't affect turn boundaries at all.
		require.Len(t, script.Turns, 1)
		require.Equal(t, "beforeafter", script.Turns[0].Text())
		require.Len(t, script.Objects, 1)
		require.JSONEq(t, `{"title":"Failed workspace logs"}`, string(script.Objects[0].Value))
	})

	t.Run("requires exactly one of value/error", func(t *testing.T) {
		t.Parallel()
		_, err := fakellm.ParseString(`{"object": {}}`)
		require.ErrorContains(t, err, "exactly one of value/error")

		_, err = fakellm.ParseString(`{"object": {"value": {"a":1}, "error": {"message": "boom"}}}`)
		require.ErrorContains(t, err, "exactly one of value/error")
	})

	t.Run("object turn can carry usage", func(t *testing.T) {
		t.Parallel()
		script := fakellm.MustParseString(`{"object": {"value": {"title": "x"}, "usage": {"input_tokens": 11, "output_tokens": 7, "total_tokens": 18}}}`)
		require.Len(t, script.Objects, 1)
		require.NotNil(t, script.Objects[0].Usage)
		require.EqualValues(t, 11, script.Objects[0].Usage.InputTokens)
	})

	t.Run("object turn can carry an error instead of a value", func(t *testing.T) {
		t.Parallel()
		script := fakellm.MustParseString(`{"object": {"error": {"message": "rate limited"}}}`)
		require.Len(t, script.Objects, 1)
		require.NotNil(t, script.Objects[0].Err)
		require.Equal(t, "rate limited", script.Objects[0].Err.Message)
	})
}

func TestModel_GenerateObject(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`{"object": {"value": {"title": "Failed workspace logs"}, "usage": {"input_tokens": 11, "output_tokens": 7, "total_tokens": 18}}}`)
	model := fakellm.NewModel(script)

	resp, err := model.GenerateObject(context.Background(), fantasy.ObjectCall{SchemaName: "propose_title"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"title": "Failed workspace logs"}, resp.Object)
	require.Equal(t, int64(11), resp.Usage.InputTokens)
	require.Equal(t, int64(7), resp.Usage.OutputTokens)
	require.Equal(t, int64(18), resp.Usage.TotalTokens)

	// Captured for post-hoc assertions, replacing the "assert inside
	// GenerateObjectFn" pattern.
	calls := model.ObjectCalls()
	require.Len(t, calls, 1)
	require.Equal(t, "propose_title", calls[0].Call.SchemaName)

	// Exhausted: no more object turns scripted.
	_, err = model.GenerateObject(context.Background(), fantasy.ObjectCall{})
	require.ErrorContains(t, err, "script exhausted")
}

func TestModel_GenerateObject_Error(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`{"object": {"error": {"message": "rate limited"}}}`)
	model := fakellm.NewModel(script)

	_, err := model.GenerateObject(context.Background(), fantasy.ObjectCall{})
	require.ErrorContains(t, err, "rate limited")
}

func TestModel_StreamObject(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`{"object": {"value": {"label": "Submitted PR"}}}`)
	model := fakellm.NewModel(script)

	stream, err := model.StreamObject(context.Background(), fantasy.ObjectCall{SchemaName: "propose_turn_status_label"})
	require.NoError(t, err)

	var gotObject any
	var gotFinish bool
	for part := range stream {
		switch part.Type {
		case fantasy.ObjectStreamPartTypeObject:
			gotObject = part.Object
		case fantasy.ObjectStreamPartTypeFinish:
			gotFinish = true
		}
	}
	require.Equal(t, map[string]any{"label": "Submitted PR"}, gotObject)
	require.True(t, gotFinish)
}

// TestModel_ObjectAndTurnTimelinesAreIndependent proves that a single
// Model can be used for both a plain conversational turn (Stream) and a
// structured-output call (GenerateObject) without either consuming from
// the other's counter -- mirroring how a real chatd Model instance is
// reused across the main chat loop and title generation.
func TestModel_ObjectAndTurnTimelinesAreIndependent(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`
		{"text": "hello there"}
		{"object": {"value": {"title": "Greeting"}}}
	`)
	model := fakellm.NewModel(script)

	resp, err := model.Generate(context.Background(), fantasy.Call{})
	require.NoError(t, err)
	require.Equal(t, "hello there", resp.Content.Text())

	obj, err := model.GenerateObject(context.Background(), fantasy.ObjectCall{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"title": "Greeting"}, obj.Object)
}
