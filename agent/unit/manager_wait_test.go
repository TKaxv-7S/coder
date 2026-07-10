package unit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/testutil"
)

func TestManager_WaitUntilReady(t *testing.T) {
	t.Parallel()

	t.Run("ReadyImmediately", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))

		// A unit with no dependencies is ready immediately.
		require.NoError(t, manager.WaitUntilReady(ctx, unitA))
	})

	t.Run("BlocksThenUnblocks", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.Register(unitB))
		// unitA depends on unitB reaching "completed".
		require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusComplete))

		ready, err := manager.IsReady(unitA)
		require.NoError(t, err)
		require.False(t, ready, "unitA should not be ready before its dependency completes")

		waitErr := make(chan error, 1)
		go func() {
			waitErr <- manager.WaitUntilReady(ctx, unitA)
		}()

		// The waiter should still be blocked because unitB has not completed.
		select {
		case err := <-waitErr:
			t.Fatalf("WaitUntilReady returned early: %v", err)
		case <-time.After(testutil.IntervalFast):
		}

		// Satisfy the dependency; the waiter should unblock.
		require.NoError(t, manager.UpdateStatus(unitB, unit.StatusComplete))

		require.NoError(t, testutil.RequireReceive(ctx, t, waitErr))
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.Register(unitB))
		require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusComplete))

		waitErr := make(chan error, 1)
		go func() {
			waitErr <- manager.WaitUntilReady(ctx, unitA)
		}()

		cancel()

		err := testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, waitErr)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("UnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := unit.NewManager()

		err := manager.WaitUntilReady(ctx, unitA)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})
}
