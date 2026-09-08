---
id: "04-make-gitlab-lifecycle-evaluation-lean"
title: "Make GitLab lifecycle evaluation lean"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 04: Make GitLab lifecycle evaluation lean

## Acceptance

- A normal subscribed poll reuses the reviewer observation from its sole MR
  status fetch, performs no second MR read or authenticated-user lookup, and
  loads only the target automation options/checkpoint.
- Internal events distinguish observed-empty reviewers from an absent
  observation, survive map/NATS round-trip, and remain backward compatible.
- Observation-absent events retain the strict MR fallback, while reviewer
  identity comes from the persisted workspace GitLab configuration.

## Verification

- `cd apps/backend && go test -race ./internal/gitlab ./internal/orchestrator`

Add event round-trip, provider-call-count, and targeted-store-query tests first.
The normal-poll budget must fail against the current repeated reads.

## Files likely touched

- `apps/backend/internal/gitlab/models.go`
- `apps/backend/internal/gitlab/poller.go`
- `apps/backend/internal/gitlab/service.go`
- `apps/backend/internal/gitlab/service_task_mr_link.go`
- `apps/backend/internal/gitlab/service_mr_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation_eval.go`
- Adjacent `*_test.go` files

## Dependencies

None.

## Parallelism

Parallel-safe with Tasks 01 and 06 only. It owns GitLab lifecycle evaluation and
the orchestrator's GitLab MR evaluation handler.

## Inputs

- Existing `GetMRStatus` result and `MR.Reviewers`
- Existing `TaskMRUpdatedEvent` map/NATS decoding path
- Existing persisted `GitLabConfig.Username` and task-to-workspace resolution
- Spec `Lean GitLab lifecycle evaluation` and related scenarios

## Risks

- A plain empty reviewer slice cannot represent both observed-empty and missing;
  retain an explicit validity marker through serialization.
- Do not change public `TaskMR`, HTTP, or MCP response contracts while exposing
  an internal sync observation.

## Output contract

Report RED call/query budgets, internal event compatibility, fallback behavior,
files changed, exact verification result, and risks. Mark this task `done` and
update its plan checkbox in the same conversation.

## Result

- RED evidence: the new reviewer-observation map/NATS test failed before the
  event fields existed; the evaluation budget tests also covered the previous
  repeated-reviewer-read and authenticated-user-rebind paths.
- The poller now retains the reviewer list returned by its single
  `GetMRStatus` call in an internal sync result and publishes it with an
  explicit validity marker. Observed-empty and absent observations are
  distinct, and legacy/manual events still use the strict `GetMR` fallback.
- Evaluation now uses a targeted task-options/exact-checkpoint snapshot and
  the persisted workspace GitLab `Username`; it no longer rebinds through
  `GetAuthenticatedUser` per MR or scans all task checkpoints. The public
  `SyncTaskMR`, `TaskMR`, and automation response contracts remain unchanged.
- Review remediation: evaluation now rebinds the persisted reviewer username
  from workspace configuration and atomically resets review-request baselines
  before reading the exact target checkpoint. A changed-account regression
  covers both persisted options and baseline reset.
- Added event round-trip, poll-carrier, provider-call, fallback, config-identity,
  and exact-checkpoint tests.
- Verification: `cd apps/backend && go test -race ./internal/gitlab ./internal/orchestrator`
  passed.
- Risk retained intentionally: observation-absent legacy/manual events still
  perform one strict MR read, and public state-publish responses continue to
  use their existing full response loader outside the lifecycle snapshot.
