---
id: "01-broaden-cursor-retriable-errors"
title: "Broaden Cursor RetriableError matching"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.12
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.13
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.14
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.15
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 01: Broaden Cursor RetriableError Matching

## Summary

Make the Cursor ACP transport and shared routing catalogue recognize any
bounded, non-empty diagnostic after the anchored `Error: RetriableError:`
prefix. The reported `[unavailable] PING timed out` and `Connection stalled`
messages shall enter the existing transient transport-loss path.

The current notification barrier, stable safe provider error, same-provider
retry owner, and pre-result effect-safety gate remain authoritative.

## In scope

- Update the Cursor ACP matcher and its focused tests.
- Update the existing Cursor routing rule and its focused tests.
- Add end-to-end backend assertions that broad matches still settle through one
  structured error and only safe prompt evidence schedules replay.

## Out of scope

- New retry policy or provider-switch behavior.
- Other ACP adapters or arbitrary prose scanning.
- ACP `RequestError.Data` classification.
- Frontend, localization, persistence, and public API changes.

## Acceptance

- A current Cursor message beginning with `Error: RetriableError:` and carrying
  either reported suffix is suppressed and becomes one post-barrier structured
  provider error; progress before settlement clears the marker. Cancellation,
  deadline, and retry-escalation suffixes remain ordinary output.
- The routing catalogue maps both suffixes to high-confidence transient
  `agent_transport_lost` with the existing rule identity and cancellation
  veto. The adapter and catalogue use the same Unicode-trimmed 256-byte bound;
  prose before the prefix, empty/partial markers, stale generations, and other
  adapters do not match.
- Automatic replay still requires the current execution and prompt generation
  with known pre-result evidence and no output or tool effect; broad matching
  alone cannot authorize replay.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CursorRetriable|ObserveCursorRetriable|SendPrompt.*Cursor)')
(cd apps/backend && go test -race ./internal/agent/runtime/routingerr -run 'Test(ClassifyCursorRetriable|MatchRuntimeEnvironmentRules_CursorRetriable|IsTransientProviderError_Cursor)')
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'Test(HandleTransientFailure.*Replay|PromptAttemptEvidence|CursorTransportLost)')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/cursor-retriable-error-expansion
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_retriable_test.go`
- `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`
- `apps/backend/internal/agent/runtime/routingerr/cursor_retriable_stream_reset_test.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_transport_lost_test.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_replay_safety_test.go`

## Dependencies

The completed `docs/plans/cursor-retriable-stream-reset/` package supplies the
existing projection, classifier, and retry-owner contracts. No new dependency
or schema migration is required.

## Risks

- An unanchored or unbounded match could turn provider prose into retry
  evidence.
- A classifier match without the existing evidence fence could replay a turn
  after output or a tool side effect.
- The stable `cursor.retriable_stream_reset.v1` rule identity must not be
  changed while its accepted suffix set expands.

## Parallelism

`sequential`

## Inputs

- Provider Error Recovery criteria `.12`, `.13`, `.14`, and `.15`.
- Cursor projection and retry-safety sections in
  `docs/specs/platform/system-design/provider-error-recovery.md`.
- Existing Cursor ACP, routing catalogue, and orchestrator transport-loss
  tests.

## Results

- Broadened the Cursor ACP matcher to accept any bounded, non-empty suffix
  after the anchored `Error: RetriableError:` prefix.
- Broadened `cursor.retriable_stream_reset.v1` to classify the same diagnostics
  as high-confidence transient `agent_transport_lost` failures while retaining
  its stable rule identity and cancellation veto.
- Added coverage for `[unavailable] PING timed out`, `Connection stalled`,
  structured post-barrier settlement, cancellation vetoes at the adapter
  boundary, byte and Unicode-whitespace limits, anchored negatives, and
  existing safe retry scheduling.
- Verification passed:

  ```text
  (cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CursorRetriable|ObserveCursorRetriable|SendPrompt.*Cursor)' -count=1)
  (cd apps/backend && go test -race ./internal/agent/runtime/routingerr -run 'Test(ClassifyCursorRetriable|MatchRuntimeEnvironmentRules_CursorRetriable|IsTransientProviderError_Cursor)' -count=1)
  (cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'Test(HandleTransientFailure.*Replay|PromptAttemptEvidence|CursorTransportLost)' -count=1)
  (cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -count=1)
  (cd apps/backend && go test -race ./internal/agent/runtime/routingerr -count=1)
  (cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -count=1)
  make -C apps/backend lint
  python3 scripts/list-docs.py validate
  python3 scripts/lint-spec-files.test.py
  python3 scripts/lint-spec-files.py --all
  git diff --check
  ```
