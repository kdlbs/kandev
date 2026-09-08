---
spec: docs/specs/tasks/requirements/multi-branch.md
related_specs:
  - docs/specs/integrations/requirements/gitlab-integration.md
created: 2026-07-24
status: completed
---

# Implementation Plan: Remote Repository Entry Reliability

## Overview

Make Remote task creation an explicit submit flow: URL-shaped input remains editable until Enter, then repository resolution begins. Preserve the committed URL when provider lookups fail, surface the exact failure with a retry action, and ensure preview environments can resolve public GitHub/GitLab resources without configured credentials.

## Backend

### Public GitHub task-creation metadata

- `apps/backend/internal/github/service_pr.go`: let `GetPR` and `GetIssue` fall back only from an absent/no-op client to an unauthenticated public GitHub REST read. Authenticated-client failures remain authoritative.
- `apps/backend/internal/github/service_reviews.go` or a focused sibling file: reuse the existing anonymous GitHub request rules and error/status mapping for public branches, PRs, and issues.
- `apps/backend/internal/github/service_test.go`: prove public PR, issue, and branch reads work through the no-client path and preserve 404/403 errors.

### Public GitLab branch discovery

- `apps/backend/internal/gitlab/controller_watches.go`: keep workspace-scoped branch listing, but use an anonymous `gitlab.com` client only when that workspace has no configured GitLab connection and the request targets the public host.
- `apps/backend/internal/gitlab/pat_client.go`: omit `PRIVATE-TOKEN` when the client has no token so public REST reads are truly anonymous.
- `apps/backend/internal/gitlab/controller_test.go` and `pat_client_test.go`: prove unconfigured public branch lookup succeeds, private/not-found errors remain visible, and no other integration route gains anonymous behavior.

## Frontend

### Explicit URL submission

- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`: remove commit-on-paste, blur, and Tab. Keep the staged value in the input, submit only on plain Enter, and render a visible `Remote URL` / Enter hint when the staged text is URL-shaped.
- Preserve picker-option selection as an immediate explicit action.
- Clear validation errors while the user edits and keep unsupported-provider validation on Enter.

### Resolution errors and retry

- `apps/web/hooks/domains/github/use-branches-by-url.ts`: retain the request error per normalized URL and expose it through the hook result.
- `apps/web/hooks/domains/github/use-pr-info-by-url.ts`: retain PR/issue metadata request errors per normalized URL instead of swallowing them.
- `apps/web/components/task-create-dialog-remote-repo-chips.tsx`: pass per-row resolution state and a retry callback that clears and re-runs both lookups.
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`: render an accessible inline error/retry state attached to the committed repository row while preserving the URL and any successfully resolved branch.

### Mobile design contract

- Desktop outcome and mobile entry point: the existing Remote repository chip opens the same picker; Enter is the explicit keyboard submit action, while provider options remain touch-selectable.
- Nearest shipped exemplar: the existing Remote repository popover and its 44px mobile input/option rows remain the geometry and interaction baseline.
- Information hierarchy and surface: the popover remains the single scroll owner. The URL/search field is primary, the `Remote URL` hint sits directly below it, and the row-level error places Retry beside the failed repository without introducing another overlay.
- Shared behavior: staged input, URL classification, error state, and retry are viewport-independent.
- Touch and containment: Retry is a visible touch-sized control; the popover remains viewport-contained with no document horizontal overflow.

## Tests

- **What:** paste, blur, and Tab do not commit; Enter commits once; URL-shaped input shows the hint; unsupported URLs validate on Enter. **File:** `apps/web/components/task-create-dialog-remote-repo-chip-url.test.tsx`. **How:** component tests against the real popover input.
- **What:** branch failures retain their error and clear after explicit retry. **File:** `apps/web/hooks/domains/github/use-branches-by-url.test.ts`. **How:** hook tests with first-call rejection and second-call success.
- **What:** PR and issue failures retain errors and retry without stale callbacks overwriting the successful result. **File:** `apps/web/hooks/domains/github/use-pr-info-by-url.test.ts`. **How:** hook tests with controlled promises.
- **What:** the repository row preserves its URL, announces the error, and invokes retry. **Files:** `apps/web/components/task-create-dialog-remote-repo-chip.test.tsx` and `task-create-dialog-remote-repo-chips.test.tsx`. **How:** component tests with per-row failure fixtures.
- **What:** unconfigured public GitHub PR/issue and GitLab branch reads succeed while upstream errors keep their status. **Files:** backend service/controller tests named above. **How:** `httptest` provider servers through the real service/controller path.

## E2E Tests

- **Scenario:** GIVEN a supported URL on phone, WHEN it is pasted and edited before Enter, THEN no repository resolution request fires, the `Remote URL` hint is visible, and Enter commits the final URL. **File:** `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`.
- **Scenario:** GIVEN branch resolution fails, WHEN the user sees the preserved row and taps Retry after the transient failure clears, THEN the branch appears and task creation becomes available. **File:** `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`.
- **Scenario:** GIVEN no GitHub credentials in a preview-like backend, WHEN a public GitHub repository/PR/issue is entered, THEN its branches and public metadata resolve. **File:** focused backend integration coverage; the browser E2E remains mocked to avoid external-network flake.

## Implementation Waves

Wave 1:

- [x] [task-01-public-provider-resolution](task-01-public-provider-resolution.md) — backend public GitHub/GitLab resolution

Wave 2:

- [x] [task-02-explicit-url-entry](task-02-explicit-url-entry.md) — staged input and Enter hint
- [x] [task-03-resolution-errors-and-retry](task-03-resolution-errors-and-retry.md) — hook errors and row retry UI

Wave 3:

- [x] [task-04-mobile-e2e](task-04-mobile-e2e.md) — phone interaction and retry coverage

## Verification

- `cd apps/backend && go test -run 'Test(ListRepoBranches|GetPR|GetIssue|HttpListProjectBranches|PATClient)' ./internal/github ./internal/gitlab`
- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-chip-url.test.tsx components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx hooks/domains/github/use-branches-by-url.test.ts hooks/domains/github/use-pr-info-by-url.test.ts`
- `make build-backend build-web`
- `cd apps && pnpm --filter @kandev/web e2e -- e2e/tests/task/mobile-create-task-remote-repo.spec.ts --project=mobile-chrome`
- Post-commit change-aware `/verify`.

## Risks

- Anonymous provider APIs are rate-limited. Failures must remain visible and retryable; authenticated workspace connections still take precedence.
- Public fallbacks must activate only for absent credentials and explicitly named public-host resources, never as an authorization bypass after an authenticated request fails.
- A PR metadata retry and branch retry can settle in either order. Per-URL sequence guards must prevent stale results from clearing newer success/error state.
- Enter handling must not break selecting a provider repository option or IME composition.
