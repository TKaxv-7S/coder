package fakellm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// anthropicRequest is the minimal subset of the Anthropic Messages API
// request body fakellm needs to decide how to respond. It deliberately
// does not validate/echo the rest of the request (messages, tools,
// system prompt) -- unlike chattest's AnthropicRequest, fakellm's
// scripts don't branch on request content, so there's nothing to parse
// it for yet. That's a real capability gap vs. chattest's per-request
// handler closures; see the package doc.
type anthropicRequest struct {
	Stream bool `json:"stream"`
}

func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	var req anthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("fakellm: decode anthropic request: %v", err), http.StatusBadRequest)
		return
	}
	s.record(WireAnthropic, req.Stream, nil)

	turn, idx, err := s.next()
	if err != nil {
		writeServerError(w, err)
		return
	}
	if turn.Err != nil {
		writeAnthropicError(w, turn.Err)
		return
	}

	if req.Stream {
		writeAnthropicStreaming(w, turn, idx)
	} else {
		writeAnthropicBlocking(w, turn, idx)
	}
}

func anthropicMessageID() string {
	return "msg_" + uuid.NewString()[:8]
}

func anthropicToolUseID(turnIdx, callIdx int) string {
	return "toolu_" + toolCallID(int64(turnIdx), callIdx)
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func anthropicContentBlocks(turn Turn, turnIdx int) []anthropicContentBlock {
	var blocks []anthropicContentBlock
	for _, p := range turn.Parts {
		switch p.Kind {
		case PartText:
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: p.Text})
		case PartThink:
			blocks = append(blocks, anthropicContentBlock{Type: "thinking", Text: p.Text})
		}
	}
	for i, tc := range turn.ToolCalls {
		input := tc.Args
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		blocks = append(blocks, anthropicContentBlock{
			Type:  "tool_use",
			ID:    anthropicToolUseID(turnIdx, i),
			Name:  tc.Name,
			Input: input,
		})
	}
	return blocks
}

func anthropicStopReason(turn Turn) string {
	if turn.FinishedToolCalls() {
		return "tool_use"
	}
	return "end_turn"
}

func writeAnthropicBlocking(w http.ResponseWriter, turn Turn, turnIdx int) {
	resp := map[string]any{
		"id":          anthropicMessageID(),
		"type":        "message",
		"role":        "assistant",
		"content":     anthropicContentBlocks(turn, turnIdx),
		"stop_reason": anthropicStopReason(turn),
		"usage":       map[string]any{"input_tokens": 0, "output_tokens": 0},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeAnthropicStreaming(w http.ResponseWriter, turn Turn, turnIdx int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	send := func(eventType string, data any) bool {
		b, err := json.Marshal(data)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":   anthropicMessageID(),
			"type": "message",
			"role": "assistant",
		},
	}) {
		return
	}

	blocks := anthropicContentBlocks(turn, turnIdx)
	for i, block := range blocks {
		startBlock := block
		startBlock.Text = "" // Anthropic sends empty text/input in content_block_start; content arrives via deltas.
		if block.Type == "tool_use" {
			startBlock.Input = json.RawMessage("{}")
		}
		if !send("content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": startBlock}) {
			return
		}

		var delta map[string]any
		switch block.Type {
		case "text":
			delta = map[string]any{"type": "text_delta", "text": block.Text}
		case "thinking":
			delta = map[string]any{"type": "thinking_delta", "thinking": block.Text}
		case "tool_use":
			delta = map[string]any{"type": "input_json_delta", "partial_json": string(block.Input)}
		}
		if !send("content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": delta}) {
			return
		}
		if !send("content_block_stop", map[string]any{"type": "content_block_stop", "index": i}) {
			return
		}
	}

	if !send("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": anthropicStopReason(turn)},
		"usage": map[string]any{"output_tokens": 0},
	}) {
		return
	}
	send("message_stop", map[string]any{"type": "message_stop"})
}

func writeAnthropicError(w http.ResponseWriter, e *ErrorStep) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": e.Message,
		},
	})
}
