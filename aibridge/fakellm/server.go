package fakellm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"golang.org/x/xerrors"
)

// Server is fakellm's wire-level fake: a real httptest.Server that
// speaks the actual Anthropic Messages API and OpenAI Chat Completions
// API wire formats (streaming SSE and blocking JSON), driven by the same
// Script/Turn types as Model. It replaces chattest.NewAnthropic/
// NewOpenAI and testutil.MockUpstream for the common "just echo/
// tool-call" cases: instead of hand-building AnthropicChunk/OpenAIChunk
// sequences or checked-in txtar fixtures, a test writes the same JSONL
// script it would use for an in-process Model.
//
// Scope of this spike: Anthropic Messages and OpenAI Chat Completions
// only. The OpenAI Responses API (reasoning items, web-search-call
// items, response.output_item.* event framing) is a substantially
// bigger wire format and is deliberately left out for now -- see the
// package doc for the running list of what's covered.
//
// Turns are consumed from one shared counter across both endpoints and
// across streaming/non-streaming requests, mirroring Model: whichever
// endpoint a request hits, it gets "the next turn" in script order.
// Object turns (GenerateObject/StreamObject) have no wire-level
// equivalent here since chatd's structured-output calls go through
// Chat Completions' tool_choice=required mechanism, not a separate
// endpoint; this is left as a follow-up.
type Server struct {
	URL string

	t          testing.TB
	httpServer *httptest.Server
	script     *Script

	mu       sync.Mutex
	call     int
	requests []CapturedRequest
}

// CapturedRequest records one inbound request for post-hoc assertions,
// replacing MockUpstream.ReceivedRequests / MockAIBridgeTransport.RequestsSnapshot.
type CapturedRequest struct {
	Provider WireProvider
	Stream   bool
	Body     json.RawMessage
}

// WireProvider identifies which wire format an inbound request used.
type WireProvider int

const (
	WireAnthropic WireProvider = iota
	WireOpenAIChatCompletions
	WireOpenAIResponses
)

// NewServer starts an httptest.Server driven by script. Turns are
// required (parse-time, not serve-time): a script with zero turns
// will fail every request immediately.
func NewServer(t testing.TB, script *Script) *Server {
	t.Helper()

	s := &Server{t: t, script: script}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)
	mux.HandleFunc("POST /chat/completions", s.handleOpenAIChatCompletions)
	mux.HandleFunc("POST /responses", s.handleOpenAIResponses)

	s.httpServer = httptest.NewServer(mux)
	t.Cleanup(s.httpServer.Close)
	s.URL = s.httpServer.URL
	return s
}

// Requests returns every request received so far, in order.
func (s *Server) Requests() []CapturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CapturedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

// next returns the next turn in script order and its index, or an error
// if the script is exhausted. Shared across both wire endpoints so a
// script's turn order reflects the real call sequence regardless of
// which provider/endpoint chatd happened to route each call through.
func (s *Server) next() (Turn, int, error) {
	s.mu.Lock()
	idx := s.call
	s.call++
	s.mu.Unlock()

	if idx >= len(s.script.Turns) {
		return Turn{}, idx, xerrors.Errorf("fakellm: script exhausted: call %d made, but only %d turn(s) scripted", idx+1, len(s.script.Turns))
	}
	return s.script.Turns[idx], idx, nil
}

func (s *Server) record(provider WireProvider, stream bool, body json.RawMessage) {
	s.mu.Lock()
	s.requests = append(s.requests, CapturedRequest{Provider: provider, Stream: stream, Body: body})
	s.mu.Unlock()
}

func writeServerError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
