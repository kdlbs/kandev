---
id: "02-task-pr-badge"
title: "Share the PR conflict glyph across surfaces"
status: complete
wave: 2
depends_on:
  - "01-conflict-observation"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.5
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.6
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.7
system_design:
  - ../../specs/integrations/system-design/github-pr-conflict-indicator.md
---

# Task 02: Share the PR conflict glyph across surfaces

## Summary

Add a top-right conflict bubble to one GitHub PR glyph used by task rows and
the top bar. Keep current status colors and automation signals visible. The
single-PR top bar uses only that leading glyph and its PR number.

## In scope

- Derive one warning from full PR data or the compact summary fallback.
- Extract one presentational glyph component for task-row and top-bar sizes;
  move auto-merge to lower-right where automation dots are supplied.
- Use it in the sidebar, Home Kanban card, pipeline row, rich task list,
  single and multi-PR top bar, per-PR top-bar menu rows, and phone task switcher.
- Remove the single-PR trailing top-bar status icon; keep the multi-PR chevron
  and existing top-bar actions.
- Add localized status and conflict text to each complete indicator's
  accessible name.

## Out of scope

- Provider sync, persistence, merge behavior, and unrelated provider icons.

## Acceptance

- CI failure or draft styling and the conflict bubble appear together when
  the independent conflict observation is true.
- Warning, auto-fix, auto-merge, count, and glyph remain discernible together.
- Failing CI color and conflict bubble appear together on the top bar's
  leading icon, with no trailing status icon. The single-PR button keeps one
  accessible name with localized PR status and detail activation; multi-PR
  buttons retain count, color, and chevron while menu rows identify individual
  conflicts.
- The existing hover disclosures, top-bar actions, phone row action, and
  touch drawer work.

## ASCII UI preview

`UI-01: Task PR indicator`, excerpt from [the full preview](plan.md#ascii-ui-preview).
The same glyph is used in `UI-02: Phone task switcher` and `UI-03: Task top bar`.

```text
Desktop or phone row:  Task title        PR(red)!
Glyph corners:        o  !  (auto-fix, conflict)
                       PR
                         o  (auto-merge)
Top bar:             [ PR(red)!  #3932 ]
Multi-PR menu:         PR(red)!  #3932
                       PR(green) #3933
```

The corner placement and simultaneous visibility are structural requirements
for AC-001.1, .2, .3, .6, and .7. Text and spacing are illustrative.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-task-icon.test.ts components/github/pr-task-icon.render.test.tsx components/github/pr-status-refresh-routes.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-topbar-popover.spec.ts -- --grep "single badge shows a conflict bubble")
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/pr/pr-multi-popover.spec.ts -- --grep "aggregate warning and menu rows identify only the conflicted open PR")
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts -- --grep "tapping the chip opens the drawer with PR CI content")
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts)
```

Run `pnpm install --frozen-lockfile` from `apps/` before the first pnpm command
in a fresh worktree. Inspect the rendered phone screenshot against `UI-02`.

## Files likely touched

- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon-disclosure.tsx`
- `apps/web/components/github/pr-status-glyph.tsx`
- `apps/web/components/github/pr-status-glyph.test.tsx`
- `apps/web/components/github/pr-topbar-button.tsx`
- `apps/web/components/integrations/change-request-status-chrome.tsx`
- `apps/web/components/github/pr-task-icon.render.test.tsx`
- `apps/web/components/github/pr-task-icon-draft.test.ts`
- `apps/web/components/github/pr-status-refresh-routes.test.tsx`
- `apps/web/lib/task-pr-info.test.ts`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/pr/pr-topbar-popover.spec.ts`
- `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
- `apps/web/e2e/tests/kanban/pipeline-view.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`

## Dependencies

Task 01 supplies the conflict observation and compact projection.

## Risks

The 14-pixel task glyph has little corner space. Badge geometry and translated
accessible names need direct rendered checks. The top-bar button/menu behavior
must survive removal of the single-PR trailing status icon.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-pr-conflict-indicator.md)
- [System design](../../specs/integrations/system-design/github-pr-conflict-indicator.md)
- Existing automation dots, top-bar status, and desktop/phone PR indicator tests.

## Results

The shared glyph is used by task contribution icons, Home Kanban cards and
pipeline rows, the single/multi-PR topbar, and menu rows. Compact accessible
labels use neutral wording when aggregate failure or pending does not identify
its cause, and do not claim merge readiness from an open/ready bucket without
check evidence; the awaiting-review bucket retains its specific review label.
All 95 focused component tests, typecheck, ESLint, i18n checks, production
build, and specification validation pass. Desktop single- and multi-PR
Playwright scenarios pass. Phone drawer and task-switcher scenarios pass,
including conflict and automation coexistence, detail disclosure, focus
return, and conflict cleanup.

PR fixup keeps checks and review claims on fully hydrated PRs only while they
are open. It also announces known pending-check counts, count-only check
progress, branch-protection blocking, and behind-base status. The compact
fallback exposes localized status, conflict, and automation labels while
keeping bounded wording. Verification passed with
`cd apps/web && pnpm exec vitest run components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.test.ts components/github/pr-task-icon-conflicts.test.ts`
(95 tests), `pnpm run typecheck`, ESLint on the changed components and PR specs,
and `pnpm run i18n:check && pnpm run i18n:ratchet`. The documentation-coverage
test suite passed (82 tests), and all specifications passed validation.

PR fixup browser checks passed in the managed harness:

- `pnpm e2e:run --project chromium tests/pr/pr-topbar-popover.spec.ts -- --grep 'single badge shows a conflict bubble'`
- `pnpm e2e:run --project chromium tests/pr/pr-status-badge.spec.ts -- --grep 'shows and clears one conflict warning'`
- `pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`

The top-bar scenario confirms an explicit conflict observation survives mock
PR hydration. The cross-surface scenario confirms conflict appears and clears
in the task sidebar, Home card, and pipeline row. The phone scenario confirms
automation indicators, conflict state, detail disclosure, and focus behavior
remain intact.

The PR fixup consolidated the duplicate top-bar UI requirement/design/plan
into the GitHub-owned requirement and design. The single-PR layout and
accessible status, multi-PR count and chevron, menu behavior, and existing
phone drawer remain covered by AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.3,
.6, and .7.

Post-fixup documentation checks pass:

- `node --test .github/scripts/pr-docs.test.cjs` (81 tests).
- `python3 scripts/list-docs.py validate` (305 decisions and 1,155
  specifications validated).
- `python3 scripts/lint-spec-files.test.py` (36 tests).
- `python3 scripts/lint-spec-files.py --all`.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py` (7 tests).
- `git diff --check`.
