---
status: draft
system: tasks
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-004
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-005
created: 2026-10-05
updated: 2026-10-05
owners:
  - kandev
---

# Managed clone relocation experience system design

## Purpose and boundaries

Extend [managed clone relocation](managed-clone-relocation.md) within the task
system. The environment owns artifact identity and progress. The workspace
system still owns source clones and credentials. The worktree manager owns
filesystem proof. Task services own durable projections and notifications.

This design changes storage and presentation. It retains exclusive claims,
exact commits, ignored files, modes, symlinks, and all-slot admission. It does
not change generic metadata recovery, introduce a job queue, or expire claims.
The [proposed ADR](../../../decisions/2026-10-05-managed-clone-recovery-operation-storage.md)
records the storage and liveness choices.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-MANAGED-CLONE-RELOCATION-001 | Authority, publication, compatibility |
| REQ-TASKS-MANAGED-CLONE-RELOCATION-002 | Artifact layout, legacy readers, failure |
| REQ-TASKS-MANAGED-CLONE-RELOCATION-003 | Shared recovery presentation |
| REQ-TASKS-MANAGED-CLONE-RELOCATION-004 | Artifact registry and file presentation |
| REQ-TASKS-MANAGED-CLONE-RELOCATION-005 | Operation projection, hydration, liveness |

## Current implementation and failure mechanism

`managed_clone_relocation.go` writes adjacent relocation journals. Dirty transfer
uses `recovery.go` to create adjacent snapshots and recovery journals.
`managed_clone_relocation_archive.go` already retains originals under the private
`TasksBasePath/.kandev-recovery` namespace. Active replacements remain sibling
worktrees with physical relocation suffixes. Files renders their physical names.

`session-recovery-pending.ts` owns a browser-local map. Recovery hooks use it
while awaiting the synchronous WebSocket request. Reload loses the map, and
WebSocket closure rejects pending requests. Neither event proves that the
server stopped. Persisted error stamps describe the refusal, not live progress.

The copier deliberately preserves ignored trees and verifies manifests. It can
spend minutes in one phase. Progress must not require another file-tree scan.

## Artifact layout and registry

For new managed clone relocations, reuse the existing private operation bucket:

```text
<TasksBasePath>/.kandev-recovery/<slot-operation-digest>/
  <original-basename>/             retained original, existing archive convention
  snapshots/<snapshot-identity>/   preserved snapshot and optional retry snapshot
  records/recovery.json
  records/relocation.json
  claims/recovery.claim
  claims/relocation.claim
```

Use the existing digest inputs: task, environment, original worktree, operation.
Validate each identifier before deriving paths. Keep the existing retained
original location compatible. Replacement paths remain canonical sibling
worktrees; their physical names do not change for presentation.

Add `task_environment_recovery_artifacts` through the idempotent schema path in
`task/repository/sqlite/base_schema.go`. Key each row by environment, operation,
and original slot identity. Store the owner generation, captured inventory
identity, layout version, exact artifact locations, and provenance. Preserve
rows for retained operations when the latest progress row changes. Do not make
them temporary artifacts or ordinary workspace cleanup candidates.

Use a narrow injected artifact-registry capability beside recovery admission.
The worktree manager must not import the task service. Registry creation and
updates require the existing exact claim and ownership checks. Persist intended
locations before creating artifacts, and record additions before a mode retry.
An absent registry capability refuses the new layout before any transfer.

Give new relocation and recovery journals `layout_version: 2`. Look up the
record through the authenticated registry before probing a legacy sibling.
Validate the registry's slot, operation, generation, and allowed namespace.
Pin directory identities and reject symlink substitution. Snapshot validation
accepts the new layout only with this proof. Do not loosen the generic sibling
snapshot validator or accept an arbitrary journal-supplied path.

Keep private directories owner-accessible and protect their metadata like the
retained checkouts. Apply existing content-mode preservation to copies inside
them. Do not hardlink, skip ignored files, or replace full manifest verification.

## Legacy compatibility

Versionless adjacent journals remain layout version 1. Continue their operation
at its existing paths and lock files. This includes blocked snapshots, retained
failed copies, and `mode_retry` provenance. Never move an acquired lock or infer
permission-only retry authority from a path suffix.

An existing authoritative admission can register verified legacy artifact paths
under its claim. A completed legacy operation can be registered during the
existing authenticated reconciliation of its published replacement. Require
the current slot, published replacement, retained operation identity, and record
proof to agree. Use the existing reconciliation checks; filenames alone are
insufficient. Registration is metadata-only and retains every filesystem path.

Do not scan every task on startup. Read-only Files and progress requests must
not perform this reconciliation. Until ownership is registered, legacy artifacts
remain visible. Corrupt or ambiguous journals remain visible and recovery refuses
them through its existing rules. This is a deliberate compatibility limit.

Crashes before registry publication leave legacy readers usable. A registry row
that points to incomplete or substituted artifacts cannot authorize adoption.
Retain old records after successful registration; do not perform automatic cleanup.

## Durable latest-operation projection

Add `task_environment_recovery_operations`, with one latest row per environment.
Keep it separate from `task_environment_recovery_claims`. Releasing a claim must
not erase the latest result. Use shared database conventions for SQLite and
PostgreSQL, including task-before-environment row locking.

Persist these fields:

| Field | Meaning |
| --- | --- |
| environment, owner task, ownership generation | Current authority binding |
| initiating session, operation ID, error stamp | Accepted request identity |
| attempt ID | Fences successive authorized continuations of the same operation |
| kind | `managed_clone_relocation` |
| revision | Monotonic environment projection revision |
| runner instance | Internal backend boot identity, excluded from public DTOs |
| state | `running`, `completed`, `failed`, or `interrupted` |
| phase | Closed set defined below |
| repository ID, position, total, completed slots | Selected inventory progress |
| workspace complete, agent ready | Distinct completion outcomes |
| started, updated, ended timestamps | Progress timing, not a liveness proof |
| reason code | Bounded sanitized failure category |

Capture the complete selected inventory once, after authority is established
and before the first filesystem mutation inside admission. Process it in
the existing stable order. Repository position counts selected slots; unchanged
verified slots can complete without copying. Never add a newly discovered slot
to an operation. Keep snapshot manifests and detailed paths in internal journals.

Write progress under the exact operation, attempt, and generation fence. Before each
filesystem phase, require its durable phase write to succeed. If persistence
fails, stop at the next safe boundary and retain the original and snapshots.
Filesystem journal and inventory publication remain authoritative after a crash.
On restart, recovery revalidates them rather than trusting a progress phase.

Increment revision across operation replacement and terminal updates. Expose
revision and generation as decimal strings to avoid JavaScript precision loss.
Status is not authorization, and a progress row never substitutes for a claim.

## Phases and completion

Use these public phases: `checking`, `snapshotting`, `verifying_snapshot`,
`restoring`, `verifying_replacement`, `publishing`, and `resuming`.
Separate snapshot copying from verification around the existing manager calls.
Emit phase transitions and slot completions. While a phase remains active,
write a heartbeat at most once per 15 seconds while the runner is executing.
The heartbeat time means runner activity, not bytes copied or an ETA.

A slot completes only after its replacement validates and publication succeeds.
Earlier published slots remain valid if a later slot fails. Keep `workspace_complete`
false until the entire selected inventory passes final admission. Then set it
true and enter `resuming`. Set `agent_ready` only after the existing resume path
reports readiness. Only that result completes the whole request.

If relaunch fails after migration, retain `workspace_complete: true` and report
the actual bootstrap error. Existing ordinary resume rules apply to the now-valid
inventory. Do not repeat copying or offer relocation for a repaired mismatch.
Clear only the matching relocation error through the existing fenced error path.
Do not fabricate a lifecycle transition or discard a provider resume token.

## Runner liveness, disconnect, and interruption

Keep the existing server execution path for `relocate_and_resume`. After the
authorized request is admitted, bind its runner to backend lifecycle ownership.
A browser disconnect stops waiting for the response, not the accepted transfer.
Use the existing operation deadlines and backend shutdown cancellation; do not
detach work from shutdown or create an unbounded background worker.

Maintain a scoped runner registry for accepted operations. A live operation
requires the current runner instance and exact registered operation/attempt binding.
A durable claim, heartbeat age, or CANCELLED session is insufficient proof.
When a runner exits, persist its terminal result before releasing its projection
binding. Claim release retains its existing fences and ordering.

On backend startup, reconcile only nonterminal database operation rows from an
older instance. Mark them interrupted with a conditional revision update. Do
not inspect or mutate checkouts, remove claims, or restart recovery. A request
that died before creating its progress row remains governed by existing recovery
errors and journals. This adds no install-wide filesystem scan.

Interrupted UI says the operation stopped and files remain preserved. Reuse
`relocate_and_resume` only when current stamped admission permits continuation
of that same operation. If proof or claim ownership is uncertain, show the
existing manual repair guidance. Do not promise a retry for every interruption.

Admission checks the live operation before any competing repair or workspace
reconstruction. A duplicate authorized request returns an `in_progress` outcome
with the current projection. It never queues another transfer. Background Files,
Changes, and commit requests receive the existing busy classification promptly;
they cannot overwrite the recovery error or wait inside its lifecycle flight.

## Hydration and wire contract

Add nullable `workspace_recovery` to the rich `TaskSessionDTO` and corresponding
web session type. Expose it for sessions bound to the selected environment;
identify the initiating session in the projection. Include it in boot state,
task session detail/list reads, and reconnect hydration. Keep unrelated lightweight
summaries unchanged unless they need this projection for a recovery surface.

Add `session.workspace_recovery.changed` through the existing task event bus
and session broadcaster. Publish only after persistence. Notify every subscribed
session in the same authorized environment, including sibling conversations.
The payload contains the same projection as hydration, with task/session binding
and revision. It contains no checkout paths, credentials, manifests, or file data.

Provide `session.workspace_recovery.get`, a read-only task-service status query.
It authorizes the session/environment and reads the row plus runner binding.
It must not call `GetOrEnsureExecution`, Git, or worktree inspection. Use it after
a lost action response or unresolved reconnect. No timer may replay the repair.

Omitted fields from an older server preserve legacy compatibility. A versioned
null clears a current projection; an unversioned missing field cannot clear it.
The client store compares revisions, operation identity, environment binding,
and hydration epochs. Discard stale notifications and action responses. Keep a
bounded orphan projection for notification-before-hydration, using existing
session projection patterns. Unknown phases render a generic localized busy
state and keep competing actions disabled when liveness is verified.

## Shared recovery presentation

Extend the existing shared recovery model and action hook. The local pending
map still prevents double taps before acknowledgment. Durable status owns the
state after admission. A lost connection enters a localized status-checking
state until a read-only status response resolves it. Do not briefly re-enable
repair actions while hydration is incomplete.

Reuse `SessionRecoveryCard`, stopped/bootstrap recovery surfaces, and message
actions. Only the current recovery surface presents progress. Historical errors
remain in history without duplicating active controls. Show repository position,
localized phase, last update, and retained-copy guidance. No percentage, ETA,
new cancel button, or unconditional retry appears.

Desktop uses the existing inline card and dialog confirmation. Phone uses its
focused full-width card and inset confirmation drawer. Reuse existing tokens,
one scroll owner, 44-pixel targets, keyboard focus return, and safe-area spacing.
Announce phase changes through a polite status region; do not announce heartbeat
updates. Preserve expansion state across progress changes within an operation.

Localize new copy in all seven supported locales. Generate the Traditional
Chinese pair through the existing conversion command. Use existing plural
handling for repository counts and localized phase/reason keys.

## Files and workspace search

Produce an exact exclusion list from the authenticated artifact registry.
Scope it to the selected task root and environment. Exclude only registered
artifact roots with valid path and directory identity. An absent or invalid
entry remains visible. Do not filter by `.kandev-*` patterns, repository suffixes,
or empty-directory heuristics. Keep `.agents`, `.codex`, and root user files
visible under ordinary file browser rules.

Apply the same exclusion contract to tree enumeration, filename search, and
content search before traversal. Pass trusted exclusions through runtime setup
and the existing workspace tracker contract; clients cannot supply them. Private
new artifacts are outside the workspace and require no tree filter. This is a
presentation filter, not a new file-access or deletion boundary.

Derive repository display names from `session.worktrees` and repository metadata.
Match exact authoritative paths before replacing a rendered label. Keep
`FileTreeNode.path`, editor keys, chat attachments, search results, and Git paths
unchanged. Disambiguate duplicate labels with existing repository identity.
Single-repository headers follow their canonical replacement path as today.

After inventory publication, update worktree projections and invalidate tree,
filename-search, and content-search caches using environment/inventory revision.
Reject responses captured against an earlier revision. Apply refreshed exclusions
when legacy ownership is registered, without restarting a live agent.

## Verification and observability

Use real Git fixtures for artifact placement, restart journal discovery, ignored
content, modes, symlinks, partial publication, and permission-only retries.
Test SQLite migration idempotence and conditional PostgreSQL concurrency.
Verify claim release leaves progress available and older writers cannot win.

Use existing real-Git multi-repository Playwright helpers. Hold a small fixture
at an actual recovery phase with an isolated test-only barrier. Test reload,
disconnect, a second browser, partial failure, and completion on desktop and
phone. Do not create huge dependency trees or rely on timing sleeps.

Structured logs record operation, phase, repository position, and outcome under
existing diagnostic policy. Public errors remain path-free. Avoid new metrics
with task, repository, operation, session, or environment identifiers as labels.
No new metrics request is part of this package.

## Related contracts and implementation

- [Draft requirements](../requirements/managed-clone-relocation-experience.md)
- [Existing preservation contract](../requirements/managed-clone-relocation.md)
- [Metadata recovery](worktree-metadata-recovery.md)
- [Permission-only retry](../../../decisions/2026-10-04-permission-only-snapshot-retry.md)
- [Fix package and work orders](../../../plans/managed-clone-recovery-experience/plan.md)
