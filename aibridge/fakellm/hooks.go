package fakellm

import (
	"context"

	"charm.land/fantasy"
)

// Hook intercepts a specific numbered call to one of Model's four
// fantasy.LanguageModel methods, for live, imperative control over
// cancellation/timing/fault-injection behavior that a declarative
// script can't (and shouldn't) express -- e.g. blocking until a
// context is canceled, or panicking to test a crash-recovery path.
//
// This is a deliberate, narrow escape hatch, not a general-purpose
// callback mechanism: content still comes from the Script by default; a
// Hook only overrides that for the one call index it's registered
// against. It's generic over the method's response type (T is
// *fantasy.Response for Generate, fantasy.StreamResponse for Stream,
// and so on) so all four methods share one Hook shape instead of four
// near-identical declarations -- Generate/Stream/GenerateObject/
// StreamObject differ only in what they return, and a hook's job
// (intercept, optionally call through to the scripted behavior) is
// identical in every case.
//
// next runs the scripted behavior for this call (consuming the next
// Turn/ObjectTurn as usual). A hook may call next() zero, one, or more
// times, and may wrap, delay, or entirely replace its result.
//
// Pair Hook with a controllable clock (e.g. github.com/coder/quartz) on
// the production side when testing timer-driven cancellation: the mock
// clock is what deterministically fires the context cancellation a
// hook waits for, so no hook here ever needs to know about time itself.
type Hook[T any] func(ctx context.Context, next func() (T, error)) (T, error)

// BlockUntilContextDone blocks the call itself -- before running the
// scripted behavior at all -- until ctx is done, then returns the zero
// value of T and ctx.Err(). It never calls next(), so the scripted
// turn/object at this call index is left unconsumed for a later call.
//
// Matches "the model never even acknowledges the call" -- e.g. testing
// a silence guard that fires while the stream is still being opened.
func BlockUntilContextDone() Hook[fantasy.StreamResponse] {
	return func(ctx context.Context, _ func() (fantasy.StreamResponse, error)) (fantasy.StreamResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

// ErrorAfterContextDone returns a StreamResponse immediately, but its
// first (and only) yield blocks until ctx is done, then yields a
// StreamPartTypeError carrying ctx.Err(). It never calls next().
//
// Matches "the model acknowledges the call but never produces a first
// part" -- e.g. testing a silence guard that fires before any content
// arrives.
func ErrorAfterContextDone() Hook[fantasy.StreamResponse] {
	return func(ctx context.Context, _ func() (fantasy.StreamResponse, error)) (fantasy.StreamResponse, error) {
		return func(yield func(fantasy.StreamPart) bool) {
			<-ctx.Done()
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: ctx.Err()})
		}, nil
	}
}

// SilentlyBlockUntilContextDone returns a StreamResponse whose iterator
// yields nothing at all, blocks until ctx is done, and then simply
// returns without yielding anything -- a stream that hangs and then
// closes silently, with no error part.
//
// Matches "the stream goes silent forever and is only ever cleaned up
// by context cancellation," as opposed to ErrorAfterContextDone's
// explicit error signal.
func SilentlyBlockUntilContextDone() Hook[fantasy.StreamResponse] {
	return func(ctx context.Context, _ func() (fantasy.StreamResponse, error)) (fantasy.StreamResponse, error) {
		return func(func(fantasy.StreamPart) bool) {
			<-ctx.Done()
		}, nil
	}
}

// PauseAfterFirstPart calls next() and lets the scripted turn's stream
// through unchanged up to and including its first yielded part. After
// that first part, it blocks until either release is closed or ctx is
// done, whichever happens first: if release wins, the rest of the
// scripted stream continues normally; if ctx wins, it yields a
// StreamPartTypeError with ctx.Err() and stops.
//
// Matches "the first part disarms a timeout guard, but a later part is
// still in flight when something else happens" -- release lets a test
// control that race explicitly instead of guessing timing.
func PauseAfterFirstPart(release <-chan struct{}) Hook[fantasy.StreamResponse] {
	return func(ctx context.Context, next func() (fantasy.StreamResponse, error)) (fantasy.StreamResponse, error) {
		inner, err := next()
		if err != nil {
			return nil, err
		}

		return func(yield func(fantasy.StreamPart) bool) {
			first := true
			for part := range inner {
				if !yield(part) {
					return
				}
				if first {
					first = false
					select {
					case <-release:
					case <-ctx.Done():
						yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: ctx.Err()})
						return
					}
				}
			}
		}, nil
	}
}
