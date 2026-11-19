---
id: "01-background-bootstrap"
title: "Run bootstrap outside the HTTP request"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.9
  - AC-WORKSPACES-IMPROVE-KANDEV-001.10
  - AC-WORKSPACES-IMPROVE-KANDEV-001.11
  - AC-WORKSPACES-IMPROVE-KANDEV-001.12
  - AC-WORKSPACES-IMPROVE-KANDEV-001.14
  - AC-WORKSPACES-IMPROVE-KANDEV-001.15
  - AC-WORKSPACES-IMPROVE-KANDEV-001.16
  - AC-WORKSPACES-IMPROVE-KANDEV-001.17
  - AC-WORKSPACES-IMPROVE-KANDEV-001.18
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
  - ../../specs/workspaces/system-design/improve-kandev-bootstrap.md
---

# Task 01 Run bootstrap outside the HTTP request

## Summary

Finish the existing setup in a background attempt. The dialog reads its
result and shows a retry action if preparation fails.

## In scope

- Add `TestImproveKandevBootstrapAsyncDeadline` first. Drive the real route
  through a loopback proxy with gated GitHub and clone calls. Require the
  POST and a pending GET to return while the calls wait. Observe the ready
  result through the same route when both finish.
- Extract the existing preparation path once. Add opt-in, a small job map,
  and an owner-scoped status route. Register worker cancellation and drain
  with backend runtime cleanup, including database restore.
- Stage only the initial managed checkout. Use the existing clone options,
  credentials, and path lock. Cover interrupted staging and retry in
  `TestImproveKandevClonePublishesOnSuccess`.
- Poll in the current bootstrap hook. Reuse the success payload, the shared
  form, and existing localized progress and error messages. Use **Retry**
  in the error banner. Keep draft state and ignore old responses after close
  or workspace change.
- Update the desktop and phone Improve Kandev Playwright fixtures to cover
  `202` followed by ready. Include one failure and retry scenario.
  Add brief recovery guidance near the Improve Kandev entry in
  `docs/public/use-kandev.md`.

## Out of scope

- A generalized queue, database schema, or WebSocket protocol.
- Changes to task launch, GitHub credential policy, or existing checkouts.

## Acceptance

1. The opt-in route returns while Git is blocked. The status endpoint
   restricts results to the initiating user and returns the existing
   bootstrap payload on success. Synchronous callers keep their contract.
2. A later attempt can retry after a clone failure or backend restart.
   Concurrent attempts reuse workspace resources and use separate
   diagnostic directories. Shutdown cancels active work.
3. The existing desktop and phone dialogs keep the draft during waiting
   and retry. Task creation becomes available after the current result is
   ready. A result from a closed or replaced attempt has no later effect.

## ASCII UI preview

**UI-01 Improve Kandev preparation** in the shared task form.
[Full preview](plan.md#ui-01-improve-kandev-preparation)

```text
Could not prepare Improve Kandev  [Retry]
[Start task disabled]
```

Keep Retry inside the form's scroll body. Use a standard button on desktop
and a target of at least 44px on phones. Covers
`AC-WORKSPACES-IMPROVE-KANDEV-001.15`.

## Lifecycle evidence

- Use the deadline regression to show that a worker survives the end of
  the admitting request. Cover failure followed by retry, overlapping
  requests, another caller's status read, and shutdown in the Go tests.
- Use the clone test to show that an interrupted attempt cannot publish
  a partial checkout. Recreate the handler to check that a fresh attempt
  reuses completed resources after the job map is cleared.
- Use dialog tests to show that pending and failed attempts block
  submission. Cover retry and canceling polling on close.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/improvekandev ./internal/integration ./internal/repoclone -run 'Test(BootstrapJob|ImproveKandevBootstrap|ImproveKandevClonePublishesOnSuccess)' -count=1)
(cd apps/web && pnpm exec vitest run lib/api/domains/improve-kandev-api.test.ts components/improve-kandev-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/improve-kandev.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-improve-kandev.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/improvekandev/handler.go` and `bootstrap_jobs.go`
- `apps/backend/internal/backendapp/helpers.go` and `main.go` for worker cleanup
- `apps/backend/internal/repoclone/` for staged publication and its test
- `apps/backend/internal/integration/improve_kandev_bootstrap_jobs_test.go`
- `apps/web/lib/api/domains/improve-kandev-api.ts` and its tests
- `apps/web/components/improve-kandev-dialog.tsx`,
  `improve-kandev-dialog-create.tsx`, and `improve-kandev-dialog.test.tsx`
- `apps/web/e2e/tests/improve-kandev.spec.ts` and `mobile-improve-kandev.spec.ts`
- `docs/public/use-kandev.md`

## Dependencies

None.

## Risks

The result contains identity-bound data. Keep its access checks inside the
Improve Kandev handler. Staging must preserve any existing checkout.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/improve-kandev.md)
- [Bootstrap lifecycle](../../specs/workspaces/system-design/improve-kandev-bootstrap.md)

## Results

Implemented the async owner-scoped bootstrap job endpoint, retrying dialog
poller, cleanup integration, and staged initial clone publication. Added
coverage for deadline independence, ownership, retry after clone failure,
restart reuse, overlapping jobs, shutdown cancellation, capacity/retention,
and canceled clone staging. Desktop and phone flows retain the draft through
retry; the phone Retry target is at least 44px. Added public recovery guidance.

All listed verification passed, including the race-enabled Go suite, web unit
tests, typecheck, i18n check, desktop and phone Playwright tests, and public
documentation tests and validation. Backend and Vite production builds passed.
