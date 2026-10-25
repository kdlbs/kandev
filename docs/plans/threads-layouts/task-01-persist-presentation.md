---
id: "01-persist-presentation"
title: "Persist presentation preferences"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-005
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-005.1
  - AC-UI-THREADS-SAVED-VIEWS-005.2
  - AC-UI-THREADS-SAVED-VIEWS-005.3
  - AC-UI-THREADS-SAVED-VIEWS-005.7
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 01: Persist presentation preferences

## Summary

Persist layout and composer auto-hide with each saved view and draft, using the
existing backend settings lifecycle. Keep task querying and ordering independent
from presentation. This order adds data support without exposing new controls.

## In scope

- Add typed backend/frontend fields and Columns/false defaults, stored-field
  normalization, incoming validation, and view/draft round trips.
- Extend every view/draft construction, clone, snapshot, retry, rollback,
  effective-view merge, and action type. Update existing typed test fixtures
  only where the new required fields demand it.
- Cover current settings delivery through DTO, service, store, handlers, and
  the common frontend wire mapper. Preserve query fingerprints and admission.

## Out of scope

Rendered controls, grid composition, composer disclosure, DB schema changes,
new endpoints, and changing the existing five-chat limit.

## Acceptance

1. Valid preferences round-trip with views/drafts; legacy and malformed stored
   presentation fields recover independently, while unsupported new writes
   fail atomically.
2. Save/Save as/Duplicate/Discard and rapid-write rollback/retry retain every
   presentation/query field and preserve the existing settings revision path.
3. Presentation-only changes do not change query fingerprints, admitted IDs,
   stable order, active session, or unrelated settings.

## Verification

From the repository root. Bootstrap once in this fresh worktree before the
first pnpm command. Use /tdd for the new behavior, record behavioral RED, then
run these final commands after GREEN:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/user/... -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/ui/thread-view-wire.test.ts lib/state/slices/ui/thread-view-actions.test.ts lib/threads/thread-view-query.test.ts app/threads/threads-page-client.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/state/slices/ui/thread-view-types.ts lib/state/slices/ui/thread-view-wire.ts lib/state/slices/ui/thread-view-builtins.ts lib/state/slices/ui/thread-view-actions.ts lib/state/slices/ui/types.ts lib/types/http-user-settings.ts lib/threads/thread-view-query.ts --max-warnings 0)
git diff --check
```

Browser persistence and cross-client behavior are exercised through the user
controls in Task 04; this data boundary does not need an artificial UI test.
If a changed delivery mapper adds another test file, include that suite in
Results before completion.

## Files likely touched

- `apps/backend/internal/user/models/thread_views.go`
- `apps/backend/internal/backendapp/boot_thread_views.go` and its focused test
  for the native page payload's camel-case projection.
- `apps/backend/internal/user/service/thread_views.go`
- `apps/backend/internal/user/store/thread_views.go` and `sqlite.go`
- Existing `thread_views_test.go` in user service/store/DTO/handlers; model
  transport fields in `dto/dto.go` only if the typed model flow needs it.
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/ui/thread-view-{types,wire,builtins,actions}.ts`,
  `types.ts`, and existing wire/actions tests.
- `apps/web/lib/threads/thread-view-query.ts` and its tests.
- Compilation-only `ThreadView`/`ThreadViewDraft` fixtures in dependent tests.

## Dependencies

None. Read the completed saved-view package as the existing pattern.

## Risks

A zero-value boolean must not create a different omitted-field contract from
layout. Old draft/cloning paths can silently discard fields. Tolerant stored
reads and strict new-write validation must remain separate.

## Parallelism

sequential

## Inputs

- [Saved preferences requirement](../../specs/ui/requirements/threads-saved-views.md#req-ui-threads-saved-views-005-saved-presentation-preferences)
- [Presentation design](../../specs/ui/system-design/threads-saved-views.md#presentation-preferences)
- Existing `TestScanUserSettingsThreadViewDefaultsAndRoundTrip`,
  `thread-view-wire.test.ts`, and saved-view write-recovery tests.

## Results

Completed 2026-09-11.

- Behavioral RED: backend storage omitted both preferences; service/HTTP accepted
  unsupported writes. Frontend had 12 failing assertions covering mapping,
  defaults, effective drafts, persistence actions, and retry. Additional HTTP
  RED proved explicit empty/null fields needed strict request decoding.
- GREEN: all six packages in `go test ./internal/user/... -count=1` passed,
  including the new `handlers/thread_presentation_test.go` and DTO checks.
  Request JSON validation remains separate from tolerant stored decoding;
  `models/thread_view_json.go` contains the small request-decoding seam.
- `go run ./cmd/sqlguard ./internal` passed. Final
  `go test -race ./internal/persistence/storeconformance -count=1` passed (42s).
- The four required Vitest suites plus `lib/ssr/user-settings.test.ts`,
  `lib/ws/handlers/users.test.ts`, `lib/state/store.test.ts`, and the two
  `components/threads/threads-view-controls*.test.tsx` suites passed: 159 tests.
  After adding the rapid-write retry case, the final actions suite passed all
  nine tests (160 unique passing cases across these suites).
- `pnpm run typecheck`, the exact targeted ESLint command above, and
  `git diff --check` passed. Existing fixtures now include required defaults.
- Bootstrap completed with the pinned lockfile. Cache access needed sandbox
  escalation for Go and pnpm; no lockfile or dependency versions changed.
- No rendered behavior changes in this order. Desktop/phone integration and
  cross-client user controls remain assigned to Tasks 02–04.
- Task 02 browser integration exposed an omitted initial-page projection in
  `backendapp/boot_thread_views.go`, separate from the API/WS wire mapper.
  `go test ./internal/backendapp -run TestBootThreadPresentation -count=1`
  failed with both presentation fields missing, then passed after mapping
  them for saved views and drafts. The browser now restores Grid on reload.
