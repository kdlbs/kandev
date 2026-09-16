---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002
---

# Workflow Move Preview System Design

## Purpose and boundaries

Add an advisory projection of the existing move decision. Preserve the routing
and best-effort conditional-setting contracts. The tasks system owns the
projection; the agent runtime owns actual provider acceptance.

[Requirements](../requirements/workflow-move-preview.md) define the outcome.
[Lifecycle design](workflow-profile-session-lifecycle.md) defines recipients.
[Conditional settings](../requirements/workflow-session-settings.md) defines
set, keep, restore-original, original-session eligibility, and partial failure.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001 | Resolution, API, Freshness and failures |
| REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002 | Presentation, Mobile, Verification |

## Resolution

Introduce a read-only decision layer in `internal/orchestrator/workflow_move_preview.go`.
Extract shared selection decisions from `prepareWorkflowStepSession`,
`shouldKeepCurrentWorkflowStepSession`, and `workflow_session_target.go`.
The existing execution paths consume those decisions but retain their atomic
selection, route-ledger, retry, credential, and terminal-session checks.
Do not call mutating prepare/ensure helpers to implement preview.

Resolve the same authoritative move source as `httpMoveTask`; do not use the
browser's viewed session as routing authority. Read the destination, source,
sessions, explicit target bindings, profile configuration, and available runtime
configuration. Apply destination start policy and source end policy. No profile
override still follows existing workflow-default resolution. Reuse may select
a matching other session or create one if no eligible candidate exists and the
destination can actually launch a session for this move. When the destination
has no auto-start action, or when `skip_step_prompt` removes the only launch
turn because it has no instructions, report an idle `no_session` outcome
instead of claiming that a session will be created. The shared
`workflowmove.ShouldAutoStartAgent` predicate keeps this projection aligned
with the no-session execution path.

Preserve terminal-session exclusion, explicit target identity, deterministic
candidate ordering, and same-profile/new behavior. A zero-session task is
reported through the existing launch decision; if that decision cannot be
resolved without provisioning, return unknown rather than ensuring a session.
Dynamic profile selection without a concrete recipient reports its profile and
an unknown model; preview never probes or launches a provider to choose one.

For an existing recipient, use the runtime's effective configuration projection
and durable overrides with their existing precedence. Profile defaults alone
cannot describe a reused conversation. For new recipients, resolve launch
configuration without carrying overrides from the outgoing session.

Factor pure rule selection and configuration projection out of
`workflow_session_config.go`. Reuse `selectConfigureSessionRule`,
`sessionConfigurationTargetForRule`, and existing option normalization.
`resolveWorkflowSessionConfigTarget` currently emits conversation warnings;
preview must instead collect structured diagnostic codes without calling that
side-effecting path. Share decisions, not warning publication.

Evaluate original-session eligibility, canonical family matching and ambiguity,
keep/no-match, set's named fields, and restore's immutable original snapshot.
Report skipped rules accurately. Preserve restrictions on model/mode options.
Use available model-aware capability snapshots only. Provider acceptance remains
uncertain until application; do not synthesize capabilities or restore defaults.

Project applicable context-reset entry actions and the normalized one-shot reset
option using execution order. Include whether the configured prompt will run,
is suppressed, or waits for turn end/queue admission in expanded details.
Do not duplicate reset effects or treat instruction text as a configuration change.

## API

Proposed read-only POST `/api/v1/tasks/:id/move-preview` accepts the destination
`workflow_id`, `workflow_step_id`, and the existing `entry_options` shape.
It shares request validation and task/workspace authorization with the move
handler. It accepts no client-selected source session. POST carries the draft
without putting instructions in a URL. Neither instructions nor raw request
bodies are logged or echoed. Return `Cache-Control: no-store`.

Wire the handler through `task_handlers.go`, a narrow orchestrator preview
interface, and existing dependency construction. New DTO fields:

- `task_id`, `workflow_step_id`, `source_session_id`, `evaluated_at`.
- `outcome`: `reuse_current`, `reuse_other`, `create_new`, `no_session`, or
  `unknown`.
- `recipient`: nullable session ID/name, profile ID/name, agent family.
- `model`: before/after IDs and labels, known/unknown flags, and source
  (`runtime`, `override`, `profile`, `step_rule`, `original`).
- `changes`: normalized field key, safe label, before/after value, and
  `planned`/`unchanged`/`skipped`/`unknown` applicability.
- `context_reset`, `source_disposition`: keep/park/complete/unknown,
  `dispatch`: prompt/no_prompt/deferred/unknown.
- `notices`: closed reason codes with safe parameters, including retained
  model override, missing snapshot, unsupported option, and stale capability.

Only selectable, non-sensitive model configuration enters this DTO. Never expose
credentials, environment, raw session metadata, prompts, or provider errors.
Unauthorized and missing tasks use existing API errors. Uncertain routing uses
an unknown projection; a `no_session` projection has no recipient, and no
fabricated session ID is returned for a new session.
This response is additive; no migration or durable preview ledger is needed.

## Freshness and failures

`use-workflow-move-preview.ts` under `hooks/domains/kanban/` owns requests through
`lib/api/domains/kanban-api.ts`. Fetch only while an applicable disclosure is open.
Use the existing hover delay. Every disclosed movable drawer row may enqueue a
request, while a shared queue keeps at most two requests in flight and cancels
rows that close or unmount. Do not request previews for every step across every
mounted task.
Deduplicate pending requests by task, destination, normalized options, and a local
invalidation generation. Abort on close/context change and reject late generations.

Invalidate on task placement/primary-session changes, relevant session runtime
updates, profile updates, workflow-step updates, reconnect, and draft changes.
Clear on close and fetch fresh on reopen. Debounce draft edits; typing instruction
text need not refresh unless its presence changes prompt dispatch semantics.

Keep two-line loading/error states compact. Retry is explicit and does not move
the task. Preview failure does not introduce a new move barrier. Show predictions
as planned, not guaranteed; details explain that execution rechecks current state.
Deferred moves show that settings are checked again at turn end. The actual move
continues through existing execution paths and handles provider partial failures.
No preview token is required to move and no confirmation flow is added.

## Presentation

Add `workflow-move-preview.tsx` as a shared projection renderer with no fetches.
`StepMoveControls` in `workflow-stepper.tsx` owns the draft and passes the same
normalized entry options to preview and move. Thread task/workflow identity down
from the stepper. Integrate the renderer in `workflow-step-disclosure.tsx` rows.

Collapsed layout is two centered lines below actions and capabilities. Line one distinguishes current,
other named, new conversations, and an idle task with no recipient. Line two shows an unchanged effective model
or a before/after arrow, followed by an optional additional-change count.
A retained model/profile mismatch uses the compact override label from UI-01.

Count actual non-model field changes plus one context-reset effect. Do not count
source retirement as a setting change. Deduplicate model and config-option model
representations. If only reasoning changes, show the unchanged model with +1.
Use the generic New session label for a permitted fresh launch. Use a separate
localized no-session label when the move leaves the task idle. Show the model on
line two for a fresh launch when known; profile name belongs in details when it
differs from the model label.

The API returns stable field, value, and diagnostic reason codes for Kandev-owned
settings. The renderer localizes those codes in the active locale and displays
provider-defined labels only when they are safe display data. Model and session
names remain data.

Center both footer lines and use an icon-only info button with a localized
accessible name. A separator divides actions from the muted footer.

The chat and passthrough next-step controls pass the destination directly from
`usePlanActions` through `WorkflowMoveProceedButton` to the options form. The
form previews its current draft below the Move action, on desktop and touch.
Mount the preview only while the options surface is open. Use the existing
revision, debounce, cancellation, and concurrency logic.

Use a keyboard/touch-operable details toggle with `aria-expanded`, not a nested
hover-only tooltip. Details list full session/profile identity, planned changes,
source retirement, dispatch, and applicable notices. Arrow summaries mean planned
changes; accessible text and expanded details explain this. Keep Options, existing
capability icons, progress, move errors, and move pending ownership intact.
Unknowns take priority over misleading before/after arrows. Status changes use
one localized status region without repeatedly announcing unchanged predictions.

## Mobile

Reuse `CompactWorkflowStepDisclosure` and its `useTouchDrawer` branch, including
its `max-h-[80dvh]` surface and fixed header. The curated MobilePickerSheet pattern
supplies a discoverable header entry, inset drawer, focus return, and safe areas.
This is a short temporary step choice, so it belongs in the existing drawer.

Each visible step row gains the same two-line summary. Details expand inline
inside the drawer's single scroll body; avoid nested popovers or drawers.
Move here, Options, Retry, and details have at least 44px touch hit areas.
Desktop keeps compact 24px action density. Long names truncate in the summary
and wrap in details. Verify 320px, the default mobile viewport, and breakpoint
neighbors 767/768px with the applicable pointer mode. No document horizontal
scroll or separate persisted mobile selection is introduced.

## Verification

Backend fixtures compare preview selection/configuration with actual move
outcomes on isolated tasks. Assert preview does not change rows, metadata,
route bindings, messages, or runtime call counters. Cover the historical
Luna-profile/Astra override scenario plus conditional settings, the two
no-session launch gates, task-profile fallback, and deferred moves. Frontend
tests cover live store/reconnect invalidation, queued rows beyond the first two,
and localized field/value/diagnostic rendering.
Browser tests inspect the preview, perform the move, then verify recipient and
model through fixture API state. See the [plan](../../../plans/workflow-move-preview/plan.md).

## Related decisions

- [Profile session lifecycle](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Original configuration](../../../decisions/2026-08-01-workflow-session-original-configuration.md)

This projection preserves those decisions. Shared read-only resolution avoids
frontend routing duplication; an independent frontend approximation would drift
on explicit bindings and runtime overrides. A new ADR is unnecessary for this
additive feature because these boundaries and rationale fit the paired design.
