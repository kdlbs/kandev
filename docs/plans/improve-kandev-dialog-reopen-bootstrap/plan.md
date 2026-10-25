---
spec: docs/specs/workspaces/requirements/improve-kandev.md
created: 2026-08-10
status: implemented
---

# Implementation Plan: Improve Kandev Dialog Reopen Bootstrap

## Overview

After the first contribution is filed, the Improve Kandev dialog hangs on
reopen with the "Preparing kandev repository in background" banner and a
permanently blocked submit button. Root cause: `useResetOnClose` resets
`workspaceChoiceConfirmed` to `false` on close, but the effect that
auto-confirms the choice when the dedicated workspace exists only re-ran when
`improveWorkspaceMissing` changed — a value that is stable once the workspace
is known. The bootstrap gate (`!workspaceChoiceConfirmed`) then early-returns
on every reopen, so no bootstrap request is ever sent. Fix: re-assert the
confirmation on every dialog open by keying the effect on `open` as well.

---

## Frontend

### `components/improve-kandev-dialog.tsx`
- The auto-confirm effect becomes:

  ```ts
  useEffect(() => {
    if (open && !improveWorkspaceMissing) {
      setWorkspaceChoiceConfirmed(true);
    }
  }, [improveWorkspaceMissing, open]);
  ```

  `open` in the dependency array makes the effect re-run on every open
  transition, so `workspaceChoiceConfirmed` is re-asserted after
  `useResetOnClose` clears it — regardless of whether `improveWorkspaceMissing`
  changed. The `open &&` guard keeps the mount-time ordering safe (the effect
  never confirms while the dialog is closed) and leaves the
  workspace-creation choice gate untouched when the workspace is missing.

### Tests
- `components/improve-kandev-dialog.test.tsx`: component regression test —
  render with the dedicated workspace present and skip-intro set, wait for one
  bootstrap call, rerender with `open={false}` then `open`, assert bootstrap
  is called again.
- `e2e/tests/improve-kandev.spec.ts`: end-to-end regression test — activate
  the dedicated workspace, file a first bug (skip intro, submit), reopen the
  dialog, assert the contributor banner (`@octocat`) appears (bootstrap ready)
  and a second bug can be submitted.

---

## Tests

- **What:** reopening the dialog in the dedicated workspace re-runs bootstrap
  (spec regression scenario). **File:**
  `apps/web/components/improve-kandev-dialog.test.tsx`. **How:** component
  test asserting `bootstrapImproveKandev` is called on the second open.
- **What:** a second bug can be filed end-to-end with the dedicated workspace
  active. **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`. **How:** real
  backend + mocked bootstrap; two submissions, both asserted in the dedicated
  workspace.

---

## E2E Tests

- **Scenario:** GIVEN the dedicated `Improve Kandev` workspace already exists
  and the intro has been dismissed, WHEN the user closes the dialog and
  reopens it to file another report, THEN bootstrap re-runs and the submit
  button becomes enabled.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **What to verify:** after reopening, `@octocat` (contributor banner =
  bootstrap ready) is visible and the second task is created in the dedicated
  workspace.

---

## Verification Results

Completed for task 01 (see its `## Results`). Summary on the final tree:

- Unit: `pnpm vitest run` on the dialog/model/sidebar suites → 49/49 passed,
  including the new reopen regression test.
- Lint: eslint clean on all three touched files.
- Typecheck: `tsc --noEmit` clean.
- E2E: `pnpm e2e:raw tests/improve-kandev.spec.ts
  tests/mobile-improve-kandev.spec.ts` → 15 passed.
- Both new regression tests (component + e2e) were verified red before the
  fix and green after.

### PR fixup (PR #2484)

- Remediation commit `5a034e86b` (comment-only) documents that the per-open
  bootstrap re-probe is intentional, addressing the non-blocking review
  suggestion; the second suggestion (pre-existing double `readSkipIntro()`)
  was deferred per the reviewer.
- Final head `5a034e86b`: 27/27 checks green (incl. CodeRabbit, full E2E
  matrix, frontend suite, Build), no failed or pending checks, no unresolved
  review threads, `mergeable_state: clean`. Claude review at the substantive
  head (`0d96473ad`) verdict: "Ready to merge", 0 blockers.

---

## Implementation Waves And Parallel Candidates

Wave 1 (single task, sequential):
- [x] [task-01-reopen-bootstrap-regression](task-01-reopen-bootstrap-regression.md)

---

## Open Questions

None.
