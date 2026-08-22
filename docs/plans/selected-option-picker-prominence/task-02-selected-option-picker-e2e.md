---
id: "02-selected-option-picker-e2e"
title: "Verify selected picker prominence"
status: done
wave: 2
depends_on: ["01-selected-option-picker-ui"]
plan: "plan.md"
spec: "../../specs/ui/selected-option-picker-prominence.md"
---

# Task 02: Verify selected picker prominence

## Acceptance

- Desktop model, agent, repository, and branch picker flows show the current
  value first with the selected surface and preserve existing filtering and
  selection behavior.
- The mobile model flow proves the current option is visible first before
  scrolling, remains touch-selectable, and stays within the viewport without
  document horizontal overflow.
- The focused frontend tests and Playwright projects pass against a fresh
  production web build.

## Verification

```bash
cd apps && rtk pnpm install --frozen-lockfile
cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/model-selector-error.spec.ts tests/task/create-task.spec.ts tests/task/create-task-branch-selector.spec.ts
cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts tests/settings/mobile-no-silent-model-fallback.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts
cd apps/web && rtk pnpm run typecheck
rtk git diff --check
```

The managed E2E runner must rebuild the production web assets. If an existing
task-launch recovery test is the correct branch-sheet fixture, extend that
test rather than creating a duplicate seed flow.

## Files likely touched

- `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`
- `apps/web/e2e/tests/chat/model-selector-error.spec.ts`
- `apps/web/e2e/tests/task/create-task.spec.ts`
- `apps/web/e2e/tests/task/create-task-branch-selector.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts` only if
  its existing agent-picker flow is the minimal mobile proof.

## Dependencies

Task 01.

## Parallelism

Sequential. E2E selectors and assertions depend on the final UI markup.

## Inputs

- The spec scenarios for model, combobox, branch, unavailable values, and
  narrow touch viewports.
- The mobile parity contract in `plan.md`.
- Existing fixture, page-object, causal-wait, and selector-scoping guidance in
  `.agents/skills/e2e/SKILL.md`.

## Output contract

Report exact desktop and mobile commands, test counts and outcomes, fresh-build
evidence, screenshots or artifacts if captured, cleanup evidence, changed test
files, blockers, risks, and synchronized task/plan status.

## Results

Desktop fresh production-build E2E passed 35 tests. Mobile fresh
production-build E2E passed 3 tests covering the model picker, settings agent
picker, and the existing launch-recovery branch sheet. The recovery fixture
starts with a missing branch, so its assertion verifies that the first
available replacement branch is visible, touch-sized, and viewport-contained;
selected styling is covered by the current-value branch tests. The fixup pass
also scopes option assertions through each active listbox to avoid matching a
different mounted picker.
