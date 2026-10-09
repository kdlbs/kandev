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
   every manifest row names the same old task and recorded environment,
   worktree, and repository identity. Validate workspace authorization against
   the retained cleanup snapshot's workspace ID; manifest rows contain no
   workspace field. Missing, malformed, stale, foreign, or absent-worktree
   evidence is `UNKNOWN`.
2. Require a platform-verified immutable archive-byte receipt that binds both
   exact task IDs, the repository/environment/worktree identities, manifest
   and Git-index digests, complete file/link/metadata inventory, and a
   successful source-byte rehash. The platform verifier must read back and
   hash every immutable archive entry, compare it with the independently
   captured source-byte hash, and verify exact file/link/metadata inventory
   equality before `PASS`. Matching paths and metadata alone never prove byte
   equality; missing, extra, or changed archive entries cannot pass. It must
   also bind the preserved commit OIDs
   and independently verified reachability evidence for those exact commits,
   including the repository/ref identity and observed generation. Include
   those identities, verification results, and generations in the evidence
   digest so later cleanup can revalidate them before claiming any operation.
   Missing, stale, or contradictory commit/reachability evidence is `UNKNOWN`;
   proven unpreserved or unreachable commits are `BLOCKED`. A caller path or
   source-manifest hash alone is never proof. Without the immutable archive
   receipt, return `UNKNOWN/ARCHIVE_BYTES_UNVERIFIED`.
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
4. Inventory ordinary in-flight dispatch claims and SendNow claims for every
   exact old-session incarnation, including sources removed from the FIFO.
   Bind claim/attempt IDs, source entry IDs and ordering, session and operation
   generations, acceptance state, delivery protocol/submission identity, and
   payload/body/attachment/settings digests. Each claim needs an exact terminal
   disposition; acceptance alone does not prove replacement preservation or
   acknowledgement. Undisposed claims are `BLOCKED`; missing, stale, foreign,
   or contradictory claim evidence is `UNKNOWN`. The adapter must be read-only:
   never restore, recover, acknowledge, dispatch, or mutate schema. Existing
   recovery listing is not a read-only census. Until independently reviewed
   claim-inspection contracts exist, dispatch evidence remains `UNKNOWN`.
5. Read one exact pending-move census per old session through the #3155
   contract. `found: false` is a terminal absence receipt only when the live
   Coordinator and reachability predicates are satisfied. A found row remains
   `BLOCKED/PENDING_MOVE_REQUIRES_EXACT_DISPOSITION` until an independently
   authorized operation records its exact terminal disposition; W03 never
   invokes cancellation.

## Remaining terminal-disposition designs

W03 also owns the read-only receipt design and focused validation for every
remaining predicate in W02's closed registry. The task service coordinates
these designs with each evidence-owning subsystem; it does not perform the
terminal operation. No Task 04-07 work order currently assigns these adapter
designs, so they must not be treated as covered by an unspecified later task.

| Predicate | W03 design responsibility | Required focused validation |
| --- | --- | --- |
| `relationships` | Exact dependency/subtask inventory and terminal-disposition evidence from the task relationship owner. | Outstanding dependencies/subtasks block; foreign identities, changed generations, or unavailable inspection are `UNKNOWN`. |
| `pr_watch` | Exact PR association/watch inventory and terminal-disposition evidence from the PR/watch owner. | An active watcher or unresolved PR association blocks; omitted watches, stale generations, or unavailable inspection are `UNKNOWN`. |
| `environment_runtime` | Exact environment/runtime/session inventory and terminal-disposition evidence from their owners. | A live runtime or session blocks; foreign environment identity, uncertain liveness, or unavailable inspection are `UNKNOWN`. |
| `lease_consumer` | Exact lease/consumer inventory and terminal-disposition evidence from the owning subsystem. | An active lease or consumer blocks; omitted consumers, changed ownership/generations, or unavailable inspection are `UNKNOWN`. |
| `replacement_ownership` | Exact handoff inventory and replacement-bound preservation/acceptance evidence from the task service and replacement owner. | An undisposed handoff or unacknowledged intake blocks; foreign replacement/session identity, stale evidence, or unavailable inspection are `UNKNOWN`. |

Each design must bind the exact resource identities, observed generations, and
terminal evidence digests without exposing resource contents, and validate no
mutation for every outcome. These adapters remain unimplemented and
`UNKNOWN/INVENTORY_UNAVAILABLE` until their independently reviewed read-only
contracts exist. Recording this design work order does not complete
AC-TASKS-EXACT-RETIREMENT-002.1 or make the preview eligible.

The preview retains W02's full registry: its existing identity authorization,
session/FIFO, move/dispatch, preservation/Git, and every predicate above. It
must never aggregate a passing subset. Any adapter failure, identity mismatch,
stale generation, or unimplemented integration is `UNKNOWN` and keeps the
preview ineligible.

## Focused test plan

- Source manifest: reject task/workspace/environment/worktree/repository
  mismatch, missing index digest, absent worktree, and a manifest-only claim
  without an immutable archive-byte receipt.
- Archive receipt: reject byte/index-digest or task/repository/environment/
  worktree-ID mismatch, unverified archive location, incomplete metadata
  inventory, missing commit OIDs or reachability proof, and unpushed/unreachable
  commit evidence. Assert commit/ref/generation changes alter the evidence
  digest and invalidate the receipt before any later cleanup claim. Reject
  differing archive bytes even when paths and metadata match; reject missing
  or extra archive entries and absent per-entry readback verification.
- FIFO handoff: assert deterministic `(session incarnation, position, entry
  ID)` ordering and one-to-one source-to-intake acknowledgement mapping; reject
  changed, omitted, duplicated, reordered, hash-mismatched, attachment- or
  delivery-setting-mismatched entries, duplicate-body substitution, and a
  receipt without a replacement durable acknowledgement.
- Dispatch claims: an empty FIFO with an ordinary or SendNow claim still
  blocks without its exact terminal disposition. Cover accepted-but-not-
  acknowledged claims, absent or stale claim generations, changed attempts or
  submission/payload identities, omitted SendNow sources, and foreign session
  incarnations. Unavailable read-only inspection is `UNKNOWN`. Assert no
  recovery, acknowledgement, dispatch, or schema mutation for every outcome.
- Pending moves: map an authorized #3155 `found: false` census to a terminal
  absence receipt; map `found: true` to `BLOCKED`; map authorization, stale,
  or unavailable census outcomes to `UNKNOWN`. Assert no queue, session, task,
  or pending-move mutation for every result.

- Remaining predicates: exercise every case in the ownership table, preserve
  all closed-registry categories, and assert the preview stays ineligible when
  any category is unavailable or lacks an exact terminal disposition. None of
  these read-only tests may terminate sessions, settle relationships, stop
  watches, release leases, acknowledge handoffs, or repair inventories.

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
archive-byte receipt, replacement-bound ordered FIFO acknowledgement, or
read-only exact dispatch-claim census. The remaining terminal-disposition
adapters assigned above are also unavailable. These are W03 design and
validation responsibilities still to be completed with their subsystem
owners after the branch incorporates the merged source-manifest baseline.
Until every closed-registry predicate has its required verified evidence,
W03 remains fail-closed and W04-W07 remain out of scope.
