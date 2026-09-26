---
id: "02-shortcut-switcher-e2e"
title: "Prove review shortcut switching end to end"
status: done
wave: 2
depends_on: ["01-held-review-switcher"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-review-shortcut.md"
---

# Task 02: Prove review shortcut switching end to end

## Acceptance

- Playwright proves held default-chord cycling, PR/MR order, wrap, Shift-only
  release, primary-modifier activation, and final provider URL.
- Playwright proves Escape prevents activation on later modifier release while
  retaining existing single-review and settings behavior.
- Existing mobile linked-review drawer and multi-PR Review selector specs still
  pass as parity evidence; no touch or viewport composition changes.

## Verification

- `cd apps/web && pnpm e2e:run tests/pr/pr-open-shortcut.spec.ts`
- `cd apps/web && pnpm e2e:run -- --project=mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts tests/review/mobile-review-multi-pr.spec.ts`

## Files likely touched

- `apps/web/e2e/tests/pr/pr-open-shortcut.spec.ts`
- `apps/web/e2e/helpers/gitlab.ts` only if existing exported seeds cannot express
  the linked-MR fixture cleanly
- `apps/web/e2e/pages/session-page.ts` only if a reusable selected-row locator is
  warranted

## Dependencies

- Task 01 complete.

## Parallelism

Sequential; depends on Task 01 production behavior.

## Inputs

- Spec scenarios.
- Plan `E2E tests` and `Mobile design contract`.
- Existing `apps/web/e2e/tests/pr/pr-open-shortcut.spec.ts` seed and popup
  interception pattern.
- Existing `apps/web/e2e/helpers/gitlab.ts` and
  `ApiClient.linkTaskGitLabMR` for a deterministic MR target.

## Risks

- Popup timing can race modifier release; register popup waits before keyup.
- Test must use separate `keyboard.down` / `press` / `up` calls so Playwright
  preserves held-modifier state.
- Provider routes must remain offline-deterministic.

## Output contract

Report RED and GREEN E2E results, exact commands, changed files, mobile parity
evidence, blockers, and remaining risks. Mark this task `done` and its plan
checkbox complete only after both verification commands pass.
