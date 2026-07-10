package unit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/quartz"
)

func TestDeriveIntervals(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		require.Empty(t, unit.DeriveIntervals(nil))
	})

	t.Run("SingleUnitLifecycle", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))
		start := clock.Now()

		require.NoError(t, manager.Register(unitA))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusStarted))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusComplete))

		intervals := unit.DeriveIntervals(manager.Events())
		require.Equal(t, map[unit.ID][]unit.Interval{
			unitA: {
				{Unit: unitA, Phase: unit.PhaseReady, Start: start, End: start.Add(time.Second)},
				{Unit: unitA, Phase: unit.PhaseRunning, Start: start.Add(time.Second), End: start.Add(2 * time.Second)},
				{Unit: unitA, Phase: unit.PhaseDone, Start: start.Add(2 * time.Second)},
			},
		}, intervals)
	})

	t.Run("BlockedUntilDependencySatisfied", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))
		start := clock.Now()

		// unitB registers with an immediate dependency on unitA, which
		// registers later and runs to completion.
		require.NoError(t, manager.Register(unitB))
		require.NoError(t, manager.AddDependency(unitB, unitA, unit.StatusComplete))
		clock.Advance(time.Second)
		require.NoError(t, manager.Register(unitA))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusStarted))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusComplete))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitB, unit.StatusStarted))

		intervals := unit.DeriveIntervals(manager.Events())
		require.Equal(t, []unit.Interval{
			// Registration opens a ready interval that the immediately
			// following dependency_added event closes at the same instant.
			{Unit: unitB, Phase: unit.PhaseReady, Start: start, End: start},
			{Unit: unitB, Phase: unit.PhaseBlocked, Start: start, End: start.Add(3 * time.Second)},
			{Unit: unitB, Phase: unit.PhaseReady, Start: start.Add(3 * time.Second), End: start.Add(4 * time.Second)},
			{Unit: unitB, Phase: unit.PhaseRunning, Start: start.Add(4 * time.Second)},
		}, intervals[unitB])
	})

	t.Run("LateDependencyFlipsReadyToBlocked", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))
		start := clock.Now()

		// unitA registers with no dependencies, so it is immediately
		// ready. A dependency added afterwards flips it back to blocked
		// with no status change. This is the edge case that motivates
		// recording dependency_added events.
		require.NoError(t, manager.Register(unitA))
		clock.Advance(time.Second)
		require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusComplete))

		intervals := unit.DeriveIntervals(manager.Events())
		require.Equal(t, []unit.Interval{
			{Unit: unitA, Phase: unit.PhaseReady, Start: start, End: start.Add(time.Second)},
			{Unit: unitA, Phase: unit.PhaseBlocked, Start: start.Add(time.Second)},
		}, intervals[unitA])
	})

	t.Run("MultipleDependencies", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))
		start := clock.Now()

		// unitC depends on both unitA and unitB completing. It only
		// becomes ready when the last dependency is satisfied.
		require.NoError(t, manager.Register(unitC))
		require.NoError(t, manager.AddDependency(unitC, unitA, unit.StatusComplete))
		require.NoError(t, manager.AddDependency(unitC, unitB, unit.StatusComplete))
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.Register(unitB))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusStarted))
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusComplete))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitB, unit.StatusStarted))
		require.NoError(t, manager.UpdateStatus(unitB, unit.StatusComplete))

		intervals := unit.DeriveIntervals(manager.Events())
		require.Equal(t, []unit.Interval{
			{Unit: unitC, Phase: unit.PhaseReady, Start: start, End: start},
			{Unit: unitC, Phase: unit.PhaseBlocked, Start: start, End: start.Add(2 * time.Second)},
			{Unit: unitC, Phase: unit.PhaseReady, Start: start.Add(2 * time.Second)},
		}, intervals[unitC])
	})

	t.Run("DependencyOnRunningStatus", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		manager := unit.NewManager(unit.WithClock(clock))
		start := clock.Now()

		// A dependency requiring StatusStarted is satisfied while the
		// dependency runs and unsatisfied again once it completes.
		require.NoError(t, manager.Register(unitB))
		require.NoError(t, manager.AddDependency(unitB, unitA, unit.StatusStarted))
		require.NoError(t, manager.Register(unitA))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusStarted))
		clock.Advance(time.Second)
		require.NoError(t, manager.UpdateStatus(unitA, unit.StatusComplete))

		intervals := unit.DeriveIntervals(manager.Events())
		require.Equal(t, []unit.Interval{
			{Unit: unitB, Phase: unit.PhaseReady, Start: start, End: start},
			{Unit: unitB, Phase: unit.PhaseBlocked, Start: start, End: start.Add(time.Second)},
			{Unit: unitB, Phase: unit.PhaseReady, Start: start.Add(time.Second), End: start.Add(2 * time.Second)},
			{Unit: unitB, Phase: unit.PhaseBlocked, Start: start.Add(2 * time.Second)},
		}, intervals[unitB])
	})

	t.Run("UnregisteredDependencyHasNoIntervals", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager(unit.WithClock(quartz.NewMock(t)))

		// unitB never registers; it only appears on the depends-on side
		// of an edge, so it must not get intervals of its own.
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusComplete))

		intervals := unit.DeriveIntervals(manager.Events())
		require.Contains(t, intervals, unitA)
		require.NotContains(t, intervals, unitB)
	})
}
