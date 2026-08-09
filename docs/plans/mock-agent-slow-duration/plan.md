---
spec: docs/specs/mock-agent-slow-duration/spec.md
created: 2026-08-08
status: done
---

# Implementation Plan: Mock-agent slow command duration syntax

## Overview

Extend the mock agent's existing duration parsing so `/slow 60` resolves to
60 seconds while preserving explicit-unit durations and the five-second
fallback. Reuse the already-tested explicit-unit-first parsing behavior used by
the background commands, then add focused parser and handler coverage without
changing the ACP command advertisement or any frontend code.

## Backend

### Shared duration parsing

- `apps/backend/cmd/mock-agent/handler.go`
  - Generalize the existing background-duration parser, or extract its argument
    parsing into a clearly command-independent helper.
  - Try an explicit Go duration first (`60s`, `500ms`, `2m`) so unit-bearing
    values are not altered.
  - Treat a positive bare numeric value as seconds (`60` → `60s`).
  - Keep the caller-provided default for missing, zero, negative, or malformed
    values.
  - Route `emitSlowResponse` through this parser with a five-second default and
    preserve the existing background-command behavior.
  - Preserve the existing `/background 12` behavior already covered by the
    parser table; the new behavior is limited to the `/slow` caller.

## Tests

- **What:** Duration argument resolution for `/slow` and existing background
  commands, including bare seconds, explicit units, defaults, and invalid
  values.
  **File:** `apps/backend/cmd/mock-agent/background_test.go` (rename or
  generalize the existing parser test as needed).
  **How:** Table-driven unit test; the `/slow 60` case must fail before the
  parser change because it currently resolves to the supplied eight-second
  default rather than 60 seconds.
- **What:** The `/slow` handler uses the resolved duration in its simulated
  response path.
  **File:** `apps/backend/cmd/mock-agent/slow_test.go` (new) or the nearest
  existing mock-agent handler test file.
  **How:** Invoke the handler with a short explicit duration to avoid a
  60-second test, and assert that the response reports that resolved duration;
  the parser table separately covers the 60-second bare-number contract.

No frontend or E2E changes are planned: the ACP command transport and existing
slash-command UI flow are unchanged, and this repair is confined to the mock
agent's backend fixture behavior.

## Verification Results

- `cd apps/backend && go test -run '^TestSlowResponseBareNumberUsesSeconds$' -count=1 ./cmd/mock-agent` failed before the production change with the expected five-second marker, then passed after the change.
- `cd apps/backend && go test -run 'Test(ParseCommandDuration|Slow)' ./cmd/mock-agent` — passed (`12 passed`).
- `cd apps/backend && go test ./cmd/mock-agent` — passed (`193 passed`).
- `make -C apps/backend build-mock-agent` — passed; the host mock-agent binary
  was rebuilt. The containers E2E project was not run, so the Linux binary was
  not required.
- `gofmt -w cmd/mock-agent/handler.go cmd/mock-agent/background_test.go cmd/mock-agent/slow_test.go` — passed.
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-slow-duration-parser](task-01-slow-duration-parser.md)

## Open Questions

None.
