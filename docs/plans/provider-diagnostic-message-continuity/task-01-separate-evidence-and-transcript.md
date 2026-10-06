---
id: "01-separate-evidence-and-transcript"
title: "Separate output evidence from transcript buffering"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.16
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.21
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.23
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-03.md
---

# Task 01: Separate Output Evidence from Transcript Buffering

## Summary

Make lifecycle transcript buffering independent of provider-diagnostic markers.
Deliver original chunks to guarded recovery/progress observation separately,
while retaining existing transcript identity, tool and terminal boundaries.

## In scope

- Add the permanent four-chunk regression before production changes. Recreate
  the RED described in the [plan](plan.md#evidence-and-assumption-check), and
  extend it across no protocol ID, one explicit ID, existing newline emission,
  repeated marker transitions, and a following real tool/turn boundary.
- Publish original `message_chunk`/`reasoning` evidence copies at lifecycle
  intake with original text, role, candidate and immutable correlation identity.
  Ignore user and empty input; preserve supplied generation and fill an absent
  one once. Use the existing event publisher/carrier and watcher.
- Move orchestrator output observation and foreground progress to original
  events. They do not persist/broadcast. Stop observing evidence or progress
  from accumulated transcript projections. Keep original-event guard ordering,
  tool effects, terminal snapshots, and fail-closed absent-marker treatment.
- Remove all transient diagnostic state and arguments from visible buffers,
  helpers, flush/reset paths and coalescing. Preserve prompt/attempt/message
  keys, flush boundaries, history order and response-attempt retraction.
- Update tests that inject `message_streaming` as evidence to inject original
  output events instead; retain independent transcript-writing tests. Replace
  the legacy splitting test and marker-sensitive coalescer assertions.
- Cover genuine diagnostic visibility, matching terminal failure, earlier/later
  ordinary output, thought clearing, mixed tool effects, stale identity, and
  delayed terminal delivery without relying on synchronous bus callbacks.

## Out of scope

Classifier changes, retry policies, public API/schema changes, frontend changes,
historical rows, and the browser proof owned by Task 02.

## Acceptance

1. `TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity` fails
   before the fix with three IDs, then passes with one ID and exact joined
   content. Real tool/turn/changed-protocol-ID boundaries still create distinct
   messages, and original per-event evidence retains its marker and identity.
2. Transcript projections never change recovery/progress evidence. Genuine
   diagnostics remain visible and eligible only under the existing matched
   terminal-error, containment, identity and effect gates. Async terminal
   ordering and no-marker ordinary-output regressions pass.
3. All verification commands pass. No provider candidate reaches message
   storage, no marker-dependent visible buffer/coalescer code remains, and
   existing attempt retraction/history and foreground tests retain coverage.

## Verification

Run from repository root, using an independently rooted subshell:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity$' -count=1 -v)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/agentctl/server/adapter/transport/acp -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the first command's RED before production changes, then GREEN and all
final results. The three package suites cover the changed integration boundary,
including replay fixtures; no full backend suite is required.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_streaming.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go` if a copy helper is needed
- `apps/backend/internal/agent/runtime/lifecycle/stream_coalescer.go`
- `apps/backend/internal/agent/runtime/lifecycle/provider_diagnostic_transition_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/provider_diagnostic_accumulation_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/provider_diagnostic_marker_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_coalescer_test.go`
- Existing lifecycle streaming/history/response-attempt tests requiring signature updates
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_provider_diagnostic_test.go`
- `apps/backend/internal/orchestrator/dynamic_evidence_terminal_ordering_test.go`
- Existing orchestrator tests/helpers injecting projections as output evidence
- This work order and `plan.md` for status/results only

## Dependencies

None. Producers and consumers must change together in this work order.

## Risks

Double observation, missed reasoning text, stale prompt snapshots, per-token
backend traffic, and asynchronous terminal ordering. Never classify merged
transcript text or use publish completion as a consumer acknowledgement.

## Parallelism

`sequential`

## Inputs

- [Active provider recovery requirement](../../specs/platform/requirements/provider-error-recovery.md), `.16`, `.20`, `.21`, `.23`.
- [Part-3 design](../../specs/platform/system-design/provider-error-recovery-03.md#marker-propagation).
- Existing `createTestManagerWithTracking`, `newTransientTestService`, diagnostic
  correlation/replay fixtures, and terminal-ordering tests.
- Scoped backend/agentctl guidance and `/tdd` backend test patterns.

## Results

Complete on 2026-10-06. The permanent
`TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity` was run
before production edits and failed with three message IDs for one assistant
message; after the implementation, it passed with one stable identity and
exact joined content. It covers absent and explicit protocol IDs, repeated
marker changes, newline flushing, and a real tool boundary.

Lifecycle now publishes original message/reasoning evidence after existing
attempt, context-reset, idle-suspension, and duplicate-event guards, filling
only a missing prompt generation. Transcript buffers and coalescing no longer
key or split on diagnostic markers. The orchestrator observes only original
events for diagnostic and foreground evidence; visible message/thinking
projections continue to persist and broadcast without re-observing or clearing
genuine diagnostic evidence. Focused regressions cover ordinary progress,
candidate non-progress, diagnostic correlation, and matching-terminal safety.

The PR review follow-up moves dynamic streak persistence out of the raw stream
callback. Original output, reasoning and effect events update in-memory safety
evidence and coalesce one per-session reset intent; persistence retries only at
semantic boundaries. Failed resets remain pending and prevent automatic
fallback until safely cleared. Permanent deletion retires the intent only
after the session row is deleted. Focused regressions prove blocked persistence
cannot block transcript delivery, joined transcript content stays intact,
retries survive prompt changes, stale identities do not write successor state,
and failed deletion retains the intent.

The named regression passed on the host with task-owned `GOCACHE`. The initial
three-package race suite passed in the task-owned Go 1.26 Linux container with
`TMPDIR=/tmp`, private `GIT_CONFIG_GLOBAL`, and all requested tests enabled:

- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity$' -count=1 -v)` with `GOCACHE=/private/tmp/kandev-provider-diagnostic-go-cache`: passed.
- `(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/agentctl/server/adapter/transport/acp -count=1)`: passed; lifecycle 219.463s, orchestrator 105.202s, ACP transport 21.881s.

This is the initial implementation receipt before PR review remediation. The
final post-review race and CI package results are recorded in the plan's
[review remediation](plan.md#pr-review-remediation) section.

The earlier macOS and first Linux runner failures and their resolved setup
causes are documented in the [plan verification results](plan.md#verification-results).
Final PR-fixup race, API package and lint receipts are recorded in the plan's
review-remediation results.
