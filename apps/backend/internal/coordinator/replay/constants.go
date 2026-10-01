// Package replay re-runs decided coordinator turns against a candidate change
// to the coordinator's instructions and scores what the candidate would have
// proposed against what a manager decided. It is a library: it has no route,
// no tool and no write other than its own result row.
package replay

import "time"

// The harness constants have no setter and no configuration key.
const (
	// MinHeldOut is the least number of held-out compared cases an
	// improvement needs.
	MinHeldOut = 20
	// MinGainThousandths is the least held-out score gain, in thousandths,
	// that makes a candidate an improvement.
	MinGainThousandths = 50
	// CaseWindow is the most turns one replay selects.
	CaseWindow = 50 + 50
	// MaxOutputTokens bounds the output a run is priced at when it is reserved.
	MaxOutputTokens = 4000
	// ReplayTimeout is the wall-clock bound of one replay.
	ReplayTimeout = 20 * time.Minute
	// RunTimeout is the bound of one model call.
	RunTimeout = 90 * time.Second
	// Concurrency is the most model calls in flight at once.
	Concurrency = 4
	// PriceTimeout bounds the price lookup.
	PriceTimeout = 5 * time.Second
	// FinalWriteTimeout bounds the cost updates and the final write, which run
	// on a context detached from cancellation.
	FinalWriteTimeout = 10 * time.Second
	// Attempts is the number of runs of one case on one side.
	Attempts = 3
	// MinAttemptsRan is the number of successful attempts that make a case
	// run on a side.
	MinAttemptsRan = 2
	// FirstGroupSize and SecondGroupSize are the two selection groups.
	FirstGroupSize  = 50
	SecondGroupSize = 50
	// OverrideHorizon is how far back the second selection group looks.
	OverrideHorizon = 90 * 24 * time.Hour
	// StaleAfter is the age past which a running result row is settled.
	StaleAfter = 30 * time.Minute
	// Retention is how long result rows are kept.
	Retention = 400 * 24 * time.Hour
	// PromptVersion keys baseline reuse; it is bumped on any change to the
	// prompt frame, the replay paragraph, the note section or the answer schema.
	PromptVersion = "replay-v1"
)

// Guard results, verdicts and reasons of a replay.
const (
	GuardPass       = "pass"
	GuardBlocked    = "blocked"
	GuardUnmeasured = "unmeasured"

	VerdictImprovement      = "improvement"
	VerdictNotAnImprovement = "not_an_improvement"
	VerdictUnmeasured       = "unmeasured"

	ReasonBudget        = "budget"
	ReasonCostUnknown   = "cost_unknown"
	ReasonTooFew        = "too_few"
	ReasonNoCases       = "no_cases"
	ReasonAllSkipped    = "all_skipped"
	ReasonNoCompared    = "no_compared"
	ReasonNoTarget      = "no_target"
	ReasonProfileUnsafe = "profile_unsafe"
	ReasonReadFailed    = "read_failed"
	ReasonCancelled     = "cancelled"
	ReasonInterrupted   = "interrupted"
)

// Skip reasons of one turn.
const (
	SkipDreamTurn     = "dream_turn"
	SkipNoSnapshot    = "no_snapshot"
	SkipInputGone     = "input_gone"
	SkipNoExpectation = "no_expectation"
)

// Row statuses.
const (
	StatusRunning = "running"
	StatusDone    = "done"
)

// Candidate kinds a replay accepts.
const (
	KindContextDiff        = "context_diff"
	KindStandingOrderAdd   = "standing_order_add"
	KindStandingOrderRetir = "standing_order_retire"
	KindNoteAdd            = "note_add"
	KindNoteUpdate         = "note_update"
	KindNoteRetire         = "note_retire"
)

// Proposal kinds a case can hold, as the coordinator stores them. The
// improvement kind is never an expectation.
const (
	ProposalCreateTask  = "create_task"
	ProposalMessage     = "message"
	ProposalMove        = "move"
	ProposalResume      = "resume"
	ProposalImprovement = "improvement"
)

// Decisions of an outcome row and the ledger triggers, redeclared so that the
// package depends on no coordinator package; replay/wire asserts they match.
const (
	DecisionApproved = "approved"
	DecisionEdited   = "edited"
	DecisionRejected = "rejected"
	DecisionReturned = "returned"
	DecisionUndone   = "undone"

	TriggerMessage = "message"
	TriggerWake    = "wake"
	TriggerDream   = "dream"
)
