---
id: "01-shell-snapshots"
title: "Bound streamed shell snapshots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.4
  - AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.5
system_design:
  - ../../specs/platform/system-design/delivery-stream-storage.md
---

# Task 01: Bound streamed shell snapshots

## Summary

Reduce consecutive foreground ACP shell-output snapshots before lossless event
admission. Keep normalization, final output, ordered state boundaries and durable
sequence assignment intact. Extend the existing notification worker's ownership.

## In scope

- One pending immutable normalized snapshot and fixed 250 ms timer on the existing
  worker; immediate initial output, quiet trailing flush and immediate terminal result.
- Explicit output-only eligibility, session/generation/tool identity, provider
  delta/cumulative/final carriers, UTF-8 tail and separate stdout/stderr truncation.
- Flush before any ordered noneligible event, queue barrier or session/prompt transition;
  audit direct and locked send sites so they cannot overtake pending output.
- Lifecycle cancellation, backpressure and deterministic timer injection for tests.

## Out of scope

Native Codex snapshot changes, wire deltas, journal rewrite, configuration knobs,
new goroutines, database cleanup and changes to output-tail limits.

## Acceptance

1. The saturated 10,000-update/ten-second fixture emits at most 42 full snapshots
   and under 16 MiB, with the same final bounded stdout/stderr/truncation/exit code
   as processing every source notification without emission reduction.
2. A quiet pending update flushes by the fixed deadline; tool/status/metadata changes,
   text, permissions, background work, reset and terminal/generation boundaries are
   preserved in order. Unknown shapes use the existing lossless path.
3. Close/cancel/backpressure tests drain the existing worker without leaking timers,
   blocking on Adapter.mu or publishing output into a successor session/turn.

## TDD evidence

New `adapter_shell_snapshot_test.go`: `TestShellSnapshotCoalescingStorageBound`,
`TestShellSnapshotQuietFlush`, `TestShellSnapshotOrderedBoundaries`,
`TestShellSnapshotProviderShapes` and `TestShellSnapshotShutdown`.
First demonstrate excess emission with the current adapter. Use controlled clocks
and changing tail content; identical-only suppression must fail the storage test.
Retain existing `shell_output_test.go`, notification-barrier and background tests.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -count=1 -timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_snapshot.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_shell_snapshot_test.go` (new)

## Dependencies

None. Read current base guidance and symbols before editing. Keep changes within the
ACP notification/emission owner, with focused helpers to satisfy complexity limits.

## Risks

Timers must not debounce indefinitely or overtake FIFO barriers. Never coalesce raw
deltas or compare only output length; a full moving tail changes after truncation.
Legacy and durable consumers share this adapter path.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/platform/system-design/delivery-stream-storage.md), snapshot admission.
- [ADR](../../decisions/2026-10-13-superseding-shell-snapshots.md).
- Existing `runUpdateWorker`, `syncNotifQueueThen`, `sendUpdate`,
  `convertToolCallResultUpdate`, `NormalizeShellToolUpdate` and neighboring tests.
- Backend and agentctl AGENTS guidance; `/tdd` backend-test reference.

## Results

Completed. Behavioral RED emitted 10,000 snapshots; the optimized equivalent saturated-delta fixture also failed at 43 snapshots against the 42 limit. GREEN includes storage bounds, quiet trailing flush, ordered external emissions, generation changes, provider delta/cumulative/content carriers, terminal/barrier handling and cancellable shutdown. Unknown nested output metadata had a separate RED and now stays lossless. The installed Codex ACP 2.2.2 producer was then checked against its versioned source; a real `{data, terminal_id}` output carrier had a separate behavioral RED and now coalesces only for an exact tool-call identity.

`go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -count=1 -timeout=5m` passed after the final implementation edit (194.885 seconds, including the combined storage test). The delayed-output admission-generation regression also failed before the fix and now preserves the source generation for the existing process-manager submission binding. No new wire field, schema or output-tail change was needed.
