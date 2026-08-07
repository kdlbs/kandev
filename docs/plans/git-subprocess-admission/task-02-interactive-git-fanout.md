---
id: "02-interactive-git-fanout"
title: "Interactive Git fan-out"
status: done
wave: 2
plan: "plan.md"
spec: "../../specs/platform/git-subprocess-admission.md"
depends_on:
  - "01-class-aware-admission"
---

# Task 02: Interactive Git Fan-Out

## Acceptance

- Status, log, and diff share one capacity-derived worker policy.
- Multiple simultaneous handler requests never exceed the process-wide Git cap.
- All user-request agentctl Git commands declare `interactive`, with no classless
  or hardcoded secondary concurrency path.
- Existing per-repository order, response schemas, and partial results remain
  unchanged.

## Verification

```bash
# Direct Go commands are required because the backend Makefile has no focused or
# race-test targets.
cd apps/backend && go test ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(FanOutRepos|Git.*Admission|MultiRepo.*Class)' -count=1
cd apps/backend && go test -race ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(FanOutRepos|Git.*Admission|MultiRepo.*Class)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_fanout_test.go`
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers.go`
- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_commits.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- Focused sibling tests for migrated operators

## Dependencies

Task 01 (`01-class-aware-admission`).

## Parallelism

Parallel-safe with Tasks 03 and 05 after Task 01. This task owns agentctl Git API
and non-poller process files; Task 03 owns backend lifecycle packages and Task 05
owns control/debug files.

## Inputs

- Spec sections: explicit work classes and consistent multi-repository behavior.
- Plan section: Interactive operations and repository fan-out.
- Existing indexed fan-out behavior in `api/git.go` and typed admission API from
  Task 01.

## Output Contract

Report red tests, classification audit, fan-out worker rule, preserved response
semantics, files changed, exact command outcomes, race-test evidence, residual
risks, and synchronized task/plan status.

## Results

- RED: `TestFanOutReposRespectsLimit` failed after deriving its expected bound
  from the configured capacity; the old fixed fan-out admitted five workers with
  capacity three.
- GREEN: status, log, and cumulative-diff fan-out now derive their worker limit
  from `subproc.GitCapacity()` while preserving indexed result order.
- User-triggered Git operator, PR-provider, workspace file, and content-at-ref
  operations use `GitInteractive`; tracker-owned background paths were completed
  by Task 04 with `GitBackground` admission and post-admission timeouts.
- `cd apps/backend && go test ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(FanOutRepos|Git.*Admission|MultiRepo.*Class)' -count=1` — 3 passed.
- `cd apps/backend && go test -race ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(FanOutRepos|Git.*Admission|MultiRepo.*Class)' -count=1` — 3 passed.
