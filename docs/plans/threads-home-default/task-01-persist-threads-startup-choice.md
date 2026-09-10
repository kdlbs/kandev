---
id: "01-persist-threads-startup-choice"
title: "Persist the Threads startup choice"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003
acceptance_criteria:
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.1
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.5
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.3
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
---

# Task 01: Persist the Threads Startup Choice

## Summary

Extend the existing `startup_page` setting with `threads` and prove that all
storage and delivery paths retain it. The existing default, user ownership,
revision ordering, and partial-update semantics remain authoritative.

## In scope

- Use `/tdd`; start with failing enum, persistence, and mapper cases.
- Add the backend constant and validation/normalization case. Exercise
  controller/DTO, JSON scan/marshal, update event, and Go boot payload.
- Add focused repository tests that save/reopen Threads, preserve it through
  an unrelated patch, keep users independent, and retain old/unknown-value
  behavior. Test SQLite and environment-gated PostgreSQL via the existing
  isolated-database helper.
- Extend the frontend `StartupPage` union, common parser, HTTP/save mapping,
  and WS tests. Update E2E API helper type declarations to accept Threads.

## Out of scope

- Home routing, settings markup/copy, new fields, SQL schema changes, and
  localStorage writes.

## Acceptance

- `threads` survives save, repository reopen, HTTP DTO, WS event, Go boot
  mapping, and frontend mapping; existing values still round-trip.
- Unsupported submitted values fail without mutation; absent/unknown stored
  values retain overview fallback and omitted patches preserve saved values.
- The field stays per-user in existing JSON settings and does not affect
  listing/recent-task storage, workspace selection, or unrelated settings.

## Verification

Install dependencies once in this worktree before the first pnpm command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run from the repository root; each directory change is scoped to one command:

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/user/... -run StartupPage -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/backendapp -run TestMapUserSettingsStateIncludesNormalizedStartupPage -count=1)
make -C apps/backend sqlguard
(cd apps/backend && go test -tags fts5 -race ./internal/persistence/storeconformance -count=1)
(cd apps && pnpm --filter @kandev/web test lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Configure `KANDEV_TEST_POSTGRES_DSN` to a disposable test PostgreSQL database
before the Go checks so repository and conformance PostgreSQL subtests run.
The conformance command includes fresh, replay, and previous-Stable upgrade
coverage. No fixture/manifest edits are needed without a schema-history change.
If PostgreSQL cannot run, record its exact missing evidence. New Go test names
must contain `StartupPage` so the focused command discovers them.

PR review aligned these commands with CI's `fts5` build tag and made the
blocks safe to paste from the repository root. The tagged startup-page, boot
mapping, and SQLite fresh/replay/upgrade conformance commands passed during
fixup. PostgreSQL was not rerun: the original disposable fixture had been
removed after the successful implementation checks, and remediation changed
no backend implementation or persistence contract.

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/models/startup_page_test.go` (new)
- `apps/backend/internal/user/service/service.go` and existing startup tests in
  `service_test.go`; put new event cases in `startup_page_test.go` if needed
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/controller/controller_test.go`
- `apps/backend/internal/user/store/sqlite_test.go` (existing cases) and
  `startup_page_test.go` (new repository tests)
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/ssr/user-settings.ts` and `user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/e2e/helpers/api-client.ts`

Existing DTO/store/event/boot production code already calls the normalizer.
Change it only if a focused regression proves an additional propagation gap.
Do not grow already oversized Go test files with new helper suites.

## Dependencies

None.

## Risks

- A forgotten normalizer would coerce a successfully saved Threads value back
  to overview during hydration.
- A settings patch must retain the existing revision and user isolation rules.
- PostgreSQL tests silently skip without their DSN; inspect subtest results.

## Parallelism

`sequential`

## Inputs

- Owning requirements `002.1`, `002.5`, and `003.3`.
- Design sections **Preference ownership** and **Portable settings contract**.
- `TestApplyStartupPage`, `TestScanUserSettingsStartupPage`, DTO startup patch
  tests, and `TestMapUserSettingsStateIncludesNormalizedStartupPage`.
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, and ADR 0041.

## Results

Implemented after frontend and backend assertion failures confirmed the missing
enum support. Startup settings tests pass with SQLite and isolated PostgreSQL;
boot mapping, SQL guard, 87 frontend mapper/WS tests, and typecheck pass.
Fresh/replay and previous-Stable storage conformance passed on SQLite and
PostgreSQL (128s), using localhost access after the sandbox blocked its initial
PostgreSQL connections. No schema changes were needed.
