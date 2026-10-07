---
id: "01-prioritize-options"
title: "Prioritize selected filter options"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILTER-SELECTED-FIRST-001
acceptance_criteria:
  - AC-UI-FILTER-SELECTED-FIRST-001.1
  - AC-UI-FILTER-SELECTED-FIRST-001.2
  - AC-UI-FILTER-SELECTED-FIRST-001.3
  - AC-UI-FILTER-SELECTED-FIRST-001.4
  - AC-UI-FILTER-SELECTED-FIRST-001.5
system_design:
  - ../../specs/ui/system-design/filter-selected-options-first.md
---

# Task 01: Prioritize Selected Filter Options

## Summary

Implement selected-first ordering in the shared view-filter multi-select picker
with stable group context. Prove the result with targeted TDD and desktop/phone
browser evidence in one delivery pass.

## In scope

Own `filter-option-groups.ts`, its tests, `filter-multi-select.tsx`, the extracted
`filter-multi-select-options.tsx`, a new
component test, and the two focused E2E specs. Preserve existing search ranking,
controlled toggle behavior, retained unknown values, and the current responsive
surfaces. Update this work order and plan results after checks pass.

## Out of scope

Backend, stores, query semantics, single-select sorting, source deduplication,
new translated labels, and unrelated UI refactors.

## Acceptance

1. Stable partition and actual rendered global selected-first order satisfy .1
   and .2, including selections spanning workflow groups.
2. Controlled selection, unavailable IDs, keyboard focus, search, and reopen
   behavior satisfy .3 and .4 without mutation on opening or reordering.
3. Desktop and phone browser evidence satisfy .5 and the preview structures.

## ASCII UI preview

UI-01: Repository multi-select (shared option structure on phone).

```text
+-------------------+
| Search...         |
| [x] Repository B  |
| [x] Repository D  |
|-------------------|
| [ ] Repository A  |
| [ ] Repository C  |
+-------------------+
```

Use [the full previews](plan.md#ascii-ui-preview), including UI-02 grouped
context and UI-03 phone drawer entry. Option order is structural; illustrative
labels/spacing are not new UI copy. Search is above the list; options scroll
internally. Covers .1, .2, .4, and .5.

## Verification

Write failing unit/component assertions first. Run them against unchanged
production code, implement, then rerun. Use the managed browser runner so the
Vite assets and backend are rebuilt before rendered evidence. Run projects
sequentially; do not overlap suites or override worker budgets.

From the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/filter-option-groups.test.ts components/task/sidebar-filter/filter-multi-select.test.tsx)
(cd apps/web && pnpm exec eslint components/task/sidebar-filter/filter-option-groups.ts components/task/sidebar-filter/filter-multi-select.tsx components/task/sidebar-filter/filter-multi-select-options.tsx components/task/sidebar-filter/filter-option-groups.test.ts components/task/sidebar-filter/filter-multi-select.test.tsx e2e/tests/task/sidebar-filter-selected-first.spec.ts e2e/tests/task/sidebar-filter-selected-first-helpers.ts e2e/tests/task/mobile-sidebar-filter-selected-first.spec.ts --max-warnings 0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-filter-selected-first.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-filter-selected-first.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Install dependencies from `apps/` with `pnpm install --frozen-lockfile` once if
resuming in a fresh worktree. Include any added E2E helper in lint and typecheck
coverage. Record a focused screenshot and compare with the preview. Inspect
Playwright's discovered test names/counts before recording browser results.

## Files likely touched

- `apps/web/components/task/sidebar-filter/filter-option-groups.ts` and `.test.ts`
- `apps/web/components/task/sidebar-filter/filter-multi-select.tsx` and new `.test.tsx`
- New `apps/web/components/task/sidebar-filter/filter-multi-select-options.tsx`
- New `apps/web/e2e/tests/task/sidebar-filter-selected-first.spec.ts`
- New `apps/web/e2e/tests/task/mobile-sidebar-filter-selected-first.spec.ts`
- New `apps/web/e2e/tests/task/sidebar-filter-selected-first-helpers.ts`
- This work order, plan, and paired spec lifecycle metadata after completion

## Dependencies

None. A later explicit implementation request is required by the repository's
design-package checkpoint. No delegation is authorized by this work order.

## Risks

`cmdk` relevance sorting and item remounting can differ from static React array
order. Do not treat helper tests alone as rendered proof. Selected values may
be absent from current options and must remain retained.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/filter-selected-options-first.md)
- [System design](../../specs/ui/system-design/filter-selected-options-first.md)
- `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`
- Existing `filter-option-groups.test.ts`, `lib/utils/selector-options.ts`,
  `SidebarFilterPopoverPage`, and `mobile-sidebar-views.spec.ts`

## Results

Implementation and task checks are complete. Full-project TypeScript validation
passed with Node 22.23.3 after the earlier Node 24 runtime failures.

| Check | Result |
| --- | --- |
| Targeted Vitest command above | 15 passed in two files; the initial three ordering assertions failed against unchanged production code |
| Targeted ESLint command above | Passed, zero warnings |
| Full `pnpm run typecheck` | Runtime failure: default Node heap exhausted (exit 134) |
| `NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck` | Runtime failure: V8 trapped while checking types (exit 133) |
| `NODE_OPTIONS='--max-old-space-size=4096 --jitless' pnpm run typecheck` | Runtime failure: segmentation fault (exit 139) |
| `pnpm dlx node@22 --max-old-space-size=4096 node_modules/typescript/bin/tsc --noEmit` in `apps/web` | Full-project check passed with Node 22.23.3; same TypeScript version and compiler options |
| Focused TypeScript compiler API diagnostics | Passed for all eight changed TS/TSX files, using the web compiler options and the web working directory; not a replacement for the full gate |
| Managed Chromium command above | One test passed; final test-only rerun used `--no-build` with the fresh managed build |
| Managed mobile-chrome command above | One test passed; verifies 44px actual row height, selection/search/reopen, group context, and viewport containment |
| `pnpm run i18n:check` and `pnpm run i18n:ratchet` in `apps/web` | Passed |
| `python3 scripts/list-docs.py validate` | Passed, 355 decisions and 1402 specifications |
| `python3 scripts/lint-spec-files.py --all` | Passed |
| `node --test scripts/validate-public-docs.test.mjs` | 62 passed |
| `node scripts/validate-public-docs.mjs` | Passed, 47 published pages |
| PR documentation coverage preflight | Passed, complete requirement/design/work-order references |
| `git diff --check` | Passed |

The initial backend build hit a Go linker panic. Retrying with `GOMAXPROCS=2`
passed; managed E2E builds and tests used that limit thereafter. No backend
source was changed. Phone rows use 48px nominal height because drawer scaling
reduced a nominal 44px target to 43.07px in browser evidence.

The phone screenshot was inspected against UI-02/UI-03: selected workflow steps
lead the list with their headings and checks, followed by unselected steps in
the existing internally scrolling popover. Capture:
`apps/web/e2e/test-results/task-mobile-sidebar-filter-5eff0-irst-through-touch-controls-mobile-chrome/selected-first-phone.png`.
Screenshots and diagnostic logs are ignored/temporary verification artifacts.

The user later authorized commit, push, and a ready PR. Requirements are active
and the system design is current. Fresh desktop and phone PR captures passed
using the managed runner, with finite animations settled before capture. Four
compressed assets are listed in the ignored PR asset manifest. The user-requested
isolated manual-test instance was stopped through its ownership-checked script;
ports 48429 and 49429 were verified closed. Main :9998 was untouched.
No delegation or persistent Kandev task/session creation was performed.
