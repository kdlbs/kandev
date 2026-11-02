---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001
---

# GitHub PR watch retirement design

## Ownership and mapping

The GitHub service owns watch retirement. Task state and resource lifecycle remain unchanged.
ACs .1-.4 and .8 map to persistence and admission. ACs .5-.6 map to API controls.
AC .7 maps to result fencing and concurrency.

## Persistence

Add nullable `retired_at` to `github_pr_watches` and `PRWatch` in `internal/github`.
A null value means active. A timestamp means explicitly retired.
Keep retired rows in the existing canonical unique indexes. Do not create a second identity system.
Canonical lookup must inspect both searching and numbered identities, including retired rows.
Retirement reserves the exact task/repository/branch and, for numbered watches, task/repository/PR identity.

`Service.DeletePRWatch` authorizes the stored workspace, then marks the row retired.
Keep low-level hard deletion distinct for permanent task cleanup.
The retirement update is conditional and idempotent. Remove discovery-health consumers only after commit.
Provider health removal must not clear evidence still owned by another active watch.

Active list queries and cardinality exclude retired rows. Task/session watch lists used for polling also exclude them.
Identity and authorization lookups retain access to retired rows.
Archive cleanup deletes active watches only. It retains explicit retirement through unarchive.
Hard task deletion and existing orphan cleanup remove both states.

Use the store's additive, driver-aware migration path. Mirror the column in every watch-table rebuild and copy list.
Existing rows receive null. Migration replay must preserve non-null retirement timestamps.
Update required-store conformance expectations and the explicit-tag upgrade fixture when schema history changes.

## Creation and admission

Both `createPRWatch` and `ensurePRWatch` inspect retirement after effective task-owner resolution.
They return a typed `ErrPRWatchRetired` for a matching retired target.
Automatic callers treat this result as a normal skip. They must not dereference a missing watch or log repeated errors.
Explicit association paths may preserve their association operation, but cannot clear retirement or start watch polling.

Apply the same admission at polling and sync entry points. Cover single-watch REST, GraphQL batches, passive refresh, and explicit refresh.
Admission precedes credential resolution and provider calls where possible.
A shared batch contains only eligible members. A retired member neither accelerates its cadence nor receives its result.
The remaining active members still permit the provider request.

`buildTaskBranchList` can nominate a retired branch because active-watch inventory omits it.
The service gate is authoritative. A bulk retirement-key filter can avoid repeated attempts without creating a second policy.
Do not filter only `ListSessionsWithBranches`: session state does not define watch consent.
Guard branch changes, terminal resets, and collision merges so they cannot rewrite or remove retired identities.

## API controls

Keep `DELETE /api/v1/github/watches/pr/:id?workspace_id=...` and the existing WS delete action.
Both now retire the stored watch and retain their successful response shapes.
Repeated DELETE of a retained retired ID remains successful after authorization.
Unknown IDs retain the existing not-found behavior.

Extend `GET /api/v1/github/watches/pr?workspace_id=...` with optional `include_retired=true`.
The default remains active-only. The inclusive response adds `retired_at` on retired watch objects.
Use a dedicated authorized list method. Do not make internal active-list methods inclusive.

Add `POST /api/v1/github/watches/pr/:id/resume?workspace_id=...`.
No caller-supplied task, repository, session, owner, or branch replaces the stored target.
Validate workspace access, current task existence, unarchived state, and attached repository before mutation.
Return 404 for an unknown ID, the existing workspace authorization error for denied access, and 409 for an ineligible target.
An already active ID returns its current watch without another write.

For a retired ID, atomically replace it with a new active watch ID and return `{watch: ...}`.
Preserve target identity and available provenance. Clear polling watermarks so the next cycle is eligible.
A numbered watch resumes numbered monitoring. A searching watch resumes discovery.
The old ID becomes not found. After an uncertain response, list the target to obtain its current ID before retrying.
Reset passive refresh admission after commit. Reuse normal quota and authentication gates.

## Result fencing and concurrency

The old watch ID is an execution generation. Resumption allocates a new ID to fence delayed results.
A provider response must still refer to a present, active watch before applying associations, timestamps, resets, or feedback.
Serialize retirement, resumption, and result application for that watch through the same short database transaction boundary.
Do not hold a transaction across remote I/O.
For PostgreSQL, acquire the existing-style transaction advisory lock for the canonical identity before reads and writes.
SQLite uses its writer transaction. Unique indexes remain a final constraint.
A separate preflight read alone is insufficient: retirement can commit between that read and the association write.

Preserve the existing event publication model. Complete any result application admitted before retirement before reporting retirement success.
Do not start a new watch-driven event publication after retirement wins the serialized boundary.
Test blocked provider responses and retire/resume interleavings with deterministic channels.
A request already sent before retirement is allowed to complete, but its stale result is discarded.

## Compatibility

REST and GraphQL share the same retirement decision. Legacy session provenance does not change canonical identity.
GitHub App and PAT clients use the existing workspace authorization and credential-resolution paths.
An archived or detached target cannot resume. A retired target with incomplete identity fails closed for polling.
No new runtime flag, polling loop, or credential source is introduced.

## Related records

- [Requirements](../requirements/github-pr-watch-retirement.md)
- [Decision](../../../decisions/2026-10-26-explicit-pr-watch-retirement.md)
- [Task-owned identity](../../../decisions/2026-08-31-task-owned-pr-watch-identity.md)
- [Discovery schedule](github-pr-discovery-health.md)
- [Implementation plan](../../../plans/github-pr-watch-retirement/plan.md)
