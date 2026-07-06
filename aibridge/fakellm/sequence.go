package fakellm

import (
	"context"
	"sync"
	"sync/atomic"
)

// sequence is the single mechanism behind all four fantasy.LanguageModel
// methods: an ordered sequence of calls, each optionally intercepted by
// a Hook. Before this existed, each method (Generate/Stream/
// GenerateObject/StreamObject) carried its own hook map, its own call
// counter, and its own near-identical "if a hook is registered, call
// it; otherwise run the default" branch. sequence collapses that
// duplication into one generic type used four times, one per method.
//
// It deliberately knows nothing about Script/Turn -- "the default
// behavior" is just whatever function the caller passes as scripted.
// This keeps sequence reusable and small: it is pure call-sequencing
// and hook dispatch, nothing more.
type sequence[T any] struct {
	mu    sync.Mutex
	hooks map[int64]Hook[T]
	n     atomic.Int64
}

func newSequence[T any]() *sequence[T] {
	return &sequence[T]{hooks: make(map[int64]Hook[T])}
}

// set registers hook to intercept the callIdx'th (0-based) call in this
// sequence.
func (s *sequence[T]) set(callIdx int64, hook Hook[T]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks[callIdx] = hook
}

// call runs the next invocation in sequence. If a hook is registered
// for this call's index, it wraps scripted (the default, declarative
// behavior, e.g. consuming the next Turn); otherwise scripted runs
// directly.
func (s *sequence[T]) call(ctx context.Context, scripted func(context.Context) (T, error)) (T, error) {
	idx := s.n.Add(1) - 1
	s.mu.Lock()
	hook := s.hooks[idx]
	s.mu.Unlock()

	if hook == nil {
		return scripted(ctx)
	}
	return hook(ctx, func() (T, error) { return scripted(ctx) })
}
