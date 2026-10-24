---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
legacy_specs: []
---

# Fix plan: replay successor setup events before the generation commit

## Overview

An authorized context continuation starts a new native conversation on the
next harness generation, but the orchestrator commits that generation only
after launch. Setup events retained on the successor stream in that window make
the stream preflight fail with `agent delivery checkpoint missing`, because the
only zero-cursor exception is the generation-one fresh bootstrap. Even when the
stream is accepted, the event processor does not wait for a continuation commit,
so the first setup event fails the owner check. One work order fixes both
halves for every predecessor-replacing launch.

## Scope

In scope: `agent_delivery_retention.go`, `delivery_stream_preflight.go`, and
`durable_silent_restore_generation.go` in
`apps/backend/internal/agent/runtime/lifecycle/`, plus a permanent regression
test beside them.

Excluded: orchestrator generation commit ordering, the generation-one bootstrap
path, and any UI change.

## Work orders

- [x] [Task 01: Replay successor setup events before the generation commit](task-01-continuation-setup-replay.md)

## Changed-path coverage

The work order owns the lifecycle files above, the new
`continuation_setup_replay_test.go`, AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.3 in
`docs/specs/agents/requirements/harness-session-continuity.md`, the pointer in
the continuation design, and the "Successor setup replay" section of the stream
processing design.

## Verification

`continuation_setup_replay_test.go` drives the real stream preflight against a
local agentctl status server and a real SQLite repository, and drives
`prepareAgentEvent` against a real repository while the continuation generation
is still pending. Both tests fail on the base revision and pass with the fix.

## Risks

- Accepting a zero cursor too broadly could hide lost history; the exception
  requires a predecessor-replacing execution, an adjacent pending generation, a
  fully retained unacknowledged stream, and no retained prompt submissions.
- Waiting for a commit that never happens would hang startup; the existing
  bounded successor wait and cancellation still apply.
