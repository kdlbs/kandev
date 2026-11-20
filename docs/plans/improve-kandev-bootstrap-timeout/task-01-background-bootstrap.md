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
- Cancel the manager while a worker is held at its completion boundary, then
  let preparation return success. The successful response must remain ready
  even when cancellation races with result recording.
- Use the clone test to show that an interrupted attempt cannot publish
  a partial checkout. Recreate the handler to check that a fresh attempt
  reuses completed resources after the job map is cleared.
- Use dialog tests to show that pending and failed attempts block
  submission. Cover retry and canceling polling on close.

## Verification

Run from the repository root unless a working directory is shown. All commands
listed here passed after the review fixes.
For a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` before
the first frontend command. The shared worktree already had dependencies
installed, so that setup command was not needed here.

- `bash scripts/lint-go-changed apps/backend/internal/backendapp/helpers.go apps/backend/internal/backendapp/httpserver_test.go apps/backend/internal/backendapp/main.go apps/backend/internal/backendapp/restore_quiesce.go apps/backend/internal/backendapp/restore_quiesce_test.go apps/backend/internal/backendapp/startup_mcp_order_test.go apps/backend/internal/improvekandev/bootstrap_jobs.go apps/backend/internal/improvekandev/bootstrap_jobs_test.go apps/backend/internal/improvekandev/goleak_test.go apps/backend/internal/improvekandev/handler.go apps/backend/internal/improvekandev/handler_test.go apps/backend/internal/integration/improve_kandev_bootstrap_jobs_test.go apps/backend/internal/integration/improve_kandev_bootstrap_recovery_test.go apps/backend/internal/integrations/workspacescope/workspacescope.go apps/backend/internal/integrations/workspacescope/workspacescope_test.go apps/backend/internal/repoclone/checkout_options.go apps/backend/internal/repoclone/clone.go apps/backend/internal/repoclone/clone_staging_test.go apps/backend/internal/repoclone/clone_test.go` — all 19 changed Go files, 0 lint issues.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp ./internal/improvekandev ./internal/integration ./internal/repoclone ./internal/integrations/workspacescope -count=1)` — passed.
- `go test -v -run '^TestBootstrapJobManagerRetainsSuccessfulResultAfterCancellation$' ./internal/improvekandev` — passed after the test first reproduced the interrupted-result regression.
- `(cd apps/backend && go test -c -trimpath -tags fts5 -o /tmp/repoclone-staging.test ./internal/repoclone && runuser -u nobody -- /tmp/repoclone-staging.test -test.run '^TestCloneIntoExistingEmptyDestinationWhenParentIsNotWritable$' -test.count=1)` — passed as an unprivileged user.
- `(cd apps/backend && make build)` — passed.
- `(cd apps/web && pnpm exec vitest run lib/api/domains/improve-kandev-api.test.ts components/improve-kandev-dialog.test.tsx && pnpm run typecheck && pnpm run i18n:check && pnpm run build:vite)` — 23 tests passed; typecheck, i18n checks, and Vite production build passed.
- `(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project chromium tests/improve-kandev.spec.ts -g 'retries a failed bootstrap')` — passed.
- `(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project mobile-chrome tests/mobile-improve-kandev.spec.ts)` — passed at 700px touch width; Retry target measured at least 44px.
- `node --test scripts/validate-public-docs.test.mjs .github/scripts/pr-docs-git.test.cjs .github/scripts/pr-docs.test.cjs` — 174 tests passed.
- `python3 scripts/list-docs.py validate` — passed; 457 decisions and 2,048 specifications validated.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `node scripts/validate-public-docs.mjs` — passed; 55 published pages validated.
- `git diff --check` — passed.

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

The review fixes register bootstrap cancellation and join in the restore path,
preserve empty destination cloning while rejecting occupied paths, cancel
workers waiting for a repository lock, and bound each browser status read.
Coverage now exercises production restore registration with a gated clone,
empty-target compatibility, lock cancellation, cold overlapping attempts, and
recovery of a repository row whose checkout is missing. Integration fixtures
close their servers before database cleanup. The desktop and phone retries keep
the draft, and the phone retry target is at least 44px.

The job manager also keeps a successful preparation response when cancellation
arrives just before the worker records its result. A deterministic test cancels
the manager while preparation is gated, then returns a successful response.

The UI behavior remains covered by the existing public recovery guidance in
`docs/public/use-kandev.md`; this remediation changed no public contract or
terminology. The screenshots in the PR were recaptured after the touch-target
change.
