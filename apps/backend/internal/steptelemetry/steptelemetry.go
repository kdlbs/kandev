// Package steptelemetry carries the attribution (who and why) for a
// committed change to a task's workflow step, from the caller that knows it
// to the repository transaction that writes the task_step_transitions ledger
// row. It is imported by the sqlite repository, the task service, the
// orchestrator, and the MCP handlers; it imports none of them, so no import
// cycle is possible.
package steptelemetry

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
)

// ContractVersion is the collection-contract version stamped on every ledger
// row. Bump it (and register a new telemetrycontract activation) only for a
// change to what a row means, not for adding a new Trigger/ActorKind value.
const ContractVersion = 1

// ContractKey identifies this contract in the telemetry_activations registry.
const ContractKey = "task_step_transitions"

// Trigger is why a task's workflow step changed. The enum is closed;
// unrecognised or unattributable causes use TriggerUnknown.
type Trigger string

const (
	TriggerTaskCreated      Trigger = "task_created"
	TriggerManualMove       Trigger = "manual_move"
	TriggerTaskUpdate       Trigger = "task_update"
	TriggerMCPMove          Trigger = "mcp_move"
	TriggerMCPDeferredMove  Trigger = "mcp_deferred_move"
	TriggerEngineTransition Trigger = "engine_transition"
	TriggerUserCancellation Trigger = "user_cancellation"
	TriggerWIPPull          Trigger = "wip_pull"
	TriggerBulkMove         Trigger = "bulk_move"
	TriggerUnarchiveRestore Trigger = "unarchive_restore"
	TriggerWorkflowAttached Trigger = "workflow_attached"
	TriggerWorkflowDetached Trigger = "workflow_detached"
	TriggerPluginMove       Trigger = "plugin_move"
	TriggerUnknown          Trigger = "unknown"
)

// ActorKind is what kind of thing caused the step change. The enum is
// closed; not-determinable causes use ActorUnknown.
type ActorKind string

const (
	ActorHuman       ActorKind = "human"
	ActorAgent       ActorKind = "agent"
	ActorSystem      ActorKind = "system"
	ActorIntegration ActorKind = "integration"
	ActorUnknown     ActorKind = "unknown"
)

// Attribution is who and why. The zero value is the spec's recorded fact for
// a caller that declares nothing: unknown trigger, unknown actor, no IDs.
type Attribution struct {
	Trigger   Trigger
	ActorKind ActorKind
	// ActorID is an opaque identifier — never a display name, email, title,
	// or prompt text. For ActorKind agent it deliberately duplicates
	// SessionID, so actor_id has one uniform meaning across every actor kind.
	ActorID string
	// SessionID is the initiating session, when one exists and is genuinely
	// the initiator. Left empty when there is none or several candidates
	// exist with no single initiator — never guessed.
	SessionID string
}

type attributionContextKey struct{}

// WithAttribution attaches an Attribution to ctx for the repository
// transaction that eventually writes the ledger row to read back via
// FromContext. It does not overwrite an Attribution already on ctx unless
// the caller replaces it explicitly — see the package doc on outermost-caller
// wins for how callers should compose this.
func WithAttribution(ctx context.Context, a Attribution) context.Context {
	return context.WithValue(ctx, attributionContextKey{}, a)
}

// FromContext reads the Attribution on ctx, normalizing an absent one (or one
// with an empty Trigger/ActorKind) to unknown/unknown — a recorded fact, not
// an error, per the spec.
func FromContext(ctx context.Context) Attribution {
	a, _ := ctx.Value(attributionContextKey{}).(Attribution)
	if a.Trigger == "" {
		a.Trigger = TriggerUnknown
	}
	if a.ActorKind == "" {
		a.ActorKind = ActorUnknown
	}
	return a
}

// HasTrigger reports whether ctx already carries an explicit (non-empty)
// trigger. Callers that must not clobber an outer caller's attribution
// (e.g. MoveTaskWithOptions, which an MCP move trigger must survive) check
// this before calling WithAttribution with their own default.
func HasTrigger(ctx context.Context) bool {
	a, ok := ctx.Value(attributionContextKey{}).(Attribution)
	return ok && a.Trigger != ""
}

// HumanOrSystemActor resolves the actor kind/id from the identity already on
// ctx (the existing authn seam, not this package's own wiring): an
// authenticated identity (including the synthetic single-user identity
// injected when auth is disabled) is ActorHuman with that identity's UserID;
// no identity on ctx is ActorSystem with an empty ActorID. Used by call sites
// where "who did this" reduces to "whoever is making the request, or nobody"
// — task creation and manual board moves.
func HumanOrSystemActor(ctx context.Context) (ActorKind, string) {
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		return ActorHuman, identity.UserID
	}
	return ActorSystem, ""
}
