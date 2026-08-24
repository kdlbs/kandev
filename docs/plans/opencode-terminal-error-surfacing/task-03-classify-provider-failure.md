---
id: "03-classify-provider-failure"
title: "Classify and persist provider failures"
status: done
wave: 3
depends_on: ["02-settle-correlated-opencode-prompt"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 03: Classify and Persist Provider Failures

## Acceptance

- The winning generation's safe provider-error record and concrete agent ID
  reach `agent.failed`; stale or duplicate terminal events cannot replace them.
- OpenCode `usage limit reached` classifies as high-confidence
  `quota_limited`, with the existing sanitizer removing URLs and long
  identifiers from every persisted excerpt.
- Non-Office recovery messages persist `provider_quota_limited` metadata with
  only safe provider/model/reset/details fields and existing recovery actions;
  absent or unrecognized details use the generic error card.

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/agent/runtime/routingerr ./internal/orchestrator ./internal/orchestrator/watcher -run 'Test(.*ProviderError|.*OpenCode.*UsageLimit|HandleRecoverableFailure.*Quota|CreateRecoveryStatusMessage.*Quota)'`

Use TDD at the lifecycle event, classifier, watcher, and persisted-message
boundaries. Assert the observed OpenCode workspace URL and identifier are
absent from error messages, raw excerpts, and message metadata.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go`
- `apps/backend/internal/agent/runtime/routingerr/rules.go`
- `apps/backend/internal/agent/runtime/routingerr/classify_test.go`
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/watcher/watcher_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`

## Dependencies

Task 02.

## Parallelism

Sequential. Task 04 consumes the persisted metadata contract defined here.

## Inputs

- Spec `Diagnostic contract`, persistence guarantees, and quota scenarios
- Plan sections `Lifecycle propagation and quota classification` and Tests
- Existing `MarkCompleted`, `newAgentEventPayload`, `routingerr.Classify`,
  `handleRecoverableFailure`, and `createRecoveryStatusMessage` paths

## Risks

- Provider classification must use the concrete `opencode-acp` agent ID and
  must not infer trust from arbitrary error prose.
- Raw recent stderr remains outside the persisted safe record.

## Output contract

Report RED assertions, event and metadata shapes, classification rule and
confidence, sanitization proof, exact tests, files changed, blockers, and
risks. Mark this task `done` and update its plan checkbox in the same
conversation.

## Results

The safe provider record and concrete `opencode-acp` ID now reach
`agent.failed`. The exact five-hour usage-limit signature is high-confidence
`quota_limited`; URL/session/workspace identifiers are redacted before
persistence. Non-Office recovery metadata includes only the specialized
failure kind, provider/model/reset values, sanitized details, and existing
actions. The exact verification command passed with 13 tests across four
packages.
