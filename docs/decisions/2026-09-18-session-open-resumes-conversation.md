# ADR-2026-09-18-session-open-resumes-conversation: Opening resumes the selected conversation

**Status:** accepted
**Date:** 2026-09-18
**Area:** workflow

## Context

Workflow switches stop earlier sessions and preserve their conversations.
The previous inspection policy exposed a parked-session banner and prevented
open-time recovery. Users had to understand workflow parking before continuing
a conversation. The user explicitly rejected that interaction on 2026-09-18.

## Decision

Opening or selecting a workflow-stopped conversation uses normal automatic
session recovery. Current or historical workflow parking is not an eligibility
restriction. The interface has no parked-session banner, toolbar, or replacement
parking explanation.

Existing auto-start prevention, automatic capacity admission, terminal-session
rules, authorization, and archive rules still apply. Session opening resumes
provider context; it does not send a new prompt or replay an interrupted turn.

Conversation activation does not transfer workflow ownership. A selected older
session cannot become the primary workflow recipient, consume a destination's
queued prompt, or replace its deferred launch merely because the user opens it.
A queued destination remains scheduler-owned. A different conversation can
recover when capacity permits without altering that accepted launch.

Keep execution-stamped stop tombstones for delayed callbacks. Legacy
`workflow_parking` metadata can remain stored but does not control recovery or
presentation. Remove its policy-only readers and writers when they have no
remaining lifecycle purpose; no bulk live-data rewrite is required.

## Consequences

The user sees ordinary conversations and normal recovery behavior. Opening an
older conversation can consume automatic capacity. It never gets a manual
ceiling override. If a task already owns a deferred launch, a capacity refusal
must preserve that record rather than enqueue conflicting work.

This supersedes the parking-based suppression and parked-note portions of
[the previous inspection decision](2026-09-16-passive-session-inspection.md).
Its exact-recipient queue ownership, admission, and reconciliation rules remain.
The narrow historical-stop exception in the earlier repair plan is replaced by
this general policy. No primary-session or committed-route exception is needed
solely to disregard parking.

## Alternatives Considered

- Keep the banner and require explicit Resume: rejected by the user.
- Hide the banner but retain suppression: leaves the reported interaction defect.
- Resume only the current workflow destination: does not meet the user's request
  to resume even a parked conversation when opened.
- Transfer workflow ownership on open: conflates conversation access with workflow routing.
- Ignore capacity or delete stop tombstones: breaks independent runtime protections.

## Related records

- [Requirements](../specs/tasks/requirements/queued-session-ownership.md).
- [Design](../specs/tasks/system-design/queued-session-ownership.md).
- [Revised implementation plan](../plans/session-open-recovery-eligibility/plan.md).
