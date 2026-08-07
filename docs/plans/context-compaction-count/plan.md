---
spec: docs/specs/context-compaction-count/spec.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Context Compaction Count

## Overview

Extend the existing context-window persistence boundary so one database update stores the latest sample and atomically increments a session-owned lifetime count when usage decreases. Propagate that count through the existing session metadata event, hydrate it into the shared session-runtime store, and add a compact translated row with an accuracy disclosure to the existing context hover. Focused repository/orchestrator tests prove restart and duplicate-delivery behavior; existing desktop and mobile context-hover E2E scenarios prove the user-visible path.

## Backend

### Session metadata contract

- Add `SessionMetaKeyContextWindow` and `SessionMetaKeyContextCompactionCount` constants in `apps/backend/internal/task/models/models.go` so reset, model-change, repository, and orchestrator paths use one spelling for the two session-owned metadata keys.
- Keep the feature in `task_sessions.metadata`; no schema migration or backfill is needed. A missing `context_compaction_count` reads as zero.

### Atomic context-window persistence

- Add `UpdateSessionContextWindow(ctx context.Context, sessionID string, contextWindow map[string]interface{}) (int64, error)` to the concrete task repository and the orchestrator's narrow `sessionExecutorStore` in `apps/backend/internal/orchestrator/service.go`. Keeping it on the narrow context-window boundary avoids forcing unrelated repository test doubles to implement a method they do not use. The returned value is the persisted lifetime count.
- Implement the method in `apps/backend/internal/task/repository/sqlite/session.go` with dialect-specific SQL that, in one row update, compares the incoming `used` value with the previously persisted `metadata.context_window.used`, increments `metadata.context_compaction_count` only for a strict decrease, stores the new `context_window`, and returns the resulting count.
- Treat an absent/non-numeric previous sample or absent count as zero without incrementing. Re-delivering the same decreased sample is idempotent because the first update makes it equal to the persisted baseline.
- Add SQLite behavior coverage in `apps/backend/internal/task/repository/sqlite/session_test.go` for the first sample, increase/equality, decrease, repeated decrease, a legacy missing count, and preservation of unrelated metadata.
- Add the equivalent environment-gated PostgreSQL behavior coverage in `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`, as required for changed dialect-sensitive task-repository methods.

### Orchestrator event propagation

- Update `apps/backend/internal/orchestrator/context_window.go` so the existing per-session generation guard calls `UpdateSessionContextWindow` and returns the persisted count with the generation result.
- Update `apps/backend/internal/orchestrator/event_handlers_git.go` so a successful write publishes one `session.state_changed` metadata patch containing both `context_window` and `context_compaction_count`. Failed or generation-stale writes publish neither.
- Replace literal context-window metadata keys in the touched reset/model-change paths with the model constants while preserving the existing behavior: clearing the current sample never clears or increments the lifetime count.
- Include the persisted count in context-window replay snapshots used during WebSocket session hydration.
- Extend `apps/backend/internal/orchestrator/event_handlers_git_test.go` to prove a decrease increments, a backend-service recreation continues from persisted metadata, duplicate delivery does not double count, reset leaves the count intact, and the published patch contains the resulting count.

## Frontend

### Session-runtime state and hydration

- Extend `ContextWindowEntry` in `apps/web/lib/state/slices/session-runtime/types.ts` with `compactionCount: number`.
- Update `parseContextWindowEntry` in `apps/web/lib/state/slices/session-runtime/context-window.ts` to accept the adjacent metadata count, normalize missing/invalid/negative values to zero, and include it in the parsed entry.
- Update `apps/web/lib/ws/handlers/agent-session.ts` to read `context_compaction_count` from the same `metadata` or `session_metadata` object as `context_window`, then pass it to the parser. Preserve explicit `context_window: null` cache invalidation.
- Update `apps/web/hooks/domains/session/use-session-context-window.ts` so both Zustand subscriptions and initial session-metadata hydration retain the count across refresh and restart recovery.
- Cover parsing, live metadata patches, legacy missing counts, and hydrated session metadata in the existing focused unit tests.

### Context hover

- Update `apps/web/components/task/chat/token-usage-display.tsx` to render a `Compactions` row with the tabular count and an inline info control. The help text states that Kandev infers the count from observed token drops and that missing samples or provider resets can make it approximate.
- Reuse the component's current inline help pattern instead of nesting another Radix tooltip inside `TooltipContent`; the help must remain reachable by pointer hover, keyboard focus, and touch focus while the parent context hover stays open.
- Add the new user-facing strings to `apps/web/src/locales/en/common.json`, regenerate `apps/web/src/locales/pseudo/common.json`, and resolve them at render time with `useTranslation`.
- Extend `apps/web/components/task/chat/token-usage-display.test.tsx` for zero and non-zero counts plus the accessible disclosure. Update context-window store/parser tests and fixtures that construct `ContextWindowEntry` values.

### Mobile design contract

- **Desktop outcome:** the existing context ring hover adds the lifetime count and accuracy help below the token/source row.
- **Mobile entry point and exemplar:** the existing context ring remains tap-pinnable; `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts` and the component's existing source-help control are the nearest shipped pattern.
- **Hierarchy and presentation:** current percent, progress, token count, and source remain primary; compaction history is a secondary row inside the same compact hover. No drawer, route, new scroll owner, breakpoint branch, or persistent mobile preference is introduced.
- **Shared behavior:** desktop and mobile use the same session-runtime entry, translated copy, count row, and help behavior. Existing parent-tooltip pinning continues to own outside-tap and Escape dismissal.
- **Mobile proof:** the mobile E2E scenario taps the ring and compaction help, asserts the disclosure remains visible inside the pinned hover, and confirms no document-level horizontal overflow.

## Tests

- **What:** first/equal/increasing/decreasing/duplicate samples update one durable count while preserving unrelated metadata.
  - **File:** `apps/backend/internal/task/repository/sqlite/session_test.go`
  - **How:** table-driven repository test against a real SQLite database.
- **What:** the same atomic compare-and-increment contract works with PostgreSQL JSONB.
  - **File:** `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
  - **How:** environment-gated real PostgreSQL repository test.
- **What:** restart-style service recreation uses persisted metadata, resets retain the count, and live session patches include it exactly once.
  - **File:** `apps/backend/internal/orchestrator/event_handlers_git_test.go`
  - **How:** real task repository plus fresh `Service` instances and the recording event bus.
- **What:** live and hydrated session metadata normalize the count into `ContextWindowEntry`, including legacy missing values.
  - **Files:** `apps/web/lib/state/slices/session-runtime/context-window.test.ts`, `apps/web/lib/ws/handlers/agent-session.test.ts`, `apps/web/hooks/domains/session/use-session-context-window.test.ts`
  - **How:** focused parser, store-handler, and hook tests.
- **What:** the hover shows zero/non-zero counts and exposes the translated accuracy explanation accessibly.
  - **File:** `apps/web/components/task/chat/token-usage-display.test.tsx`
  - **How:** render with mocked session state under the existing tooltip test boundary and focus the inline help control.

## E2E Tests

- **Scenario:** **GIVEN** a counted session has a reliable context sample, **WHEN** a desktop user hovers the context ring and then the compaction info control, **THEN** the count and approximate-count explanation remain visible in the same hover.
  - **File:** `apps/web/e2e/tests/chat/context-window-source.spec.ts`
  - **What to verify:** count text, pointer-hover disclosure, and parent-hover stability.
- **Scenario:** **GIVEN** the same counted session on a phone viewport, **WHEN** the user taps the ring and compaction info control, **THEN** the count and explanation are reachable without horizontal overflow.
  - **File:** `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`
  - **What to verify:** tap-pinned hover, disclosure visibility, and document containment.

## Verification Results

- Task 01: SQLite, backend-app replay, task-service, orchestrator, and full `go test ./...` checks passed; PostgreSQL behavior test is present but skipped without `KANDEV_TEST_POSTGRES_DSN`.
- Task 02: focused frontend suite passed (4 files, 68 tests); typecheck, i18n check, and i18n ratchet passed.
- Task 03: managed Chromium desktop and mobile-chrome E2E scenarios passed (1 test each).
- Review fixup: the context-update handler now persists synchronously to preserve arrival order; model-change clears use the guarded reset boundary; the compaction help has explicit touch toggle state; and parser/E2E selectors now require the adjacent count contract. Full `go test -race ./...` was not rerun locally; the PR's race-enabled backend job had already exposed an unrelated `internal/integration` failure before this fixup.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation because the frontend contract depends on the backend metadata shape and E2E depends on both implementation slices.

- [x] [Task 01: Persist inferred compaction counts](task-01-persist-compaction-count.md) — done
- [x] [Task 02: Show compactions in context hover](task-02-context-hover-ui.md) — done
- [x] [Task 03: Cover desktop and mobile hover](task-03-context-hover-e2e.md) — done

## Risks and boundaries

- ACP supplies usage samples, not compaction events. Strict decreases are intentionally approximate and the UI must not describe them as provider-confirmed.
- The database update must compare and increment atomically so duplicate event delivery or multiple backend subscribers cannot double count one observed drop.
- Reset and model-change paths may clear `context_window`, but must not clear `context_compaction_count`; the next sample after a cleared baseline must not increment.
- Public session documentation now mentions the inferred count and its approximate nature; no separate reference page is needed for this contextual hover detail.
