---
id: "03-pr-reconciliation-and-lifecycle"
title: "Reconcile PR targets into live workspaces"
status: done
wave: 3
depends_on: ["02-agentctl-comparison-target"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 03: PR reconciliation, lifecycle refresh, and summaries

Connect authoritative provider associations to the persisted comparison target and every running or
recovering workspace.

## Red tests first

Add failing tests for:

- GitHub association of a fork head to an upstream base updating only the exact attachment;
- same-repository PR association retaining normal origin comparison;
- missing head repository identity, head mismatch, historical branch, and ambiguous siblings as typed
  no-ops that still persist the PR association;
- same-PR base retarget updating the target, unrelated PR sync not replacing it, and detach removing only its
  own binding while close/merge retain it;
- post-persist side effects resetting session base SHAs, publishing task/status events, and pushing the target
  to every live execution even when the request context is cancelled;
- agentctl-ready hydration on full launch, existing workspace, lazy recovery, backend restart, and resume;
- deterministic per-worktree keys for multi-repo and multi-branch tasks;
- one failed execution/repository not blocking siblings; and
- task status summaries setting `comparison_unavailable` and excluding misleading additions/deletions.

## Implementation

- Add a narrow provider-association observer to GitHub service wiring. Invoke it only after provider data is
  fetched and TaskPR persistence succeeds. Pass structured head/base identities and branches, never an
  inferred URL-only target.
- Invoke reconciliation from new association/restore and from sync when the same PR's base identity or branch
  changes. Invoke source-aware removal after detach. Preserve the PR association when reconciliation is a
  no-op or runtime materialization fails.
- Wire the task-service reconciler in `backendapp/services.go` or a small adapter to avoid a GitHub-to-task
  import cycle. Keep provider-neutral candidate types in the task domain.
- Extend the E2E GitHub mock PR payload/route so fixtures can provide head/base repository IDs and clone URLs;
  do not let test-only direct TaskPR insertion bypass the real reconciler in the new regression.
- Add lifecycle `ComparisonTargetProvider` and live pusher equivalents beside base-branch propagation. Hydrate
  from task attachments at agentctl-ready time and carry desired targets in initial instance config so no
  origin-based snapshot is emitted first.
- After a durable target change, reset affected sessions' `base_branch`/`base_commit_sha`, publish task update,
  push to agentctl with a detached bounded context, and let tracker refresh publish Git status.
- Carry comparison availability through lifecycle Git events and the status-summary projector. Aggregate an
  unavailable flag across repositories and omit/suppress numeric totals for the task card until a valid target
  observation arrives.
- Add structured logs for association outcome, attachment identity, target display identity, materialization
  status, and stable error code. Do not log clone URLs or raw credential/provider errors.

## Acceptance

- Opening or linking the matching GitHub PR corrects a running task without restart.
- Restart/resume reconstructs the exact target from metadata.
- Association remains successful when local comparison cannot safely reconcile, but the failure is visible and
  never converted into fork-origin statistics.
- GitLab uses the provider-neutral reconciler only if its current MR payload proves equivalent head/base
  identities; otherwise behavior remains unchanged and no identity is inferred.

## Likely files

- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/e2e_reset.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_comparison_targets.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- corresponding focused tests in each package

## Verification

```bash
cd apps/backend && go test ./internal/github ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/task/service ./internal/task/statussummary
```

## Parallelism

`sequential`. Depends on Tasks 01-02 and changes the status protocol consumed by Task 04.

## Output contract

Record red/green commands, provider callback boundary, exact live fan-out behavior, restart evidence, summary
behavior, and any GitLab limitation. Update task/plan status after focused tests pass.

## Completion record

- Green: GitHub/GitLab/backendapp and lifecycle/orchestrator/executor focused suites, plus the task and
  status-summary suites, pass.
- Provider callbacks run after association persistence and use explicit head/base identities. Retarget and
  source-aware detach reconcile only their owning change; close/merge retain the binding.
- Launch, restart, recovery, resume, and live execution updates hydrate the complete comparison-target map.
  GitLab remains a typed no-op when source identity is incomplete.
- Unavailable comparison state propagates through Git status and task summaries, suppressing misleading totals.
