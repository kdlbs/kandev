# ADR-2026-08-02-explicit-user-cancel-completion: Explicit User Cancellation May Complete a Workflow Step

**Status:** accepted
**Date:** 2026-08-02
**Area:** workflow

## Context

Normal agent turn completion evaluates a workflow step's `on_turn_complete` actions, while the visible user cancel path only reconciles session and runtime task state. That split can leave a cancelled Kanban task review-ready but still positioned on its work step. Cancellation also occurs internally for clarifications, peer interrupts, parent stops, archival, failures, and teardown, so treating every cancelled or stopped runtime event as workflow completion would create false transitions.

## Decision

Workflow steps gain an opt-in `cancel_triggers_turn_complete` boolean. Only the existing explicit, authorized user cancel operation may use it to evaluate the current step's ordinary `on_turn_complete` actions; internal and system-owned cancellation paths remain non-completing.

The persisted/API default is `false` for compatibility. The embedded `simple` Kanban template sets it to `true` on its action-bearing `Backlog` and `In Progress` steps for newly instantiated workflows, without backfilling existing workflow rows.

Configured user cancellation is a human completion decision, so it bypasses the agent-owned `auto_advance_requires_signal` gate while retaining the pending-clarification barrier. The workflow engine continues to evaluate and record the transition as `on_turn_complete`; no parallel cancellation action language is introduced.

## Consequences

- Users can choose pause-in-place or complete-and-advance semantics per step.
- Standard Kanban cancellation aligns workflow position with its existing review-ready runtime state for new workflows.
- Existing workflows do not change behavior during upgrade.
- Reusing the completion pipeline preserves `on_exit`, first-transition-wins, terminal handling, and destination `on_enter`; consequently a configured cancel may auto-start the destination step.
- Runtime and workflow code must carry an explicit cancellation source rather than infer intent from terminal session states or agent stop reasons.
- Transition history does not distinguish a natural completion from configured user cancellation; the cancellation status message remains the user-visible evidence.

## Alternatives Considered

- **Run completion for every cancelled/stopped turn.** Rejected because clarification pauses, failures, parent stops, and teardown are not user declarations that a step is complete.
- **Add `on_turn_cancelled` actions.** Rejected for this iteration because the requested behavior is to reuse the existing completion contract; a second action language would duplicate configuration and complicate templates, import/export, and the editor.
- **Honor `auto_advance_requires_signal` during configured user cancellation.** Rejected because the user explicitly requested cancellation completion and an interrupted agent normally cannot emit the required signal.
- **Enable the database default or backfill every `simple` workflow.** Rejected because it would silently change customized and existing workflows. The product default belongs in the embedded template used for new instances.
