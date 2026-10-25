---
id: "04-adapter-steer-admission"
title: "Admit a steer while the foreground is generating"
status: done
wave: 2
depends_on: ["01-negotiate-steer-capability", "03-fold-handoff-fixtures"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
---

# Task 04: Admit a steer while the foreground is generating

- **Acceptance:** A human prompt carrying a steer intent transfers the existing
  prompt-gate token to itself while the predecessor `session/prompt` is still
  open, for a steer-capable agent, without waiting for an
  `EventTypeForegroundIdle` event and without cancelling the predecessor. A
  synthetic (`ScheduleWakeup`, generation 0) prompt can neither trigger nor
  consume that transfer.
- **Acceptance:** When the predecessor then settles early, it emits no
  completion, does not sweep tool ownership that was live at the boundary, does
  not consume the successor's usage, and does not clear the successor's trace.
  Usage is recorded once, against the turn the agent reported it on.
- **Acceptance:** Cancelling during an in-flight steer ends both the predecessor
  and the steer and releases the gate; a session replaced by
  `session/new`/`session/load` drops a waiting steer instead of delivering it.
- **Verification:** `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp/...`
- **Files likely touched:**
  `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`,
  `adapter_prompt_cancel.go` (extend `newPromptTurnState` /
  `acquirePromptTurn` / `tryTransferPromptTurn`; reuse
  `claimPromptTurnCompletion`, `finishPromptTurn`,
  `invalidatePromptTurnOwnership`, `protectActiveBackgroundWorkForHandoff`
  unchanged), `adapter.go`, `prompt_handoff_test.go` and new focused tests.
- **Dependencies:** Task 01 for the negotiated gate; task 03 for the replay
  fixture this task's tests assert against.
- **Inputs:** Spec "State machine" (`sent` → `folded` | `deferred`) and "Failure
  modes". Plan facts 2 and 3: the bridge settles the predecessor 3 ms after the
  tool completes and 1.9 s before the answer exists, and background work
  survives the boundary — so tool-ownership protection is required, not
  optional. ADR-0049 already specifies the generation-keyed suppression rule to
  reuse verbatim.
- **Risks:** This is the sharpest edge in the feature. A predecessor settlement
  mistaken for a real completion corrupts transcript, usage, and tool state.
  Keep the existing foreground-idle handoff tests passing without modification;
  if they need edits, the abstraction is wrong. Two overlapping RPCs also make
  cancel ordering subtle — assert the gate is released in every terminal path.
- **Output contract:** Report the steer transfer trigger, how predecessor
  finalization is suppressed, the cancel and session-replacement behavior,
  `-race` results, and update only this task's status.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp/...`: passed.
- Steer transfer trigger: `beginSteerHandoff` (`adapter_prompt_cancel.go`) hands
  the in-flight turn off at the operator's send; the successor then inherits the
  gate token via `tryTransferPromptTurn`. Synthetic (generation 0) prompts set
  `allowHandoff=false` and can neither trigger nor consume the transfer.
- Predecessor finalization is suppressed by `claimPromptTurnCompletion`, which
  refuses a turn that no longer owns the gate, and by
  `protectActiveBackgroundWorkForHandoff`, which keeps the predecessor's live
  background work out of the successor's prompt-end sweep.
- Cancel and session-replacement behavior is covered by the adapter tests;
  `finishPromptTurn` releases the gate on every exit path, handed off or not.
