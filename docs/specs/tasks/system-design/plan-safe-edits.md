---
status: current
system: tasks
created: 2026-09-16
requirements:
  - REQ-TASKS-PLAN-SAFE-001
  - REQ-TASKS-PLAN-SAFE-002
  - REQ-TASKS-PLAN-SAFE-003
  - REQ-TASKS-PLAN-SAFE-004
  - REQ-TASKS-PLAN-SAFE-005
owners:
  - kandev
---

# Safe agent plan edits System Design

## Purpose and evidence

The September 16 incident replaced 16,332 characters with an 807-character checklist.
Raw ACP and backend records both contained an explicit `mode="replace"` request.
The server saved the request, preserved revision 3, and instructed the agent to stop.
Compaction preceded the request. The evidence does not establish that compaction caused it.

This design changes agent admission and recovery within the tasks system.
It reuses `PlanService`, `planLockTable`, `WritePlanRevision`, and the existing history tables.
It adds no browser controls and no automatic session continuation mechanism.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PLAN-SAFE-001` | Admission and history preservation |
| `REQ-TASKS-PLAN-SAFE-002` | Edit versions |
| `REQ-TASKS-PLAN-SAFE-003` | Exact edit |
| `REQ-TASKS-PLAN-SAFE-004` | Revision recovery |
| `REQ-TASKS-PLAN-SAFE-005` | Agent results and continuation |

## Components

- `internal/task/service` owns authorization, state reads, version comparison, composition, truncation policy, and recovery eligibility.
- `internal/task/repository/sqlite` owns durable versions and atomic HEAD/history writes for SQLite and PostgreSQL.
- `internal/mcp/handlers` selects the agent policy and maps typed outcomes through `internal/task/planws`.
- `internal/mcp/server` owns tool schemas, forwarding, capability exposure, and compact agent results.
- `pkg/websocket` owns added action names. Existing browser actions keep their payloads and interaction behavior.

The service and repository methods below are implemented. Existing method names
identify the integration points that remain shared with browser plan behavior.

## Edit versions

Add `task_plans.write_version TEXT NOT NULL DEFAULT ''` through an idempotent migration.
Backfill empty values with unique UUIDs through a portable Go migration loop.
The migration runs before reads become available and does not change content, history, or timestamps.
Add the field to `models.TaskPlan` and repository scans. Expose it in an MCP-specific response wrapper as `version`.
The browser DTO does not need a new public field.

Generate a fresh UUID for each successful content/title write in the repository transaction.
Cover `CreateTaskPlan`, `UpdateTaskPlan`, and `WritePlanRevision`/`upsertPlanHead`, including browser and revert callers.
Generate versions for identical-content writes too. Rollback must roll back the version with HEAD and history.
Do not derive versions from timestamps, revision numbers, or content hashes.
Coalescing mutates an existing revision, and content can return to a previous value.
Deletion followed by recreation receives a fresh UUID. Marker and comment operations preserve the existing version.

Add a coherent service read for agent plan snapshots.
Read HEAD and its version together under the existing per-task lock, after authorization.
Return exact content, title, task ID, version, and current revision metadata when verified against HEAD.
Do not label a mismatched latest revision as the current snapshot.

Version comparison and the write occur within the same existing service critical section.
The supported concurrency boundary remains one backend process with its shared `PlanService` instance.
This package does not introduce cross-process plan writers or a second lock table.
Repository PostgreSQL transactions retain the task locking used by `WritePlanRevision`.

## Admission and history preservation

Extend agent create/update requests with optional schema fields `expected_version` and `allow_truncation` (default false).
They remain schema-optional because first creation and ordinary append need no version.
The service requires a nonempty, matching version for replacement of an existing plan.
The create tool remains an upsert only when that precondition passes. This closes the alternative overwrite path.
If no plan exists, creation without a token succeeds. A supplied stale token must not create a new plan.

The MCP adapter selects a trusted agent policy. A caller cannot disable it through a payload field.
Browser writes retain their existing admission policy but always advance the durable version.
Agent admission fails closed on an unreadable HEAD. Browser read-failure behavior remains under its current contract.

Validation order for new agent writes:

1. Validate argument types and mode names without accessing task state.
2. Authorize the addressed task.
3. Validate required content and standalone replacement size under existing rules.
4. Acquire the task lock and read HEAD through the existing tri-state read.
5. Reject missing state or read failure, as applicable to the requested operation.
6. Enforce the version precondition.
7. Compose append or exact-edit content and enforce its final byte size.
8. Evaluate suspicious reduction and explicit acknowledgement.
9. Verify preservation, commit HEAD/history/version, release, and publish existing events.

Reuse `planTruncationDetected`: prior length at least 2,000 Unicode code points and retained length strictly below 50 percent.
Never detect truncation from an append fragment alone.
An unacknowledged reduction returns `plan_truncation_rejected` before any mutation.
An acknowledged reduction requires a latest revision whose title/content match HEAD.
An unreadable or divergent history returns `plan_history_unavailable` without mutation.
Force a new revision for an acknowledged reduction. Do not coalesce away the preserved predecessor.
This choice avoids a speculative repair mechanism for historically divergent HEAD/history pairs.

Every rejection preserves title, content, version, revision timestamps/counts, comments, and implementation markers.
It emits no plan or revision mutation event. A later valid write must still acquire the lock and succeed.

## Exact edit

Add `edit_task_plan_kandev(task_id?, expected_version, old_text, new_text, allow_truncation?)`.
Use a dedicated tool instead of extending append-mode parsing or introducing Markdown section identities.
`old_text` must be nonempty. `new_text` is required but can be empty.
Both values represent exact UTF-8 text. There is no whitespace normalization.

Within the task lock, count all literal occurrences, including overlapping occurrences.
Zero occurrences return `plan_edit_not_found`. More than one returns `plan_edit_ambiguous`.
Replace the unique byte span and preserve all surrounding bytes and the title.
Use the same version, final-size, truncation, revision, attribution, and event path as an agent update.
Reject a final empty plan under the existing agent empty-content rule.
An identical fragment replacement is an ordinary successful write with a new version.

The tool describes checklist updates as its primary example.
New sections use `update_task_plan_kandev(mode="append")`.
Append retains its separator and non-idempotence rules. It checks `expected_version` only when supplied.
Reject `allow_truncation=true` on append as inapplicable instead of suggesting that append can remove content.

## Revision recovery

Add these task-scoped tools:

| Tool | Arguments beyond optional `task_id` | Result |
| --- | --- | --- |
| `list_task_plan_revisions_kandev` | `before_revision_number?`, `limit?` | Metadata page, no content, next cursor |
| `get_task_plan_revision_kandev` | `revision_id` | Exact title/content and `revision_version` |
| `restore_task_plan_revision_kandev` | `revision_id`, `expected_revision_version`, `expected_version` | Compact write acknowledgement |

List defaults to 20 entries and permits 1 through 100.
Use descending revision numbers and an exclusive positive integer cursor.
Add a bounded repository metadata query. Do not load all content with `ListRevisions` and discard it afterward.
Metadata includes ID, number, author, timestamps, revert source, and byte size.
It does not claim immutable snapshot identity. The caller fetches the selected revision before restore.

`revision_version` is an opaque SHA-256 digest of canonical length-prefixed fields:
revision ID, task ID, title, content, and canonical `updated_at`.
It detects changes to the selected snapshot during coalescing. It is not an authorization credential.
Fetch selected revisions with a task-scoped query after task authorization.
An ID from another task returns not found without revealing that other task.

Use the same atomic `WritePlanRevision` and event sequence as the existing revert
implementation. Keep agent preflight separate because it holds the non-reentrant task
lock; do not call `RevertPlan` from another method that already holds that lock.
The agent branch requires existing, readable HEAD, matching `expected_version`, and matching source `revision_version`.
It also verifies that the latest revision preserves current HEAD before restoring.
Both source and destination checks occur under the task lock before the write.
Copy the source title/content exactly. Write a new revision with `revert_of_revision_id` and agent attribution from trusted session context.
Preserve current implementation markers and comments. The browser branch retains user attribution and its existing absent-HEAD behavior.
Restoration retains the existing oversized-history exemption and needs no truncation override.
An already-current target can return `already_current` with no mutation when title/content and both expected versions match.

## Agent results and continuation

Use MCP error results for rejected writes. Preserve structured reason data through the WebSocket bridge.
Render a concise text equivalent for agents that consume only text blocks.
Implemented fields include `code`, `operation`, `task_id`, `write_applied=false`,
`next_action`, and authorized current-version/count metadata.
Never include plan content in an error or a successful write acknowledgement.

| Reason | Correction guidance |
| --- | --- |
| `plan_version_required` | Read the current plan, then submit the intended edit with its version. |
| `plan_version_conflict` | Read and reconcile current content. Do not retry the old body with a new token. |
| `plan_truncation_rejected` | Plan unchanged. Use exact edit or append. Acknowledge only a deliberate reduction within the user's request. |
| `plan_edit_not_found` / `plan_edit_ambiguous` | Read current content and select a unique exact fragment. |
| `plan_revision_changed` | Fetch the selected revision again before deciding whether to restore it. |
| `plan_history_unavailable` / read failure | Do not invent recovery data or recreate the plan. Report the unresolved storage condition if it persists. |

`get_task_plan_kandev` must stop discarding metadata in `getTaskPlanHandler`.
Return a metadata text block plus the unchanged content text block, with equivalent structured fields where supported.
`planWriteAck` returns task ID, title, stored byte length, and the committed version.
The returned version comes from the committed write, including the post-write read-failure fallback.
Schema and forwarding tests must prove that each new field reaches its service consumer.

Guidance permits correction within the same turn. No server response requests a task stop for a rejected write that changed nothing.
The server does not automatically relaunch, prompt, or complete the session.
LLM continuation is guidance, not a deterministic product guarantee.

Automatic restore guidance applies only to an agent's identified accidental write.
The agent needs the successful write's returned version and an exact, identified recovery source.
If current version differs, it must reconcile instead of replacing intervening work.
Legacy incidents without that evidence require review of current state and explicit restoration intent.
A list order or the label “previous revision” alone is not enough to select a recovery target.

A transport error does not establish whether a write committed.
After a lost response, the agent reads current state before deciding its next action.
An old expected token rejects a repeated edit/restore after success. Append remains non-idempotent without a supplied version.
No retry loop automatically refreshes a version and resends an old body.

## Security and exposure

Register each tool wherever `registerPlanTools` currently provides plan writes, including applicable Office task surfaces.
Preserve external, configuration, and automation exclusions according to the current profile contracts.
Register backend actions with the guarded MCP dispatcher and existing task-reach checks.
Tool-supplied task IDs never replace trusted session identity for authorization or attribution.
The selected revision must belong to the addressed task, even when the caller can access both tasks.

## Migration and observability

Use replayable migrations for both database dialects and update the required-store upgrade fixture when schema history changes.
The token backfill does not rewrite documents or create revisions.
Old clients lacking tokens receive a corrective rejection for replacements of existing plans.
There is no compatibility bypass. Existing initial-create and tokenless append calls continue to work.
The existing runtime registers these schemas on agent startup. Cached clients can refresh discovery or resume with current tools.

Use bounded structured logs for rejected operations: task/session correlation, operation, reason, and character counts.
Do not add plan content, replacement fragments, or revision bodies to logs.
An error result distinguishes rejection from storage failure and uncertain transport completion.
This package adds no metrics endpoint or feature toggle.

## Contract transition

The following changes apply only when this package is implemented:

- Consistency `001.1` through `001.5` and `001.7` become pre-write rejection or acknowledged-write preservation on agent paths.
- Consistency `001.6` and `001.9` no longer permit an unreadable HEAD on agent replacement. Browser behavior remains unchanged.
- Consistency `003.4`, `004.1`, `004.2`, `005.1`, and `005.2` gain the agent preconditions and MCP metadata specified here.
- Append `005.1` no longer freezes agent replacement admission. Append `006.2` and `006.8` use preventive guidance and available revision tools.
- Append `006.9` retains its append guidance but describes create-overwrite preconditions. New exact-edit tooling does not change append parsing.
- Append `001.7` keeps existing mode/authorization precedence, with new optional version checks at the defined state-check stage.
- Size-limit `001.8` remains subject to the new agent preconditions. Historical restore exemptions remain unchanged.

The implementation must update affected clauses and tests, rather than keeping two active contradictory policies.
Existing companion designs remain the source for browser behavior, append composition, size limits, and serialization.
No linked companion implementation package was found in those plan-write specifications at planning time.

## Verification strategy

Use repository migration and rollback tests, deterministic service concurrency tests, and real MCP-to-service integration tests.
The incident regression seeds a long plan, appends a checklist, attempts fragment replacement, then corrects one checkbox through exact edit.
Assert unchanged content/history after rejection, preserved review sections after correction, and no forced workflow transition.
Recovery tests cover exact restoration, mutable revision targets, intervening browser edits, authorization, and unchanged markers/comments.
The work orders define the exact commands and test locations. The implementation
and verification are recorded in the completed work orders.

## Related decisions and implementation

- [Conditional agent plan writes](../../../decisions/2026-09-16-conditional-agent-plan-writes.md)
- [Implementation-start marker](../../../decisions/0033-durable-plan-implementation-start.md)
- [Implementation package](../../../plans/plan-safe-edits/plan.md)
