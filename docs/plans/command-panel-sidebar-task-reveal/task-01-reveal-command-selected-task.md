---
id: "01-reveal-command-selected-task"
title: "Reveal command-selected sidebar task"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001
acceptance_criteria:
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.1
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.2
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.3
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.4
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.5
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.6
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.7
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.8
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.9
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.10
system_design:
  - ../../specs/ui/system-design/command-panel-sidebar-task-reveal.md
---

# Task 01: Reveal command-selected sidebar task

## Acceptance

- The desktop regression test fails before production changes because Cmd+K navigation leaves the
  chosen rendered task row outside the overflowing sidebar viewport, then passes after the fix.
- Cmd+K task selection navigates normally and reveals an available row with nearest-block scrolling
  without moving focus, scrolling the document, or changing sidebar view/collapse preferences.
- Guarded navigation can outlive the initial reveal retry budget without losing the queued reveal;
  a newer selection supersedes an older pending reveal.
- Phone Cmd+K task navigation remains direct and does not target the hidden desktop sidebar.

## Verification

1. `cd apps && pnpm install --frozen-lockfile`
2. RED, before production changes: `cd apps/web && pnpm e2e:run --host --no-build tests/task/sidebar-scroll-preservation.spec.ts --grep "after a delayed settings navigation blocker"`
3. Unit: `cd apps/web && pnpm exec vitest run lib/sidebar/task-navigation.test.ts`
4. Typecheck: `cd apps/web && pnpm exec tsc --noEmit`
5. Build: `cd apps && pnpm --filter @kandev/web build:vite`
6. Desktop GREEN: `cd apps/web && pnpm e2e:run --host --no-build tests/task/sidebar-scroll-preservation.spec.ts`
7. Mobile GREEN: `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-command-panel-task-navigation.spec.ts`

Confirm Playwright discovers the expected focused tests before treating either browser command as
evidence. The managed runner performs the required production build and teardown.

## Files likely touched

- `apps/web/lib/sidebar/task-navigation.ts`
- `apps/web/lib/sidebar/task-navigation.test.ts`
- `apps/web/hooks/use-command-panel-task-navigation.ts`
- `apps/web/components/command-panel.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`
- `apps/web/e2e/tests/task/mobile-command-panel-task-navigation.spec.ts`

## Dependencies

None.

## Parallelism

Sequential; the browser regression, navigation helper, command-panel integration, and responsive
guard form one TDD vertical slice.

## Inputs

- Behavioral contract: `docs/specs/ui/requirements/command-panel-sidebar-task-reveal.md`.
- Root-cause trace and frontend design: `plan.md`.
- Retryable navigation precedent: `apps/web/lib/review/navigation.ts` and its unit test.
- Existing overflowing-sidebar fixture: `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`.
- Mobile composition precedent: `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`.

## Output contract

Report the RED failure, implementation summary, actual files changed, exact test commands and
counts, generated artifact paths, cleanup evidence, blockers, and remaining risks. Mark this task
`done`, check it in `plan.md`, and replace the plan/task verification placeholders only after all
targeted checks pass.

## Results

Implemented the bounded visible-sidebar reveal helper, latest-request cancellation, post-navigation
command-panel queue, task-row marker, and desktop/mobile coverage. The RED supersession unit test
and delayed-blocker desktop regression failed before their respective fixes and passed afterward.

- Unit: 1 file, 9 tests passed; task-navigation hook and command-panel consumers: 3 files, 24 tests passed.
- Typecheck and Vite build: passed.
- Desktop full sidebar-scroll E2E: 8 passed, including delayed guarded navigation and above-viewport reveal.
- Mobile focused E2E: 1 passed under `mobile-chrome`.
- Focused Prettier and ESLint (`--max-warnings 0`): passed.
- `git diff --check`: passed.
- Generated E2E output is disposable and excluded from the change; no security or trust-boundary
  changes were introduced.


PR #3598 CI follow-up, 2026-10-06: run `37450559149`, shard 11 job
`112233810114`, exposed an assertion that sampled the 1,400 ms reveal cue
after waiting for route hydration and active-row settlement. Both reveal
scenarios now observe the cue concurrently with command selection, before
checking the destination URL, active identity, viewport containment, and
unchanged document scroll. The existing one-second cue assertion deadline
and production cue duration are unchanged.

`cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/sidebar-scroll-preservation.spec.ts tests/lsp/lsp-file-intelligence.spec.ts -- --retries 0 --trace=retain-on-failure`: all 27 cases passed in 9.8 minutes, including all eight sidebar-scroll cases, delayed guarded navigation, and above-viewport selection. `cd apps/web && pnpm exec vitest run lib/sidebar/task-navigation.test.ts`: 19 tests passed. Full web lint and typecheck, focused Prettier/ESLint, and whitespace checks passed. No production navigation change or deadline increase; exact pushed-head CI remains pending.


The same follow-up updates this legacy work order's `spec` frontmatter to the
current requirement/acceptance/design contract. Hosted documentation coverage
rejected the obsolete field on `73a04b32bb4`; the exact local coverage
preflight failed with that field and passed after migration across all 50
changed work orders. This metadata correction changes no implementation scope
or product behavior.

### CI follow-up: observe the complete reveal transition

The browser shard failed because its one-second class assertion began before
command selection completed. The fixture now installs a narrowly scoped DOM
observer before selection and records the target row's actual reveal class.
Route, active identity, viewport containment, and document-scroll assertions
remain unchanged. Observation disconnects and its handle is disposed on exit.

Validation used the managed runner from the repository root:
`TMPDIR=/root/.cache/kandev-pr3598-e2e-tmp GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/task/sidebar-scroll-preservation.spec.ts -- --repeat-each=3 --retries=0 --trace=retain-on-failure`.
All 24 cases passed. The phone command-navigation case passed three independent
runs with retries disabled. Focused ESLint and web typecheck passed.

A temporary 1.5-second option animation reproduced the original failure; the
observer passed under the same delay. A temporary negative probe suppressed
only the reveal class and failed after route, identity, and viewport assertions
passed. Both probes were removed. Production timing and navigation are unchanged.
Fresh pushed-head CI remains required.
