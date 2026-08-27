---
id: "01-expand-mobile-submit-target"
title: "Expand mobile clarification Submit target"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/clarification-submit-feedback.md"
---

# Task 01: Expand mobile clarification Submit target

## Intent

Give the shared multi-question clarification header stable touch targets and a
separate phone action row while preserving compact desktop geometry and all
existing submission behavior.

## Root cause

`ClarificationHeaderActions` applies `text-xs px-3 py-1` to the Submit button,
which renders about 24px tall beside the header's 44px collapse control. The
single inline phone row also crowds progress, Submit feedback, Skip, and
Collapse. Existing mobile E2E checks state and overflow but not action geometry.

## Acceptance

- On coarse-pointer viewports, the shared Submit button is at least 44px tall
  in idle and submitting states, and those states differ in height by no more
  than 1px.
- The pending button remains disabled, shows translated `Submitting...` text
  plus the decorative spinner, and causes no document horizontal overflow.
- Phone progress occupies a row above batch actions; Submit uses available
  width; Skip and Collapse remain at least 44px in each dimension.
- Fine-pointer desktop sizing, answer submission, task-chat/Quick-Chat sharing,
  labels, and colors remain unchanged.

## Regression test (write first; must fail before production change)

Extend `separates batch actions from the stepper while showing submission feedback` in
`apps/web/e2e/tests/chat/mobile-clarification.spec.ts`:

1. Capture the enabled Submit button's `boundingBox()` before tapping it.
2. Hold the response, tap Submit, and capture the pending button's box.
3. Assert pending height is at least 44px and differs from idle height by no
   more than 1px, while retaining the existing spinner and overflow checks.

Run the focused mobile test before editing the component. Record the expected
failure showing the current roughly 24px height, then apply the smallest class
change and rerun green.

## Files likely touched

- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`
- `docs/specs/ui/requirements/clarification-submit-feedback.md`
- `docs/plans/clarification-mobile-submit-target/plan.md`
- `docs/plans/clarification-mobile-submit-target/task-01-expand-mobile-submit-target.md`

## Dependencies

None.

## Parallelism

Sequential. Test and component share one mobile geometry contract.

## Inputs

- Spec `What`, phone and desktop `Scenarios`, and `Out of scope`.
- Plan `Root cause`, `Frontend`, `Mobile design contract`, and `Tests`.
- `ClarificationHeaderActions` and the same header's `h-11 w-11` collapse
  control.
- Coarse-pointer sizing pattern in
  `apps/web/components/task/chat/queued-ghost-panel-header.tsx`.

## Verification

Bootstrap once if this fresh worktree lacks dependencies:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Then run from repository root:

```bash
# Red before component change; green after.
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "separates batch actions from the stepper")

# Existing fine-pointer pending-state behavior.
(cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep "question shortcuts stay disabled while answers are submitting")

# Static checks for touched source and test.
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/clarification-overlay-header.tsx e2e/tests/chat/mobile-clarification.spec.ts)
(cd apps/web && pnpm exec prettier --check components/task/chat/clarification-overlay-header.tsx e2e/tests/chat/mobile-clarification.spec.ts)
git diff --check
```

Capture and inspect one phone pending-state screenshot during the green E2E
run. Remove any temporary capture-only code or files before completion and
record capture path plus cleanup in `## Results`.

## Output contract

Report responsive layout and target changes, red and green geometry values,
focused command results, screenshot inspection and cleanup, remaining risks,
and synchronized task/plan/spec status. Do not change translations, response
lifecycle, or Skip and Collapse semantics.

## Results

- Production change: added `[@media(pointer:coarse)]:min-h-11` to Submit, split
  phone progress and actions into two rows, let Submit fill available width,
  and gave Skip and Collapse 44px coarse-pointer targets. Fine-pointer desktop
  keeps the compact inline layout.
- Regression change: the existing mobile held-response test now captures idle
  and pending bounding boxes, requires pending height to be at least 44px, and
  limits state-to-state height movement to 1px. It also proves action-row
  separation, secondary target sizing, containment, spinner, disabled state,
  completion, and no overflow.
- `cd apps && pnpm install --frozen-lockfile` passed in 3.3s; 1,115 workspace
  packages linked from the existing store.
- Initial RED command, `cd apps/web && pnpm e2e:run --project mobile-chrome
tests/chat/mobile-clarification.spec.ts -- --grep "loading spinner while batch
answers submit on mobile"`, completed the fresh build but was manually
  stopped after the failing assertion left the intentionally held response
  unreleased. Test-only cleanup ordering was corrected before any production
  edit.
- Clean RED command, `cd apps/web && pnpm e2e:run --no-build --project
mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "loading
spinner while batch answers submit on mobile"`, failed twice as expected
  because the configured retry repeated `Expected: >= 44; Received: 24`.
- GREEN command after the production edit, `cd apps/web && pnpm e2e:run
--project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep
"loading spinner while batch answers submit on mobile"`, rebuilt production
  assets and passed 1 test in 6.0s.
- Final change-aware GREEN after removing capture-only code, `cd apps/web &&
pnpm e2e:run --no-build --project mobile-chrome
tests/chat/mobile-clarification.spec.ts -- --grep "loading spinner while batch
answers submit on mobile"`, passed 1 test in 6.7s.
- Desktop command, `cd apps/web && pnpm e2e:run --project chromium
tests/chat/clarification.spec.ts -- --grep "question shortcuts stay disabled
while answers are submitting"`, rebuilt production assets and passed 1 test
  in 7.4s.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm exec eslint
components/task/chat/clarification-overlay-header.tsx
e2e/tests/chat/mobile-clarification.spec.ts` passed with no output.
- `cd apps/web && pnpm exec prettier --check
components/task/chat/clarification-overlay-header.tsx
e2e/tests/chat/mobile-clarification.spec.ts` passed; both files match
  Prettier style.
- `git diff --check` passed.
- Visual check: captured
  `/tmp/kandev-mobile-clarification-submit-green.png` during the pending state.
  The 44px Submit control aligned with the same-header collapse control; label
  and spinner were centered with no clipping or horizontal page overflow.
  Capture-only test code and the PNG were removed (`rm
/tmp/kandev-mobile-clarification-submit-green.png`).
- Final files match `## Files likely touched`. Managed E2E used isolated local
  backends and cleaned them; ignored production build outputs remain under
  `apps/web/dist/` and backend build paths.
- Security/trust boundary: none. External side effects: none. Subagents: none.
