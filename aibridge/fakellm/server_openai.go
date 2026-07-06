package fakellm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// openAIRequest is the minimal subset of the OpenAI Chat Completions API
// request body fakellm needs. Same scope caveat as anthropicRequest:
// fakellm doesn't parse/validate the rest of the request.
type openAIRequest struct {
	Stream bool `json:"stream"`
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req openAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("fakellm: decode openai request: %v", err), http.StatusBadRequest)
		return
	}
	s.record(WireOpenAIChatCompletions, req.Stream, nil)

	turn, idx, err := s.next()
	if err != nil {
		writeServerError(w, err)
		return
	}
	if turn.Err != nil {
		writeOpenAIError(w, turn.Err)
		return
	}

	if req.Stream {
		writeOpenAIStreaming(w, turn, idx)
	} else {
		writeOpenAIBlocking(w, turn, idx)
	}
}

func openAICompletionID() string {
	return "chatcmpl-" + uuid.NewString()[:8]
}

func openAIToolCallID(turnIdx, callIdx int) string {
	return "call_" + toolCallID(int64(turnIdx), callIdx)
}

// openAIText concatenates PartText parts only. Chat Completions has no
// standard reasoning/thinking content field, so PartThink is dropped --
// a real, documented gap: fakellm can't exercise OpenAI reasoning-item
// handling over this wire endpoint. That needs the Responses API, which
// this spike doesn't implement.
func openAIText(turn Turn) string {
	return turn.Text()
}

type openAIToolCallWire struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

func openAIToolCalls(turn Turn, turnIdx int) []openAIToolCallWire {
	if len(turn.ToolCalls) == 0 {
		return nil
	}
	calls := make([]openAIToolCallWire, len(turn.ToolCalls))
	for i, tc := range turn.ToolCalls {
		calls[i].Index = i
		calls[i].ID = openAIToolCallID(turnIdx, i)
		calls[i].Type = "function"
		calls[i].Function.Name = tc.Name
		calls[i].Function.Arguments = string(tc.Args)
	}
	return calls
}

func openAIFinishReason(turn Turn) string {
	if turn.FinishedToolCalls() {
		return "tool_calls"
	}
	return "stop"
}

func writeOpenAIBlocking(w http.ResponseWriter, turn Turn, turnIdx int) {
	message := map[string]any{"role": "assistant", "content": openAIText(turn)}
	if calls := openAIToolCalls(turn, turnIdx); calls != nil {
		message["tool_calls"] = calls
	}
	resp := map[string]any{
		"id":      openAICompletionID(),
		"object":  "chat.completion",
		"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": openAIFinishReason(turn)}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeOpenAIStreaming(w http.ResponseWriter, turn Turn, turnIdx int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	id := openAICompletionID()
	send := func(delta map[string]any, finishReason *string) bool {
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finishReason}},
		}
		b, err := json.Marshal(chunk)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send(map[string]any{"role": "assistant"}, nil) {
		return
	}
	if text := openAIText(turn); text != "" {
		if !send(map[string]any{"content": text}, nil) {
			return
		}
	}
	for _, tc := range openAIToolCalls(turn, turnIdx) {
		if !send(map[string]any{"tool_calls": []openAIToolCallWire{tc}}, nil) {
			return
		}
	}

	finish := openAIFinishReason(turn)
	if !send(map[string]any{}, &finish) {
		return
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeOpenAIError(w http.ResponseWriter, e *ErrorStep) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": e.Message,
		},
	})
}
