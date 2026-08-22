---
id: "01-gate-startup-model-events"
title: "Gate unsettled startup model events"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 01: Gate Unsettled Startup Model Events

## Acceptance

- An unsettled `session_models` event during `STARTING` cannot replace durable runtime configuration or the boot selector snapshot.
- The backend does not publish that intermediate event to the session WebSocket subject.
- The settled startup event and later live model changes keep their current behavior.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator -run 'TestHandleSessionModelsEvent.*Startup'
make -C apps/backend test
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `docs/plans/resumed-session-model-labels/plan.md`
- `docs/plans/resumed-session-model-labels/task-01-gate-startup-model-events.md`

## Dependencies

None.

## Parallelism

`sequential`. The frontend E2E depends on this startup event contract.

## Inputs

- Spec requirements for persisted runtime state and restart convergence.
- `handleSessionModelsEvent` and `configOptionsSettled` in `apps/backend/internal/orchestrator/event_handlers_streaming.go`.
- `InitializeAndPromptWithLayers` startup order in `apps/backend/internal/agent/runtime/lifecycle/session.go`.
- The observed `Luna -> Sol` resume sequence from session `83adf08e-d628-4b03-a711-7e43fd633358`.

## TDD Sequence

1. Add a test for an unsettled Luna event on a `STARTING` session that has persisted Sol state.
2. Run the focused test and record the expected failure.
3. Add the smallest startup-state gate after original-configuration capture.
4. Add or retain a test that accepts an unsettled live update outside `STARTING`.
5. Run the focused test and the backend test target.
6. Update this task, the plan checkbox, and verification results.

## Risks

- The gate must not skip `original_config_settled` persistence.
- The gate must not suppress user model changes after startup.
- A settled empty configuration is valid and must pass the gate.

## Output Contract

Report the RED assertion, code change, files changed, exact test results, blockers, risks, and synchronized task and plan status.

## Results

- RED: `cd apps/backend && go test -run 'TestHandleSessionModelsEvent.*Startup' ./internal/orchestrator` failed because the unsettled startup event changed the persisted model from `gpt-5.6-sol` to `gpt-5.6-luna`.
- GREEN: The same focused test passed after adding the `STARTING` plus unsettled gate.
- Full backend check: `make test` from `apps/backend` passed (`CGO_ENABLED=1 go test -tags fts5 ./...`).
- The test also confirms the original effective configuration candidate is captured before the gate and an unsettled live update is accepted after the session leaves `STARTING`.
- Rebase/fixup verification: rebased cleanly onto `origin/main` at `032ea05b`, then `go build ./...` and `go test -race ./...` passed (`10,069` tests in `136` packages). The focused GitHub and orchestrator race suites also passed; the former CI compile failure is fixed by the current `main` base.
