package models

// ActorKind names who performed the action that caused an enqueue, per
// AC-OFFICE-RUN-CAUSATION-001.15: exactly one of a human user, an Office
// agent, or the system itself. It is a property of the causing action,
// not of the agent being woken.
type ActorKind string

// Actor kind values. There is deliberately no "unspecified" zero value
// that resolves silently — an absent or unrecognized kind is handled by
// ResolveActorKind, never by relying on ActorKind's Go zero value.
const (
	ActorKindUser   ActorKind = "user"
	ActorKindAgent  ActorKind = "agent"
	ActorKindSystem ActorKind = "system"
)

// String implements fmt.Stringer.
func (k ActorKind) String() string { return string(k) }

// Valid reports whether k is one of the three declared actor kinds.
func (k ActorKind) Valid() bool {
	switch k {
	case ActorKindUser, ActorKindAgent, ActorKindSystem:
		return true
	default:
		return false
	}
}

// PriorityClass is the coarse bucket that decides which queued run is
// claimed first when capacity is scarce (AC-OFFICE-BACKPRESSURE-001.1).
// Values are ordered most to least preferred and the integer values are
// persisted on runs.priority_class, so they must not be renumbered.
type PriorityClass int

const (
	// PriorityClassRecovery is assigned only by re-queue (retry, recovery
	// sweep, routing re-dispatch); no wake reason maps here directly.
	PriorityClassRecovery PriorityClass = 1
	// PriorityClassEvent is the default class for ordinary wake reasons.
	PriorityClassEvent PriorityClass = 2
	// PriorityClassPeriodic is for unattended, schedule-driven fires.
	PriorityClassPeriodic PriorityClass = 3
	// PriorityClassHuman is the highest-preference class, assigned when
	// the actor is a human user or the wake reason is human-only.
	PriorityClassHuman PriorityClass = 0
)

// String implements fmt.Stringer.
func (c PriorityClass) String() string {
	switch c {
	case PriorityClassHuman:
		return "human"
	case PriorityClassRecovery:
		return "recovery"
	case PriorityClassEvent:
		return "event"
	case PriorityClassPeriodic:
		return "periodic"
	default:
		return "unknown"
	}
}
