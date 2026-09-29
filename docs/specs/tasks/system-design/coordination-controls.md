---
status: draft
system: tasks
requirements:
  - REQ-TASKS-COMPLETION-003
  - REQ-TASKS-COMPLETION-001
  - REQ-TASKS-COMPLETION-004
created: 2026-09-25
updated: 2026-09-28
owners:
  - kandev
---

# Task coordination controls system design

## Requirement mapping

| Requirement                  | Design section                          |
| ---------------------------- | --------------------------------------- |
| `REQ-TASKS-COMPLETION-003` | [Management claims](#management-claims) |
| `REQ-TASKS-COMPLETION-001` | [Completion gates](#completion-gates)   |
| `REQ-TASKS-COMPLETION-004` | [UI and contracts](#ui-and-contracts) |

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

Human commands remain permitted under normal workspace access. Authorized APIs
expose the manager and support explicit release/transfer with a reason. Mutations
record when they supersede managed intent. Disable or uninstall retains the claim
until human release, transfer, or task deletion. A reinstalled plugin has a
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

A blocked result includes criterion IDs and current versions. Authorized APIs expose
criteria, provenance, stale evidence, and blockers. A human may override
with an explicit reason bound to the current criteria revision and intended move.
An override is one action, not a persistent bypass. Reopening retains history and
rechecks criteria on the next completion. Tasks already complete before gates are
introduced remain complete; criteria edits do not silently reopen them.

## UI and contracts

The September 28 amendment removes the native task-detail controls from PR #3994.
The task system owns this amendment because it owns task details and the claims
and gates behind those controls. Plugin presentation remains a plugin concern.
The host retains enforcement, authorization, and public contracts.

Remove `TaskCoordinationControls` from `task-page-inner.tsx`. Remove its private
`task-management-claim-*`, `task-completion-gate-*`, and
`use-task-completion-gate-actions.ts` component tree when no other consumer exists.
These components are not exported through `host.ui`. Preserve the API clients,
SDK methods, status projection, workflow completion field, and domain services.
Do not add a replacement task menu entry or conditional empty-state hiding.

Restore the phone layout contract in `task-layout.tsx` and
`mobile/session-mobile-layout.tsx`. Before #3994, the panel reserved `3.5rem`
for the fixed header when no shared task error existed. A shared task error
already supplied that spacing, so its panel offset was zero. Use that behavior
as the reference, while preserving later changes. Remove the toolbar's `mt-14`
spacing and the assumption that it owns the header offset. Keep the current
safe-area calculation and `3.25rem` bottom navigation offset.

The nearest phone exemplar is the existing `SessionMobileLayout`: fixed task
header and bottom navigation, one active panel, and an internal scroll owner.
No new drawer or route replaces the removed toolbar. Chat remains the primary
interaction; error feedback and composer controls remain reachable by touch.
Verify normal, blocked-move, and shared-error states, including reload and resize.

Keep existing workflow error feedback. Detailed blocker reads and human recovery
remain available through the authorized claim, gate, and workflow-move APIs.
This package does not add a plugin UI or expose private HTTP calls as a plugin API.
It removes the native recovery dialogs, so browser users lose those shortcuts.
The removal must not weaken human identity checks or make overrides plugin-callable.

Replace browser tests that use the deleted controls with task-layout regressions.
Keep claim conflict, unavailable-owner release, stale evidence, and one-move
override coverage in handler/service tests. Add focused HTTP coverage for any
recovery behavior that only the removed browser tests previously exercised.
Check both phone geometry and actual chat/navigation interaction.

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

- [Requirements](../requirements/task-completion.md)
- [Implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
- [Native control removal plan](../../../plans/remove-native-coordination-ui/plan.md)
- [Coordination platform decision](../../../decisions/2026-09-25-plugin-coordination-platform.md)
