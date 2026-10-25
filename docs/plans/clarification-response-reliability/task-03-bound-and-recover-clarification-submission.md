---
id: "03-bound-and-recover-clarification-submission"
title: "Bound and recover clarification submission"
status: done
wave: 3
depends_on:
  - "02-bound-clarification-response-resolution"
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-response-reliability.md"
---

# Task 03: Bound and recover clarification submission

## Outcome

Desktop and phone clarification controls leave their progress state within 40
seconds, preserve the user's answer, and safely retry the authoritative backend
operation.

## In scope

- Add a 40-second `AbortController` deadline to the shared clarification POST
  helper and clear its timer on success, error, and abort.
- Treat timeout, network failure, 503, and unexpected 5xx as the existing
  retryable error state.
- Preserve selected answers, last action, submit-time bundle ownership, and
  generation/mutex guards across failure and Retry.
- Restore existing dismiss/collapse/Escape and Skip actions after a retryable
  failure while keeping them disabled during the bounded request.
- Extend unit coverage with fake timers and controlled fetch promises.
- Extend desktop Chromium and Pixel 5 E2E coverage for 503, preserved answers,
  Retry, dismissal reachability, and the final accepted outcome.

## Exclusions

- No new drawer, dialog, route, toast, or mobile-only state.
- No optimistic success on timeout or server error.
- No change to local-dismiss or Skip semantics.
- No increased Playwright timeout to hide the failure mode.

## Traceability

- `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.1`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.3`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.4`
- `docs/specs/tasks/system-design/clarification-response-reliability.md`

## Implementation acceptance

- A non-resolving fetch enters the retryable error state at 40 seconds under
  fake timers. It aborts once and clears the timer. It releases the in-flight
  guard and retains all selected answers.
- Retry after timeout, 503, or network failure applies the authoritative winner
  response. It cannot update a replacement bundle or release a newer request's
  guard.
- Desktop and Pixel 5 tests expose a touch-accessible and keyboard-accessible
  Retry. They restore dismiss and Skip after an error. They complete the same
  answer without horizontal overflow or re-entry.

## TDD sequence

1. Add failing fake-timer tests for a non-resolving fetch, abort cleanup,
   preserved input, and Retry ownership.
2. Add failing component assertions that retryable failure restores the
   existing local actions.
3. Implement the client deadline in the shared hook without adding viewport
   branches.
4. Extend the existing desktop failure E2E and mobile clarification E2E with
   backend-shaped 503 then successful retry.
5. Run type, lint, localization, desktop, and mobile gates.

## Likely files

- `apps/web/hooks/domains/session/use-clarification-group.ts`
- `apps/web/hooks/domains/session/use-clarification-group.test.ts`
- `apps/web/hooks/domains/session/use-clarification-group.timeout.test.ts`
- `apps/web/hooks/domains/session/use-clarification-group.regressions.test.ts`
- `apps/web/components/task/chat/clarification-input-overlay.test.tsx`
- `apps/web/e2e/tests/chat/clarification-submit-failure.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Dependencies

- Task 02 defines the backend timeout envelope and preserves the server deadline
  below the browser deadline.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/session/use-clarification-group.test.ts hooks/domains/session/use-clarification-group.timeout.test.ts hooks/domains/session/use-clarification-group.regressions.test.ts components/task/chat/clarification-input-overlay.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run lint`
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
- `cd apps/web && pnpm e2e:raw --project=chromium tests/chat/clarification-submit-failure.spec.ts`
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/chat/mobile-clarification.spec.ts`

## Results

Implemented the shared 40-second `AbortController` deadline while preserving
answers, retry ownership, and the existing desktop/mobile recovery controls.
The hook and overlay tests passed, including timeout, 503, Retry, Skip, and
local Escape dismissal coverage. Typecheck, lint, localization, and ratchet
checks passed. Managed Chromium clarification failure coverage passed 2 tests,
and Pixel 5 clarification coverage passed 8 tests.
