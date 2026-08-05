---
id: "08-mock-agent-and-e2e"
title: "Replay steering in the mock agent and cover it end to end"
status: in_progress
wave: 5
depends_on: ["04-adapter-steer-admission", "05-orchestrator-steer-admission", "07-composer-steer-affordance"]
plan: "plan.md"
spec: "../../specs/platform/mid-turn-steering.md"
---

# Task 08: Replay steering in the mock agent and cover it end to end

- **Acceptance:** The mock agent can replay both outcomes on demand — `folded`
  (predecessor settles `end_turn` with zeroed usage and no answer text, successor
  carries the answer) and `deferred` (predecessor answers, then the steer runs as
  its own turn) — with no paid model call.
- **Acceptance:** E2E covers four scenarios from the spec: steer delivered while
  generating; queue when the toggle is off; queue when the agent does not
  advertise the capability; order preserved when a message is already queued.
- **Acceptance:** The `deferred` path asserts the operator sees no error and no
  version warning.
- **Verification:** `cd apps/backend && go test -race ./cmd/mock-agent/...` then `cd apps/web && pnpm e2e`
- **Files likely touched:** `apps/backend/cmd/mock-agent/` (script/emitter, which
  already models `claude-agent-acp` wire shapes including Monitor and subagent
  metadata), a new spec under `apps/web/e2e/tests/chat/`, and the e2e backend
  fixture if the toggle needs setting per spec.
- **Dependencies:** Tasks 04, 05, 07.
- **Inputs:** Spec "Scenarios". Task 03's fixtures define the wire shape to
  replay. `apps/web/e2e/tests/chat/busy-signal.spec.ts` already sets
  `KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF` per spec and is the pattern
  for toggling a runtime flag in E2E. `apps/web/e2e/README.md` documents project
  selection.
- **Risks:** The mock is a dev/E2E-only trusted path and must not widen
  production agent support. Keep the negotiated capability real in the mock
  (advertise the `_meta`) rather than bypassing the gate, or the E2E proves
  nothing about task 01.
- **Output contract:** Report the mock's replay modes, the four E2E scenarios and
  their assertions, exact commands/results, and update only this task's status.

## Validation Results

Re-run on 2026-08-05 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/orchestrator/messagequeue`: passed.
- `cd apps/backend && go test -race -run 'TestSteerTask_DoesNotOvertakeQueueWriter|TestSessionKeyedEntryPointsGuardBeforeDependencies' ./internal/orchestrator`: passed.
- `cd apps/backend && go test -race -run 'TestInitializePromptQueueingCanBeDisabled' ./cmd/mock-agent`: passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm e2e`: **not run locally** — the full Playwright suite is
  CI-owned for this branch. The mid-turn spec now covers delivery, disabled
  toggle, missing agent capability, and order behind an existing queue entry.
- The mock agent advertises `_meta.claudeCode.promptQueueing` by default and can
  suppress it with `KANDEV_MOCK_AGENT_PROMPT_QUEUEING=false`, so the capability
  gate is exercised through the same negotiation path as a real bridge.

The folded/deferred replay modes described by the first acceptance item are
still not implemented. Keep this task in progress until the mock can drive both
outcomes and the corresponding transcript assertions exist.
