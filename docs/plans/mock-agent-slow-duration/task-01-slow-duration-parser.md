---
id: "01-slow-duration-parser"
title: "Support unitless slow durations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/mock-agent-slow-duration.md"
---

# Task 01: Support unitless slow durations

Implement the `/slow` duration parsing repair after adding the regression
coverage.

## Acceptance

- `/slow 60` resolves to 60 seconds, while `/slow 60s`, `/slow 500ms`, and
  `/slow 2m` retain their explicit durations.
- `/slow` and invalid, zero, or negative arguments retain the five-second
  default; existing `/background` duration behavior remains unchanged.
- The mock-agent response path uses the resolved duration and reports it using
  the existing output format.

## Verification

Run the focused parser/handler tests first, then the complete mock-agent
package:

```bash
cd apps/backend
go test -run 'Test(ParseCommandDuration|Slow)' ./cmd/mock-agent
go test ./cmd/mock-agent
```

## Files likely touched

- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/background_test.go`
- `apps/backend/cmd/mock-agent/slow_test.go`

## Dependencies

None.

## Parallelism

Sequential. The parser and its tests share the same behavior and should land as
one focused change.

## Inputs

- `docs/specs/agents/requirements/mock-agent-slow-duration.md`
- `docs/plans/mock-agent-slow-duration/plan.md`
- Existing command-duration parser coverage in
  `apps/backend/cmd/mock-agent/background_test.go`
- `apps/backend/cmd/mock-agent/AGENTS.md`

## Output contract

Report the final files changed, the red and green targeted test results, the
complete mock-agent package result, any risks, and synchronized task/plan
statuses. Do not change the ACP command advertisement, frontend code, or
unrelated mock-agent commands.

## Results

- RED: `cd apps/backend && go test -run '^TestSlowResponseBareNumberUsesSeconds$' -count=1 ./cmd/mock-agent` — failed as expected because `/slow 1` reported the five-second path (`0 passed, 1 failed`).
- GREEN: `cd apps/backend && go test -run '^TestSlowResponseBareNumberUsesSeconds$' -count=1 ./cmd/mock-agent` — passed (`1 passed`).
- Focused: `cd apps/backend && go test -run 'Test(ParseCommandDuration|Slow)' ./cmd/mock-agent` — passed (`12 passed`).
- Package: `cd apps/backend && go test ./cmd/mock-agent` — passed (`193 passed`).
- Formatting: `gofmt -w cmd/mock-agent/handler.go cmd/mock-agent/background_test.go cmd/mock-agent/slow_test.go` — passed.
- Diff hygiene: `git diff --check` — passed.
- `make -C apps/backend build-mock-agent` — passed; the host mock-agent binary
  was rebuilt. The containers E2E project was not run, so the Linux binary was
  not required.
- Review follow-up: the handler-path test uses `/slow 50ms` to keep the suite
  fast while the parser table covers `/slow 60`.
