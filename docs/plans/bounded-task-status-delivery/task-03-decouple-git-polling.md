---
id: "03-decouple-git-polling"
title: "Decouple Git polling from viewers"
status: completed
wave: 3
depends_on: ["02-publish-live-task-status"]
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 03: Decouple Git Polling From Viewers

## Acceptance

- A live execution contributes slow workspace Git-monitoring interest even
  when no browser subscribes to its session; selected focus upgrades that
  workspace to fast mode.
- Runtime/viewer interest is reference-safe across sibling sessions and every
  stop, failure, cancellation, reconnect, and recovery path; monitoring pauses
  only when neither interest source remains.
- Latest per-repository observations feed the task summary, while settled tasks
  retain their persisted completion/live snapshot and a poll failure never
  fabricates a clean aggregate.

## TDD sequence

1. RED: add lifecycle aggregation cases for runtime-only slow mode, focus fast
   mode, sibling sessions, duplicate transitions, and final release.
2. RED: add Git observation tests for repository-keyed aggregation, settle
   persistence, and poll failure retention without any WebSocket subscriber.
3. GREEN: add runtime interest to workspace poll-mode calculation and connect
   bounded Git observations to the projector.
4. REFACTOR: centralize interest precedence and ensure cleanup paths are
   idempotent.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... ./internal/task/statussummary
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/**`
- `apps/backend/internal/gateway/websocket/hub_session_mode.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/task/statussummary/**`
- related runtime/poll/Git tests

## Dependencies

Task 02 supplies the Git-summary observation sink.

## Parallelism

Sequential. The switcher subscription removal in Task 05 is safe only after
runtime-owned monitoring exists.

## Inputs

- Spec **Persistence guarantees** Git clauses.
- Existing focused/subscribed/paused poll-mode aggregator.
- Existing `UpsertLatestLiveGitSnapshot` and completion snapshot semantics.

## Risks

- Leaked runtime interest would poll forever; premature release would freeze
  background badges. Exercise every terminal path.
- Count interest by execution/workspace identity, not repeated stream events.
- Preserve multi-repository identity before aggregating numeric totals.

## Verification results

- `cd apps/backend && go test ./internal/agent/runtime/lifecycle` — passed.
- Broad backend integration packages — passed.
- Runtime-only executions retain slow workspace monitoring without a browser
  subscriber; focus upgrades to fast mode, duplicate interest is idempotent,
  and removal/recovery paths release the runtime reference safely.
- Git observations remain keyed by environment/repository and feed the task
  summary without making browser session membership responsible for polling.

## Output contract

Report the interest state machine, cleanup paths audited, Git freshness
behavior, focused test results, exact files changed, and remaining lifecycle
risks.
