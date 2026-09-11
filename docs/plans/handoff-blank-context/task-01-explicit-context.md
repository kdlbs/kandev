---
id: "01-explicit-context"
title: "Make handoff context explicit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001
acceptance_criteria:
  - AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.4
  - AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.6
  - AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.7
  - AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.9
  - AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.10
  - AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.11
system_design:
  - ../../specs/ui/system-design/agent-launch-prompt-composer.md
---

# Task 01: Make handoff context explicit

## Summary

Initialize handoff context as Blank and remove automatic context-action execution.
Preserve explicit context actions and target-profile selection on both viewports.

## In scope

- Implement the helper and mount-effect correction described in the plan.
- Update the existing unit/component and desktop/mobile handoff regressions.
- Update both public handoff descriptions with the delivered behavior.

## Out of scope

Backend changes, stored preferences, profile precedence, dialog layout changes,
new translations, and summary cancellation or stale-result redesign.

## Acceptance

- Fresh opening, reopening, and new handoff identity show Blank with an empty
  prompt, preserve compatible target choice, and perform no automatic summary.
  Ordinary rerenders and data hydration preserve typed text without summary work.
- Explicit context selection retains copy, Blank, selected-session summary,
  loading/failure behavior, and launch guards. Typed launch needs no summary.
- Desktop and phone E2E prove the outcomes in the plan's scenario matrix;
  public instructions describe the delivered default and optional summary action.

## ASCII UI preview

UI-01 from the [full preview](plan.md#ascii-ui-preview), AC-001.6 and .9-.11.
Shared desktop/phone form; phone entry uses the existing mobile session actions.

```text
Hand off to <target>
Environment / Agent Profile
Context [Blank v]
Prompt  [empty]
[Cancel] [Start Agent: disabled]
```

Only user selection changes context to a session summary. Retain the current
phone form geometry and touch treatment. Compare both rendered views with this
structure during the focused E2E runs.

## Verification

Start with TDD: change the helper expectation to Blank and replace the automatic
component test with `opens handoff with Blank context without summarizing`.
Confirm a behavioral RED (actual summarize value/call), then remove both causes.
Add explicit-summary, reopen, alternate-session, hydration, and typed-launch
coverage specified by the plan. Extend the existing context-selector mock to
expose selected value and explicit summary actions if needed.

Run from repository root. Install dependencies once before package commands.
Managed E2E rebuilds production assets and owns isolated instance cleanup.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/handoff-types.test.ts components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts components/task/session-context-summary.test.ts components/task/new-session-profile-selection.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/handoff-types.ts components/task/handoff-types.test.ts components/task/new-session-dialog.tsx components/task/new-session-dialog.test.tsx e2e/tests/session/session-handoff.spec.ts e2e/tests/session/mobile-handoff.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-handoff.spec.ts tests/session/session-handoff-unhealthy-profile.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-handoff.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/handoff-blank-context
```

If supporting test helpers change, include them in targeted lint and test checks.
Record RED and final results, discovered test counts, rendered comparison, and
cleanup. Do not mark done if a required command fails or no tests are discovered.

## Files likely touched

- `apps/web/components/task/handoff-types.ts`
- `apps/web/components/task/handoff-types.test.ts`
- `apps/web/components/task/new-session-dialog.tsx`
- `apps/web/components/task/new-session-dialog.test.tsx`
- `apps/web/e2e/tests/session/session-handoff.spec.ts`
- `apps/web/e2e/tests/session/mobile-handoff.spec.ts`
- `docs/public/sessions-and-review.md`
- This work order and `plan.md` for execution status and results.

## Dependencies

None. Start after the user's explicit implementation request.

## Risks

The old tests encode automatic summaries. Retain their summary-delivery coverage
by selecting context explicitly. Keep session/profile initialization and the
existing form key intact. Count only summary utility requests in browser checks.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/agent-launch-prompt-composer.md), AC-001.4, .6-.7, .9-.11.
- [Design](../../specs/ui/system-design/agent-launch-prompt-composer.md), Context lifecycle and Responsive behavior.
- `new-session-form-actions.ts`, `session-dialog-shared.tsx`, and `hooks/use-summarize-session.ts` for explicit-action behavior.
- Existing handoff E2E files and `e2e/pages/session-page.ts` for desktop/mobile entry helpers.
- Use `/tdd`, `/e2e`, and `/docs-maintainer` during execution.

## Results

Implemented the correction in the shared handoff composer. `buildHandoffInitialState`
now preserves the target profile while selecting `blank`; the automatic mount
effect was removed, so only an explicit ContextSelect action can request a
summary. Desktop and phone regressions cover fresh open, explicit summary
selection, reopen reset, typed launch, target preservation, and phone overflow.

TDD evidence:

- RED: the updated helper test failed before the helper change because the
  actual value was `summarize:session-a`.
- RED: after the helper change, the unchanged dialog test failed because the
  old mount effect no longer received an automatic summary call.
- GREEN: the final targeted suite passed with 5 files and 38 tests.

Validation:

- `pnpm install --frozen-lockfile`: passed from `apps`.
- `pnpm exec vitest run ...`: 5 files, 38 tests passed.
- `pnpm run typecheck`: passed.
- Targeted ESLint: passed with 0 errors and 2 duplicate-string warnings in
  `new-session-dialog.test.tsx`.
- `pnpm run i18n:ratchet`: passed with 0 new violations.
- The exact desktop command passed with 2 tests: handoff and
  unhealthy-profile compatibility.
- The mobile command passed with 1 `mobile-chrome` test, including touch
  selection, typed launch, and horizontal-overflow checks.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

The E2E runs used the managed isolated runner and left no instance artifacts.
The public documentation correction is included in this implementation package.
