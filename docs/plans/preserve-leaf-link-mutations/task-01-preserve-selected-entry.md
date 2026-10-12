---
id: "01-preserve-selected-entry"
title: "Preserve selected leaf entry"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-FILE-ENTRY-MUTATIONS-001
acceptance_criteria:
  - AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.1
  - AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.2
  - AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.3
  - AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.4
  - AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.5
  - AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.6
system_design:
  - ../../specs/workspaces/system-design/file-entry-mutations.md
---

# Task 01: Preserve Selected Leaf Entry

## Summary

Write permanent real-filesystem regressions, prove causal RED, then correct
Delete/Rename leaf identity and destination availability within existing rooted
authority. Prove GREEN through tracker methods and registered HTTP requests,
including routing, notification and native containment controls.

## In scope

Own `workspace_files.go`, an optional focused `workspace_file_entry_mutations.go`
companion, and new same-named process/API test files. Keep the correction local
to entry mutations. Reuse actual temporary workspace/tree and manager/router
fixtures rather than mocks or predicate-only assertions.

Implement the plan's complete process/API test matrix. Start with independently
authored file-link Delete, file-link Rename and directory-link Delete tests
plus ordinary and contained-parent controls. Require actual production tracker
and tree calls, distinct bytes, Lstat/Readlink identity, and target preservation.
Register HTTP tests on `Server.Router()` and use `workspaceRequest` /
`decodeWorkspaceBody`; a real `Manager` over a git-free temporary workspace is
sufficient. Reuse fixture patterns from `workspace_handlers_test.go` and
immediate subscription assertions from `workspace_file_save_target_test.go`.
Stop manager/tracker resources with owned cleanup; do not start a watcher or
live app. Selected and aggregate requests must cover alpha versus beta/root,
both operation response types, and remove/rename notifications after effects.

## Out of scope

No changes to forwarding or UI, general target resolver, source allowlist,
schema/API/copy, product builds/browser/E2E, public docs, lifecycle, global
security abstractions, protected ROOT proof, shared resources or other tasks.
No dangling/loop source enablement, root-alias deletion, relative-link rewriting,
blanket native skips, optional cleanup/refactors, or semantic policy weakening.

## Acceptance

1. Permanent RED reaches all three causal real-disk target/link assertions at
   the pinned pre-fix base; controls pass. GREEN preserves entry identity,
   target/unselected bytes and every applicable AC from the owning pair.
2. Registered HTTP selected/aggregate routing, exact response paths/status,
   immediate notifications, ordinary/allowed-parent behavior and root/escape/
   cross-root/stale-allowlist/parent-swap controls pass. No rejected operation
   emits a notification or mutates an entry.
3. Required targeted commands pass under ROOT's single heavy lease; record
   original handles, actual terminal exits, UTC cutoffs, joins, and fresh-group
   absence. Synchronize this Results and the manifest; no stale or inferred
   success receipts.

## Dependencies and execution barrier

None. The package must first receive ROOT concrete review and a later explicit
implementation INTERRUPT to the existing primary. Then mark this work order
`in_progress`; acquire ROOT's global local-heavy lease before any test/install/
lint/hook command. No delegation, recursive task, new session/tab, model switch,
or sibling contact. ROOT owns integration and serial merge.

## Verification

The bounded absolute-contained leaf correction first runs these new selectors
against the published implementation before changing production:

```bash
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/process -run '^TestWorkspaceFileEntryMutations_AbsoluteLeafIdentity$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/api -run '^TestRegisteredWorkspaceAbsoluteLeafMutations$')
```

After causal RED, verify affected process/HTTP cases with existing Rename,
authority and mutation-barrier controls, scoped lint and the full changed-code
fixup lint below. Do not repeat unchanged forwarding or unrelated passing suites.

The following commands are future implementation commands, **not authorized in
the design turn**. Run each from repo root with isolated package working
directories; retain the original native session and join to actual termination.
Use an owned fresh process group and record UTC cutoff and group absence. No
automatic retry or timeout recovery. Run commands sequentially under the lease.

Initial permanent RED (before any production edit), then repeat for GREEN:

```bash
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/process -run '^TestWorkspaceFileEntryMutations_(LeafIdentity|Compatibility)$')
```

Targeted GREEN including all new process tests and existing preservation suites:

```bash
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/process -run '^(TestWorkspaceFileEntryMutations_(LeafIdentity|DestinationOccupied|Compatibility|Authority|ParentSwap)|TestDeleteFile|TestRenameFile|TestWorkspaceFileOperationsAllowRegisteredLinkedSource|TestWorkspaceFileOperationsWithNoAllowedSourceRootsFailClosed|TestWorkspaceFileMutationsRejectDescendantSymlinkSwap|TestRenameFileRejectsCrossRootFileMove|TestRenameFileRejectsCrossRootSameRelativePath|TestRenameFileRejectsCrossRootDirectoryMove|TestRenameFileCrossRootCollisionLeavesBothPathsUntouched)$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/api -run '^(TestRegisteredWorkspaceFileEntryMutations|TestHandleFileDelete_RemovesFile|TestHandleFileDelete_Rejections|TestHandleFileRename_MovesFile|TestHandleFileRename_Rejections)$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -count=1 -timeout=5m ./internal/agent/runtime/agentctl -run '^(TestDeleteFile_.*|TestRenameFile_.*)$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=1GiB golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=e3732c65f8292cb1061cd327a5c9516223197cf2 --concurrency=2 --allow-serial-runners --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

RED/GREEN read/edit, registered-root and no-op assertions belong in the named
new suites, not an unanchored all-backend run. Windows fixtures skip only actual
native symlink privilege-not-held error 1314; fail other creation errors, keep
ordinary tests running. Deterministic barrier cases must not run in parallel.

After a valid backend PR finding, standing ROOT delivery requires **one** full
changed-code lint against the actual PR base, recorded before push:

```bash
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=1GiB golangci-lint run ./... --new-from-rev="$KANDEV_REVIEW_BASE_SHA" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Set `KANDEV_REVIEW_BASE_SHA` to the verified exact PR base before running. This
is a later fixup-only gate, not an extra design or generic pre-PR audit. A
timeout, resource/transport failure, unknown failure or out-of-scope problem
requires ROOT checkpoint, not a duplicate run, cache wipe or foreign kill.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_file_entry_mutations.go` (only if needed for focused local helpers)
- `apps/backend/internal/agentctl/server/process/workspace_file_entry_mutations_test.go`
- `apps/backend/internal/agentctl/server/api/workspace_file_entry_mutations_test.go`
- The four artifacts in this design package for status/results synchronization.

## Inputs

- [Requirement](../../specs/workspaces/requirements/file-entry-mutations.md), all ACs.
- [System design](../../specs/workspaces/system-design/file-entry-mutations.md), especially entry resolution and containment.
- [Manifest](plan.md), evidence, complete matrix and session next action.
- `workspace_files_test.go` registered-source, root/cross-root and barrier controls;
  `workspace_tracker_test.go` ordinary Delete/Rename; `workspace_handlers_test.go`
  registered router fixtures; `workspace_file_save_target_test.go` event/routing patterns.
- Current task Kandev plan with system marker, identity and standing delivery
  constraints. Re-read current versions and preserve user edits.

## Risks

Full-source validation and no-follow leaf operations must coexist; neither
preserving a leaf nor destination occupancy grants new root access. Retain
authority-root handles across the barrier and close them on every path.
Move may leave a relative link dangling, which is native entry behavior.
Linux receipts do not establish Windows native execution.

## Parallelism

`sequential`

## Results

ROOT concretely reviewed all four artifacts, then released test authoring and
full implementation with exclusive GLOBAL LOCAL-HEAVY82. Permanent causal RED
ran before production edits. The three targeted race suites, scoped lint,
documentation catalog, specification lint, actual changed-file documentation
coverage and whitespace checks passed. Normal active hooks passed, the
implementation was committed and pushed, and ready
[PR #4338](https://github.com/kdlbs/kandev/pull/4338) was opened. The local-heavy
lease was returned after every original local command joined with its actual
terminal verdict and absent process group; historical receipts and UTC
boundaries remain in the [manifest](plan.md) and task plan.

ROOT authorized a documentation-only correction for the duplicate delivery
handoff findings while the original hosted collector remained live. That
correction preserved production, tests and their passing verification.

ROOT subsequently released a bounded absolute-contained leaf correction. Four
tracker and eight registered HTTP Rename/Move cases reached causal RED before
the production edit: the post-barrier source `Stat` followed the preserved
absolute link and returned a native path-escape error. Canonical target admission
still uses `Stat`; only the post-barrier selected entry check changes to `Lstat`.
The new cases retain tree metadata, target identity/type/bytes, stored link value,
unselected sentinels, exact response and immediate notification assertions.
Affected process and registered HTTP race verification and scoped lint passed.
The required full changed-code backend lint first timed out with actual exit
124 and no diagnostics; its original process joined and group was absent.
ROOT reviewed that failed receipt and authorized one identical recovery, which
passed with zero issues. The original failure remains historical evidence;
no cause or passing result is inferred for it. Exact commands, UTC boundaries,
native joins and absent-group receipts remain in the task plan and manifest.

Live delivery status,
current-head CI/review evidence, finding dispositions and the exact next action
belong in this task's versioned Kandev plan. That plan owns continuation of the
single original collector and ROOT's separate serial merge gate; this work order
does not claim terminal hosted CI or a merge.
