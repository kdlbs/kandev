---
id: "08-mock-agent-and-e2e"
title: "Replay steering in the mock agent and cover it end to end"
status: done
wave: 5
depends_on: ["04-adapter-steer-admission", "05-orchestrator-steer-admission", "07-composer-steer-affordance"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
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

Re-run on 2026-08-06 against the branch merged with `main`, after adding the
`steer-fold-setup` / `steer-defer-setup` mock scenarios this task's status was
blocked on.

- `cd apps/backend && go test -race ./cmd/mock-agent/...`: passed, including
  the two new scenario-pinning tests
  `TestSteerFoldSetupEmitsNoAnswerUntilCancelled` and
  `TestSteerDeferSetupAnswersBeforeHolding`, which assert directly on the
  mock's own replay contract (no answer text vs. one real answer, in both
  cases honoring context cancellation) without any live backend or paid model
  call.
- `cd apps/backend && golangci-lint run ./cmd/mock-agent/... --new-from-rev=e381ed9884873ac924fe6cc75543271ad8be06e6 --timeout=5m`: 0 issues.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && npx playwright test --config e2e/playwright.config.ts tests/chat/mid-turn-steering.spec.ts --project=chromium --workers=1`:
  **run locally**, all 6 tests passed — the four scenarios from the spec
  (delivered while generating, queued behind an existing message, queued when
  the capability isn't advertised, queued when the toggle is off) plus the two
  new folded/deferred outcome tests added by this pass. `pnpm e2e` (the full
  suite) remains CI-owned; this file's local run is scoped to the one spec.
- The mock agent advertises `_meta.claudeCode.promptQueueing` by default and can
  suppress it with `KANDEV_MOCK_AGENT_PROMPT_QUEUEING=false`, so the capability
  gate is exercised through the same negotiation path as a real bridge.

**Replay modes (first acceptance item), now implemented:**
`scenarioSteerFoldSetup` (`/e2e:steer-fold-setup`) holds the foreground turn
open and never emits text of its own, so the predecessor produces no answer at
all and the transcript carries only the steer's own successor turn — the
`folded` outcome. `scenarioSteerDeferSetup` (`/e2e:steer-defer-setup`) emits a
real answer immediately (flushed via a trivial tool-call boundary, since text
otherwise stays buffered until a tool call or turn end — see
`cmd/mock-agent/AGENTS.md`), then holds silently; a steer sent during the hold
runs as a genuinely separate turn with its own answer — the `deferred`
outcome. Both are driven purely by mock-side scripting (which predecessor
scenario the E2E test seeds), not by racing real wall-clock timing against the
adapter's `beginSteerHandoff`/`claimPromptTurnCompletion` transfer, which is
what makes the replay deterministic "on demand" rather than flaky.
The deferred E2E test additionally asserts `last-agent-error-notice` and
`toast-message` are not visible, covering the "no error and no version
warning" acceptance item (third).
