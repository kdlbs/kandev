---
id: "01-guard-root-commit-revert"
title: "Guard root commit revert"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-ROOT-REVERT-001
acceptance_criteria:
  - AC-WORKSPACES-ROOT-REVERT-001.1
  - AC-WORKSPACES-ROOT-REVERT-001.2
  - AC-WORKSPACES-ROOT-REVERT-001.3
system_design:
  - ../../specs/workspaces/system-design/root-commit-revert-availability.md
---

# Task 01: Guard root commit revert

## Summary

Carry existing first-parent evidence through the Changes row types and hide Revert only for known parentless local commits. Preserve all other action eligibility and repository routing on both history paths.

The frontend slice is complete. The additional producer slice corrects the tracker notification sent immediately after `GitOperator.Commit`; an independent real-producer diagnostic found root error text in `parent_sha` while an ordinary commit's parent matched the Git graph.

## In scope

- Independently author a rendered causal regression and passing ordinary repository-dispatch control before production edits; run them and record actual RED and control results.
- Apply the smallest shared eligibility guard and cover unified/separate mappings, both history row adapters, root, parented, unknown, older, pushed and provider-only cases, and Amend/Reset controls.
- Exercise real desktop hover and context menu, desktop coarse pointer and mobile long-press using focused Playwright tests with real rendered controls. The shared existing row/menu composition remains unchanged.
- Independently author a neighboring Go regression for the real commit operator and tracker subscription: root, ordinary, and merge first-parent notifications. Fix only the successful parent lookup used by that producer.

## Out of scope

- Changing Git history or refs, new root undo, copy changes, broad refactoring, full suites, and private qualification fixtures.

## Acceptance

- Known parentless local rows expose no Revert action or empty revert-only action container; independently eligible Amend and Reset remain available.
- Latest parented and unknown-parent local rows keep prior eligibility and selected SHA/repository dispatch; older, pushed and provider-only restrictions remain unchanged in both history paths.
- Focused rendered RED/control, GREEN, desktop/coarse/phone browser checks and scoped gates have recorded actual outcomes; limitations are explicit.
- Successful commit notifications carry an empty parent for root and Git's actual first parent for ordinary and merge commits; failed parent queries are not claimed as authoritative ancestry.

## Rendered behavior

See the [plan](plan.md#rendered-behavior). The existing desktop row and context menu and the touch long-press menu retain their structure; only known-root Revert is omitted (AC .1-.3).

## Verification

Only after ROOT fully reads the four artifacts and sends a later separate explicit implementation and heavy-resource release. Use pnpm 9.15.9 ahead of Node 24.21.0 on PATH, with Go 1.26.0 when needed. In a fresh worktree install once from `apps/` using `pnpm install --frozen-lockfile`. Run sequentially; retain every native session handle through actual join and exit. The task-owned quiet directory is `/tmp/kandev-root028-revert-checks`; set `KANDEV_RUN_QUIET_DIR` for each wrapper invocation.

For the additional producer slice, wait for ROOT's full review of this delta and a later separate corrective implementation and heavy-resource release. Independently author `apps/backend/internal/agentctl/server/process/git_commit_parent_notification_test.go`. Before changing the producer, run the exact test below and record a root-specific failure plus the ordinary first-parent control. Include a real two-parent merge control. After the smallest producer correction rerun the same test to GREEN, then run the canonical changed-Go lint and documentation checks. Keep GOTOOLCHAIN local, Go 1.26.0, and the test resource bounds; lint uses its canonical 3 GiB default rather than the test budget. The previously passing browser, capture, frontend unit, and typecheck results need not be repeated for this backend-only correction. Earlier captures prove the UI guard with seeded commit records; they do not prove the later backend notification or a newly built backend.

```bash
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB KANDEV_RUN_QUIET_DIR=/tmp/kandev-root028-revert-checks ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p 1 ./internal/agentctl/server/process -run '^TestGitCommitParentNotification$' -count=1 -v)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/git.go apps/backend/internal/agentctl/server/process/git_commit_parent_notification_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The producer query's failure behavior remains as it was: combined error output is forwarded, and an empty failed output can still be misread as a root by the existing string-only event. Do not treat failure as a successful ancestry observation, invent a parent, introduce a new wire flag or backfill, or broaden Git operation scope. Eliminating the ambiguous failure case while preserving notification behavior requires a separate representation or recovery path; stop and explain that boundary before widening.

The frontend slice already completed the following independent RED/control and GREEN checks; keep these commands as its recorded verification, without replaying them solely for the producer correction:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/task/root-commit-revert.test.tsx)
```

After the fix run that same command to GREEN, then the following focused checks from the repository root:

```bash
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/commit-row.tsx components/task/changes-panel-helpers.ts components/task/root-commit-revert.test.tsx e2e/tests/git/root-commit-revert.spec.ts e2e/tests/git/mobile-root-commit-revert.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
KANDEV_RUN_QUIET_DIR=/tmp/kandev-root028-revert-checks scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/git/root-commit-revert.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/tmp/kandev-root028-revert-checks scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project mobile-chrome tests/git/mobile-root-commit-revert.spec.ts -- --workers=1 --retries=0
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The desktop managed E2E run builds the current source. Use the mobile `--no-build` command only after proving the exact production inputs and dist fingerprint still match that build; otherwise rebuild before mobile. Add any shared E2E helper to the scoped ESLint command if created. Use CDP touch events and the repository's `dwell` long-press pattern for actual phone/coarse context menus; ordinary tap cannot hold. Capture only actual UI. Run the staged i18n ratchet and normal active hooks during authorized delivery; include new frontend paths when staged. Record actual join/exit and known current owned process absence. Do not run Vitest through `run-quiet --summary`.

## Files likely touched

- `apps/web/components/task/changes-panel-helpers.ts`, `apps/web/components/task/commit-row.tsx`.
- `apps/web/components/task/root-commit-revert.test.tsx`.
- `apps/web/e2e/tests/git/root-commit-revert.spec.ts`, `apps/web/e2e/tests/git/mobile-root-commit-revert.spec.ts`, and a shared sibling helper only if needed.
- `apps/backend/internal/agentctl/server/process/git.go` and `apps/backend/internal/agentctl/server/process/git_commit_parent_notification_test.go` for the producer correction.
- This plan, work order, and paired requirement/design for status and results.

## Dependencies

None within this package; implementation and local heavy work require the later explicit release.

## Risks

The local mappers currently spread data but their declared types omit ancestry. Keep omitted fields unknown. Backend HEAD verification still handles races. Browser proof requires fresh production assets and genuine pointer input.

## Parallelism

Sequential.

## Inputs

- [Requirement](../../specs/workspaces/requirements/root-commit-revert-availability.md) and [system design](../../specs/workspaces/system-design/root-commit-revert-availability.md).
- Existing `commit-row.test.tsx`, `changes-panel-remote.test.ts`, `git-changes-panel.spec.ts`, and `mobile-subtask-reparent-drag-drop.spec.ts` patterns.

## Results

- Frozen workspace install: passed, pnpm 9.15.9.
- Independent rendered RED: root displayed two Revert controls; ordinary selected-repository dispatch passed (1 failed, 1 passed). After the fix the focused suite passed 7 tests, including both history adapters.
- Existing affected component/mapping tests: 29 passed. Scoped ESLint and 6 GiB canonical typecheck passed. i18n check passed with no new copy.
- Managed fresh-build desktop Chromium: 2 passed, covering hover, context menu, and coarse-pointer long-press. Mobile Chrome on the identical fingerprinted build: 1 passed, covering phone long-press. Capture reruns of those same flows also passed (2 desktop, 1 phone).
- Specification catalog and linter passed; no public docs update was needed because the existing Git operations page already describes `HEAD~1` and the selected-repository contract remains unchanged.
- Subsequent temporary independent producer diagnostic: root notification contained the `HEAD~1` Git error text, while the ordinary notification matched its actual parent. Temporary checkout copy was checksum-verified and removed.
- Independently authored producer regression before correction: root failed with Git error text; ordinary and real two-parent merge controls passed. After the correction the bounded race-enabled test passed all three cases. Canonical changed-Go lint reported zero issues. Query-failure ambiguity remains as described above.
