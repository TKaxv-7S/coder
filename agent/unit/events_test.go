package unit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/quartz"
)

func TestManager_Events(t *testing.T) {
	t.Parallel()

	t.Run("RegisterRecordsStatusChange", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))

		require.NoError(t, manager.Register(unitA))

		events := manager.Events()
		require.Len(t, events, 1)
		require.Equal(t, unit.EventStatusChange, events[0].Kind)
		require.Equal(t, unitA, events[0].UnitID)
		require.Equal(t, unit.StatusNotRegistered, events[0].FromStatus)
		require.Equal(t, unit.StatusPending, events[0].ToStatus)
		require.Equal(t, uint64(1), events[0].SequenceNumber)
		require.Equal(t, clock.Now(), events[0].Time)
	})

	t.Run("UpdateStatusRecordsTransition", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))

		require.NoError(t, manager.Register(unitA))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusStarted))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusComplete))

		events := manager.Events()
		require.Len(t, events, 3)

		require.Equal(t, unit.StatusPending, events[1].FromStatus)
		require.Equal(t, unit.StatusStarted, events[1].ToStatus)
		require.Equal(t, unit.StatusStarted, events[2].FromStatus)
		require.Equal(t, unit.StatusComplete, events[2].ToStatus)

		// SequenceNumber is strictly increasing and matches time order.
		require.Equal(t, uint64(2), events[1].SequenceNumber)
		require.Equal(t, uint64(3), events[2].SequenceNumber)
		require.True(t, events[1].Time.Before(events[2].Time))
	})

	t.Run("RejectedTransitionsNotRecorded", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager(unit.WithClock(quartz.NewMock(t)))

		require.NoError(t, manager.Register(unitA))
		before := manager.Events()

		// Same-status update is rejected and must not pollute the log.
		err := manager.UpdateStatus(unitA, unit.StatusPending)
		require.ErrorIs(t, err, unit.ErrSameStatusAlreadySet)

		// Updates to unregistered units are rejected.
		err = manager.UpdateStatus(unitB, unit.StatusStarted)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)

		// Duplicate registration is rejected.
		err = manager.Register(unitA)
		require.ErrorIs(t, err, unit.ErrUnitAlreadyRegistered)

		require.Equal(t, before, manager.Events())
	})

	t.Run("AddDependencyRecordsEvent", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager(unit.WithClock(quartz.NewMock(t)))

		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusComplete))

		events := manager.Events()
		require.Len(t, events, 2)
		require.Equal(t, unit.EventDependencyAdded, events[1].Kind)
		require.Equal(t, unitA, events[1].UnitID)
		require.Equal(t, unitB, events[1].DependsOnUnit)
		require.Equal(t, unit.StatusComplete, events[1].RequiredStatus)
	})

	t.Run("FailedAddDependencyNotRecorded", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager(unit.WithClock(quartz.NewMock(t)))

		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.Register(unitB))
		require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusComplete))
		before := manager.Events()

		// A dependency that would create a cycle is rejected.
		err := manager.AddDependency(unitB, unitA, unit.StatusComplete)
		require.ErrorIs(t, err, unit.ErrFailedToAddDependency)

		// A dependency from an unregistered unit is rejected.
		err = manager.AddDependency(unitC, unitA, unit.StatusComplete)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)

		require.Equal(t, before, manager.Events())
	})

	t.Run("CrossUnitOrdering", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))

		require.NoError(t, manager.Register(unitA))
		clock.Advance(time.Millisecond)
		require.NoError(t, manager.Register(unitB))
		clock.Advance(time.Millisecond)
		require.NoError(t, manager.AddDependency(unitB, unitA, unit.StatusComplete))
		clock.Advance(time.Millisecond)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusStarted))
		clock.Advance(time.Millisecond)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusComplete))
		clock.Advance(time.Millisecond)
		require.NoError(t, manager.UpdateStatus(unitB, unit.StatusStarted))

		events := manager.Events()
		require.Len(t, events, 6)
		want := uint64(1)
		for i, ev := range events {
			require.Equal(t, want, ev.SequenceNumber)
			want++
			if i > 0 {
				require.True(t, events[i-1].Time.Before(ev.Time))
			}
		}
	})

	t.Run("EventsReturnsCopy", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager(unit.WithClock(quartz.NewMock(t)))

		require.NoError(t, manager.Register(unitA))

		events := manager.Events()
		events[0].UnitID = unitB

		require.Equal(t, unitA, manager.Events()[0].UnitID)
	})

	t.Run("DefaultClock", func(t *testing.T) {
		t.Parallel()

		// Without WithClock, the Manager uses a real clock.
		manager := unit.NewManager()

		require.NoError(t, manager.Register(unitA))

		events := manager.Events()
		require.Len(t, events, 1)
		require.False(t, events[0].Time.IsZero())
	})
}
