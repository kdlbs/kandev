---
id: "04-poller-admission-liveness"
title: "Poller admission liveness"
status: done
wave: 3
plan: "plan.md"
spec: "../../specs/platform/git-subprocess-admission.md"
depends_on:
  - "01-class-aware-admission"
  - "02-interactive-git-fanout"
  - "03-lifecycle-git-classification"
---

# Task 04: Poller Admission Liveness

## Acceptance

- Admission wait does not consume the Git execution timeout.
- Admission cancellation does not increment the tracker failure counter or stop a
  healthy tracker; genuine post-admission failures retain the five-failure rule.
- Tracker setup uses `lifecycle`, repeating polls use `background`, and a queued
  poll progresses within the bounded class rotation after slots release.
- No production package can access the classless Git throttle or run helpers once
  all migrations finish.

## Verification

```bash
# Direct Go commands are required because the backend Makefile has no focused or
# race-test targets.
cd apps/backend && go test ./internal/agentctl/server/process -run 'TestWorkspace.*(Admission|Timeout|Monitor)|TestRunPollingGit' -count=1
cd apps/backend && go test -race ./internal/agentctl/server/process -run 'TestWorkspace.*(Admission|Timeout|Monitor)|TestRunPollingGit' -count=1
cd apps/backend && ! rg -n 'subproc\.Git\(\)' internal --glob '*.go' --glob '!*_test.go'
make -C apps/backend build
```

## Files Likely Touched

- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_poll.go`
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/common/subproc/shared.go`
- Focused `workspace_*_test.go` files and cross-platform helper-process fixture

## Dependencies

Tasks 01 (`01-class-aware-admission`), 02 (`02-interactive-git-fanout`), and 03
(`03-lifecycle-git-classification`).

## Parallelism

Sequential after Wave 2. This completes the agentctl process migration and its
mixed interactive/background tests overlap Task 02's package ownership.

## Inputs

- Spec sections: admission and execution are different phases; interactive load
  cannot disable workspace tracking.
- Plan section: Workspace tracker admission and timeout ownership.
- Existing 10-second polling context, five-consecutive-failure behavior, and the
  completed production call-site migrations from Tasks 02 and 03.

## Output Contract

Report red contention evidence, admission-versus-execution result semantics,
classless-API removal/search evidence, files changed, exact command outcomes,
race-test and cleanup evidence, preserved failure behavior, residual risks, and
synchronized task/plan status.

## Results

- RED: `TestWorkspaceGitAdmissionWaitDoesNotConsumeTimeout` observed the
  queued command inheriting the already-expired 100 ms context and returning
  `context deadline exceeded` before the admission/timeout split.
- GREEN: polling and tracker setup now use explicit `background`/`lifecycle`
  classes, execution timers start only after admission, and admission
  cancellation resets rather than advances the tracker failure counter.
  Classless Git run wrappers were removed after the production call-site audit;
  the class-aware throttle also rejects classless `Acquire` calls.
- `cd apps/backend && go test ./internal/agentctl/server/process -run 'TestWorkspace.*(Admission|Timeout|Monitor)|TestRunPollingGit' -count=1` — 2 passed.
- `cd apps/backend && go test -race ./internal/agentctl/server/process -run 'TestWorkspace.*(Admission|Timeout|Monitor)|TestRunPollingGit' -count=1` — 2 passed.
- `cd apps/backend && ! rg -n 'subproc\.Git\(\)' internal --glob '*.go' --glob '!*_test.go'` — no production matches.
- `make -C apps/backend build` — passed for agentctl, backend, and companion
  binaries (the environment reported only the existing codesign-tool warning).
- Admission helper tests additionally cover timeout freshness and wrapped
  cancellation: `cd apps/backend && go test -race ./internal/common/subproc -run 'TestRunGitOutputAfterAcquire|TestClassAdmission' -count=1` — 8 passed.
