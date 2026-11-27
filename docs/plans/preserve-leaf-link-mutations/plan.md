---
created: 2026-10-08
status: implemented
requirements:
  - REQ-WORKSPACES-FILE-ENTRY-MUTATIONS-001
system_design:
  - ../../specs/workspaces/system-design/file-entry-mutations.md
legacy_specs: []
---

# Implementation Plan: Preserve Leaf Link Mutations

## Overview

Restore native Delete/Rename/Move entry semantics for eligible leaf symlinks
without changing target-following reads or edits. One sequential work order
owns causal real-filesystem RED/GREEN, the narrow rooted correction, registered
HTTP routing, and preservation controls. ROOT reviewed this package and later released implementation in the existing
primary session with the exclusive GLOBAL LOCAL-HEAVY82 lease, following
CHILD81's joined return.

Workspaces owns the missing entry-mutation contract. Existing link identification
owns metadata, saved-file-content owns editing, UI file-tree scope owns relative
inventory addresses, and executors own restoration of registered source roots.
None defines this destructive leaf behavior; no adjacent pair is duplicated.

## Scope

In scope: Delete/Rename leaf identity, destination occupancy, native rooted
operations, immediate notifications, existing repository forwarding, and
ordinary/parent-routing/root/escape/cross-root/race-boundary controls.

Out of scope: frontend changes, schema/API changes, general path policy,
dangling/loop source support, root-alias removal, target rewriting, atomic
destination transactions, ownership coordinators, framework refactors, browser
or build/E2E work, public-doc edits, optional polish, and new sessions/delegates.

## Confirmed root cause and evidence

Base and initial local HEAD: `e3732c65f8292cb1061cd327a5c9516223197cf2`.
`resolveMutationPath` canonicalizes the final link via `resolveSafePath` before
rooted Delete/Rename. Actual Files tree inventory exposes the alias as a link.
The operation then removes or moves the target and leaves the alias dangling.

ROOT-owned read-only evidence:

- `/tmp/kandev-root-leaf-symlink-discovery-20261008/candidate_test.go`, regular
  0400, SHA256 `91abf2aa13485a445cf837c5d8db87a6e9b40811381b2a8f55b9dfdbfbb5dfc2`.
- Sibling `qualified-proof.json` and `output.log` report actual Go1.26 tagged
  race exit 1: file-link Delete, file-link Rename, directory-link Delete reached
  three causal missing-target assertions. Ordinary-file Delete and contained
  symlink-parent Rename passed.
- Original handle 29733, start 344dc9, actual join 59bf9f, wrapper exit 0,
  fresh group 3700561 gone, source removed, ROOT clean. Qualification timestamp
  `2026-10-08T09:03:22.654350+00:00`.

CHILD82 checked metadata, hash and logs read-only. No replay, copy, import,
edit, chmod, cleanup, or heavy proof run. ROOT retains and releases the protected
proof only after actual merge, independent verification, archive and absence.

## Technical approach

Follow the [owning design](../../specs/workspaces/system-design/file-entry-mutations.md).
Keep target resolution unchanged. Add a local entry resolver retaining canonical
parent admission and an authority-root handle; keep full-target source admission
and root protection before returning the preserved leaf path. Delete classifies
the leaf with rooted Lstat; Rename validates source entry existence after the
barrier and occupied destinations with rooted Lstat, then moves with rooted
Rename. Canonical target admission retains Stat. Do not change HTTP/WS/frontend forwarding.

| Operation boundary | Identity and behavior | Evidence | Unsupported outcome |
| --- | --- | --- | --- |
| Tracker Delete/Rename | Native leaf under admitted parent/root | Real temporary files/tree and barrier | Root/escape/cross-root/dangling source rejects |
| HTTP selected repo | JoinRepoPath then root tracker; request-relative response | Registered router, alpha/beta/root sentinels | Existing 400 response; no effects/event |
| HTTP aggregate path | Workspace tree path directly | Registered router and tree IsSymlink | Same authority checks |
| WS/client | Existing session/path/repo forwarding | Source audit; existing forwarding tests | Existing error conversion |
| Desktop/phone | Shared selected path and settled state | Backend operation evidence | Existing failure handling |

## Tests

New process file `workspace_file_entry_mutations_test.go`:

- `TestWorkspaceFileEntryMutations_LeafIdentity`: AC 001.1/001.2/001.6, actual
  tree link metadata, file/directory delete/rename/move, link value and all bytes.
- `TestWorkspaceFileEntryMutations_AbsoluteLeafIdentity`: absolute contained
  file/directory link Rename/Move, exact stored value and target identity.
- `TestWorkspaceFileEntryMutations_DestinationOccupied`: AC 001.3, every occupied
  kind including dangling/loop destinations and distinct links to one target.
- `TestWorkspaceFileEntryMutations_Compatibility`: AC 001.4, ordinary entries,
  contained parents, new destination parents, same-entry no-op, read/edit target.
- `TestWorkspaceFileEntryMutations_Authority`: AC 001.5, workspace/source roots
  and aliases, registered parents, stale/empty allowlist, dangling/loop source,
  outside/cross-root paths, unchanged entry and byte sentinels.
- `TestWorkspaceFileEntryMutations_ParentSwap`: AC 001.5, source/destination
  parents swapped at existing mutation barrier; outside contents never change.

New API file `workspace_file_entry_mutations_test.go`:

- `TestRegisteredWorkspaceFileEntryMutations`: AC 001.1-001.6, real manager/router,
  actual tree GET, selected `repo=alpha` and unscoped `alpha/alias` aggregate
  requests, root/alpha/beta link/target sentinels, exact 200/400 response paths,
  immediate root-tracker remove/rename events and no events on rejection/no-op.
  Cover both file and directory links and a Move through Rename to a new parent.
- `TestRegisteredWorkspaceAbsoluteLeafMutations`: selected/aggregate absolute
  contained file/directory link Rename/Move with target identity and notifications.

Existing tracker registered-source, descendant-swap and cross-root suites remain
in the targeted command. Existing HTTP Delete/Rename and client forwarding tests
protect response/routing compatibility. Use t.TempDir with genuine links,
Lstat/Readlink and independent byte reads. New RED must reach missing-target
or wrong-link assertions through production APIs. Do not import ROOT's fixture.

## E2E and mobile audit

Operation-level end-to-end evidence is the registered HTTP router through the
real manager/tracker to disk and immediate notifications. No frontend composition
or viewport-dependent logic changes. Desktop context actions and phone touch
actions share the same path callbacks; the state/data-only exception applies.
No browser, phone E2E, product build, or public docs run is required.

## Work orders

- [x] [Task 01: Preserve selected leaf entry](task-01-preserve-selected-entry.md)

Only one work order, completed for ROOT-authorized implementation.
Its bounded absolute-link follow-up has passed local verification under ROOT's
exclusive lease; changing delivery status remains in the live task plan.
Execution is sequential in this primary, without delegation. ROOT's concrete
review receipt is `/tmp/kandev-root-child82-design-review-20261008.json`;
its lease-return qualification is
`/tmp/kandev-root-child81-heavy-return-qualified-20261008.json`.

## Verification results

Design checks passed on 2026-10-08: `python3 scripts/list-docs.py validate`
(363 decisions, 1439 specifications), `python3 scripts/lint-spec-files.py --all`,
and `python3 scripts/lint-spec-files.test.py` (36 tests). The catalog discovers
both new Workspaces files. Four-artifact local-link and whitespace checks passed.
The exported `.github/scripts/pr-docs.cjs:validateCoverage` passed the planned
production/test-scope preflight with one work order and `errors: []`; this is
documentation traceability evidence, not a claim of production changes.
The preflight used the existing Node binary at
`/home/jcfs/.nvm/versions/node/v24.18.0/bin/node` because `node` is absent from
the shell PATH. No runtime or dependencies were installed.

Permanent independently authored regressions compiled and reached causal RED
before any production edit. The process leaf/compatibility selector reported six
leaf identity failures; its ordinary/contained-parent/read-edit controls passed.
The full process matrix additionally caught same-target no-op, dangling
collision and root-alias/source-authority failures. Registered HTTP tests reached
missing-target and collision assertions for workspace, selected alpha and
aggregate alpha paths. Existing protection and response controls passed.

The narrow correction resolves parents while retaining the selected leaf name,
keeps full-target source admission/root protection, and uses rooted Lstat for
Delete classification and Rename destination occupancy. General read/edit
resolution and forwarding are unchanged.

Targeted process, registered HTTP and client suites passed with Go1.26,
`-trimpath -tags=fts5 -race -p=1 -count=1`, GOMAXPROCS=2 and GOMEMLIMIT=512MiB.
Scoped lint passed with zero issues. Documentation catalog, specification lint,
actual changed-file documentation coverage and whitespace checks passed before
the normal active hooks, commit, push and ready PR. Owned execution
receipts and raw logs are under `/tmp/kandev-child82-execution-20261008/`:

| Command | Original handle / start / actual join | Actual exit | Fresh group |
| --- | --- | --- | --- |
| RED leaf + controls | 51615 / 3ab43a / ecc4a3 | 1, causal | 3794741 absent |
| RED process matrix | 36512 / 30ce20 / c08e0d | 1, causal | 3798008 absent |
| RED registered HTTP | 58393 / 0a21d4 / 91a284 | 1, causal | 3798806 absent |
| GREEN process matrix | 85431 / a9edf3 / 3d55a6 | 0 | 3811212 absent |
| GREEN registered HTTP | 24304 / 2a8193 / ea2594 | 0 | 3811887 absent |
| GREEN forwarding client | 96352 / 1b77f7 / 96c71c | 0 | 3814088 absent |
| Scoped changed-code lint | 89549 / 61baa0 / 420299 | 0, zero issues | 3814790 absent |

Each JSON receipt retains the exact command, original UTC start/cutoff/end,
actual command exit and fresh group scan. Wrapper exit 0 is not the command
verdict. Native Linux behavior is verified; no Windows native pass is claimed.

The bounded absolute-contained link follow-up reached causal RED at the
published implementation before its production correction: four tracker and
eight selected/aggregate HTTP Rename/Move cases rejected the preserved leaf
through following post-barrier Stat. Only that entry check changed to Lstat;
full canonical target Stat and root/authority admission remain intact. Affected
race suites and scoped lint passed. The one required full changed-code lint
timed out without diagnostics; ROOT authorized one identical recovery, which
passed with zero issues. The timeout's cause remains unproved.

| Follow-up command | Original handle / start / actual join | Actual exit | Fresh group |
| --- | --- | --- | --- |
| RED absolute tracker | 81688 / 1eb854 / 4b96a3 | 1, causal | 3958959 absent |
| RED absolute HTTP | 28475 / 055922 / bee21a | 1, causal | 3971153 absent |
| GREEN affected tracker | 78283 / 58c05e / 27c8cf | 0 | 3974034 absent |
| GREEN affected HTTP | 92768 / d3ccbe / 481975 | 0 | 3976087 absent |
| Scoped changed-code lint | 49470 / d4c88a / e51b5a | 0, zero issues | 3978253 absent |
| Full changed-code lint | 18951 / bdef0c / 21d94e | 124, timeout | 3980379 absent |
| ROOT-authorized identical recovery | 68730 / e7e7e7 / b55ffa | 0, zero issues | 4009096 absent |

Both full lint commands used the verified actual PR base
`e3732c65f8292cb1061cd327a5c9516223197cf2`, `./...`, GOMAXPROCS=2,
GOMEMLIMIT=1GiB, concurrency 2, serial-runner admission, CLI 5m and GNU 6m
with kill-after 10s. No cache/scope/timeout change or automatic retry occurred.

## Risks

- Preserving a basename without validating the target could relax root/escape
  authority. Preserve full source admission and fail closed before effects.
- Following destination Stat can overlook dangling occupancy; Lstat must apply
  at the operation boundary, not merely in a standalone predicate test.
- Native Windows symlink privilege may be unavailable; skip only error 1314
  for the individual link fixture, never all native tests.
- Relative links moved across directories retain their stored value and can
  become dangling; do not silently rewrite them or promise target stability.
- os.Root preserves the existing external-parent boundary, not a new general
  atomic no-clobber guarantee for concurrently created in-root destinations.

## Session, resources and next action

Current task `2e14c718-e93c-4967-a4b0-69ea59157422`, session
`8be3885e-4724-4854-9ae5-8d0ebca59239`, CHILD82. Live phase/identity/standing
delivery constraints are persisted version-safely in this task's Kandev plan,
including the `<kandev-system>` marker; task title and user edits remain owned
by their existing authors.

Task 01 implementation and initial local delivery are complete. Normal active
hooks passed, the implementation was committed and pushed, and ready
[PR #4338](https://github.com/kdlbs/kandev/pull/4338) was opened. All twelve
original local commands joined with actual terminal verdicts and absent groups
before the explicit local-heavy RETURN and the single hosted observer.

ROOT authorized a documentation-only correction for duplicate stale handoff
findings while that original observer remained live. That correction did not
change production or tests. The later bounded absolute-link correction changed
only the final Rename entry check and its real-filesystem regressions; its
local verification and original timeout/recovery receipts are recorded above.
Current-head CI/review qualification, finding dispositions, delivery
receipts and the exact next action belong in the versioned task plan, preserving
task/session identity, system marker, user edits and protected proof. ROOT
retains the separate serial merge gate; terminal hosted CI and merge are not
claimed here. No operator/model question, ACK loop, sibling contact, or new
task/session.
