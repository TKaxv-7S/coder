package unit

import "time"

// EventKind identifies the type of change an Event records.
type EventKind string

const (
	// EventStatusChange records a unit status transition, including the
	// initial registration into StatusPending.
	EventStatusChange EventKind = "status_change"
	// EventDependencyAdded records a dependency edge added to the graph.
	// Dependency declarations are recorded because they can change a
	// unit's readiness without a status change, so readiness intervals
	// can only be reconstructed if edge additions are part of the log.
	EventDependencyAdded EventKind = "dependency_added"
)

// Event is an immutable record of a change to the dependency graph.
// Events are appended by the Manager under its write lock, so SequenceNumber
// order equals Time order within a single Manager.
type Event struct {
	// SequenceNumber is a monotonic, Manager-global sequence number.
	SequenceNumber uint64
	// Time is captured from the Manager's clock at the point of the
	// change, inside the write lock. It is monotonic-clock-backed.
	Time time.Time
	Kind EventKind
	// UnitID is the unit the event applies to.
	UnitID ID

	// FromStatus and ToStatus are set for EventStatusChange. FromStatus is
	// StatusNotRegistered for the registration event.
	FromStatus Status
	ToStatus   Status

	// DependsOnUnit and RequiredStatus are set for EventDependencyAdded.
	DependsOnUnit  ID
	RequiredStatus Status
}
