---
status: draft
system: tasks
requirements:
  - REQ-TASKS-COORDINATION-001
  - REQ-TASKS-COORDINATION-002
created: 2026-09-25
owners:
  - kandev
---

# Task coordination controls system design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-COORDINATION-001` | [Management claims](#management-claims) |
| `REQ-TASKS-COORDINATION-002` | [Completion gates](#completion-gates) |

## Purpose and boundaries

Tasks own optional management claims and completion gates because these invariants
must hold for native UI, MCP, automation, workflow, and plugin callers. Plugin policy
chooses whether to adopt work and what evidence to collect. The task service stores
and enforces the resulting state. No Coordinator table or role enters this system.
The [existing completion requirements](../requirements/task-completion.md) still
define completing steps and follow-up conversation behavior. This draft adds a gate
before completion; tasks without criteria keep their current behavior.

## Management claims

Persist `task_management_claim` with task/workspace, installation, opaque instance
key, claim generation, acquired timestamp, resource version, and audit actor.
A unique task constraint enforces one managing owner. Watching is plugin state;
worker assignee is unchanged. Claims never time out into automatic ownership.

Shared commands `AcquireTaskManagementClaim`, `ReleaseTaskManagementClaim`, and
`TransferTaskManagementClaim` compare both task and claim versions. Public exact
Host wrappers add capability checks, receipts, and execution provenance. A transfer
increments the fencing generation. A management mutation checks that generation
inside the same domain transaction. An absent claim allows an otherwise authorized
command; an existing claim denies competing plugin writers. This is an opt-in
ownership rule, not a new global task ACL or parent-child permission shortcut.

Human commands remain permitted under normal workspace access. The UI shows the
manager and offers explicit release/transfer with a reason; mutations record when
they supersede managed intent. Disable or uninstall leaves the claim visible and
inert until human release, transfer, or task deletion. A reinstalled plugin has a
new installation identity and cannot inherit it. Linked descendants have separate
claims. No automatic recursive adoption.

## Completion gates

Persist task completion criteria as an optional revisioned set. A criterion has
stable ID, description, verification status, evidence references, verifier actor,
observed subject versions, timestamp, and verification revision. Evidence subjects
are typed task revisions, execution IDs, artifact revisions, or repository/PR head
revisions; free-text evidence remains an attributed assertion, not independent proof.

Shared commands set criteria, submit evidence, verify an observed criterion, and
record human override. Setting or changing criteria increments the set revision
and invalidates affected verification. An ordinary plugin mutation cannot remove,
clear, or weaken unmet criteria to evade the gate. Such a change requires a native
human confirmation bound to the current set revision; it remains in the audit trail.
A required evidence subject change makes
its verification stale. Subject dependencies must be explicit; unrelated task
comments cannot invalidate every criterion. Gates evaluate stored current evidence,
never a live plugin RPC. Plugin unavailability therefore cannot deadlock a DB write.

The shared workflow transition service evaluates the gate immediately before a
transition enters a completing step, in the same guarded commit as the move.
Manual/bulk moves, agent step signals, automatic transitions, queued moves, and
automation calls all route through this guard. Admission may preview blockers,
but queued moves must recheck at application time. The guard precedes completion
events and cannot be bypassed through `UpdateTask` state fields. Preserve existing
pending-transition expiry and WIP checks. No new task state enum is required.

A blocked result includes criterion IDs and current versions. Task detail shows
criteria, provenance, stale evidence, and a completion blocker. A human may override
with an explicit reason bound to the current criteria revision and intended move.
An override is one action, not a persistent bypass. Reopening retains history and
rechecks criteria on the next completion. Tasks already complete before gates are
introduced remain complete; criteria edits do not silently reopen them.

## UI and contracts

Add a manager row and completion section to existing task detail, available on
desktop and phone. Use a bottom drawer for mobile claim transfer and evidence
inspection; use shared dialogs on desktop. Keep workflow controls visible with an
explanation when blocked. MCP and Host adapters return the same typed blocker data.

New proposed Host methods are `AcquireTaskManagementClaimExact`,
`ReleaseTaskManagementClaimExact`, `TransferTaskManagementClaimExact`,
`SetTaskCompletionCriteriaExact`, and `VerifyTaskCompletionCriterionExact`.
Human override and claim-transfer confirmations originate from native UI/MCP human
interaction services. Plugins cannot manufacture consent by passing a boolean.
Extend task read DTOs and canonical status projection with summary fields; paginate
detailed evidence separately to keep list payloads bounded.

## Persistence and recovery

Add task-owned SQL migrations for claims, criteria/evidence, and append-only history.
Foreign keys follow existing task deletion policy. Task deletion previews list
claims and criteria that will be removed. Receipt/operation IDs make retries
idempotent. On restart, rebuild status summaries from task-owned records; no plugin
must be running. Concurrent verify/edit and transfer/write tests prove fencing.

## Security and observability

Validate workspace scope, caller provenance, capability, claim generation, and
resource revision at the service boundary. Avoid storing credentials or executable
instructions in evidence. Audit owner transfers, verifier identity, evidence changes,
blocked completions, and overrides. Metrics use reason/status only, not task IDs.

## Related documents

- [Requirements](../requirements/coordination-controls.md)
- [Implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
- [Coordination platform decision](../../../decisions/2026-09-25-plugin-coordination-platform.md)
