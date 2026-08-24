---
spec: docs/specs/ui/requirements/executor-settings-card-spacing.md
created: 2026-08-03
status: complete
---

# Implementation Plan: Executor settings card spacing

## Overview

The regression is limited to the two modern executor profile forms. Manual-save fieldsets use `display: contents`, which removes the layout box needed for the parent `space-y-8` utility to separate the section-card siblings. Restore a vertical spacing container on both fieldsets, then prove the shared edit and create surfaces with desktop and mobile bounding-box checks.

Confirmed root cause: `apps/web/app/settings/executors/[profileId]/page.tsx` and `apps/web/app/settings/executors/new/[type]/page.tsx` both render their card fragments inside `className="contents"` fieldsets introduced with manual-save coordination. The older executor routes keep spacing because their cards remain direct children of a normal `space-y-8` wrapper.

---

## Frontend

### Modern executor profile edit

- `apps/web/app/settings/executors/[profileId]/page.tsx`: keep the existing disabled fieldset and replace its layout-only `contents` class with a vertical `space-y-8` container so all conditionally rendered cards receive the shared settings rhythm.

### Modern executor profile create

- `apps/web/app/settings/executors/new/[type]/page.tsx`: apply the same fieldset layout treatment to the shared create form, covering local, worktree, Docker, Sprites, and any other supported non-SSH type routed through `CreateProfileSections`.

No API, store, state, copy, card primitive, or mobile navigation changes are required.

## Tests

- **What:** adjacent cards on a modern edit form have the intended separation and remain horizontally contained.
  **File:** `apps/web/e2e/tests/settings/executor-profile-spacing.spec.ts`
  **How:** seed data through the existing worker fixture, open the seeded worktree profile and a new worktree profile form, locate the visible card sections by their headings, and assert each adjacent bounding-box gap is at least the intended `2rem` rhythm.
- **What:** the same edit and create card spacing contract holds on a phone viewport.
  **File:** `apps/web/e2e/tests/settings/mobile-executor-profile-spacing.spec.ts`
  **How:** use the `mobile-chrome` Pixel 5 project, repeat the shared geometry assertion, and assert `document.documentElement.scrollWidth <= document.documentElement.clientWidth`.

The shared geometry and overflow assertions live in `apps/web/e2e/tests/settings/executor-profile-spacing-helpers.ts`.

The regression tests must be written and run red before the class change, then rerun green after the fix. No backend or unit test is needed because the defect is a rendered layout relationship with no non-trivial domain logic.

## E2E Tests

- **Scenario:** GIVEN a saved worktree executor profile, WHEN the modern profile editor is opened, THEN the visible settings cards have a consistent vertical gap.
  **File:** `apps/web/e2e/tests/settings/executor-profile-spacing.spec.ts`
  **What to verify:** the Profile Details, Environment Variables, Prepare Script, and subsequent visible card bounding boxes are separated.
- **Scenario:** GIVEN a new worktree executor profile form on desktop or phone, WHEN the form is opened, THEN its visible cards use the same gap and fit the viewport.
  **File:** `apps/web/e2e/tests/settings/executor-profile-spacing.spec.ts` and `apps/web/e2e/tests/settings/mobile-executor-profile-spacing.spec.ts`
  **What to verify:** card geometry and, on mobile, absence of document-level horizontal overflow.

## Verification Results

- RED: `cd apps/web && pnpm e2e:run --project chromium tests/settings/executor-profile-spacing.spec.ts` — failed as expected with a `0px` first card gap.
- RED: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-executor-profile-spacing.spec.ts` — failed as expected with a `0px` first card gap.
- GREEN: `cd apps/web && pnpm e2e:run --project chromium tests/settings/executor-profile-spacing.spec.ts` — 1 passed.
- GREEN: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-executor-profile-spacing.spec.ts` — 1 passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1 — sequential:

- [x] [task-01-restore-card-spacing](task-01-restore-card-spacing.md)

The production class changes and their regression tests are intentionally one sequential task because the test must establish the failing geometry before the fix is applied.

## Open Questions

None.
