---
id: "01-class-aware-admission"
title: "Class-aware admission controller"
status: done
wave: 1
plan: "plan.md"
spec: "../../specs/platform/requirements/git-subprocess-admission.md"
depends_on: []
---

# Task 01: Class-Aware Admission Controller

## Acceptance

- Active work never exceeds the configured capacity across mixed classes.
- A continuously queued class is selected within three successful releases while
  all classes remain queued; FIFO, cancellation, and single-class full throughput
  remain deterministic.
- Aggregate `git` metrics remain compatible, class snapshot values match scheduler
  state, and the generic `gh` throttle is unchanged.
- The class-aware Git throttle rejects classless `Acquire` calls so production
  callers cannot silently default to a scheduling class.

## Verification

```bash
# Direct Go commands are required because the backend Makefile has no focused or
# race-test targets.
cd apps/backend && go test ./internal/common/subproc -run 'Test(Class|GitAdmission|Throttle|Metrics)' -count=1
cd apps/backend && go test -race ./internal/common/subproc -run 'Test(Class|GitAdmission|Throttle|Metrics)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/common/subproc/throttle.go`
- `apps/backend/internal/common/subproc/shared.go`
- `apps/backend/internal/common/subproc/metrics.go`
- `apps/backend/internal/common/subproc/admission.go`
- `apps/backend/internal/common/subproc/admission_test.go`
- Existing sibling throttle, shared, and metrics tests as needed

## Dependencies

None.

## Parallelism

Sequential. Every later task depends on this scheduler and typed API.

## Inputs

- Spec sections: one global hard cap, explicit work classes, fair
  work-conserving admission, and diagnostics.
- Plan section: Shared class-aware admission.
- Existing `Throttle`, environment capacity resolver, and expvar compatibility in
  `internal/common/subproc`.
- ADR: `docs/decisions/2026-08-02-class-aware-git-subprocess-admission.md`.

## Output Contract

Report red tests, scheduler/API shape, metric compatibility, files changed,
exact command outcomes, race-test evidence, residual risks, and synchronized
task/plan status.

## Results

- RED: `TestGitAdmissionPublishesClassMetrics` failed because the class-level
  expvar maps did not exist.
- GREEN: added class-aware FIFO queues, deterministic round-robin admission,
  cancellation removal, idempotent release, capacity test seam, typed Git
  admission, snapshots, and additive per-class metrics.
- `cd apps/backend && go test ./internal/common/subproc -run 'Test(Class|GitAdmission|Throttle|Metrics)' -count=1` — 17 passed.
- `cd apps/backend && go test -race ./internal/common/subproc -run 'Test(Class|GitAdmission|Throttle|Metrics)' -count=1` — 17 passed.
- `cd apps/backend && go test ./internal/common/subproc -count=1` — 30 passed.
- `cd apps/backend && go test ./internal/common/subproc ./internal/worktree ./internal/agentctl/server/process ./internal/repoclone ./internal/agent/runtime/lifecycle -run '^$'` — all affected packages compiled.
- Classless Git run wrappers were removed after the Task 04 production
  call-site audit, and class-aware `Throttle.Acquire` now returns
  `ErrGitWorkClassRequired`; generic `Throttle`/`gh` compatibility behavior
  remains unchanged.
