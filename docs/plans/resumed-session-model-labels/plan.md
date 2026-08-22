---
spec: docs/specs/ui/acp-model-configuration-summary.md
created: 2026-08-21
status: implemented
---

# Implementation Plan: Resumed Session Model Labels

## Overview

The backend currently persists and publishes every ACP `session_models` event during resume. Codex first reports its provider default, then Kandev applies the saved profile model.

The frontend can also trust that first event before startup configuration settles. The tab resolver then prefers its model option over the session current-model field.

Gate mutable startup events in the backend first. Then keep the frontend hydration barrier active through settlement and make all model labels use one current-model value.

## Confirmed Root Cause

The Sol session resumed with profile `077cca54-38eb-4dc3-b859-846fe8e90280`. ACP first reported `gpt-5.6-luna`, then Kandev applied `gpt-5.6-sol` before the execution became ready.

`handleSessionModelsEvent` persisted and published the intermediate Luna event. The browser accepted the event before `config_options_settled` and let the tab-title model option override `currentModelId`.

The process ran Sol before it accepted input. The fault was a transient durable and visual model regression, not an execution-profile mix-up.

## Backend

### Startup event gate

- Update `apps/backend/internal/orchestrator/event_handlers_streaming.go` in `handleSessionModelsEvent`.
- Continue to persist an `original_config_settled` candidate before the gate.
- Read the task-session state after the original-configuration path.
- During `STARTING`, do not persist or publish an event unless `config_options_settled` is true.
- Continue to accept unsettled live updates after startup, so user model changes still converge.
- Add a small predicate such as `shouldDeferUnsettledStartupModelsEvent` to keep the state rule testable.

No WebSocket payload or database schema changes are required.

## Frontend

### Runtime hydration barrier

- Update `apps/web/lib/ws/handlers/session-models.ts`.
- Keep persisted runtime values active while a session is `STARTING` and the payload is unsettled.
- Mark the session hydrated only after a matching settled startup payload arrives.
- Normalize the model config option to the resolved `currentModelId` before the store write.
- Continue to trust later live provider updates after startup settlement.

### Session tab title

- Update `apps/web/components/task/session-tab-title.ts`.
- Prefer `currentModelId` when it is available.
- Use the model config option only when the current-model field is absent.
- Preserve custom-name, active-model, agent-label, and snapshot fallbacks.

### Mobile parity

The dock tab exists only in `DockviewDesktopLayout`. Phone and tablet task layouts use the shared session runtime store and their existing model selector.

This repair changes state normalization, not composition, navigation, touch behavior, scroll ownership, or safe-area behavior. The nearest mobile exemplar remains `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`.

No new mobile Playwright case is required. The shared handler unit test and the existing mobile selector test cover the affected mobile state path.

## Tests

- **What:** An unsettled startup default cannot replace a saved model, runtime configuration, or selector snapshot.
  **File:** `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`.
  **How:** Set a seeded session to `STARTING`, persist Sol, send an unsettled Luna event, and assert no mutable write or WebSocket publication. Then send settled Sol and assert one publication.
- **What:** An unsettled `Sol -> Luna -> Sol` startup sequence keeps Sol until settlement.
  **File:** `apps/web/lib/ws/handlers/session-models-startup.test.ts`.
  **How:** Use persisted Sol metadata and a `STARTING` session. Assert that both `currentModelId` and the model option remain Sol until the settled Sol payload.
- **What:** A contradictory model option cannot override the explicit current model in a tab title.
  **File:** `apps/web/components/task/session-tab-title.test.ts`.
  **How:** Give the resolver Sol as `currentModelId` and Luna as the model option. Assert the title is Sol. Keep the option-only fallback case.
- **What:** Live model changes still work after startup.
  **Files:** `apps/backend/internal/orchestrator/event_handlers_streaming_test.go` and `apps/web/lib/ws/handlers/session-models.test.ts`.
  **How:** Send an unsettled update in a non-`STARTING` state or after hydration. Assert that the new model is accepted.

## E2E Tests

- **Scenario:** GIVEN a task has two sessions with different models, WHEN the backend resumes both, THEN each desktop tab keeps its session model.
  **File:** `apps/web/e2e/tests/session/session-resume.spec.ts`.
  **What to verify:** Create two mock profiles with distinct models, launch one session per profile, and capture both session-tab texts across reload. No observation can show the same default model on both tabs.
- **Mobile coverage:** Run the existing `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts` focused model-selection case. It proves that the shared state path still supports the phone selector.

## Verification Results

- Backend focused startup regression: `cd apps/backend && go test -run 'TestHandleSessionModelsEvent.*Startup' ./internal/orchestrator` passed.
- Full backend suite: `make test` from `apps/backend` passed with `CGO_ENABLED=1 go test -tags fts5 ./...`.
- Frontend focused unit suite: `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/session-models.test.ts lib/ws/handlers/session-models-startup.test.ts components/task/session-tab-title.test.ts` passed with 3 files and 30 tests.
- Frontend typecheck: `cd apps/web && pnpm run typecheck` passed.
- Desktop resume E2E: `cd apps/web && pnpm e2e:run tests/session/session-resume.spec.ts -- --grep "keeps distinct model titles during multi-session resume"` passed with 1 test.
- Mobile selector E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts -- --grep "model"` passed with 1 test.
- PR evidence capture passed for both desktop session tabs and the existing mobile selector; the manifest maps both fresh PNGs and both images were inspected and compressed.
- PR fixup remediation: a settled startup payload now releases hydration even when it differs from persisted runtime data; the added regression passed, backend focused tests passed, web lint passed with zero warnings, typecheck passed, and the production desktop resume E2E passed.
- PR review follow-up: persisted runtime configuration remains authoritative for an unsettled `STARTING` payload even when the store already marked the session hydrated after an earlier settled payload. The same-store restart regression passed, the affected frontend suite passed with 31 tests, changed-file ESLint passed with zero warnings, typecheck passed, the production desktop resume E2E passed with 1 test, and the existing mobile model-selector E2E passed with 1 test. The restart history assertion now checks each session's expected model label rather than only comparing the two labels.
- Rebase/fixup verification: rebased cleanly onto `origin/main` at `032ea05b`, which includes the fix for the former `service_pr_watch.go` compile failure. `go build ./...`, `go test -race ./...` (10,069 tests in 136 packages), the focused GitHub and orchestrator race suites, the affected frontend suite (31 tests), changed-file ESLint, typecheck, the production desktop resume E2E, and the existing mobile model-selector E2E all passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Gate unsettled startup model events](task-01-gate-startup-model-events.md)

Wave 2:

- [x] [Task 02: Converge resumed session model labels](task-02-converge-session-model-labels.md)

Both tasks are sequential. Task 02 uses the backend gate from Task 01 in its restart E2E test.

## Open Questions

None.
