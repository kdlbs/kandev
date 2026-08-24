---
id: "10-persistent-request-lifecycle"
title: "Persistent review-request lifecycle"
status: done
wave: 5
depends_on:
  - "06-pr-scoped-request-state"
  - "07-mobile-pr-identity-layout"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 10: Persistent Review-request Lifecycle

## Acceptance

- An in-flight or successful optimistic request survives keyed panel unmounts
  and remounts for the same immutable PR identity.
- A deterministic deferred-write A → B → A test proves only one POST and no
  re-enabled dismissed action.
- The atomic duplicate guard does not depend on a React render occurring
  between clicks.
- Optimistic pending clears when current requested-reviewer data confirms it
  or when a newer review by that reviewer supersedes the dismissed baseline,
  even if no refresh ever observed the intermediate requested state.
- Failed writes remain retryable and lifecycle metadata is reclaimed after a
  terminal server state.
- Obsolete duplicate mutation helpers/tests are removed.

## Ownership

- `apps/web/components/github/use-pr-scoped-review-request.ts`
- `apps/web/components/github/pr-detail-panel.tsx` and `.test.ts`
- `apps/web/components/github/pr-reviews-section.tsx` and `.test.tsx` only if
  the callback needs review identity data.
- `apps/web/e2e/tests/pr/mobile-pr-rerequest-review.spec.ts`
- Task 10 status only.

Do not alter mobile layout geometry, backend, public docs, spec, or `plan.md`.

## Output contract

Use TDD. Report RED/GREEN evidence for both lifecycle races, exact checks,
state-lifetime/cleanup rationale, files changed, and set only this task file to
`done`.
