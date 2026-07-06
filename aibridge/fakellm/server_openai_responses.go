package fakellm

// This file implements just enough of the OpenAI Responses API to prove
// out an approach for avoiding hand-rolled wire-format drift: instead of
// building response bodies from ad hoc map[string]any literals (as
// server_anthropic.go/server_openai.go currently do, and as
// chattest/testutil's hand-built structs do), it constructs and
// marshals the *real* openai-go SDK response/event struct types
// (charm.land/fantasy's OpenAI provider depends on the same SDK, so
// these are already a transitive dependency, not a new one).
//
// Why this matters: the SDK's field names, "required" JSON tags, and
// discriminant fields (Type/Object/Role) are maintained by the SDK
// vendor and enforced by the Go compiler at our build time. If OpenAI
// changes the wire shape and the SDK is upgraded to match, our code
// fails to *compile* (field renamed/removed) rather than silently
// producing wire bytes that happen to still parse today but drift from
// reality unnoticed. Hand-rolled maps have no such guardrail.
//
// Scope of this slice: text-only turns (blocking + streaming) only.
// Tool calls, reasoning items, and web-search-call items are NOT
// implemented here -- those add a large amount of additional event
// types and are exactly the higher-risk, lower-value surface where
// reusing the existing captured aibridge/fixtures txtar fixtures (real
// traffic, captured once) remains the pragmatic choice over hand-deriving
// every event shape from scratch.
import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/responses"
	oaishared "github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
)

func (s *Server) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	var req openAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("fakellm: decode openai responses request: %v", err), http.StatusBadRequest)
		return
	}
	s.record(WireOpenAIResponses, req.Stream, nil)

	turn, _, err := s.next()
	if err != nil {
		writeServerError(w, err)
		return
	}
	if turn.Err != nil {
		writeOpenAIError(w, turn.Err)
		return
	}
	if turn.FinishedToolCalls() {
		http.Error(w, "fakellm: tool calls are not yet supported over the OpenAI Responses API wire format", http.StatusNotImplemented)
		return
	}

	if req.Stream {
		writeOpenAIResponsesStreaming(w, turn)
	} else {
		writeOpenAIResponsesBlocking(w, turn)
	}
}

func openAIResponseID() string {
	return "resp_" + uuid.NewString()[:8]
}

func openAIMessageItemID() string {
	return "msg_" + uuid.NewString()[:8]
}

// buildResponse constructs a real responses.Response (the SDK's own
// type) representing a completed, text-only turn.
func buildResponse(respID, itemID, text string) responses.Response {
	return responses.Response{
		ID:     respID,
		Object: constant.ValueOf[constant.Response](),
		Status: responses.ResponseStatusCompleted,
		Model:  oaishared.ResponsesModel("fakellm"),
		Output: []responses.ResponseOutputItemUnion{
			{
				ID:     itemID,
				Type:   "message",
				Role:   constant.ValueOf[constant.Assistant](),
				Status: "completed",
				Content: []responses.ResponseOutputMessageContentUnion{
					{Type: "output_text", Text: text},
				},
			},
		},
	}
}

func writeOpenAIResponsesBlocking(w http.ResponseWriter, turn Turn) {
	resp := buildResponse(openAIResponseID(), openAIMessageItemID(), turn.Text())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeOpenAIResponsesStreaming(w http.ResponseWriter, turn Turn) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	respID := openAIResponseID()
	itemID := openAIMessageItemID()
	text := turn.Text()
	var seq int64

	send := func(eventType string, v any) bool {
		seq++
		b, err := json.Marshal(v)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	inProgress := buildResponse(respID, itemID, "")
	inProgress.Status = responses.ResponseStatusInProgress
	inProgress.Output = nil
	if !send("response.created", responses.ResponseCreatedEvent{Response: inProgress, SequenceNumber: seq}) {
		return
	}

	addedItem := responses.ResponseOutputItemUnion{ID: itemID, Type: "message", Role: constant.ValueOf[constant.Assistant](), Status: "in_progress"}
	if !send("response.output_item.added", responses.ResponseOutputItemAddedEvent{Item: addedItem, OutputIndex: 0, SequenceNumber: seq}) {
		return
	}

	addedPart := responses.ResponseContentPartAddedEventPartUnion{Type: "output_text", Text: ""}
	if !send("response.content_part.added", responses.ResponseContentPartAddedEvent{
		ContentIndex: 0, ItemID: itemID, OutputIndex: 0, Part: addedPart, SequenceNumber: seq,
	}) {
		return
	}

	if text != "" {
		if !send("response.output_text.delta", responses.ResponseTextDeltaEvent{
			ContentIndex: 0, Delta: text, ItemID: itemID, OutputIndex: 0, SequenceNumber: seq,
		}) {
			return
		}
	}

	if !send("response.output_text.done", responses.ResponseTextDoneEvent{
		ContentIndex: 0, ItemID: itemID, OutputIndex: 0, SequenceNumber: seq, Text: text,
	}) {
		return
	}

	donePart := responses.ResponseContentPartDoneEventPartUnion{Type: "output_text", Text: text}
	if !send("response.content_part.done", responses.ResponseContentPartDoneEvent{
		ContentIndex: 0, ItemID: itemID, OutputIndex: 0, Part: donePart, SequenceNumber: seq,
	}) {
		return
	}

	doneItem := responses.ResponseOutputItemUnion{
		ID: itemID, Type: "message", Role: constant.ValueOf[constant.Assistant](), Status: "completed",
		Content: []responses.ResponseOutputMessageContentUnion{{Type: "output_text", Text: text}},
	}
	if !send("response.output_item.done", responses.ResponseOutputItemDoneEvent{Item: doneItem, OutputIndex: 0, SequenceNumber: seq}) {
		return
	}

	final := buildResponse(respID, itemID, text)
	send("response.completed", responses.ResponseCompletedEvent{Response: final, SequenceNumber: seq})
}
