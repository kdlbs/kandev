---
created: 2026-08-31
status: done
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
legacy_specs: []
---

# Implementation Plan: Cursor Retriable Stream Reset

## Overview

Project Cursor's terminal HTTP/2 `RetriableError` control frame into one
structured provider failure. Classify its safe diagnostic as the existing
transient transport-loss cause. Authorize the existing same-provider retry only
for prompt attempts that have no output or tool activity. Adapter detection and
classifier work can proceed independently. Replay-safety wiring depends on both
contracts and completes the end-to-end behavior.

## Scope

### In scope

- Cursor-only, prompt-generation-scoped detection of the complete terminal
  stream reset control frame.
- Suppression of the control frame and one post-barrier structured error.
- Suppression of a Kandev retry when Cursor resumes its own attempt after the
  marker.
- A dedicated deterministic classifier rule for the safe Cursor diagnostic.
- Provider-neutral replay evidence for concrete interactive retry decisions.
- Existing same-provider retry ownership, attempt limit, delay schedule, and
  manual recovery fallback.

### Out of scope

- Generic scanning of arbitrary assistant output.
- Widening the existing generic `transportLostRe` expression.
- Cursor-specific retry timers, counters, or backoff values.
- Automatic provider or model switching for `agent_transport_lost`.
- New UI, user-facing copy, persistence, or public API changes.

## Technical approach

### Cursor ACP evidence projection

- Add the allocation-conscious matcher and bounded safe diagnostic to
  `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`.
- Extend `promptTurnState` in `adapter.go` with mutex-guarded pending Cursor
  evidence. Matching control frames set the pending marker. Later
  same-generation provider progress clears it. A later matching frame re-arms
  it.
- Add `observeCursorRetriableEvidence` beside the Codex observer in
  `adapter_updates.go`. Check agent identity, event type, prompt generation, and
  the anchored prefix before the full fingerprint. Suppress only a matching
  control frame.
- In `adapter_prompt.go`, inspect pending evidence only after `syncNotifQueue()`.
  Emit one `EventTypeError` with a valid `ProviderError` and return instead of
  emitting the ordinary complete event.
- Add `ProviderErrorSourceCursorACP` to
  `apps/backend/internal/agentctl/types/streams/provider_error.go`.

### Deterministic classification

- Add `cursor.retriable_stream_reset.v1` to
  `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go` before the
  generic transport-loss rule.
- Require the complete normalized Cursor diagnostic, with the existing
  bracketed `canceled` decoration allowed.
  Apply the existing context-cancellation veto without changing
  `transportLostRe`.
- Assert the full `Classify` result is `CodeAgentTransportLost`,
  `ClassTransient`, high confidence, and auto-retryable. Pin that `[canceled]`
  does not trigger the cancellation veto.

### Replay authorization and existing retry owner

- Generalize the process-local attempt evidence in
  `apps/backend/internal/orchestrator/dynamic_evidence.go`. Concrete interactive
  prompts also retain output and tool observations. The evidence has session,
  execution, and prompt-generation scope. Preserve `DynamicRouteAttempt` as
  routing identity. Do not turn concrete prompts into dynamic attempts.
- Carry a conservative terminal evidence snapshot from lifecycle on
  `agent.failed`, captured before completion bookkeeping marks the terminal
  event as activity. The orchestrator must prefer this immutable snapshot over
  stream-subscription timing while retaining local identity fencing.
- Begin or replace concrete prompt evidence at prompt dispatch. Bind it to the
  execution and generation. Update it from `event_handlers_streaming.go`. Clear
  only the matching completed or replaced attempt.
- Cache and reserve replay identity before a model-switch restart can dispatch
  its replacement prompt, then bind it to the replacement execution.
- Make `handleTransientFailure` require known evidence with no output or tool
  activity before it schedules any automatic prompt replay. Missing, stale,
  output-bearing, or tool-bearing evidence falls through to manual recovery.
- Keep `CodeAgentTransportLost.FallbackAllowed` false. Dynamic routing retains
  its current fail-closed gate and cannot reinterpret the Cursor signature as
  permission to switch providers.
- Reuse `transientMaxAttempts`, `transientRetryDelayFor`, the existing single
  retry entry, and the existing resume-before-reprompt path without adding a
  Cursor-specific retry loop.

## Tests

- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.12`: a new ACP test file covers the
  exact captured payload, anchored negatives, partial negatives, agent scope,
  and generation scope. It also covers chunk suppression, queue-barrier
  ordering, and one structured error instead of complete.
- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.13`: ACP tests cover progress that
  clears the pending marker. They also cover a later matching marker that re-arms
  it.
- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.14`: a new `routingerr` test file
  covers the semantic code, class, rule ID, invariants, and negative cases. It
  also covers the `[canceled]` non-veto case.
- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.15`: a new orchestrator test file
  covers safe retry scheduling. It covers fail-closed behavior for missing,
  stale, output, and tool evidence. It also covers prompt-generation
  replacement.

## Work orders

- [x] [Task 01: Project Cursor terminal evidence](task-01-project-cursor-terminal-evidence.md)
- [x] [Task 02: Classify Cursor stream resets](task-02-classify-cursor-stream-resets.md)
- [x] [Task 03: Gate automatic prompt replay](task-03-gate-automatic-prompt-replay.md)

## Verification results

- Task 01: `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CursorRetriable|ObserveCursorRetriable|SendPrompt.*Cursor)'` passed.
- Task 02: `cd apps/backend && go test -race ./internal/agent/runtime/routingerr -run 'Test(ClassifyCursorRetriable|MatchRuntimeEnvironmentRules_CursorRetriable|IsTransientProviderError_Cursor)'` passed.
- Task 03 targeted suite: `cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'Test(HandleTransientFailure.*Replay|PromptAttemptEvidence|CursorTransportLost)'` passed.
- Remediation identity-fence suite: `cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestDynamicAttemptEvidence|TestDynamicPreResultRequiresExplicitKnownEvidence|TestHandleTransientFailure.*Replay|TestCursorTransportLost'` passed.
- Full lifecycle race suite: `go test -race ./internal/agent/runtime/lifecycle` passed, 2088 tests.
- Full orchestrator race suite: `go test -race -tags fts5 ./internal/orchestrator` passed, 2261 tests.
- `make -C apps/backend lint` and changed-lines `golangci-lint` passed with 0 issues.
- Changed Go files reported no `gofmt` differences; `git diff --check` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- `make -C apps/backend test` was attempted. The changed packages passed, while unrelated baseline tests failed because the host config discovery selected `/root/.kandev/config.yaml` and the GitHub fixture lacked `github_task_prs`.

## Risks

- ACP notification ordering can misclassify Cursor-owned retry progress if the
  pending marker is examined before the queue barrier.
- Clearing on an old tool update can hide a true terminal failure. Only renewed
  provider generation or a newly-started tool can clear the marker.
- Generalizing replay evidence can expose existing transient tests that assumed
  unknown evidence was safe. Those tests must provide explicit safe evidence or
  assert manual recovery.
- A stale prompt generation must not clear or authorize a successor turn.
