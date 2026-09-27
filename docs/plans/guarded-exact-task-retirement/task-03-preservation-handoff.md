---
id: "03-preservation-handoff"
title: "Design preservation and handoff evidence"
status: in_progress
wave: 2
depends_on:
  - "02-read-only-preview"
plan: "plan.md"
requirements:
  - REQ-TASKS-EXACT-RETIREMENT-002
acceptance_criteria:
  - AC-TASKS-EXACT-RETIREMENT-002.1
  - AC-TASKS-EXACT-RETIREMENT-002.2
  - AC-TASKS-EXACT-RETIREMENT-002.3
system_design:
  - ../../specs/tasks/system-design/guarded-exact-task-retirement.md
---

# Task 03: Design preservation and handoff evidence

## Objective

Define read-only evidence adapters that let a later exact-retirement preview
prove that all unique old-task state was preserved and explicitly accepted by
one replacement. This work package does not archive, delete, resume, consume,
acknowledge, cancel, transfer, or otherwise mutate a task resource.

## Dependency receipt

`b88aea31` (PR #3905) is merged. Its archive source manifest is task-scoped,
durably retained cleanup evidence that binds task, environment, worktree, and
repository IDs and includes HEAD, staged-index, status, and changed-path
digests. It deliberately retains no source bytes. W03 may read it as one
input, but it can never alone satisfy preservation.

`0f76f4f` (PR #3155) is still open. It defines a designated-Coordinator,
read-only pending-move census and a separately fenced exact cancellation. W03
must not query, cancel, or emulate a pending move until that contract is
merged into the intended base. Until then the move/dispatch receipt remains
`UNKNOWN` with reason `PENDING_MOVE_CONTRACT_UNAVAILABLE`.

## Receipt design

The eventual adapter input is the exact old/replacement pair already
authorized by W02. It must emit only identifiers, generations, reason codes,
and SHA-256 digests. Queue bodies, archive bytes, paths, credentials, and
provider tokens stay outside the retirement receipt.

1. Read the retained source manifest by exact old-task identity. Validate that
   every manifest row names the same old task and recorded workspace,
   environment, worktree, and repository identity. Missing, malformed, stale,
   foreign, or absent-worktree evidence is `UNKNOWN`.
2. Require a platform-verified immutable archive-byte receipt that binds both
   the exact old-task ID and replacement-task ID, the manifest digest, complete
   file/link/metadata inventory, and a successful byte rehash. A caller path
   or source-manifest hash alone is never proof. Its absence is
   `UNKNOWN/ARCHIVE_BYTES_UNVERIFIED`.
3. Obtain a read-only FIFO snapshot for every old-session incarnation. Each
   item contributes its stable entry ID, position, body hash, attachment
   identities and digests, and a digest of its delivery settings. The
   replacement must present an ordered, one-to-one intake receipt mapping every
   source `(session incarnation, position, entry ID, body hash)` to one
   acknowledged intake entry with matching attachment and delivery-setting
   evidence. A durable acknowledgement is bound to the exact replacement
   session incarnation. Any unread item, missing acknowledgement, reordered
   item, duplicate-body substitution, changed attachment or delivery settings,
   or changed snapshot is `BLOCKED` or `UNKNOWN`; no implicit replay is allowed.
4. Read one exact pending-move census per old session through the #3155
   contract. `found: false` is a terminal absence receipt only when the live
   Coordinator and reachability predicates are satisfied. A found row remains
   `BLOCKED/PENDING_MOVE_REQUIRES_EXACT_DISPOSITION` until an independently
   authorized operation records its exact terminal disposition; W03 never
   invokes cancellation.

The preview aggregates only these read-only receipts. Any adapter failure,
identity mismatch, stale generation, or unimplemented integration is
`UNKNOWN` and keeps the preview ineligible.

## Focused test plan

- Source manifest: reject task/workspace/environment/worktree/repository
  mismatch, missing index digest, absent worktree, and a manifest-only claim
  without an immutable archive-byte receipt.
- Archive receipt: reject a byte-digest or replacement-ID mismatch, unverified
  archive location, incomplete metadata inventory, and unpushed/unreachable
  commit evidence.
- FIFO handoff: assert deterministic `(session incarnation, position, entry
  ID)` ordering and one-to-one source-to-intake acknowledgement mapping; reject
  changed, omitted, duplicated, reordered, hash-mismatched, attachment- or
  delivery-setting-mismatched entries, duplicate-body substitution, and a
  receipt without a replacement durable acknowledgement.
- Pending moves: map an authorized #3155 `found: false` census to a terminal
  absence receipt; map `found: true` to `BLOCKED`; map authorization, stale,
  or unavailable census outcomes to `UNKNOWN`. Assert no queue, session, task,
  or pending-move mutation for every result.

The intended focused command, once adapters exist, is:

```sh
cd apps/backend
go test -count=1 ./internal/task/service ./internal/orchestrator/messagequeue ./internal/worktree \
  -run 'Test(ExactRetirementPreservation|ExactRetirementFIFO|ExactRetirementPendingMove|ArchiveSourceManifest)'
```

## Next gate and owner

The immediate external gate is PR #3155, owned by the pending-move control
plane: it must merge its read-only exact census and fenced cancellation
contract into the intended base before W03 can consume a terminal move
disposition. Independently, no current contract supplies the immutable
archive-byte receipt or the replacement-bound ordered FIFO acknowledgement;
those are W03 adapter work still to be designed and implemented after the
branch incorporates the merged source-manifest baseline. Until both are
present, W03 remains fail-closed and W04-W07 remain out of scope.
