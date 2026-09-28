---
id: "02-pace-unframed-passthrough-writes"
title: "Pace unframed passthrough writes"
status: done
wave: 2
depends_on:
  - "01-frame-passthrough-prompt-body"
plan: "plan.md"
requirements:
  - REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.1
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.4
  - AC-CLI-PASSTHROUGH-PROMPT-DELIVERY-001.7
system_design:
  - ../../specs/cli/system-design/passthrough-prompt-delivery.md
---

# Task 02: Pace Unframed Passthrough Writes

## Summary

Make the remaining unframed path safe for an agent whose capability record
disables framing. Split the body into rune-safe writes no larger than the
raw-safe size and delay each continuation, so the receiving TUI never has to
absorb more than one terminal read at a time.

## In scope

- Add failing planner tests for an unframed body larger than the raw-safe size,
  including a multi-byte body whose split would otherwise fall mid-character.
- Split the unframed body into ordered chunks within the raw-safe size, on rune
  boundaries, each continuation carrying the inter-write delay.
- Name the raw-safe size and inter-write delay as constants with the host read
  capacity they derive from.
- Keep the submit keystroke the final chunk with its own delay, after the last
  body chunk.
- Confirm a write failure mid-body still surfaces to the caller.

## Out of scope

- The framed path from work order 01.
- Changing the capability records again; this path exists for agents that
  genuinely cannot accept framing.
- Adaptive sizing or feedback from the terminal output stream.

## Acceptance

- With framing disabled, no planned body chunk exceeds the raw-safe size, the
  concatenated chunks reproduce the prompt exactly, and no chunk ends inside a
  multi-byte character.
- Every continuation body chunk carries the inter-write delay; the first body
  chunk carries none.
- A write error on any chunk aborts the remaining writes and returns to the
  caller as a failed delivery.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/agents/ -run 'TestPlanPassthroughStdin' -count=1)
(cd apps/backend && go test ./internal/agent/agents/ ./internal/orchestrator/executor/ -count=1)
(cd apps/backend && gofmt -l internal/agent/agents)
```

## Files likely touched

- `apps/backend/internal/agent/agents/passthrough_payload.go`
- `apps/backend/internal/agent/agents/passthrough_payload_test.go`
- `docs/plans/passthrough-prompt-delivery-integrity/plan.md`
- `docs/plans/passthrough-prompt-delivery-integrity/task-02-pace-unframed-passthrough-writes.md`

## Dependencies

- Work order 01 establishes the framed path and the shared raw-safe size
  constant this work order reuses.

## Risks

- Delivery time grows with body size on this path. Keep the constants in one
  place with a comment naming the host read capacity, so a future change is a
  deliberate trade rather than a guess.
- Splitting by bytes instead of runes would corrupt non-ASCII prompts, which is
  exactly the traffic that exposed the original defect.

## Parallelism

`sequential`

## Inputs

- `REQ-CLI-PASSTHROUGH-PROMPT-DELIVERY-001` and its system design.
- Probe evidence: unframed writes of 400 bytes spaced 30 ms apart delivered a
  3 KB body with no loss, while one 8 KB unframed write lost about half of it.
- The planner and tests as left by work order 01.

## Results

RED: A 3.8 KB unframed body still planned as one 3800-byte write, and the
no-submit-delay path planned one 1201-byte write, both far past the raw-safe
size.

GREEN: `paceRawBody` splits an unframed body into writes of at most
`passthroughRawSafeWriteBytes`, cutting on rune boundaries, with
`passthroughRawWriteInterval` before each continuation. The submit sequence
stays its own delayed chunk when the agent sets a submit delay and otherwise
rides on the final body write. The rune-boundary test caught a real off-by-one:
a body whose length is an exact multiple of the write size indexed one byte past
the end. `go test ./internal/agent/agents/` and
`./internal/orchestrator/executor/` pass, `go vet ./internal/agent/agents/...`
is clean, `gofmt -l internal/agent/agents` is empty, and `go build ./...`
succeeds.

Not run: `golangci-lint`, which is not installed on this machine and was not
added. The changed functions stay well inside the documented limits (longest is
28 lines, nesting depth 4).
