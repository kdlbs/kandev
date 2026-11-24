---
id: "01-notify-after-admission"
title: "Restore accepted plan feedback delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-002.2
  - AC-TASKS-PLAN-COMMENTS-002.5
  - AC-TASKS-PLAN-COMMENTS-002.6
  - AC-TASKS-PLAN-COMMENTS-003.1
  - AC-TASKS-PLAN-COMMENTS-003.3
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 01: Restore accepted plan feedback delivery

## Summary

Move accepted-feedback notification outside the session admission callback in
`wsAddMessage`. Remove ordinary queue claim updates from retained-receipt
dispatch. Prove both corrections before checking fresh agent replies on desktop
and phone.

## In scope

- Direct plan-comment and shared preview-feedback notification ordering.
- Agent admission for retained comment receipts and ordinary queue entries.
- Initial-brief eligibility, failed admission, stale retry, and exact replay.
- Existing selected-session Send and primary-session Run browser scenarios.

## Out of scope

Changes to provider adapters, cancellation intent, queue timeouts, receipt
semantics, migrations, public APIs, or rendered desktop and phone controls.
Do not use detached goroutines to hide the lock cycle.

## Acceptance

1. The real handler completes message admission before it notifies the queue.
   Notification uses a context that does not claim the released session gate.
2. Successful admission preserves the resolved message, durable receipt, exact
   consumption, and existing initial-brief eligibility. Failed attempts do not
   notify. Replay does not create a second delivery.
3. Desktop and phone Send/Run scenarios observe fresh agent output in the
   intended session after each submission, with no duplicate feedback delivery.

## TDD procedure

Create `apps/backend/internal/task/handlers/message_handlers_feedback_admission_test.go`.
Use `newCreationAttachmentHandler` or an equivalent real-SQLite fixture.
Initialize the queue schema with `messagequeue.NewSQLiteRepository` over the
fixture database. Seed an in-progress task, a waiting session, its current
plan, and one persisted comment. Submit through `wsAddMessage` with a stable
`client_message_id` and exact `plan_comment_refs`.

Extend the test coordinator with all four initial-brief admission methods.
Its admission callback records or gates its lifetime. Its notification method
asserts that admission already returned. Keep the ordinary message receipt and
comment-consumption assertions. Before the correction, this regression must
fail because notification runs while admission is active.

Add focused cases for stale references, an initial-brief contender, and exact
replay. Observe notification calls, committed messages, and remaining comments.
Use barriers to control concurrent work. Bound and join all test goroutines.
The diagnostic orchestrator experiment in the plan explains the competing lock
sequence. Do not retain a test that manually invokes the forbidden sequence.

Implement the smallest handler correction. Keep synchronous persistence and
initial-brief bookkeeping inside admission. Apply the existing notification
predicate only after successful admission. Do not carry `messageCtx` outside
the callback. Preserve replay handling in `addMessageReplayResponse`. Retain
the original request for stale-brief rebuilds. Reusing the prepared content
inserts the reserved comment marker twice and rejects the retry.

Add `event_handlers_plan_comment_delivery_test.go` in the orchestrator package.
Use the task fixture's actual SQLite database for the queue repository.
Reserve and execute direct-message comments, queued comments, and ordinary
messages through the real orchestrator. Test legacy and durable agent
capabilities. Require one provider call, one transcript, an empty pending queue,
and a persisted submission for durable delivery. The retained comment cases
must fail before the correction. Attach the ordinary `deliveryClaimUpdater`
except for durable plan comments and lifecycle prompts. Preserve ordinary
claim validation for managed inputs. Keep retained ownership
validation, protocol negotiation, and backend submission persistence.

Finally strengthen the two existing browser scenarios described in the plan.
Capture prior agent-message IDs before Send and Run. Require a new response
after the correlated accepted user message. Do not use an earlier response or
an idle composer as evidence of a new turn.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/handlers -run 'PlanComment|PreviewFeedback|InitialTaskBrief|FeedbackAdmission' -count=1 -timeout=180s)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'PlanComment|QueueDispatch|QueuedDelivery|QueuedLifecycle|ManagedInput|DurableLifecycle' -count=1 -timeout=180s)
GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=kandev.lintBaseRef GIT_CONFIG_VALUE_0=origin/main bash scripts/lint-go-changed apps/backend/internal/task/handlers/message_handlers.go apps/backend/internal/task/handlers/message_handlers_feedback_admission_test.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/event_handlers_plan_comment_delivery_test.go
env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG -u KANDEV_BUNDLE_DIR KANDEV_SERVER_HOST=127.0.0.1 pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/task-plan-comments.spec.ts -- --grep 'shares comments across tabs'
env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG -u KANDEV_BUNDLE_DIR KANDEV_SERVER_HOST=127.0.0.1 pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts -- --grep 'migrates shared context'
pnpm --dir apps/web exec eslint e2e/tests/session/task-plan-comments.spec.ts e2e/tests/session/mobile-task-plan-comments.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Install workspace dependencies once with `pnpm --dir apps install --frozen-lockfile`
if they are absent. On this Nix host, Playwright's Chromium also needs its shared
library directories in `LD_LIBRARY_PATH`. The investigation resolved each
missing library through `/nix/store/*/lib`, then checked `ldd` and Chromium
`--version`. Resolve those paths for the active host. Do not commit host paths
or change production browser configuration.

Run the browser commands sequentially. The first command rebuilds the backend
and web artifacts. The phone command reuses that exact build. Record actual
test counts and results before marking this work order done.

## Files likely touched

- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_feedback_admission_test.go`
- `apps/backend/internal/task/handlers/message_handlers_feedback_completion_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_plan_comment_delivery_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go`
- `apps/web/e2e/tests/session/task-plan-comments.spec.ts`
- `apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts`
- This work order and `plan.md` for status and results.

## Dependencies

None. Existing comment ownership and initial-brief behavior are the required
contracts, not work to reopen.

## Risks

The initial-brief callback can retry after a stale candidate. Notify only for
the final accepted result. A losing contender must keep its current deferral.
The notification context must not retain a released session admission token.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-comments.md), requirements 002 and 003.
- [System design](../../specs/tasks/system-design/plan-comments.md#atomic-acceptance).
- [Reproduction and proposed ordering](plan.md#reproduction-evidence).
- Existing handler fixtures in `message_handlers_plan_comments_test.go` and
  `message_handlers_initial_task_brief_test.go`.

## Results

The handler now notifies after session admission returns and rebuilds stale
briefs from the original request. Queued dispatch skips ordinary claim updates
for plan-comment receipts. Those receipts reach agent admission with their
existing ownership and delivery-attempt checks.

The handler ordering regression failed before its correction. The real-SQLite
orchestrator regression failed for direct and queued comments with both legacy
and durable agents before the dispatch correction. Ordinary messages passed.
All six admission cases now pass.

- Handler race suite: passed, 3.189 seconds.
- Orchestrator race suite: passed, 4.653 seconds.
- Desktop Send/Run with fresh replies and no duplicates: passed, one test,
  13.8 seconds total.
- Phone Send/Run against the same build: passed, one test, 15.7 seconds total.
- E2E ESLint, catalog validation, full specification lint, documentation
  coverage preflight, and whitespace checks: passed.
- Changed-Go lint: passed, zero issues with CI-pinned golangci-lint v2.9.0.

Cleanup removed temporary logs and diagnostic captures. All owned isolated
browser instances stopped. Implementation and validation are complete.

## PR review follow-up

Added a real-handler queue-drain fixture. Direct feedback must reach its prompt
boundary. A held ordinary first prompt must keep comment feedback queued until
the handler completes that dispatch, then deliver the feedback and acknowledge
its receipt. Both focused race tests pass. Removing the handler's asynchronous
completion call makes the deferred-feedback assertion fail. The mutation probe
restored the handler source byte-for-byte. The final related handler race
suite passed in 3.094 seconds. The orchestrator race suite passed in 6.663 seconds,
including the existing queued-delivery eligibility and lifecycle tests.

The eligibility test now accepts plan-comment delivery and keeps managed-input
rejection. Narrowing the updater bypass to durable plan comments fixes that
preserved boundary. The broad bypass failed the managed-input assertion before
this correction. Normal commit hooks must pass before publication. Remote CI
and review resolution remain external verification for the pushed head.

The design now states the process-local SQLite guard, cross-process PostgreSQL
lock, writer-connection rationale, and exact notification lock order. It links
receipt tokens and delivery-attempt validation to their defining section.

## CI integration follow-up

The branch merges the upstream background follow-up fixture correction. The
mock tool now stays active until the browser writes a release marker after
queue admission. This removes the timing window seen in CI. The merged
handler, orchestrator, and mock-agent race tests pass (2.753, 6.911, and 1.152
seconds). The merged background follow-up test passed without retries in 14.7
seconds. Desktop Send/Run passed against the same fresh build in 12.6 seconds.
Phone Send/Run passed without retries in 15.6 seconds.

The previous backend and frontend workflows passed. Kubernetes 1.34 setup
failed while Kind loaded the fixture image. Fresh CI must recheck that failure.
CI and review evidence for the previous head is historical.
