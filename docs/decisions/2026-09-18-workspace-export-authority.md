# ADR-2026-09-18-workspace-export-authority: Bind linked context to explicit receiving accounts

**Status:** accepted
**Date:** 2026-09-18
**Area:** backend, frontend, protocol

## Context

A central conversation can coordinate several workspaces, but workspace access
does not authorize copying every transcript or switching the receiving account.
Revocation cannot remove text already delivered to a provider. Replaying the
central history after a profile change is itself a possible context export.

## Decision

Keep the assistant broker's signed credential bound to its home conversation,
run and session. Each supported linked operation supplies an explicit workspace
and grant revision. Resolve a request-local target after authenticating the
original credential; never mutate its claims or widen workspace-coordinator JWTs.
Unsupported linked operations fail closed.

Human grants bind owner, assistant selection version, target workspace, operation
and export allowlists, receiving profile revision and native authority revision.
Every read, write, wake and native dispatch checks current native access and
revisions. Recheck after native reads before exporting their results. Native
mutation adapters check again immediately before their effects. Operation hashes
include the target and grant revision; uncertain effects retain their receipts.

Expose explicit bounded directory, task-summary, worker-result, input and handoff
projections. Do not return foreign task descriptions, workflow prompts, repository
paths, credential configuration or unrelated transcripts. Handoffs retain the
target worker profile, owner instruction, objective and explicitly user-wide
preferences; home-workspace memory and credential descriptors do not migrate.

Store metadata-only possible-delivery receipts by conversation, workspace,
receiving profile/configuration and field kind. Fold repeat observations without
discarding the original boundary. A new receiver requires explicit confirmation
covering every historical field before history can be sent to it. Workspace name
and grant-scope discovery also count as delivery. Receipts are conservative when
the transport outcome is unknown.

Revocation prevents subsequent operations and invalidates queued grant revisions.
Explicit forgetting removes locally stored handoff packets and advances the grant
revision, preserving revoked state. It does not delete native tasks, conversation
history, provider context or audit receipts. Those retained receipts prevent
forgetting from becoming a way to bypass account reconfirmation.

## Consequences

Grants and delivery records use the existing required Orchestration SQL owner on
both database engines. Human controls display the receiving profile, exact scope,
revocation state and historical limits. A revoked managed worker may need a human
to act directly in its native task; revocation does not undo completed effects.
Reconfirming a grant requires new context references for previously queued work.

## Alternatives Considered

- Widening the old coordinator token would silently change its isolation contract.
- Trusting cached discovery would leave reads and queued writes valid after revoke.
- Deleting local caches alone would not prevent history from reaching a new account.
- Exporting native objects or complete transcripts would exceed explicit field scope.

Implements [REQ-ORCHESTRATION-ASSISTANT-009](../specs/orchestration/requirements/personal-assistant.md)
under the [assistant design](../specs/orchestration/system-design/personal-assistant.md).
