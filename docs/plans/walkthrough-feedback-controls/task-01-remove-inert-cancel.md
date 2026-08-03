---
id: "01-remove-inert-cancel"
title: "Remove inert walkthrough Cancel"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/walkthrough-feedback-controls/spec.md"
---

# Task 01: Remove Inert Walkthrough Cancel

## Acceptance

1. Desktop floating and phone bottom-sheet walkthrough note forms do not render
   a **Cancel** action.
2. **Add**, **Run**, close, discard, and navigation behavior remain available in
   those floating variants.
3. Focused desktop and `mobile-chrome` walkthrough E2E coverage proves the
   action set in both rendered variants.

## TDD sequence

1. Add desktop and mobile assertions for the absent walkthrough **Cancel**
   action, then run both focused tests with retries disabled. Confirm RED
   because the current button is visible.
2. Make `CommentForm.onCancel` optional and conditionally render/handle the
   cancellation paths. Remove the walkthrough's no-op callback.
3. Re-run both focused tests. Confirm **Add** and **Run** remain usable and the
   mobile bottom sheet still renders within its existing surface.
4. Run the targeted typecheck and i18n ratchet. Refactor only if the minimal
   capability change is unclear, then rerun affected checks.

## Verification

Bootstrap once if workspace dependencies are absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run from `apps/web`:

```bash
pnpm e2e:run --host --project chromium -- tests/review/walkthrough.spec.ts --grep "ask box offers Add" --workers=1 --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/review/mobile-walkthrough.spec.ts --grep "opens the walkthrough as a bottom-sheet panel" --workers=1 --retries=0
pnpm run typecheck
pnpm run i18n:ratchet
```

The first managed E2E command rebuilds the production frontend. The second
reuses those unchanged artifacts and selects the Pixel 5 project explicitly.
Confirm each command discovers one intended test before treating it as
evidence.

## Files likely touched

- `apps/web/components/diff/comment-form.tsx`
- `apps/web/components/diff/walkthrough-step-card.tsx`
- `apps/web/e2e/tests/review/walkthrough.spec.ts`
- `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`
- `docs/specs/INDEX.md`
- `docs/specs/walkthrough-feedback-controls/spec.md`
- `docs/plans/walkthrough-feedback-controls/plan.md`
- `docs/plans/walkthrough-feedback-controls/task-01-remove-inert-cancel.md`

## Dependencies

None.

## Parallelism

`sequential`. Shared component and desktop/mobile regressions form one TDD
cycle.

## Inputs

- [Walkthrough Feedback Controls spec](../../specs/walkthrough-feedback-controls/spec.md)
- [Fix plan](plan.md)
- `apps/web/components/diff/comment-form.tsx` cancellation behavior
- Existing desktop and mobile walkthrough Playwright flows

## Risks

- Do not weaken cancellation behavior for existing diff/editor callers.
- Do not add a walkthrough-specific hide flag while retaining a no-op action;
  the component contract should reflect whether cancellation exists.

## Output contract

Report RED failure messages, implementation files, focused desktop/mobile E2E
results and counts, typecheck/i18n results, rendered mobile evidence, cleanup,
remaining risks, and synchronized task/plan statuses.

## Results

- **RED:** both focused E2E tests failed before implementation because the new
  absence assertion found one **Cancel** button (`Expected: 0, Received: 1`).
  The suites' configured retries repeated the same expected failure.
- **Implementation:** made `CommentForm.onCancel` optional, hid its cancellation
  UI and Escape handling when absent, and removed the walkthrough's no-op
  callback. Structural inspection confirmed existing cancellable callers still
  provide their callbacks; their runtime click/Escape paths were not rerun.
- **Desktop:**
  `pnpm e2e:run --host --project chromium -- tests/review/walkthrough.spec.ts --grep "ask box offers Add" --workers=1 --retries=0`
  passed 1 test.
- **Mobile:**
  `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/review/mobile-walkthrough.spec.ts --grep "opens the walkthrough as a bottom-sheet panel" --workers=1 --retries=0`
  passed 1 test.
- **Static checks:** `pnpm run typecheck` and `pnpm run i18n:ratchet` passed; the
  ratchet reported 0 added and 2 modified files clean.
- **Review remediation:** simplified the conditional Cancel rendering and reran
  both focused E2E tests (1 passed each), typecheck, and the i18n ratchet. The
  package now distinguishes runtime proof for the floating variants from
  structural inheritance by the legacy inline variant and existing callers.
- **Rendered proof:** captured and inspected
  `desktop-walkthrough-feedback-controls.png` (1280x720) and
  `mobile-walkthrough-feedback-controls.png` (1081x1999). Both show **Add** and
  **Run**, no **Cancel**, and synthetic E2E data only.
- **Cleanup:** all managed test commands exited successfully; a follow-up
  process check found no matching E2E, Playwright, or Vite process.
- **Security/trust:** no boundary or credential changes. Residual risk is low:
  the legacy inline walkthrough and unchanged cancellable-form click/Escape
  paths were not separately exercised at runtime.
