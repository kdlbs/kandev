---
id: "01-policy-and-analysis"
title: "Define payload retention policy and analysis"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.1
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.3
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.5
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.7
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.6
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 01: Policy and Analysis

## Summary

Establish the policy, exact payload transformation, eligibility checks, and
read-only analysis. This work order does not arm cleanup.

## In scope

- Add the versioned settings/runtime record and revision validation. Default off
  without scanning or rewriting existing messages at startup.
- Implement the pure reducer using the design's field map. Resolve generic
  control/result exclusions against MCP attachment registries and adapter tests.
  Unsupported envelopes remain unchanged with an explicit skip reason.
- Implement indexed task/session eligibility probes and keyset message scans.
  Include task activity, all session states, turns, queues, and runtime admission.
- Add admin analysis/status endpoints, progress, cancel, bounded continuations,
  policy fingerprint, and payload-byte estimates from the exact reducer.
- Expose capability for unsupported engines. Reject enable requests until the
  guarded runner exists. Return an explicit unavailable/conflict result.

## Out of scope

Payload mutation, backup preparation, scheduler activation, and rendered UI.

## Acceptance

1. Default/invalid/stale policies and denied/unsupported requests produce no
   cleanup side effects. Analysis never changes source messages.
2. The reducer preserves required metadata and unknown fields. Its byte delta
   equals the serialized before/after difference, including its removal marker.
3. Analysis excludes active/queued/ambiguous tasks and returns complete or partial
   estimates correctly within the specified work, row, and statement bounds.

## Tests and verification

Write failing tests before implementation. Cover leap years, end-of-month clamp,
cutoff equality, renamed tasks, new session activity, every session state, null
timestamps, mixed legacy rows, structured generic results, and oversize payloads.
Prove indexed eligibility queries with `EXPLAIN QUERY PLAN` on a generated fixture.
Compare messages before and after analysis and probe concurrent foreground writes.

Run each command from the repository root, independently:

```bash
(cd apps/backend && go test ./internal/system/toolretention/... ./internal/task/models/... ./internal/task/repository/sqlite/...)
(cd apps/backend && go test -race ./internal/system/toolretention/...)
git diff --check
```

## Files likely touched

- `apps/backend/internal/system/toolretention/` policy, analysis, handler,
  runtime store and tests (no new migration).
- `apps/backend/internal/task/models/tool_payload_retention.go` and tests.
- `apps/backend/internal/task/repository/sqlite/tool_payload_retention_scan.go`
  and its unit and production-schema fixture tests.
- `apps/backend/internal/system/system.go` and backend composition tests.
- Existing task/runtime admission readers and system settings persistence.

## Dependencies and parallelism

No work-order dependency. The primary session integrates the contract; authorized native subagents own separate files. The reducer and
queries define the mutation contract that Task 02 must reuse.

## Inputs

Read the linked design and requirements. Inspect `streams/tool_payload.go`,
`models/message_shell_output.go`, `service/service_messages.go`, and task indexes.
Use `/tdd` before implementation. Use the approved task-inactivity basis.

## Risks

A generic result can contain a durable plan or attachment. Do not apply blanket
JSON deletion. No audit or benchmark may open the production database.

## Results

Implemented in `internal/system/toolretention`, `internal/task/models`, and the
SQLite task repository. The policy and runtime share one versioned settings
record. There is no migration, new index, or startup scan.

- Focused reducer and eligibility tests cover field preservation, unsupported
  control envelopes, timestamp offsets, invalid dates, ownership mismatches,
  pending delivery, empty terminal sessions, and unknown states.
- Analysis tests compare source metadata before/after and assert exact serialized
  byte deltas. Cutoff tests cover UTC, equality, leap years, and month-end clamping.
- Production-schema fixtures use the real task, Office, and queue repositories.
  EXPLAIN uses covering session created/updated indexes, the task/author/type
  index, and the run status/requested index before looking up active run bodies.
- Race-enabled fixture measurements: 10,000 messages plus 2,000 terminal runs
  with 8 KiB bodies, 32.313 ms; 100,000 messages, 185.855 ms. A 250,000-message
  task was retained at the 200ms deadline. These are fixture measurements, not
  production estimates or guaranteed throughput.
- `go test ./internal/system/... ./internal/task/models/...
  ./internal/task/repository/sqlite/... ./internal/task/service/...
  ./internal/task/handlers/... ./internal/persistence/...`: passed from apps/backend.
- Scoped eligibility race tests passed. No source or audit read the production DB.
