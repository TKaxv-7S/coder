package chatloop

// Migration spike: does fakellm's Hook + real quartz mock clock close
// the "live cancellation/timing behavior" gap identified in the fakellm
// design thread? This reimplements the stream-silence-timeout suite
// from chatloop_run_internal_test.go against fakellm.Model instead of
// chattest.FakeModel, driving the exact same production code
// (GenerateAssistant + its internal streamSilenceGuard) with a real
// quartz.Mock clock -- no wall-clock delays anywhere.

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/aibridge/fakellm"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGenerateAssistant_StreamSilenceTimeoutRetryClassification_FakeLLM(t *testing.T) {
	t.Parallel()

	t.Run("timeout while opening stream", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()

		// First Stream call: blocks until the silence guard cancels its
		// context (driven entirely by the mock clock -- no real ctx.Done()
		// wait ever depends on wall-clock time), matching "the model
		// hangs before even acknowledging the call."
		// Second Stream call: succeeds with an empty scripted turn.
		model := fakellm.NewModel(fakellm.MustParseString(`{"empty_turn": true}`))
		model.SetStreamHook(0, fakellm.BlockUntilContextDone())

		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		require.Error(t, <-done)
		// model.Calls() only counts turns actually consumed from the
		// script; BlockUntilContextDone() never calls next(), so it stays
		// at 0. GenerateCalls() reflects every Stream invocation
		// regardless of hook behavior -- the equivalent of the original
		// test's "calls.Load() == 1".
		require.Len(t, model.GenerateCalls(), 1)
	})

	t.Run("timeout before first part", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()

		model := fakellm.NewModel(fakellm.MustParseString(`{"empty_turn": true}`))
		model.SetStreamHook(0, fakellm.ErrorAfterContextDone())

		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		err := <-done
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindStreamSilenceTimeout, classified.Kind)
		require.True(t, classified.Retryable)
	})

	t.Run("first part disarms timeout", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()

		release := make(chan struct{})
		model := fakellm.NewModel(fakellm.MustParseString(`{"text": "done"}`))
		model.SetStreamHook(0, fakellm.PauseAfterFirstPart(release))

		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		close(release)
		require.NoError(t, <-done)
		require.EqualValues(t, 1, model.Calls())
	})

	t.Run("silent stream close after timeout", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()

		model := fakellm.NewModel(fakellm.MustParseString(`{"empty_turn": true}`))
		model.SetStreamHook(0, fakellm.SilentlyBlockUntilContextDone())

		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		err := <-done
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindStreamSilenceTimeout, classified.Kind)
	})
}

// TestGenerateAssistant_PanicInPublishMessagePartReleasesAttempt_FakeLLM
// is the one subtest whose cancellation isn't quartz/timer-driven at
// all -- it's triggered by a panic-recovery path in the caller. Same
// Hook primitive still applies: fakellm just needs to react to ctx.Done(),
// regardless of what caused it.
func TestGenerateAssistant_PanicInPublishMessagePartReleasesAttempt_FakeLLM(t *testing.T) {
	t.Parallel()

	attemptReleased := make(chan struct{})
	model := fakellm.NewModel(fakellm.MustParseString(`{"text": "boom"}`))
	model.SetStreamHook(0, func(ctx context.Context, next func() (fantasy.StreamResponse, error)) (fantasy.StreamResponse, error) {
		go func() {
			<-ctx.Done()
			close(attemptReleased)
		}()
		return next()
	})

	defer func() {
		r := recover()
		require.NotNil(t, r)
		select {
		case <-attemptReleased:
		case <-time.After(time.Second):
			t.Fatal("attempt context was not released after panic")
		}
	}()

	_, _ = GenerateAssistant(context.Background(), GenerateAssistantOptions{
		Model: model,
		PublishMessagePart: func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {
			panic("publish panic")
		},
	})

	t.Fatal("expected GenerateAssistant to panic")
}

// TestGenerateCompactionSummary_PanicFinalizesAsError_FakeLLM reimplements
// compaction_internal_test.go's real panic-recovery test using
// fakellm.Model + SetGenerateHook instead of chattest.FakeModel.
// GenerateFn there panics directly; a scripted Turn has no (and should
// have no) way to express "this call panics" -- that's exactly what
// SetGenerateHook is for: an arbitrary Go closure escape hatch, same
// shape as SetStreamHook, just for Generate.
func TestGenerateCompactionSummary_PanicFinalizesAsError_FakeLLM(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	chatID := uuid.New()
	runID := uuid.New()

	status := make(chan string, 1)

	db.EXPECT().InsertChatDebugRun(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
	).Return(database.ChatDebugRun{
		ID:     runID,
		ChatID: chatID,
	}, nil)
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil)
	db.EXPECT().UpdateChatDebugRun(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
	).DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
		status <- params.Status.String
		return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
	})

	// The hook panics before ever calling next(), so the script is never
	// consumed -- proving the hook fires strictly before any scripted
	// content is touched. An empty script still parses fine even though
	// it's never used.
	model := fakellm.NewModel(fakellm.MustParseString(`{"empty_turn": true}`))
	model.SetGenerateHook(0, func(context.Context, func() (*fantasy.Response, error)) (*fantasy.Response, error) {
		panic("compaction model crash")
	})

	parentCtx := chatdebug.ContextWithRun(context.Background(), &chatdebug.RunContext{
		RunID:               uuid.New(),
		ChatID:              chatID,
		ModelConfigID:       uuid.New(),
		TriggerMessageID:    1,
		HistoryTipMessageID: 2,
		Kind:                chatdebug.KindChatTurn,
		Provider:            "fake",
		Model:               "fake-model",
	})

	require.PanicsWithValue(t, "compaction model crash", func() {
		_, _ = generateCompactionSummary(parentCtx, model,
			[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
			CompactionOptions{
				DebugSvc:      svc,
				ChatID:        chatID,
				SummaryPrompt: "summarize",
				Timeout:       time.Second,
			})
	})

	select {
	case s := <-status:
		require.Equal(t, string(chatdebug.StatusError), s,
			"panic path must finalize the debug run with StatusError")
	case <-time.After(testutil.WaitShort):
		t.Fatal("FinalizeRun never reached UpdateChatDebugRun on panic")
	}

	// The hook never called next(), so no turn was consumed and no
	// Generate content was ever produced -- but the call was still
	// captured, proving capture happens unconditionally, hook or not.
	require.Len(t, model.GenerateCalls(), 1)
}
