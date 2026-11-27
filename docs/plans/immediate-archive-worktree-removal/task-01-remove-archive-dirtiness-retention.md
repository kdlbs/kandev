---
id: "01-remove-archive-dirtiness-retention"
title: "Remove archive dirtiness retention"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.1
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.2
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.4
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.6
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 01: Remove Archive Dirtiness Retention

## Summary

Implement immediate removal of eligible managed worktrees and task roots on
archive, including uncommitted files. Update retained-job compatibility and
archive summaries in the same change so runtime behavior and promises agree.

## In scope

- Remove filterDirtyWorktreesForArchive and archive-only dirty-error swallowing.
- Remove RequireCleanCheckout from CleanupWorktreesPreservingBranches and
  CleanupArchivedWorktree, retaining their branch policy and identity tuples.
- Remove archiveReclaimWorktreeIsDirty and dirty-deferral branches when unused.
  Keep non-archive dirty inspection and delete consent intact.
- Add bounded startup promotion of legacy waiting archive_reclaim jobs in the
  repository and service worker. Preserve snapshots, attempt counts, cancelled
  states, archive fencing, and normal retry deadlines.
- Keep task-root capture and completion connected to original and reclaim jobs.
- Replace conflicting preservation tests; add the real filesystem RED test
  named in the plan before production edits, then implement GREEN.
- Update archive locale copy, unit/rendered tests, and desktop/phone E2E.
- Update public how-to sections in tasks-and-workflows.md, git-operations.md,
  and operations.md; inspect sessions-and-review.md, README.md, and
  docs/screenshots.md for obsolete wording. Only change relevant statements.
- Promote paired draft requirements/design to active/current after checks pass,
  mark this work order done, and record the plan implemented with actual results.

## Out of scope

New consent controls or feature flags, new schema, delete policy, independent
adoption consent, remote executor teardown, storage default changes, and
unchecked deletion of the user's existing folder.

## Acceptance

1. The real archive service removes dirty managed checkouts and the final task
   root in one successful cleanup attempt, including script-created changes,
   across direct/cascade/scheduled entry points; normal branch and delete
   consent tests still pass (004.1-.4/.7).
2. Existing waiting jobs become eligible after startup without the old delay;
   restart, mixed shared inventory, stopped-runtime guards, owner changes,
   root failure retry, and unarchive cancellation remain correct (004.3/.5/.7).
3. Single/bulk/compact/phone summaries describe deletion in all locales, with
   the existing confirmation preference and mobile controls working; public
   docs agree with the shipped behavior (004.6).

## ASCII UI preview

[Combined view](plan.md#ascii-ui-preview).

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

## Verification

Run from the repository root. For a fresh worktree first run
`(cd apps && pnpm install --frozen-lockfile)`. Regenerate translations using
`(cd apps/web && pnpm run i18n:zh-hant)` and
`(cd apps/web && pnpm run i18n:pseudo)` after the source catalog edits.
Capture the expected filesystem assertion failure for
TestArchiveTaskRemovesDirtyWorktreeAndTaskDirectory before implementation:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestArchiveTaskRemovesDirtyWorktreeAndTaskDirectory$' -count=1 -v)
```

After implementation run:

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

Also run the local .github/scripts/pr-docs.cjs validateCoverage preflight over
all changed paths and referenced plan/requirement/design documents. Keep
specification and work-order IDs resolvable. PostgreSQL execution is conditional
on KANDEV_TEST_POSTGRES_DSN; record availability explicitly.

## Files likely touched

- apps/backend/internal/task/service/service_tasks.go
- apps/backend/internal/task/service/resource_cleanup_jobs.go
- apps/backend/internal/task/service/service.go (startup cursor state, if needed)
- apps/backend/internal/task/repository/sqlite/resource_cleanup.go
- apps/backend/internal/worktree/manager_cleanup.go
- apps/backend/internal/task/service/archive_immediate_worktree_test.go (new)
- apps/backend/internal/task/service/archive_dirty_worktree_test.go
- apps/backend/internal/task/service/resource_cleanup_archive_reclaim_test.go
- apps/backend/internal/task/service/resource_cleanup_taskdir_test.go
- apps/backend/internal/task/service/resource_cleanup_archive_wake_test.go (new)
- apps/backend/internal/task/repository/sqlite/resource_cleanup_archive_wake_test.go (new)
- apps/backend/internal/worktree/manager_cleanup_recovery_test.go
- apps/web/components/task/task-cleanup-summary.ts and task-cleanup-summary.test.ts
- apps/web/components/task/task-archive-confirm-dialog.test.tsx
- apps/web/components/task/task-archive-confirmation.test.tsx
- apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json
- apps/web/e2e/tests/task/archive-confirmation-preference.spec.ts
- apps/web/e2e/tests/task/mobile-archive-confirmation-preference.spec.ts
- docs/public/{tasks-and-workflows,git-operations,operations}.md
- docs/specs/tasks/{requirements,system-design}/dirty-worktree-archive.md
- This work order and plan.md for final results and status.

## Dependencies

None. Read scoped backend/web AGENTS.md, /tdd, /e2e, /mobile-parity, and
/docs-maintainer before implementation. No subagent authorization is implied.

## Risks

Local files are intentionally deleted. Archive-specific options must not change
branch deletion or independent-checkout adoption authority. Startup waking must
be bounded and leave fresh in-use backoff intact. All cleanup targets require
current ownership and pinned path proof.

## Parallelism

sequential

## Inputs

- [Requirement 004](../../specs/tasks/requirements/dirty-worktree-archive.md).
- [Archive system design](../../specs/tasks/system-design/dirty-worktree-archive.md).
- [Policy decision](../../decisions/2026-10-17-immediate-archive-worktree-removal.md).
- Existing real-manager fixtures in archive_dirty_worktree_test.go and
  root/restart tests in resource_cleanup_taskdir*.go.

## Results

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
