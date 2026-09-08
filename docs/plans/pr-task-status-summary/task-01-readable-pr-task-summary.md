---
id: "01-readable-pr-task-summary"
title: "Build readable PR task summary"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-task-status-summary.md"
---

# Task 01: Build Readable PR Task Summary

## Acceptance

- Fine-pointer hover and keyboard focus show each linked PR as a distinct header plus labelled,
  icon-supported review, CI, and merge/state rows; long titles wrap and missing rows stay absent.
- Existing icon color, multi-PR count, merge-readiness logic, `data-pr-*` attributes, task-row
  activation, and coarse-pointer behavior remain unchanged; all new copy is localized.
- Focused component tests, desktop hover E2E, mobile task-row E2E, typecheck, and i18n checks pass.

## TDD Sequence

1. RED: add the summary component cases for ready, failure/conflict, draft/terminal, missing,
   unknown, long-title, and multi-PR states through the pure summary derivation; update icon tests
   and observe the new behavior expectations fail against the current pipe-delimited tooltip.
2. RED: extend the desktop PR badge spec with the sidebar hover hierarchy/geometry assertions and
   the mobile task-status spec with row navigation; run both focused specs before production
   changes.
3. GREEN: add `PRTaskStatusSummary`, wire both icon variants to it, add accessible trigger copy,
   and add English catalog keys. Reuse existing readiness helpers and status tones.
4. REFACTOR: remove `getPRTooltip`, keep the presentation component focused, generate the pseudo
   locale, inspect desktop and phone rendered results, then run every exact verification command.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run \
  components/github/pr-task-icon.test.ts \
  components/github/pr-task-icon-draft.test.ts \
  components/github/pr-task-icon.render.test.tsx \
  components/github/pr-task-status-summary.test.ts
cd apps/web && pnpm e2e:run tests/pr/pr-status-badge.spec.ts \
  -- --grep "renders readable task PR summary"
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-task-status-summary.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web run i18n:check
cd apps && pnpm --filter @kandev/web run i18n:ratchet
```

## Files likely touched

- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon-draft.test.ts`
- `apps/web/components/github/pr-task-icon.test.ts`
- `apps/web/components/github/pr-task-icon.render.test.tsx`
- `apps/web/components/github/pr-task-status-summary.tsx`
- `apps/web/components/github/pr-task-status-summary.test.ts`
- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/pseudo/github.json`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`
- `docs/specs/ui/requirements/pr-task-status-summary.md`
- `docs/specs/INDEX.md`
- `docs/plans/pr-task-status-summary/plan.md`
- `docs/plans/pr-task-status-summary/task-01-readable-pr-task-summary.md`

## Dependencies

None. Current `TaskPR` projections and readiness/color helpers contain every required field and
remain authoritative.

## Parallelism

`sequential` — summary presentation, shared icon wiring, locale keys, and focused tests describe
one small vertical slice and overlap on the same component contract.

## Inputs

- `docs/specs/ui/requirements/pr-task-status-summary.md`, especially status-preservation, multi-PR, fallback,
  and mobile scenarios.
- `docs/plans/pr-task-status-summary/plan.md`.
- Existing derivation and trigger attributes in `apps/web/components/github/pr-task-icon.tsx`.
- Status icon/copy precedent in
  `apps/web/components/github/my-github/pr-status-badges.tsx`.
- Tooltip portal selector guidance in `.agents/skills/e2e/SKILL.md`.
- Content-only mobile guidance and exemplars in `.agents/skills/mobile-parity/SKILL.md`.
- Typography and surface rules in `/home/zeval/.agents/skills/make-interfaces-feel-better/`.

## Risks

- Never infer **Ready to merge** from aggregate check counts or raw `mergeable_state`; use the
  existing strict helper result passed by `PRTaskIcon`.
- Keep unknown provider values as data and resolve known catalog keys only during render.
- Scope E2E assertions to the visible open Tooltip portal because Radix may mount a second
  accessibility copy.
- Keep the compact indicator passive and preserve parent row click/keyboard behavior on every
  shared surface.

## Results

RED evidence:

- `pnpm --filter @kandev/web test -- --run
  components/github/pr-task-status-summary.test.ts` failed as expected: the old implementation
  returned one pipe-delimited string instead of `{ number, title, rows }`.
- `pnpm e2e:run tests/pr/pr-status-badge.spec.ts -- --grep "renders readable task PR summary"`
  failed as expected because the old tooltip had no structured summary selector.

GREEN evidence:

- `pnpm install --frozen-lockfile` completed from `apps/` without changing the lockfile.
- The focused Vitest command above, expanded to include `pr-task-icon-draft.test.ts`, passed 78 tests
  across 4 files. A final summary-only run passed 6 tests after adding explicit failure/conflict
  coverage.
- The desktop E2E command passed 1 Chromium test. It proves hover and keyboard focus, readable
  labelled rows, long-title wrapping, viewport containment, and two linked PR groups.
- The mobile E2E command passed 1 `mobile-chrome` test. `.tap()` on the task row navigated normally
  and the document remained free of horizontal overflow.
- `pnpm run typecheck`, the task's targeted ESLint invocation, `i18n:check`, and `i18n:ratchet` all
  passed. Pseudo-locale generation produced 30 namespaces / 5,740 messages; the 918 reported
  `zh-cn` parity issues are advisory and pre-existing.
- `CAPTURE_PR_ASSETS=1` produced
  `apps/web/.pr-assets/pr-status-badge--readable-task-pr-summary.png` for visual inspection. The
  subsequent standard E2E cleanup removed the ignored capture and every E2E run completed teardown.

Files changed:

- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon-draft.test.ts`
- `apps/web/components/github/pr-task-icon.test.ts`
- `apps/web/components/github/pr-task-status-summary.tsx`
- `apps/web/components/github/pr-task-status-summary.test.ts`
- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/pseudo/github.json`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`
- This spec, plan, task, and `docs/specs/INDEX.md`.

No blockers or residual in-scope risks remain. Existing ready-to-merge, aggregate color, task-row
activation, and coarse-pointer detail ownership are preserved.

## Output contract

Report implementation outcome, files changed, status semantics preserved, localization keys,
desktop/mobile rendered evidence, exact tests and counts, blockers or risks, and synchronized
task/plan status. Present visual changes in a concise **Before / After** table.
