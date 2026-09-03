---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
---

# Preserved workspace inventory repair system design

## Purpose and boundaries

This design repairs a reusable task environment's durable
`task_environment_repos` inventory when it no longer matches the
repository/branch slots required by session resume or a fresh
additional-session launch, without ever touching the preserved checkout.

The orchestrator/executor owns admission (`validateReuseEnvironmentInventory`),
candidate selection, and preservation evidence. The task repository owns the
repair transaction, the idempotency ledger, and the append-only receipt. Both
the explicit session-recovery path and the ordinary fresh/additional-session
launch path share the same guarded repair function; neither ever deletes,
cleans, resets, reseeds, rematerializes, or broadly discovers a checkout.

## Admission guard

`validateReuseEnvironmentInventory` remains the admission guard for every
resume and additional-session launch. Zero rows and non-empty mismatches both
return `ErrWorkspaceReuseUnsafe`; neither state falls through to materialize a
new checkout.

The explicit recovery request names only a task, session, action, and
idempotency key. The orchestrator authorizes the task/session pair before any
environment, repository, runtime, or filesystem read. It then derives the
required repository slots from canonical task-repository records and resolves
the environment from the session. Caller-provided paths, repository IDs,
branches, and environment IDs are not accepted.

## Candidate selection and checkout proof

For one mismatched slot, the executor builds a candidate from an existing stale
environment row or the target session's `executors_running` snapshot. Local
worktree recovery validates all of these facts:

- task, workspace, environment, repository, and session ownership agree;
- the candidate path is the canonical path owned by the environment and is not
  a symlink;
- `git worktree list --porcelain`, the checkout's common Git directory, HEAD,
  symbolic branch, and repository path are reciprocal;
- the expected branch slot and observed branch/ref identity agree;
- no competing session or runtime claims a live writer: this checks every
  `executors_running` row for the task directly, independent of the owning
  session's own lifecycle status, because a session can already be
  failed/cancelled while its executor row has not yet reached a terminal
  status (failed, stopped, or completed) — a crash before cleanup ran, or
  cleanup still in flight.

The inspector captures HEAD, ref containment, porcelain status counts and
hash, a bounded content hash over tracked, untracked, and ignored files, a
staged-index hash over every index entry's path/mode/blob identity,
executor state, and source record revisions. The staged-index hash exists
because the content hash alone reads only working-tree bytes: a low-level
index write (for example `git update-index --cacheinfo`) can repoint a
path's staged blob without ever touching the working-tree file, which the
content hash and the porcelain status can both miss. The inspection itself
never mutates `.git/index`, including its own status call's normal
opportunistic index refresh. Host paths remain internal; public
receipts expose only a path hash.

## Repair transaction and idempotency

`workspace_inventory_recovery_receipts` is an append-only audit and
idempotency table. The repair transaction locks and rechecks the source rows,
uses their expected revisions, writes exactly one environment-repository row,
and appends one receipt. A unique `(task_id, idempotency_key)` identity returns
the receipt when its request hash matches and conflicts otherwise. The task is
the receipt's lifecycle owner; session and environment identifiers remain
immutable audit facts rather than cascading foreign keys, so routine lifecycle
deletion and the legacy environment-ownership cutover cannot erase receipts.
On PostgreSQL the task row lock (`SELECT ... FOR UPDATE`, a no-op under
SQLite's single-writer serialization) is taken before the existing-receipt
check, not after: locking second would let two concurrent transactions for
the same idempotency key both observe "no receipt exists" before either
commits, so the loser's insert collides with the winner's already-committed
row and surfaces as an occupied-slot conflict instead of a deduplicated
success.

A same-idempotency-key retry is resolved before candidate selection runs:
the executor looks up an existing receipt for `(task_id, idempotency_key)`
first. When one exists, the executor reconstructs the stable request identity
from the current server-owned task, session, environment, repository slot, and
repaired row while retaining the receipt's original preservation snapshot. It
returns the receipt only when that reconstructed request hash matches, including
on a retry issued after the canonical inventory now matches (the ordinary case
once a prior repair already committed), which would otherwise make candidate
selection see zero provable mismatches and misreport a conflict. A changed
session, environment, repository slot, branch identity, worktree identity, or
ambiguous current row is a genuine idempotency-key reuse and conflicts. An
existing valid slot, stale revisions, or any ambiguous proof returns a typed
result without changing inventory.

## Before-and-after checkout attestation

The receipt's `preservation` field is the before-repair evidence, captured
immediately before the repair transaction and persisted as part of the same
receipt row. After the transaction commits, the executor re-inspects the same
candidate checkout and persists that result as the receipt's post-repair
attestation (`post_repair_evidence`, `post_repair_matched`,
`post_repair_verified_at`) via a dedicated repository call, whether the
checkout still matches or not. A mismatch is recorded before the repair call
returns its conflict error, so an unexpected concurrent write during a
metadata-only repair is itself part of the durable audit trail rather than a
transient in-memory check that leaves no trace. Persisting the post-repair
attestation is required before any launch can use the repaired inventory. A
failure to persist it is logged and leaves the already-committed row/receipt
retryable, but the repair result is not launchable until a later retry durably
records positive matching evidence. Attestation persistence is monotonic for
divergence: once a negative observation is durable, a concurrent or later
positive observation cannot overwrite it.

This attestation gate is not scoped to the session or idempotency key that
performed the repair: launch admission for a row that already canonically
matches (no mismatch left for `validateReuseEnvironmentInventory` to find) is
looked up by the row's own `environment_repo_id`, independent of which
session or idempotency key produced it. A session-scoped lookup alone would
only ever find a receipt the *current* session's own repair attempt
produced, so a different session observing a row an earlier session repaired
could otherwise launch without the earlier repair's post-repair attestation
ever being checked — exactly the case where that other repair committed but
crashed, or its attestation write itself failed, before attestation landed.
Any session reaching the already-valid branch instead completes (or is
blocked by, if already negative) the same durable attestation a same-session
retry already requires, before it is ever handed back as an admitted launch.

## Fresh and additional-session launch integration

`LaunchPreparedSession` (the path used by a brand-new session, including
`spawn_session_kandev`, on-entry auto-start, and the "New Session" launch)
runs the same admission guard as resume. On an `ErrWorkspaceReuseUnsafe`
failure while workspace reuse is required, it attempts the same guarded
repair function with a deterministic, session-scoped idempotency key (derived
from the session ID) rather than a caller-supplied one: there is no session
identity for the caller to key on until the session row already exists, and
the session row is created before this launch path runs. Repair failure
propagates the original admission error unchanged (no regression versus the
pre-repair behavior). Repair success re-validates once, then proceeds through
the single existing launch attempt; the session-scoped `sessionLock` already
serializes concurrent launches for that session, and the repair function's own
active-session check rejects a competing sibling writer on the task. No
primary-session flag is set or changed by this path: `SetSessionPrimary` runs
earlier, at session creation, independent of whether the launch that follows
repairs or launches cleanly.

## Related decisions

- [Preserve checkouts during inventory repair](../../../decisions/2026-09-01-preserve-checkouts-during-inventory-repair.md)
