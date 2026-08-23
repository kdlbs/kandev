---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-07-31
status: done
---

# Implementation Plan: Stable GitHub Settings Refresh

## Overview

The GitHub status hook currently clears the workspace cache before every manual refresh, while the
settings components replace loaded content whenever the shared loading flag is true. Preserve the
same-workspace snapshot during an in-flight refresh, reserve the full loading placeholder for an
initial or workspace-changing load, and expose refresh progress on the existing icon control.

## Frontend

### Workspace status state

In `apps/web/hooks/domains/github/use-github-status.ts`, make `refresh` start a versioned fetch
without resetting the existing workspace entry. Keep the existing initial-load reset path and
request-version guard so workspace changes cannot render another workspace's status and older
requests cannot overwrite the newest result.

### GitHub settings status

In `apps/web/components/github/github-status.tsx`, render the initial placeholder only when the
current workspace has no loaded status. Continue rendering loaded automation and personal identity
content while `loading` is true. Pass the loading state into the automation actions so the existing
44px refresh button is disabled, marked busy, and animates its refresh icon until the request
settles. This is shared presentation behavior across desktop and mobile; no responsive composition,
scroll ownership, navigation, or touch target changes.

The nearest shipped mobile exemplar is `apps/web/components/branch-refresh-button.tsx`: retain the
visible 44px icon target and use its disabled/spinning progress treatment. The mobile entry point,
information hierarchy, primary actions, single page scroll owner, and safe-area behavior remain
those of the existing GitHub settings page.

## Tests

- **What:** A same-workspace manual refresh retains the loaded status while setting `loading`.
  **File:** `apps/web/hooks/domains/github/use-github-status.test.tsx`.
  **How:** Resolve the initial status, gate the refresh promise, and assert the cached actor remains
  available with `loading: true` before the second response resolves.
- **What:** Loaded settings remain rendered and the refresh control presents local progress.
  **File:** `apps/web/components/github/github-status.test.tsx`.
  **How:** Render the personal identity settings with loaded status plus `loading: true`; assert
  the current identity remains present instead of returning the initial empty state. The browser
  regressions cover the automation refresh control's disabled/busy/spinning presentation.

## E2E Tests

- **Scenario:** GIVEN a loaded desktop workspace status, WHEN refresh is held pending, THEN the
  current identity remains visible and the refresh control is busy without showing the initial
  placeholder. **File:** `apps/web/e2e/tests/integrations/github-authentication.spec.ts`.
- **Scenario:** GIVEN the same state on a narrow touch viewport, WHEN refresh is held pending, THEN
  the same content and 44px refresh target remain in place without horizontal overflow.
  **File:** `apps/web/e2e/tests/integrations/mobile-github-auth-settings.spec.ts`.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-stabilize-refresh](task-01-stabilize-refresh.md) — done

No task is marked parallel-safe; the hook, component, and regression coverage form one small
vertical slice.

## Risks

- Connection save and disconnect flows also call `refresh`; they must still reconcile to the new
  server status after the request completes while avoiding an interim blank state.
- Workspace changes must continue to clear unrelated status rather than carrying stale identity
  data across workspace boundaries.
