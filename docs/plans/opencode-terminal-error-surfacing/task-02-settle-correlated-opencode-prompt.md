---
id: "02-settle-correlated-opencode-prompt"
title: "Settle correlated OpenCode prompts"
status: done
wave: 2
depends_on: ["01-capture-opencode-error-diagnostics"]
plan: "plan.md"
spec: "../../specs/agent-stall-recovery/spec.md"
---

# Task 02: Settle Prompts From Correlated Diagnostics

## Acceptance

- A validated OpenCode diagnostic settles only the pending foreground prompt
  whose active ACP session ID matches the diagnostic.
- Diagnostic, ACP response, and user-cancel races emit exactly one terminal
  result for the owning prompt generation and release the losing operation.
- The resulting agent error event carries the safe provider-error record;
  wrong-session, background, stale, post-settlement, and non-OpenCode lines do
  not affect the prompt.

## Verification

- `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(OpenCodeDiagnostic|Prompt.*Diagnostic|HandleWSPrompt.*ProviderError|SendErrorEvent)'`

Use a fake ACP agent that intentionally holds `session/prompt`; do not call a
real provider or depend on wall-clock stall detection.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_cancel.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_test.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_test.go`
- `apps/backend/internal/agentctl/types/streams/provider_error.go`

## Dependencies

Task 01.

## Parallelism

Sequential. Task 03 consumes the structured terminal event created here.

## Inputs

- Spec scenarios for active, wrong-session, background, stale, and racing
  diagnostics
- Plan section `OpenCode diagnostic normalization and prompt correlation`
- Existing prompt-turn gate, generation claim, cancel acknowledgement, async
  `handleWSPrompt`, and `SendErrorEvent` paths

## Risks

- A losing `conn.Prompt` goroutine must receive cancellation and cannot retain a
  prompt gate, trace span, or later error delivery.
- Stderr delivery cannot block on the prompt consumer channel.

## Output contract

Report RED race assertions, correlation rules, single-terminal proof, exact
tests, files changed, blockers, and risks. Mark this task `done` and update its
plan checkbox in the same conversation.

## Results

The ACP adapter correlates diagnostics to the active ACP session and prompt
turn, cancels the held RPC through its context cause, and forwards one typed
provider error through the existing async prompt event. Wrong-session and
post-settlement diagnostics are ignored; duplicate lifecycle terminal events
are guarded by prompt generation. The exact verification command passed with
3 tests across three packages.
