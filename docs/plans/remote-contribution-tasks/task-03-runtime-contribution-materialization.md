---
id: "03-runtime-contribution-materialization"
title: "Runtime contribution materialization"
status: completed
wave: 3
depends_on: ["02-mcp-creation-and-persistence"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 03: Runtime Contribution Materialization

## Acceptance

- Launch, resume, reset, host, and remote/container preparation carry one optional typed contribution
  binding from task metadata into worktree and agentctl repository configuration.
- Materialization keeps target `origin`, fetches the source branch through a collision-resistant
  contribution remote, verifies the fetched ref equals the persisted head SHA, and sets the local
  branch's upstream to the exact source branch.
- Unknown/malformed bindings, unsafe refs, stale SHA, missing source projects, and remote-name identity
  conflicts fail before the agent starts; ordinary repository materialization remains unchanged.

## Verification

```bash
cd apps/backend
rtk go test ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/worktree ./internal/agentctl -run 'Test.*(RemoteContribution|ContributionRemote|ContributionCheckout|HeadSHA|RepoSpec)'
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- focused executor tests
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- lifecycle provider/adapter files and tests
- `apps/backend/internal/worktree/types.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- focused worktree contribution tests
- agentctl repository configuration/tracker types and tests

## Dependencies

Task 02's persisted task-repository metadata contract.

## Parallelism

Sequential. It changes shared launch structs and produces the push metadata consumed by Task 04.

## Inputs

- Spec: checkout behavior and **Persistence guarantees**.
- ADR: target `origin`, separate source remote, exact-SHA checkout.
- Existing patterns: `resolveTaskRepoInfo`, `buildRepoSpecs`, `CreateRequest.CheckoutBranch`, PR-ref fetch,
  checked-out-branch suffixing, and workspace repository tracker rescan.

## Risks

- Git worktree remotes and branch config may be shared across tasks; remote names must be deterministic,
  source-specific, and collision-resistant.
- A local suffixed checkout branch still pushes to the unsuffixed provider head branch.
- Do not fall back from a missing/stale source ref to local state or target `origin`.

## Output contract

Report runtime paths covered, files changed, exact temporary-repository test results, blockers/risks,
divergence, and task/plan status updates.

## Completion

Implemented typed binding projection through task repository info, executor lifecycle, worktree
materialization, agentctl configuration, resume/reset paths, and remote/container preparation. Source
refs are fetched and checked against the persisted head SHA, target `origin` is preserved, and the
source branch gets a deterministic collision-safe remote/upstream configuration.

Temporary Git tests passed for exact-head checkout, stale-head rejection, branch collision handling,
source upstream, target-origin preservation, and restart/runtime configuration paths. The complete
affected backend package suite passed with 5,603 tests; no source-ref or remote-name fallback is used
when a contribution binding is invalid or stale.
