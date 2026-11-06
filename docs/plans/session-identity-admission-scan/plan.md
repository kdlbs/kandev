---
created: 2026-11-05
status: implemented
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
  - REQ-PLATFORM-DELIVERY-RETENTION-004
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
system_design:
  - ../../specs/platform/system-design/durable-delivery-write-cost.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
legacy_specs: []
---

# Session identity admission scan

## Overview

Every task-session creation runs a full scan of `agent_delivery_effects` while
it holds the single SQLite writer. With about one million retained effect rows,
the scan holds the writer for 0.25 s when the pages are cached and for several
seconds when they are not. On 5 November 2026, two of those holds exceeded the
two-second required-persistence probe, which made all persistence unavailable
for about 13 seconds each time.

The fix rewrites one query so that every branch is an index search. It does not
change behavior, transaction scope, health policy, or schema.

## Evidence and confirmed root cause

Evidence comes from read-only inspection of
`/root/.kandev/logs/backend-logs-2026-11-05-000002.log` and from a disposable
copy of the 09:29 UTC boot snapshot
(`backups/kandev-v0.98.0-210-g067632acaa2-20261105T092955Z.db`, 14.7 GB).
See [evidence.md](evidence.md).

1. **The writer holder is inside `PrepareTaskSession`.** For all five
   creations, the interval between `preparing task session` and
   `session entry created` matches the POST latency: 2.41 s, 0.30 s, 6.86 s,
   3.68 s, and 0.26 s. A concurrent `GET /api/v1/task-sessions/:id`, which reads
   through the writer pool, was released in the same millisecond as each
   session insert. Its durations were 6,193 ms, 3,166 ms, and 142 ms. Both probe
   failures occurred inside the two slow windows, and both stalls ended when the
   session entry was created.
2. **The slow statement is the retired-identity check.** `createTaskSession`
   calls `ensureAgentDeliveryIdentityAvailable` inside the writer transaction.
   Its last branch is
   `agent_delivery_effects e LEFT JOIN agent_delivery_cursors c ... WHERE e.session_id = ? OR c.session_id = ?`.
   `EXPLAIN QUERY PLAN` on the snapshot reports `SCAN e`, which reads 1,015,205
   rows and about 260 MiB of table pages.
3. **The measured cost matches the incident.** On the copy, the query took
   0.243–0.257 s with a warm cache, which matches the two fast creations. After
   the copy's pages were evicted with `posix_fadvise(DONTNEED)`, it took
   4.69–4.91 s, which covers the 2.4–6.9 s slow creations. The incident host
   also had concurrent worktree and Git I/O.
4. **An indexed form is equivalent and fast.** An indexed form of the effect
   branch took 13–51 ms cold, compared with 4.5–4.9 s. It returned the same
   result as the original for 400 real session and delivery IDs from the
   snapshot. Task 01 re-measures the final statement below.

The query was added by #4425 (12 October 2026). Its cost grows with retained
delivery history, so the stall appeared only once the effects table had grown
large. The existing `BenchmarkDeliveryWriterContention` times `CreateTask`,
not session creation, so it did not cover this path.

Durability-barrier waits stayed under 250 ms, and no slow-run warning
appeared. That is consistent with this cause: the session transaction occupied
the writer, not the barrier.

## Requirement conformance

- New criterion **AC-PLATFORM-DELIVERY-WRITE-COST-001.5**: delivery checks inside
  interactive writes must be index-bounded by the requested identity. This
  bound was missing from the requirements, so this is classification 2.
- **AC-PLATFORM-DELIVERY-RETENTION-004.3** is preserved: a delayed old write
  must not affect a replacement session, so retired identities must still be
  rejected.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001**: the statement is shared by
  both engines and stays portable.

The system design's new section, "Identity admission inside interactive
writes", records the rule and the measurements.

## Correction

```sql
SELECT EXISTS (
    SELECT 1 FROM agent_delivery_cleanup_jobs WHERE session_id = ?
    UNION ALL SELECT 1 FROM agent_delivery_submissions WHERE session_id = ?
    UNION ALL SELECT 1 FROM agent_delivery_inbox WHERE session_id = ?
    UNION ALL SELECT 1 FROM agent_delivery_cursors WHERE session_id = ?
    UNION ALL SELECT 1 FROM agent_delivery_effects WHERE session_id = ? AND session_id <> ''
)
```

For any non-empty session ID, this returns the same result as the original.
The original effect branch matched `e.session_id = ? OR c.session_id = ?`.
Its second disjunct can only match when a cursor with that session ID exists,
and the plain cursor branch already returns true in that case. So the join is
redundant, and the effect branch reduces to a direct lookup on the partial
index. Session IDs are assigned before this call, so the empty-ID case cannot
occur.

## Scope

### In scope

- `ensureAgentDeliveryIdentityAvailable` in
  `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_admission.go`.
- Regression tests for the query plan and for rejection through each branch.

### Out of scope

- Health-probe cadence, grace periods, or readiness policy. After attribution,
  the probe failures were genuine writer unavailability, so persistence must
  still fail closed.
- Moving the check out of the transaction, schema or index changes, extra
  writer connections, and retention budget changes.
- Other writer holders: backups, plugin cleanup, and old cleanup conflicts.
- Authentication.

## Tests

| Criterion | Evidence |
| --- | --- |
| AC-PLATFORM-DELIVERY-WRITE-COST-001.5 | `TestAgentDeliveryIdentityAdmissionUsesIndexes` fails on the current `SCAN e` plan |
| AC-PLATFORM-DELIVERY-RETENTION-004.3 | `TestAgentDeliveryIdentityAdmissionRejectsRetainedReferences` covers every branch, plus a stream-only effect with an empty `session_id` whose cursor names the session, and an unrelated session that stays admissible |
| AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001 | The same rejection table runs against `KANDEV_TEST_POSTGRES_DSN` when it is set |

Timing is recorded as evidence from the disposable snapshot. It is not a CI
assertion.

## Work orders

- [x] [Task 01: Index the session identity admission query](task-01-index-identity-admission.md)

## Implementation results

Task 01 ran on 5 November 2026 in one session, without subagents. The query
became a named helper, `agentDeliveryIdentityRetainedQuery`. Its effect branch
is now `SELECT 1 FROM agent_delivery_effects WHERE session_id = ? AND session_id <> ''`.

- `TestAgentDeliveryIdentityAdmissionUsesIndexes` failed before the change
  with `SCAN e` and passes after it.
- `TestAgentDeliveryIdentityAdmissionRejectsRetainedReferences` passed before
  and after the change on SQLite. PostgreSQL was skipped because
  `KANDEV_TEST_POSTGRES_DSN` was not set.
- `go test -trimpath -race ./internal/task/repository/sqlite -run 'AgentDeliveryIdentityAdmission|Retention|Retire|ManagedConversationAdmission'`
  passed.
- `go test -trimpath -race -timeout 60m ./internal/task/repository/sqlite`
  passed in 891.7 s.
- `go test -trimpath -race ./internal/task/service -run 'AgentConversation|ManagedConversationAdmission'`
  passed.
- `go run ./cmd/sqlguard ./internal` exited 0, and
  `go test -trimpath -race ./internal/persistence/storeconformance` passed.
- `bash scripts/lint-go-changed` reported 0 issues for both changed Go files.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  and `git diff --check` passed.
- On the snapshot, the final statement took 18.8–28.1 ms cold, compared with
  10.3–28.4 s for the original on a loaded host. Results matched for 401 IDs.
  See [evidence.md](evidence.md#implementation-re-measurement). The disposable
  copy was deleted afterwards.

These results do not include a load test on the live instance.

## Risks

- A future edit could reintroduce an `OR` across the join. The query-plan test
  guards against this.
- Removing `session_id <> ''` silently brings back a scan of the effects table.
  The query-plan test catches that too.
- Other unattributed writer holders may still exist. The diagnostics from #5054
  remain the way to find them.

## Remaining unknowns

- How the page cache behaved on the live host during the incident was not
  measured directly. The cold and warm measurements bracket the observed
  durations.
- The 13-second unavailability window comes from the probe cadence. Whether
  recovery should be faster is a separate health-policy question, and this
  evidence does not justify changing it.
