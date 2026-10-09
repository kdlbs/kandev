---
created: 2026-09-22
status: complete
requirements:
  - REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001
system_design:
  - ../../specs/cli/system-design/passthrough-prompt-delivery.md
legacy_specs: []
---

# Implementation Plan: Passthrough Prompt Delivery Integrity

## Overview

A prompt delivered to a CLI passthrough session loses whole leading read-sized
blocks of its body. Measured against a live instance, every agent-to-agent
message longer than one read arrived at the agent missing exactly N x 1022
bytes from the front, with no error on any surface. The planner writes the whole
body as one unframed PTY write, so the receiving TUI has to absorb a burst many
times larger than one terminal read and silently drops the reads it cannot take.

Two work orders correct this in the one place that plans PTY writes. The first
restores bracketed-paste framing for TUIs that accept it, which a probe against
the real CLI showed carries 16 KB in a single write without loss. The second
paces the remaining unframed path so a body can never exceed one read per write.

## Scope

### In scope

- Frame passthrough prompt bodies with bracketed paste when the agent accepts
  it, keeping the submit keystroke a separate delayed write.
- Neutralize framing terminators inside the body.
- Split unframed bodies into rune-safe writes within the host read budget, with
  an inter-write delay.
- Update the agent capability records that currently disable framing, and the
  tests that pin the superseded behavior.

### Out of scope

- The structured (non-passthrough) prompt path.
- Operator keystrokes typed into the mirrored terminal view.
- Message wrapping, queueing, delivery mode, and mid-turn steering.
- Reading the terminal back to acknowledge delivery.
- The local hotfix build and install on the operator's instance, which happens
  outside the repository and is tracked in the session that requested it.

## Technical approach

`PlanPassthroughStdinChunks` stays the single planner and stays pure. It gains
three rules: neutralize framing terminators in the body; frame the body when the
capability record allows it and the body is multi-line or larger than the
raw-safe write size; otherwise emit the body as consecutive rune-safe writes no
larger than the raw-safe size, each continuation carrying an inter-write delay.
The submit keystroke keeps its own trailing chunk and its existing delay.

Callers already loop over planned chunks and honor `DelayBefore`, so no caller
changes. The raw-safe write size is set below the smallest supported host read
capacity (1022 bytes on macOS); 400 bytes with a 30 ms inter-write delay is the
combination verified against the real CLI.

`claude_acp.go` and `tui_agent.go` stop setting `DisableBracketedPaste`. The
field stays in the capability record as the lever for a TUI that genuinely
cannot accept framing, and work order 02 makes that path safe.

## Tests

- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.1`: planner tests in
  `apps/backend/internal/agent/agents/passthrough_payload_test.go` assert the
  concatenated chunk bodies reproduce the prompt exactly, for a body far larger
  than one read, on both the framed and the unframed path.
- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.2`: a framed body is one chunk
  carrying start and end markers; an unframed body is consecutive chunks in
  order.
- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.3`: the submit keystroke remains the
  final, separate chunk with `DelayBefore` equal to the configured submit delay.
- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.4`: with `DisableBracketedPaste` set,
  no chunk exceeds the raw-safe size and each continuation carries the
  inter-write delay.
- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.5`: a body containing a framing
  terminator is neutralized before framing, and the marker does not survive into
  a delivered chunk.
- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.6`: covered by the planner being the
  single source for all callers; existing caller tests in
  `manager_passthrough_autoinject_test.go` and
  `executor_passthrough_prompt_test.go` keep that wiring honest.
- `AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.7`: existing passthrough prompt error
  handling in `executor_passthrough_prompt_test.go` already asserts a write
  failure surfaces to the caller; work order 02 keeps it passing with several
  writes in flight.

## E2E tests

No Playwright scenario is added. The defect lives between the Go planner and a
vendor TUI's stdin handling, which Playwright does not observe. The behavioral
check that matters is the manual TUI probe recorded in work order 01, plus the
post-deploy comparison of stored message text against the agent's own
transcript.

## Work orders

- [x] [Task 01: Frame passthrough prompt body](task-01-frame-passthrough-prompt-body.md) (done)
- [x] [Task 02: Pace unframed passthrough writes](task-02-pace-unframed-passthrough-writes.md) (done)

## Verification results

Both work orders completed. A probe against the real vendor CLI confirmed a
framed 8 KB body written in one PTY write is accepted whole and submitted by the
delayed submit keystroke. `go test ./internal/agent/agents/` and
`./internal/orchestrator/executor/` pass; `go vet` and `gofmt` are clean and the
backend builds. Failures in `./internal/agent/runtime/lifecycle/` (worktree
preparer, Kubernetes prepare-script, and SSH identity-agent tests) are
environment-dependent: the same twelve tests fail on a pristine checkout of the
base branch, with and without this change. `golangci-lint` was not run: it is
not installed on the development machine.

End-to-end on a live instance running the patched `v0.95.0` build, against a
Claude CLI passthrough session: the initial prompt injector wrote 10174 bytes
and the agent's own transcript recorded 10174 bytes, starting at the first byte,
with all 60 line markers present. A `message_task_kandev` message of 9213 bytes,
the path that exposed the defect, arrived byte-for-byte identical to the stored
message, and the agent reported every marker from first to last. Before the
correction, every stored agent message above 5 KB had lost between 1022 and
5110 bytes from its start.

## Risks

- A TUI that does not enable bracketed-paste mode would render the markers as
  literal text. Work order 01 confirms the framed body is accepted and submitted
  by the real CLI before the capability records change.
- Custom TUI agents inherit the same change as the built-in Claude record. They
  share the same TUI family but were not individually probed.
- The unframed path becomes slower in proportion to body size, which is
  intentional: correctness over latency for a path with no acknowledgement.
