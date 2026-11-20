---
id: "01-resolve-watcher-launch-defaults"
title: "Resolve watcher launch defaults and expose failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001
acceptance_criteria:
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.1
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.2
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.3
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.4
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.5
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.6
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.7
  - AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.8
system_design:
  - ../../specs/integrations/system-design/issue-watch-auto-start.md
---

# Task 01: Resolve watcher launch defaults and expose failures

## Summary

Resolve omitted watcher agent profiles through the existing task resolver before launch.
Publish real launch failures through the existing task marker, with regression coverage from dispatch to the desktop and phone board.

## In scope

- Reproduce the fallback and visibility defects with the real `serviceTaskStarter.Start` path before production edits.
- Apply the paired design's profile resolution and conditional error marking in `watcher_dispatch_wiring.go`.
- Add the three focused Go tests from the plan. Share the two browser flows between thin desktop and phone specs.
- Reuse the existing resolver, coordinator, and marker-clear tests for unchanged behavior.
- Update the integration reference and record actual verification results in this work order and the plan.

## Out of scope

Retries, old-task repair, watch `last_error`, new settings, changed resolution precedence, frontend production markup, and unrelated launch paths.

## Acceptance

1. A Sentry watch with both profile fields empty launches one session using available task defaults before a user opens the task.
2. Genuine failures publish the existing durable task marker. Capacity deferrals and concurrent active sessions do not receive that marker.
3. The three focused Go tests, existing contract tests, and two shared browser flows pass with their stated production wiring.

## Verification

The implementation followed `/tdd`: the focused tests first failed on the missing workspace fallback and missing failure marker, then passed after the adapter change. These commands record the targeted verification.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestWatcherAutoStart_' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'TestWatcherAutoStart_|TestCoordinator_|TestWatcherDispatchCoordinator_|TestResolveTaskAgentProfile_|TestCreateIssueTask_|TestSessionStartClearsAutoStartFailedMarker' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/watcher_dispatch_wiring.go apps/backend/internal/orchestrator/watcher_auto_start_defaults_test.go
```

Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Run desktop and phone E2E sequentially. The managed runner builds current product code.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/integrations/watcher-auto-start-defaults.spec.ts tests/task/task-auto-start-failed-icon.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-watcher-auto-start-defaults.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run this local PR-documentation preflight. The production path activates reference validation during the design-only checkpoint.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/issue-watch-auto-start.md',
  'docs/specs/integrations/system-design/issue-watch-auto-start.md',
  'docs/plans/issue-watch-auto-start-defaults/plan.md',
  'docs/plans/issue-watch-auto-start-defaults/task-01-resolve-watcher-launch-defaults.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/orchestrator/watcher_dispatch_wiring.go', status: 'modified' },
    ...paths.map(filename => ({ filename, status: 'added' })),
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/backend/internal/orchestrator/watcher_dispatch_wiring.go`
- `apps/backend/internal/orchestrator/watcher_auto_start_defaults_test.go` (new)
- `apps/web/e2e/tests/integrations/watcher-auto-start-flow.ts` (new)
- `apps/web/e2e/tests/integrations/watcher-auto-start-defaults.spec.ts` (new)
- `apps/web/e2e/tests/integrations/mobile-watcher-auto-start-defaults.spec.ts` (new)
- `docs/public/integrations.md`
- This work order and its plan, plus paired specification statuses after successful implementation.

## Dependencies

None. The work order is complete.

## Risks

Use existing context and atomic marker helpers. Preserve workflow targets, explicit runner intent, and capacity queues.
Use current Sentry mock routes. Do not seed `auto_start_failed` as a substitute for the watcher failure.

## Parallelism

`sequential`

## Inputs

- [Requirement and acceptance criteria](../../specs/integrations/requirements/issue-watch-auto-start.md)
- [System design](../../specs/integrations/system-design/issue-watch-auto-start.md)
- [Plan, reproduction evidence, and complete test matrix](plan.md)
- `watcher_dispatch_test.go`, `session_ensure_test.go`, `ceiling_seam1_integration_test.go`, and `event_handlers_streaming_test.go`
- `apps/web/e2e/tests/task/task-auto-start-failed-icon.spec.ts` and `apps/backend/internal/sentry/mock_controller.go`

## Results

Implemented the watcher fallback and failure visibility in the existing adapter. Omitted agent profiles use `resolveTaskAgentProfile`; an empty fallback is passed to `StartTask` so explicit session-target routing can select its source profile before the existing launch path reports a genuinely unresolved profile. Genuine launch errors reuse `setTaskAutoStartFailedMarker`. Capacity deferrals and active sibling sessions retain their existing behavior. No new profile policy or UI markup was added.

The three focused Go regression functions, shared desktop/phone watcher flows, and public integration reference are complete.

Verification on 2026-11-16:

- The focused Go tests failed before the production change for the expected fallback and missing-marker behavior, then passed after the change. The race-enabled watcher, coordinator, resolver, task-creation, and marker-clear contract set passed.
- In initial implementation validation, the scoped lint wrapper reported zero issues but exceeded its fixed five-minute timeout. The equivalent direct package lint passed with `0 issues`; the review follow-up scoped wrapper also completed with `0 issues`.
- Desktop Chromium passed 4 tests, including the new watcher flows and existing task-marker checks. Mobile Chrome passed 2 watcher-flow tests. Both managed runs built the backend and Vite E2E assets.
- Public-doc validation tests passed (63 tests), and all 55 public pages validated. The PR-documentation preflight returned `covered` with no reference errors.
- The specification catalog validated 454 decisions and 2,023 specifications; all 36 spec-linter tests and the full spec lint passed.
- `git diff --check` passed.

Review follow-up: an explicit target owns profile selection even when an unusable workspace default is available. The adapter clears the fallback for effective steps with a `SessionTarget`, allowing `StartTask` to resolve and validate the earlier source step before validating a fallback. The regression uses no watch, workflow, or existing session profile, an unusable workspace default, and an earlier fixed-profile target; it asserts the source profile launches with no failure marker. The existing unresolved-profile case still asserts no session and a failure marker. The shared browser flow now requires the launched session to reach `STARTING` or `RUNNING` while preserving its profile assertion.

Review follow-up verification on 2026-11-16:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestWatcherAutoStart_' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'TestWatcherAutoStart_|TestCoordinator_|TestWatcherDispatchCoordinator_|TestResolveTaskAgentProfile_|TestCreateIssueTask_|TestSessionStartClearsAutoStartFailedMarker' -count=1)`: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/integrations/watcher-auto-start-defaults.spec.ts tests/task/task-auto-start-failed-icon.spec.ts)`: 4 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-watcher-auto-start-defaults.spec.ts)`: 2 passed.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/watcher_dispatch_wiring.go apps/backend/internal/orchestrator/watcher_auto_start_defaults_test.go`: `0 issues`.
- `python3 scripts/list-docs.py validate`: 454 decisions and 2,023 specifications validated; `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: passed.
- Both managed E2E runs built the backend and Vite assets. `gofmt -l` and Prettier checks passed.
