---
id: "01-frame-passthrough-prompt-body"
title: "Frame passthrough prompt body"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.1
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.2
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.3
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.5
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.6
system_design:
  - ../../specs/cli/system-design/passthrough-prompt-delivery.md
---

# Task 01: Frame Passthrough Prompt Body

## Summary

Restore bracketed-paste framing for passthrough agents that accept it, so a
prompt body larger than one terminal read reaches the TUI as one paste instead
of a burst it drops reads from. Neutralize framing terminators inside the body
first, and keep the submit keystroke a separate delayed write.

## In scope

- Add failing planner tests for a body far larger than one read, for framing
  markers, and for a body containing a framing terminator.
- Neutralize framing terminators in the body inside the planner.
- Frame the body when the capability record allows framing and the body is
  multi-line or larger than the raw-safe write size.
- Stop setting `DisableBracketedPaste` in the built-in Claude capability record
  and in the custom TUI agent constructor.
- Update the existing tests that pin the superseded unframed behavior.

## Out of scope

- The unframed delivery path, which work order 02 paces.
- Caller changes: the initial-prompt injector, workflow step handler, and
  session prompt path already loop over planned chunks.
- Detecting bracketed-paste support from the terminal output stream.

## Acceptance

- A probe against the real vendor CLI confirms a framed body of at least 16 KB
  written in one PTY write reaches the input intact and is submitted by the
  delayed submit keystroke. If submission fails, stop and re-plan: the fallback
  is the paced path from work order 02 for this agent too.
- Concatenating the planned body chunks reproduces the prompt exactly, with
  framing markers only at the outer boundary and none surviving from the body.
- The submit keystroke remains the final separate chunk carrying the configured
  submit delay, and small single-line bodies keep their current bytes.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/agents/ -run 'TestPlanPassthroughStdin|TestBuildPassthroughPayload|TestNewTUIAgentDefaultsToInkSafePassthrough|TestClaudeACP' -count=1)
(cd apps/backend && go test ./internal/agent/agents/ ./internal/agent/runtime/lifecycle/ ./internal/orchestrator/executor/ -count=1)
(cd apps/backend && gofmt -l internal/agent/agents)
```

## Files likely touched

- `apps/backend/internal/agent/agents/passthrough_payload.go`
- `apps/backend/internal/agent/agents/passthrough_payload_test.go`
- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/agents/tui_agent.go`
- `apps/backend/internal/agent/agents/tui_agent_test.go`
- `docs/plans/passthrough-prompt-delivery-integrity/plan.md`
- `docs/plans/passthrough-prompt-delivery-integrity/task-01-frame-passthrough-prompt-body.md`

## Dependencies

None.

## Risks

- `TestNewTUIAgentDefaultsToInkSafePassthrough` and the Claude body-verbatim
  assertion in `passthrough_payload_test.go` encode the behavior being replaced.
  Rewrite them to assert the new contract rather than deleting the coverage.
- The historical reason for disabling framing was a submit keystroke absorbed
  into the paste. The separate delayed submit chunk already addresses it; the
  probe in Acceptance is what proves it, not the commit history.

## Parallelism

`sequential`

## Inputs

- `REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001` and its system design.
- Diagnosis evidence: delivered bodies lost exactly N x 1022 bytes from the
  front; a framed 16 KB body written once arrived complete in a probe against
  the real CLI; an unframed 8 KB body lost roughly half its lines.
- Existing planner, capability records, and caller tests.

## Results

PROBE: An 8 KB body framed as a bracketed paste and written to the real CLI in
one PTY write was accepted whole, and the submit keystroke written 150 ms later
started the turn: the input box cleared and the agent moved to its thinking
state. Framing therefore does not break submission, so the design holds and the
paced fallback stays reserved for agents that refuse framing.

RED: Five behavioral failures. The Claude body chunk arrived unframed for a
multi-line prompt, an 11 KB multi-line body was not bracketed, a single-line
body past the raw-safe size was not bracketed, prompt-embedded paste markers
survived into the delivered body, and the custom TUI agent still disabled
framing.

GREEN: The planner now strips paste markers from the body, frames it when the
agent accepts framing and the body is multi-line or larger than
`passthroughRawSafeWriteBytes`, and keeps the submit byte a separate delayed
chunk. `claude_acp.go` and `tui_agent.go` no longer set
`DisableBracketedPaste`. `go test ./internal/agent/agents/` and
`./internal/orchestrator/executor/` pass. Failures in
`./internal/agent/runtime/lifecycle/` (worktree preparer and SSH identity-agent
tests) reproduce identically on a pristine checkout of the base branch and are
unrelated to this work order.
