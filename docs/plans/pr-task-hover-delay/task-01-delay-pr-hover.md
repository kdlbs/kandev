---
id: "01-delay-pr-hover"
title: "Delay task PR hover disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
acceptance_criteria:
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.26
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.27
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.28
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.29
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
---

# Task 01: Delay task PR hover disclosure

## Summary

Suppress PR popups during brief pointer crossings.
Preserve immediate keyboard and touch access, defer hydration until actual disclosure,
and hide the Automation section when no open PR has an enabled automation option.

## In scope

- Add an opt-in mouse delay with the existing shared hover helper.
- Connect the GitHub task icon to that delay.
- Filter automation details to open PRs with auto-fix or auto-merge enabled.
- Add hook, component, and desktop browser regressions.
- Run existing mobile drawer checks and update affected timing assumptions.

## Out of scope

Global tooltip defaults, other providers, layouts, provider refresh, status derivation, and settings.

## Acceptance

1. Delayed mouse disclosure satisfies AC 001.26, including cancellation and independent icons.
2. Focus, touch, content transfer, scrolling, and Escape satisfy AC 001.27.
3. Hydration starts once per actual disclosure and satisfies AC 001.28 without callback-rerender duplication.
4. Automation details include only open PRs with an enabled auto-fix or auto-merge option; omit the section when none qualify. Disclosure fetches missing settings even when PR records are already cached.

## ASCII UI preview

See the [complete previews](plan.md#ascii-ui-preview).

```text
UI-01 desktop: Task [PR] -> remain 500 ms -> summary
                         -> leave early -> no summary
UI-02 phone:   Task [PR] -> tap -> existing bottom drawer
                                  fixed header / scroll body
```

Timing and input paths are required by AC 001.26 through 001.29.
Preserve the existing content hierarchy and geometry in both views.

## Implementation sequence

1. Add the failing automation-summary filtering regression.
2. Add the failing 499/500 ms hook regression and cancelled-entry cases.
3. Add the delay option with a zero default. Set 500 ms only in `PRTaskIconView`.
4. Remove forced immediate opening from delayed mouse entry. Preserve immediate visible keyboard focus.
5. Trigger hydration only on actual opening. Preserve Escape suppression and timer cleanup.
6. Add rendered hydration and multi-icon browser regressions. Update existing immediate-hover test assumptions.
7. Run the commands below and record their actual results.

## Verification

Run dependency installation once if this worktree has no workspace install.
Run these commands from the repository root. Managed E2E commands rebuild their assets.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/use-task-icon-tooltip-state.test.ts hooks/domains/github/use-hover-popover.test.ts components/github/pr-task-icon.automation.test.ts components/github/pr-task-icon.automation-render.test.tsx components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.hover-render.test.tsx hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/use-task-icon-tooltip-state.ts components/task/use-task-icon-tooltip-state.test.ts components/github/pr-task-icon.tsx components/github/pr-task-icon.render.test.tsx e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/pr/pr-sidebar-hover-hydration.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use fake timers for exact boundaries and browser-controlled time for the new pass-through scenario.
Inspect the desktop and phone captures against UI-01 and UI-02.
Include any additionally edited test files in ESLint before completion.

## Files likely touched

- `apps/web/components/task/use-task-icon-tooltip-state.ts`
- `apps/web/components/task/use-task-icon-tooltip-state.test.ts`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon.render.test.tsx`
- `apps/web/components/github/pr-task-icon.negative-projection.test.tsx`
- `apps/web/components/github/pr-task-icon.workflow-approval.test.tsx`
- `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts`

## Dependencies

None. Reuse `useHoverPopover` and the existing provider-neutral alias.
Run the existing mobile spec without a new mobile surface.

## Risks

The forced opening bypasses the timer. Pointer and focus events can overlap.
Hydration callbacks change identity during rerenders. Tests must cover these boundaries.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/pr-task-status-summary.md), AC 001.26 through 001.28.
- [Disclosure timing design](../../specs/ui/system-design/pr-task-status-summary.md#disclosure-timing).
- `apps/web/components/integrations/use-hover-popover.ts`.
- Existing hook tests, GitHub icon render tests, and desktop/mobile PR disclosure E2E fixtures.

## Results

Completed on 2026-10-09.

- Added the 500 ms pointer delay with immediate keyboard/touch access, delayed hydration, and Escape/timer cleanup coverage.
- Filtered Automation details to open PRs with enabled auto-fix or auto-merge settings, including the cached-PR hydration path.
- Corrected stale task-PR cache scope after workspace-context generation changes and added a regression.
- Focused Vitest suite passed: 9 files, 135 tests. Typecheck, targeted ESLint, i18n ratchet, and the production web build passed.
- Desktop PR E2E passed: 14 tests across the sidebar hydration and multi-PR popover specs. Mobile PR E2E passed: 12 tests across automation indicators and the CI drawer.
- Catalog validation, full specification lint, and `git diff --check` passed.

### PR fixup results

- Reproduced hidden pending Automation details and uncancelled hover opening with failing rendered tests before fixing them.
- Added pending Escape listener cleanup, editable-target, and subsequent keyboard-focus coverage. Moved rendered hover cases into `pr-task-icon.hover-render.test.tsx` to retain lint limits.
- The settings-loading regression holds its response pending, then verifies disabled automation is omitted. Mobile and desktop browser scenarios exercise the same outcome.
- Replaced the hydration helper's one-time store read with `expect.poll` over current workspace, PR, and matching automation evidence.
- Final focused Vitest command above, plus `components/github/pr-ci-popover.automation.test.tsx`, `components/github/pr-status-refresh-routes.test.tsx`, and `components/github/pr-status-chip.test.tsx`: 10 files, 140 tests passed.
- Typecheck, targeted ESLint (`--max-warnings 0`) across all modified TS/TSX files, and i18n ratchet passed.
- Managed Chromium sidebar command above: 5 passed. Managed Mobile Chrome automation-indicator command above: 4 passed. Captures cover the loading state in both viewports.
- Catalog validation, full specification lint, and `git diff --check` passed. Remote checks remain pending at the remediation head.
