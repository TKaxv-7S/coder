package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
)

func TestRenderTimeline(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 14, 2, 1, 0, time.UTC)
	at := base.Add

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "No events found", renderTimeline(nil))
	})

	t.Run("SingleUnit", func(t *testing.T) {
		t.Parallel()

		events := []unit.Event{
			{Seq: 1, Time: at(0), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 2, Time: at(100 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusPending, To: unit.StatusStarted},
			{Seq: 3, Time: at(2300 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusStarted, To: unit.StatusComplete},
		}
		require.Equal(t, `*  [+0.0s]  db-migrate  registered (pending)
*  [+0.1s]  db-migrate  pending → started
*  [+2.3s]  db-migrate  started → completed`, renderTimeline(events))
	})

	t.Run("DependencySatisfaction", func(t *testing.T) {
		t.Parallel()

		// dev-server registers with a dependency on db-migrate. When
		// db-migrate completes, dev-server becomes ready: a connector
		// row and a derived ready row are drawn. db-migrate's lane
		// disappears after completion.
		events := []unit.Event{
			{Seq: 1, Time: at(0), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 2, Time: at(100 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusPending, To: unit.StatusStarted},
			{Seq: 3, Time: at(100 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "dev-server", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 4, Time: at(100 * time.Millisecond), Kind: unit.EventDependencyAdded, Unit: "dev-server", DependsOn: "db-migrate", RequiredStatus: unit.StatusComplete},
			{Seq: 5, Time: at(2300 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusStarted, To: unit.StatusComplete},
			{Seq: 6, Time: at(2400 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "dev-server", From: unit.StatusPending, To: unit.StatusStarted},
			{Seq: 7, Time: at(8 * time.Second), Kind: unit.EventStatusChange, Unit: "dev-server", From: unit.StatusStarted, To: unit.StatusComplete},
		}
		require.Equal(t, `*  [+0.0s]  db-migrate  registered (pending)
*  [+0.1s]  db-migrate  pending → started
| *  [+0.1s]  dev-server  registered (pending)
| *  [+0.1s]  dev-server  depends on db-migrate (completed)
* |  [+2.3s]  db-migrate  started → completed
\ |
  *  [+2.3s]  dev-server  ready (dependency satisfied)
  *  [+2.4s]  dev-server  pending → started
  *  [+8.0s]  dev-server  started → completed`, renderTimeline(events))
	})

	t.Run("IncrementalMatchesOneShot", func(t *testing.T) {
		t.Parallel()

		events := []unit.Event{
			{Seq: 1, Time: at(0), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 2, Time: at(100 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusPending, To: unit.StatusStarted},
			{Seq: 3, Time: at(100 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "dev-server", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 4, Time: at(100 * time.Millisecond), Kind: unit.EventDependencyAdded, Unit: "dev-server", DependsOn: "db-migrate", RequiredStatus: unit.StatusComplete},
			{Seq: 5, Time: at(2300 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "db-migrate", From: unit.StatusStarted, To: unit.StatusComplete},
			{Seq: 6, Time: at(2400 * time.Millisecond), Kind: unit.EventStatusChange, Unit: "dev-server", From: unit.StatusPending, To: unit.StatusStarted},
			{Seq: 7, Time: at(8 * time.Second), Kind: unit.EventStatusChange, Unit: "dev-server", From: unit.StatusStarted, To: unit.StatusComplete},
		}

		oneShot := newTimelineRenderer().renderEvents(events)

		// Feeding the log in overlapping chunks, as --watch does when it
		// repeatedly fetches the full event list, must produce identical
		// output: already-seen events are skipped by sequence number.
		incremental := newTimelineRenderer()
		var got strings.Builder
		got.WriteString(incremental.renderEvents(events[:2]))
		got.WriteString(incremental.renderEvents(events[:2])) // no new events
		got.WriteString(incremental.renderEvents(events[:5])) // overlap + connector boundary
		got.WriteString(incremental.renderEvents(events))

		require.Equal(t, oneShot, got.String())
	})

	t.Run("ReadyFlipNotDrawnWhenStillBlocked", func(t *testing.T) {
		t.Parallel()

		// unit-c depends on both unit-a and unit-b. Completing unit-a
		// alone must not produce a derived ready row.
		events := []unit.Event{
			{Seq: 1, Time: at(0), Kind: unit.EventStatusChange, Unit: "unit-c", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 2, Time: at(0), Kind: unit.EventDependencyAdded, Unit: "unit-c", DependsOn: "unit-a", RequiredStatus: unit.StatusComplete},
			{Seq: 3, Time: at(0), Kind: unit.EventDependencyAdded, Unit: "unit-c", DependsOn: "unit-b", RequiredStatus: unit.StatusComplete},
			{Seq: 4, Time: at(0), Kind: unit.EventStatusChange, Unit: "unit-a", From: unit.StatusNotRegistered, To: unit.StatusPending},
			{Seq: 5, Time: at(time.Second), Kind: unit.EventStatusChange, Unit: "unit-a", From: unit.StatusPending, To: unit.StatusStarted},
			{Seq: 6, Time: at(2 * time.Second), Kind: unit.EventStatusChange, Unit: "unit-a", From: unit.StatusStarted, To: unit.StatusComplete},
		}
		require.Equal(t, `*  [+0.0s]  unit-c  registered (pending)
*  [+0.0s]  unit-c  depends on unit-a (completed)
*  [+0.0s]  unit-c  depends on unit-b (completed)
| *  [+0.0s]  unit-a  registered (pending)
| *  [+1.0s]  unit-a  pending → started
| *  [+2.0s]  unit-a  started → completed`, renderTimeline(events))
	})
}
