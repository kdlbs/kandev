---
spec: docs/specs/ui/requirements/github-pr-review-actions.md
created: 2026-07-23
status: completed
---

# Implementation Plan: GitHub PR Review Re-request

## Overview

Add the missing GitHub review-request write contract, expose it beside a
dismissed review, and reuse that PR detail surface on phones. Backend and core
frontend work can proceed in parallel because the route and payload are fixed
by the spec; mobile composition and E2E follow after the shared action works.

## Backend

### GitHub client contract and implementations

- Add `RequestReviewers(ctx, owner, repo, number, reviewers)` to
  `apps/backend/internal/github/client.go`.
- Implement GitHub REST
  `POST /repos/{owner}/{repo}/pulls/{number}/requested_reviewers` in
  `gh_client.go` and `pat_client.go`.
- Keep `noop_client.go` fail-closed.
- Make `mock_client.go` record the request and update the seeded PR's
  `RequestedReviewers`, so integration and browser tests observe the same
  transition as GitHub.

### Service and HTTP route

- Add a service method in `service_pr.go` that delegates the write and deletes
  the affected `prFeedbackCache` and `prStatusCache` key after success.
- Register
  `POST /api/v1/github/prs/:owner/:repo/:number/requested-reviewers` in
  `controller.go`.
- Validate the PR number and a non-empty list of non-blank reviewer logins.
  Return the established GitHub/configuration error response and
  `{"requested": true}` on success.

## Frontend

### API and dismissed-review action

- Add `requestPRReviewers` to
  `apps/web/lib/api/domains/github-api.ts`.
- In `pr-detail-panel.tsx`, own mutation state, success/error toast, and
  feedback refresh using the existing approve-action pattern.
- In `pr-reviews-section.tsx`, reconcile review history with explicit current
  requests case-insensitively. Only the latest `DISMISSED` review on an open PR
  gets a re-request action, and a requested reviewer renders once as pending.
- Add a small accessible trailing action to `pr-shared.tsx` only if needed by
  the existing row layout. Use a compact direct sync action on wider screens
  and a minimum 44px touch target on phone/coarse-pointer layouts.

### Mobile design contract

- **Desktop outcome:** request the dismissed reviewer from the existing PR
  detail review row.
- **Phone entry:** task bottom navigation **Review**.
- **Exemplar:** GitLab's full-height mobile review in
  `apps/web/components/task/mobile/session-mobile-layout.tsx`, plus
  `ReviewPRSelector` for multi-PR selection.
- **Hierarchy/surface:** optional PR selector, then one full-height PR detail
  surface; the dismissed-review action remains inside its review row.
- **Scroll/geometry:** mobile layout owns `100dvh`; PR content keeps one
  internal vertical scroll owner; existing bottom-nav safe-area padding stays
  authoritative; no document horizontal overflow.
- **Shared logic:** the same feedback hook, request handler, PR selection, and
  reviews component serve every viewport. Only mobile composition differs.
- **Primary action:** visible re-request control with an accessible reviewer
  name and at least a 44px active touch dimension.

## Tests

- Backend transport tests assert gh/PAT method, endpoint, and reviewer JSON.
- Backend mock/service/controller tests assert mutation, validation, error
  behavior, and per-PR cache eviction.
- Frontend API tests assert method, path, and body.
- Review reconciliation tests cover dismissed-only eligibility,
  case-insensitive current requests, duplicate suppression, and closed PRs.
- Mobile layout tests cover GitHub PR Review availability and fallback to the
  existing GitLab surface when no GitHub PR exists.

## E2E Tests

- Desktop: seed an open PR with a dismissed review, use the PR detail row
  action, and assert the reviewer becomes pending with no duplicate action.
- Mobile: enter Review from bottom navigation, perform the same request, and
  assert pending state, 44px action geometry, viewport containment, and no
  document horizontal overflow.

## Implementation Waves

Wave 1 (parallel):

- [x] [Task 01: Backend review-request contract](task-01-backend-review-request.md) — done
- [x] [Task 02: Dismissed-review action](task-02-frontend-dismissed-review-action.md) — done

Wave 2:

- [x] [Task 03: Mobile GitHub PR review surface](task-03-mobile-github-pr-review.md) — done

Wave 3:

- [x] [Task 04: Desktop and mobile E2E](task-04-e2e-rerequest-review.md) — done
- [x] [Task 05: Public review-flow docs](task-05-public-review-flow-docs.md) — done

Review remediation (parallel):

- [x] [Task 06: PR-scoped optimistic request state](task-06-pr-scoped-request-state.md) — done
- [x] [Task 07: Mobile PR identity and narrow-row safety](task-07-mobile-pr-identity-layout.md) — done
- [x] [Task 08: Reviewer input hardening](task-08-reviewer-input-hardening.md) — done
- [x] [Task 09: Key-scoped PR cache invalidation](task-09-key-scoped-cache-invalidation.md) — done
- [x] [Task 10: Persistent review-request lifecycle](task-10-persistent-request-lifecycle.md) — done
- [x] [Task 11: Reclaim key invalidation metadata](task-11-reclaim-cache-epochs.md) — done

## Verification

After targeted task checks, run formatting before the full pipeline:

```bash
make fmt
make typecheck test lint
```

Then run focused production-build browser coverage:

```bash
cd apps/web
pnpm e2e:run tests/pr/pr-rerequest-review.spec.ts tests/pr/mobile-pr-rerequest-review.spec.ts
```

## Risks

- GitHub keeps the old dismissed review in review history after a new request;
  explicit `requested_reviewers` must win during rendering.
- Feedback/status caches can otherwise make a successful mutation look stale.
- Adding GitHub to the phone Review destination must not remove the existing
  GitLab fallback for tasks without a GitHub PR.
