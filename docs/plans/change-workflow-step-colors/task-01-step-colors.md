---
id: "01-step-colors"
title: "Render destination step colors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
acceptance_criteria:
  - AC-TASKS-CHANGE-WORKFLOW-001.2
  - AC-TASKS-CHANGE-WORKFLOW-001.5
  - AC-TASKS-CHANGE-WORKFLOW-001.7
  - AC-TASKS-CHANGE-WORKFLOW-001.9
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 01: Render destination step colors

## Summary

Resolve saved workflow color tokens before rendering the destination step dot.
Prove options and the selected value show colors through the existing shared
desktop and phone form.

## In scope

- Reuse `parseWorkflowStepColor` for the existing dot and mark it decorative.
- Add component regressions for saved tokens, hex colors, neutral fallback,
  and selected-value rendering.
- Add focused desktop/phone browser regressions with real computed-color checks,
  search, and destination switch reset through real domain state.

## Out of scope

Shared combobox changes, a new color registry, backend work, translations,
dialog layout changes, and agent lifecycle changes.

## Acceptance

1. Saved editor colors and custom hex colors render in option rows and the
   selected trigger, with the existing resolver's neutral fallback.
2. Step names remain searchable; order, workflow-switch reset, and submission
   semantics remain intact.
3. Desktop and phone tests prove visible colors and complete a destination move;
   phone checks preserve touch targets and containment.

## ASCII UI preview

### UI-01: Destination step picker, desktop and phone

```text
Destination step [(blue dot) Analysis v]
+-----------------------------------+
| Search steps...                   |
| (blue dot)   Analysis              |
| (green dot)  Implement             |
+-----------------------------------+
```

See [full preview and reset/fallback states](plan.md#ascii-ui-preview).
Covers AC-TASKS-CHANGE-WORKFLOW-001.9 and preserves .2/.5/.7. The existing
desktop dialog and phone drawer retain their composition. Names and color
choices are illustrative; keep a non-shrinking dot and readable step name.

## Verification

Use `/tdd` and `/e2e`. Add failing component and focused browser regressions
before changing the renderer. Rebuild through the managed runner for each
browser run; run desktop and mobile sequentially. From the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/change-workflow-dialog.test.tsx lib/task-color-presentation.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/change-workflow-form-sections.tsx components/task/change-workflow-dialog.test.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts -- --grep 'step colors')
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts -- --grep 'step colors')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.
The Go PATH prefix and Node heap setting above address this cloud environment's
toolchain location and the frontend typecheck's memory requirement.
Record actual commands, test counts, and rendered screenshot review. If any
additional existing test files change, include their targeted commands before
marking done. Check spec/work-order references with the repository PR-documentation
coverage validator and inspect `git status --short` for untracked files.

## Files likely touched

- `apps/web/components/task/change-workflow-form-sections.tsx`
- `apps/web/components/task/change-workflow-dialog.test.tsx`
- `apps/web/e2e/tests/task/change-workflow.spec.ts`
- `apps/web/e2e/tests/task/mobile-change-workflow.spec.ts`
- `apps/web/e2e/pages/change-workflow-page.ts` only if a shared color assertion
  helper reduces duplication.

## Dependencies

None. The existing form and color resolver are present.

## Risks

Existing hex fixtures pass even with the bug. Class-token tests and computed
browser styles must establish the regression. Preserve the existing resolver's
alias normalization rather than creating a competing palette.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/change-workflow.md),
  AC-TASKS-CHANGE-WORKFLOW-001.9 and destination/mobile criteria.
- [System design](../../specs/tasks/system-design/change-workflow.md#destination-step-colors).
- `apps/web/lib/task-color-presentation.ts` and its existing tests.
- `apps/web/components/kanban/mobile-column-tabs.tsx` for phone dot placement.
- Existing Change workflow desktop/mobile suites and page object.

## Results

Implemented on 2026-09-30 after the user's explicit implementation request.
The renderer uses the existing color presentation resolver; no new registry,
API, translation, or layout contract was introduced.

RED evidence: the new component cases failed before implementation, and each
desktop/phone Playwright regression failed because the dot's computed background
was transparent instead of the seeded blue. GREEN results below cover the final
formatted renderer. Browser runs rebuilt the runtime, Vite assets, and fixture
package, used one worker, and ran sequentially.

| Command from repository root | Result |
| --- | --- |
| `(cd apps && pnpm install --frozen-lockfile)` | Passed; installed missing workspace dependencies without a lockfile change. |
| Targeted Vitest command in Verification | Passed, 2 files and 17 tests (14 dialog cases and 3 resolver cases). |
| Targeted ESLint command in Verification | Passed with zero warnings across all five changed frontend/test files. |
| Typecheck command in Verification | Passed with a 4 GB Node heap. The initial default-heap run exhausted the 2 GB heap. |
| Desktop managed E2E command in Verification | Passed, 1 test; verified option and selected colors, search, workflow reset, and committed move. |
| Phone managed E2E command in Verification | Passed, 1 test; verified the same flow with taps, at least 44px control height, containment, and no document horizontal overflow. |
| `(cd apps/web && pnpm exec prettier --check components/task/change-workflow-form-sections.tsx components/task/change-workflow-dialog.test.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)` | Passed; all five files formatted. |
| `(cd apps/web && pnpm run i18n:ratchet)` | Passed; modified source clean and guard allowlist intact. No new user-facing copy. |

Rendered desktop and phone captures were inspected: blue and green dots are
visible beside readable step names, with no overlap or clipped dots. Captures
disable finite animations to avoid inspecting a partially entered popover.

Documentation and diff gates: recorded by the plan's final validation results.
The user subsequently requested PR publication. Desktop and phone PR assets
are captured through the shared manifest fixture after finite animations settle.


### PR review follow-up

Greptile identified a browser coverage gap: blue and green alone cannot prove
that the remaining saved palette and fallback classes have generated CSS.
This follow-up extends the existing desktop and phone scenarios with all nine
editor colors, missing and unsupported values (slate fallback), and a custom
hex value. Every additional case checks both the option and selected trigger
against computed CSS in the production build. The seed data is shared in
`e2e/tests/task/change-workflow-color-helpers.ts`.

This is reviewer-requested coverage of behavior already implemented; it requires
no production change or new failing behavior. The original search, reset, move,
and phone containment/touch assertions remain in the scenarios. Existing PR
screenshots remain representative because the rendered product is unchanged.

Validation: desktop managed command in Verification with `--retries=0` passed,
one test covering all twelve color cases. The matching phone command with
`--retries=0` also passed, one test covering the same twelve cases through taps.
Targeted ESLint passed for both scenarios and their new shared seed helper.
The 4 GB-heap typecheck and local PR-documentation coverage validator passed.
No product behavior, palette, copy, or layout changed; no public-docs update is
needed. Current-head CI/review readiness remains externally pending after push.
