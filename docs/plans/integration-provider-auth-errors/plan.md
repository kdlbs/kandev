---
spec: docs/specs/auth/requirements/auth.md
related_spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-07-31
status: done
---

# Implementation Plan: Integration Provider Authentication Errors

## Overview

The shared web API client currently treats every HTTP 401 as an expired Kandev browser session.
GitHub, GitLab, Jira, and Linear also return 401 when their own credentials expire, so the global
handler navigates to `/login` before each integration page can render its existing loading error.
Gate the global redirect on the backend's Kandev session challenge, then prove provider errors stay
local and invalid GitHub PAT saves remain visible and retryable.

## Confirmed root cause

- `apps/web/lib/api/client.ts` calls the global `onUnauthorized` callback for every 401 response.
- `apps/web/src/main.tsx` implements that callback with `window.location.assign("/login")`.
- Kandev's global auth middleware identifies a real session challenge with
  `WWW-Authenticate: Bearer`; upstream-provider 401 responses from integration handlers do not.
- The four provider data pages (`/github`, `/gitlab`, `/jira`, `/linear`) already catch request
  failures and render result errors, but the unconditional global redirect prevents users from
  seeing them.
- GitHub PAT replacement is already validated before persistence and the connection dialog already
  retains failed drafts; focused coverage is missing for a connection-only invalid PAT rejection.

## Frontend

### Shared API authentication classification

- Update `apps/web/lib/api/client.ts` so `onUnauthorized` runs only for a 401 carrying Kandev's
  `WWW-Authenticate: Bearer` session challenge.
- Continue parsing and throwing `ApiError` for every non-2xx response so provider pages and settings
  forms receive the original sanitized error message.
- Add `apps/web/lib/api/client.test.ts` covering challenged and unchallenged 401 responses.

### GitHub PAT save feedback

- Extend `apps/web/components/github/github-connection-dialog.test.tsx` with a connection-only
  invalid-PAT rejection. Assert the dialog stays open, the error toast is visible, the token draft
  is retained, and `onSaved` is not called.
- No production component change is expected unless the regression exposes behavior that differs
  from the amended spec.

### Mobile contract

The repair changes request classification only. It does not change composition, navigation,
overlays, touch behavior, scrolling, or viewport-dependent interaction. Desktop and mobile share
the same API client and provider-error state, so focused unit/component coverage plus the existing
mobile integration settings suites satisfy mobile parity; no new mobile-only composition is needed.

## Tests

- **What:** a Kandev session challenge still clears the stale browser identity.
  **File:** `apps/web/lib/api/client.test.ts`.
  **How:** mock a 401 response with `WWW-Authenticate: Bearer` and assert `onUnauthorized` runs once
  before `ApiError` is thrown.
- **What:** an integration/provider 401 remains a normal request error.
  **File:** `apps/web/lib/api/client.test.ts`.
  **How:** mock a 401 without the challenge header, assert `onUnauthorized` is not called, and assert
  the provider error body becomes the thrown `ApiError` message.
- **What:** an invalid GitHub PAT is visible and retryable.
  **File:** `apps/web/components/github/github-connection-dialog.test.tsx`.
  **How:** reject `setGitHubWorkspaceConnection`, save a PAT-only draft, and assert the open dialog,
  retained input, error toast, and unchanged saved callback.

## E2E Tests

- **Scenario:** GIVEN configured GitHub, GitLab, Jira, and Linear integrations, WHEN each provider
  data request returns an unchallenged 401, THEN its page remains on the provider route and displays
  the loading error instead of navigating to `/login`.
  **File:** `apps/web/e2e/tests/integrations/provider-auth-errors.spec.ts`.
  **What to verify:** seed each integration with existing E2E helpers, intercept its primary data
  endpoint with a sanitized 401 response, assert the provider-specific error text and unchanged URL.
- **Scenario:** GIVEN GitHub settings with a connected workspace, WHEN a replacement PAT save is
  rejected as invalid, THEN the connection surface remains open, displays the error, and retains the
  submitted token for correction.
  **File:** `apps/web/e2e/tests/integrations/provider-auth-errors.spec.ts`.
  **What to verify:** intercept `PUT /api/v1/github/workspace-connection`, submit the dialog, and
  assert no `/login` navigation, visible error feedback, retained PAT input, and existing connection.

## Implementation Tasks

- [x] [Task 01: Classify session challenges and preserve PAT errors](task-01-classify-session-challenges.md)
- [x] [Task 02: Prove provider page behavior end to end](task-02-provider-auth-e2e.md)

Execution is sequential: Task 02 depends on Task 01. No task is parallel-safe because the E2E proof
depends on the shared API-client behavior implemented by Task 01.

## Risks And Out Of Scope

- The challenge check must preserve redirect behavior for truly expired Kandev sessions.
- Provider error bodies remain subject to each backend handler's existing sanitization; this repair
  does not redesign provider-specific copy or HTTP status mappings.
- No credential schema, persistence, health-poller, settings layout, or mobile composition changes.
- No new behavior for integration settings beyond GitHub's requested invalid-PAT save feedback.
