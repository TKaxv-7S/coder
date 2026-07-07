package chatd //nolint:testpackage // Exercises unexported goal continuation helpers.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestFinishTurnContinuesActiveGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationTurn(ctx, machine, input, 0, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	// The chat lands back in running: the continuation turn started.
	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusActive, current.Status)
	require.Equal(t, int64(1), current.ContinuationCount)

	hidden, err := f.db.GetChatHiddenUserMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var continuations []database.ChatMessage
	for _, msg := range hidden {
		continuationGoalID, continuation, err := parseGoalContinuationMessage(msg)
		require.NoError(t, err)
		if continuation {
			require.Equal(t, goal.ID, continuationGoalID)
			continuations = append(continuations, msg)
		}
	}
	require.Len(t, continuations, 1)
	parts, err := chatprompt.ParseContent(continuations[0])
	require.NoError(t, err)
	text := textFromParts(parts)
	require.Contains(t, text, "complete_goal")
	require.Contains(t, text, "block_goal")
	// The continuation message carries the finished turn's API key so
	// hidden-message bookkeeping matches the reminder pattern.
	require.Equal(t, f.apiKey.ID, continuations[0].APIKeyID.String)
}

func TestFinishTurnPausesGoalAtContinuationCap(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	for range maxGoalContinuationTurns {
		_, err := f.db.IncrementChatGoalContinuationCount(dbauthz.AsSystemRestricted(ctx), database.IncrementChatGoalContinuationCountParams{
			RootChatID: chat.ID,
			ID:         goal.ID,
		})
		require.NoError(t, err)
	}
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationTurn(ctx, machine, input, 0, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonTurnLimit), current.PausedReason.String)
}

func TestFinishTurnPausesGoalAtUsageLimit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	insertActiveGoal(ctx, t, f, chat.ID)
	_, err := f.db.UpsertChatUsageLimitConfig(dbauthz.AsSystemRestricted(ctx), database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 0,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err = starter.finishGenerationTurn(ctx, machine, input, 0, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonUsageLimit), current.PausedReason.String)
}

func TestFinishTurnPromotedMessageWinsOverGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "queued while running", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	starter := newGoalTaskStarter(t, f)

	err := starter.finishGenerationTurn(ctx, machine, input, 0, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	// The queued message was promoted; the goal is untouched and no
	// continuation message was inserted.
	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, goal.ID, current.ID)
	require.Equal(t, database.ChatGoalStatusActive, current.Status)
	require.Equal(t, int64(0), current.ContinuationCount)
	requireNoGoalContinuationMessages(ctx, t, f, chat.ID)
}

func TestFinishTurnLeavesNonActiveGoalIdle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, ctx context.Context, f *workerTestFixture, chat database.Chat, goal database.ChatGoal)
	}{
		{
			name: "PausedGoal",
			setup: func(t *testing.T, ctx context.Context, f *workerTestFixture, chat database.Chat, goal database.ChatGoal) {
				_, err := f.db.PauseChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.PauseChatGoalByIDParams{
					RootChatID:   chat.ID,
					ID:           goal.ID,
					PausedReason: string(codersdk.ChatGoalPausedReasonUser),
				})
				require.NoError(t, err)
			},
		},
		{
			name: "PlanMode",
			setup: func(t *testing.T, ctx context.Context, f *workerTestFixture, chat database.Chat, _ database.ChatGoal) {
				_, err := f.db.UpdateChatPlanModeByID(dbauthz.AsSystemRestricted(ctx), database.UpdateChatPlanModeByIDParams{
					ID: chat.ID,
					PlanMode: database.NullChatPlanMode{
						ChatPlanMode: database.ChatPlanModePlan,
						Valid:        true,
					},
				})
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat, input := setupGoalTurn(ctx, t, f)
			goal := insertActiveGoal(ctx, t, f, chat.ID)
			tc.setup(t, ctx, f, chat, goal)
			starter := newGoalTaskStarter(t, f)
			machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

			err := starter.finishGenerationTurn(ctx, machine, input, 0, generationDecision{
				kind:         generationActionFinishTurn,
				finishReason: generationFinishReasonComplete,
			}, generationAttemptNotRequired)
			require.NoError(t, err)

			latest, err := f.db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusWaiting, latest.Status)
			requireNoGoalContinuationMessages(ctx, t, f, chat.ID)
		})
	}
}

func TestFinishErrorPausesActiveGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationError(ctx, machine, input, 0, xerrors.New("model exploded"), generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonError), current.PausedReason.String)
}

func requireNoGoalContinuationMessages(ctx context.Context, t *testing.T, f *workerTestFixture, chatID uuid.UUID) {
	t.Helper()
	hidden, err := f.db.GetChatHiddenUserMessagesByChatID(ctx, chatID)
	require.NoError(t, err)
	for _, msg := range hidden {
		_, continuation, err := parseGoalContinuationMessage(msg)
		require.NoError(t, err)
		require.False(t, continuation, "unexpected continuation message %d", msg.ID)
	}
}

func setupGoalTurn(ctx context.Context, t *testing.T, f *workerTestFixture) (database.Chat, chatWorkerTaskStartInput) {
	t.Helper()
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = acquireChat(t, f, chat.ID, workerID, runnerID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistantTextMessage(t, "done", f.model.ID)},
		})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	return chat, chatWorkerTaskStartInput{
		TaskID:            uuid.New(),
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            chat.Status,
	}
}

func insertActiveGoal(ctx context.Context, t *testing.T, f *workerTestFixture, rootChatID uuid.UUID) database.ChatGoal {
	t.Helper()
	goal, err := f.db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      rootChatID,
		Objective:       "finish the work",
		CreatedByUserID: f.user.ID,
	})
	require.NoError(t, err)
	return goal
}

func newGoalTaskStarter(t *testing.T, f *workerTestFixture) *taskStarter {
	t.Helper()
	logger := testutil.Logger(t)
	clock := quartz.NewReal()
	// The finish paths spawn detached finalize goroutines through
	// Server.goInflight, so the server needs a lifecycle context and
	// config cache even in tests.
	server := &Server{
		ctx:         context.Background(),
		db:          f.db,
		pubsub:      f.pubsub,
		logger:      logger,
		clock:       clock,
		configCache: newChatConfigCache(context.Background(), f.db, clock),
		experiments: codersdk.Experiments{codersdk.ExperimentChatGoals},
	}
	return &taskStarter{
		server: server,
		opts: chatWorkerOptions{
			Store:  f.db,
			Pubsub: f.pubsub,
			Logger: logger,
			Clock:  clock,
		},
		routeStateHint: func(context.Context, runnerStateUpdate) {},
	}
}
