---
created: 2026-10-17
status: implemented
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
legacy_specs: []
---

# Implementation Plan: Immediate Archive Worktree Removal

## Overview

Remove dirty-worktree preservation from archive and let the same cleanup
attempt remove managed checkouts and their enclosing task folder. One
sequential vertical work order changes original archive, retained-job
compatibility, truthful confirmation text, tests, and public documentation.
Tasks owns this policy because archive grants the destructive authority.

The user's explicit policy change supersedes the former preservation criteria;
it is not a regression in the completed task-folder cleanup fix.

## Evidence and intent

On 2026-10-17 at 22:25:50 +0100, task
cd7b40b7-03d7-46bc-806e-f8f35b01e0c2 logged “preserving dirty worktree during
archive cleanup”; reclaim aacacc19-0701-4ada-bee9-0ab76065a536 deferred as dirty.
Git status confirmed two modified documents and an untracked plan directory.
Fix #4483 is included in the running build. The user explicitly requested
removing the protection so archive deletes right away.

Reproduce in a disposable fixture by adding a tracked edit and untracked file
to a managed checkout and calling ArchiveTask. The current service test
TestArchiveTaskPreservesDirtyWorktreeUntrackedFile asserts the prior behavior;
the replacement TestArchiveTaskRemovesDirtyWorktreeAndTaskDirectory must first
fail because both checkout and root still exist.

## Scope

### In scope

- Remove service filtering and both archive-specific manager cleanliness gates.
- Remove cleanliness from reclaim and wake existing waiting jobs after upgrade.
- Complete final-root cleanup after all eligible checkouts are removed.
- Update localized archive summaries and desktop/phone rendered assertions.
- Ship public archive documentation with the production change.

### Out of scope

- New prompts, flags, API parameters, synchronous archive completion, or schema.
- Removing runtime, borrower, ownership, path identity, or unarchive barriers.
- Changing ordinary local checkouts, independent adopted-checkout consent,
  remote executor policies, delete consent, or committed branch recovery.
- Manually deleting the reported task folder during planning.

## Technical approach

Use the paired system design's service, manager, and retained-job flow.
Keep existing branch-preserving entry points and omit RequireCleanCheckout
there; do not reuse branch-deleting options or grant global discard authority.
Preserve universal root eligibility and retained native filesystem proof.
The bounded startup wake only promotes legacy waiting archive_reclaim jobs;
it cannot clear retry deadlines or bypass a claim/ownership check.

| Path | Identity and capability | Intended behavior | Evidence / unsupported fallback |
| --- | --- | --- | --- |
| Managed worktree archive, all entry points | Recorded task/environment, Git registration, manager archive cleaner | Remove regardless of Git changes, then eligible root | Real service/manager filesystem tests; missing ownership or archive cleaner fails closed |
| Existing retained worktree | Original archive generation, worktree/path tuple, durable job | Wake old waiting job and remove without cleanliness delay | Repository/restart/race tests; stale or cancelled identity never mutates files |
| Shared or failed runtime/environment | Current references and stop results | Retain protected checkout/root | Mixed eligible/shared and stop-failure tests |
| Ordinary local / adopted / remote executor | Existing executor or adoption contract | Keep existing cleanup authority | Existing tests; no fallback to unchecked root removal |

## ASCII UI preview

### UI-01: Archive worktree consequences (desktop and phone)

Current consequence: “Clean Git worktrees are removed. A worktree with Git
changes stays until it is clean.” Proposed shared content:

```text
Archive task?
  Git worktrees and local changes are deleted on archive.
  Any running agent sessions will be stopped.
  Unpublished local branches remain available.
  Integrated managed branches may be removed.
  [Existing in-flight warning and cascade option, when applicable]
                                      [Cancel] [Archive]
```

Desktop uses its existing AlertDialog. Phone uses the existing
MobileActionConfirmation bottom drawer, with the same content order and
Cancel/Archive controls in its footer. Compact inline and bulk surfaces use
the same archive summary; the bulk sentence uses the task count. Existing
scroll owner, safe areas, focus return, and phone touch targets are preserved.
This is a copy-only change; spacing is illustrative. The shipped archive
confirmation is the nearest exemplar, with the curated mobile bottom-drawer
pattern. AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.6 owns the wording; the desktop
and mobile archive-confirmation-preference specs prove the rendered result.

## Tests

- 004.1-.4: replace old dirty-preservation cases in archive_dirty_worktree_test.go;
  add staged, unstaged, untracked, ignored, mixed-repository and scheduled/cascade
  variants. Add TestArchiveTaskRemovesDirtyWorktreeAndTaskDirectory using the
  real archive service and root marker; assert source repository and another
  task survive and worktree metadata becomes deleted.
- 004.4: update TestArchiveCleanupRetainsCheckoutDirtiedByCleanupScript and
  manager tests to require removal after script-created dirtiness. Keep branch
  safety receipts and ordinary task-delete dirty rejection covered.
- 004.3/.7: retain and run resource_cleanup_taskdir* and archive identity,
  shared-environment, stop-failure, unarchive, and restart regressions. Include
  mixed eligible/shared inventory so one survivor blocks recursive root deletion.
- 004.5: adapt resource_cleanup_archive_reclaim_test.go for dirty reclaim;
  add TestArchiveReclaimStartupWakesLegacyWaitingJob plus bounded cursor,
  in-use re-deferral, cancellation, owner-change, and restart cases. Add
  repository wake/claim tests in a focused new resource_cleanup_archive_wake_test.go.
- 004.6: update task-cleanup-summary.test.ts and rendered archive tests for
  single/bulk destruction text. Preserve local and delete summaries.

## E2E tests

The real Go service/manager/SQLite/filesystem integration is the archive
filesystem end-to-end evidence. Browser copy is checked in
apps/web/e2e/tests/task/archive-confirmation-preference.spec.ts (chromium) and
mobile-archive-confirmation-preference.spec.ts (mobile-chrome). Extend the phone
file with confirmation enabled and a worktree task; assert the new consequence,
complete Archive, and prove the task leaves the active view. Keep the existing
confirmation-disabled scenario. Both map to 004.6 and retain preference cleanup.

## Work orders

- [x] [Task 01: Remove archive dirtiness retention](task-01-remove-archive-dirtiness-retention.md)

## Verification

Run the work order's exact commands sequentially. Install dependencies once
from apps/ in a fresh worktree before pnpm. Managed E2E commands build fresh
artifacts and enforce resource limits. If PostgreSQL is configured, set
KANDEV_TEST_POSTGRES_DSN for the same repository/conformance commands and
record both backends; do not claim PostgreSQL execution without evidence.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 -timeout 20m ./internal/task/service ./internal/worktree ./internal/task/repository/sqlite -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/inventoryrepair -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- '*.go')
(cd apps/web && pnpm exec vitest run components/task/task-cleanup-summary.test.ts components/task/task-archive-confirm-dialog.test.tsx components/task/task-archive-confirmation.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/archive-confirmation-preference.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-archive-confirmation-preference.spec.ts tests/kanban/mobile-card-archive-confirmation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implemented on 2026-10-18. Original archive and retained-worktree reclaim
remove eligible dirty managed checkouts and then their task root. Startup
wakes legacy waiting jobs once with bounded cursor/cutoff admission. Runtime,
borrower, ownership, no-follow, unarchive, and committed-branch boundaries
remain. Archive summaries and public how-to documentation now describe deletion.

Validation results:

- Filesystem RED failed as expected for staged, unstaged, and untracked work;
  the ignored-file control already passed. The new repository wake capability
  and seven updated archive-copy assertions also had behavioral RED evidence.
- The initial combined Go race command passed task/service (117.038s) and
  worktree (117.011s). The repository package hit Go's default 10-minute timeout
  while initializing a legacy-schema fixture, without assertion failures.
- The complete repository package then passed with `-trimpath -race -tags fts5
  -timeout 20m -count=1` (420.845s), using an owned temporary directory on
  /dev/shm to avoid slow disk fixture writes. That directory was cleaned up.
- SQLguard passed. Store-conformance race tests passed (55.840s). The final
  focused filesystem, scheduled-archive, and startup tests passed (2.714s).
- Scoped Go lint passed with zero issues after flattening one test assertion;
  the changed startup tests passed again under race (1.825s).
- All 69 targeted frontend tests passed. Translation generation, i18n:check,
  and i18n:ratchet passed, including the Traditional Chinese pair and pseudo.
- Managed E2E used fresh production builds: desktop 2/2 passed (12.5s), phone
  4/4 passed (19.5s). The phone case verifies archive persistence after reload.
  A focused capture rerun using the same build passed 1/1 (7.1s); visual
  inspection confirmed readable deletion copy and reachable phone controls in
  apps/web/.pr-assets/mobile-card-archive-confirmation--after-dark.png.
- Public documentation: 63 validator tests and 48 published pages passed.
  Specification validation/lint, 36 specification-linter tests, documentation
  coverage preflight, and whitespace checks passed. Final metadata checks passed
  after status synchronization.

PostgreSQL-specific execution was unavailable because KANDEV_TEST_POSTGRES_DSN
is unset; no PostgreSQL result is claimed. All work ran in the primary session.
The live instance was not restarted and its archived folder was not manually
deleted. The user subsequently requested commit, PR publication, and delivery
through merge; that delivery is in progress.

## Risks

- Uncommitted files will be intentionally lost, including in older retained
  archived checkouts. No autosave or automatic commit is introduced.
- Changing only service filtering leaves the manager's final veto in place.
- Restart wake must not repeatedly erase in-use delays or reopen cancelled jobs.
- Branch cleanup and directory cleanup are different permissions; keep both audits.

### PR review follow-up: retained owner runtime

A retained-worktree reclaim could bypass an original archive job's failed
runtime stop because borrower counting excludes the owner. Reclaim now reads
the owning task's runtime inventory before removing the checkout: existing
runtime rows defer cleanup; missing or failed inventory remains retryable.
This enforces the existing AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3 boundary.
Public docs already require runtime stops and need no additional wording.

The real-manager regression reproduced deletion after an injected owner stop
failure before the fix. After the fix, focused runtime tests passed under
race (1.844s), including successful reclaim after owner exit and missing/failed
inventory. The complete task-service race suite passed (96.256s):
`go test -trimpath -race -tags fts5 -timeout 10m ./internal/task/service -count=1`.
Specification catalog/lint and whitespace checks passed. CI, final review
resolution, and merge remain externally pending.

### PR review follow-up: phone archive causal waits

Both phone archive-preference scenarios now arm an HTTP wait for the exact
task's POST archive request before the click, require a successful response,
and assert row removal with the default render timeout. This replaces the
arbitrary 15-second DOM budget while retaining persistence-after-reload coverage.
The actual UI archive client uses HTTP, so no unrelated WS frame is accepted.

The complete phone preference spec passed against fresh managed production
builds with retries disabled: 2/2 tests passed (14.2s), using
`pnpm e2e:run --project mobile-chrome tests/task/mobile-archive-confirmation-preference.spec.ts -- --retries=0`.
Public docs and product behavior are unchanged by this test synchronization fix.
CI and merge remain externally pending.

### CI follow-up: inventory-repair archive admission

Backend shard 4 exposed a stale expectation in the inventory-repair admission
test: it required archive cleanup to preserve a dirty checkout. The focused
baseline reproduced that failure (0.928s). The test now preserves the distinct
contracts: repair leaves local edits intact, an unaudited branch identity is
refused, subsequent archive removes either clean or dirty repaired checkouts,
and both existing Git refs retain their commit identities.

The inventory-repair requirement/design now defer checkout dirtiness to the
current archive policy. No repair implementation or additional production
behavior changed, and public archive documentation already covers this policy.
The revised focused test passed (2.014s). A deliberate temporary restoration
of the archive dirty veto made its intended assertion fail (0.926s), proving
that the repaired assertion detects the old policy; production source was then
restored byte-for-byte. The complete inventory-repair race suite passed
(18.399s). Specification/catalog validation and whitespace checks passed.
CI collection, publication of this repair, and merge remain externally pending.
