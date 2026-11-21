---
id: "08-cold-resume-admission"
title: "Run executor admission on workflow cold resume"
status: in_progress
wave: 2
depends_on:
  - "03-interactive-admission"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-004
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-004.1
  - AC-EXECUTORS-OWNERSHIP-004.3
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 08: Run executor admission on workflow cold resume

A workflow move that re-enters a step whose session has no live agent reaches
`attemptColdResume`, which relaunched the session through the plain
`ResumeSession` path. That path skips executor admission, so the relaunch ran
with no identity in context: executor ownership was never rechecked and
actor-scoped secret resolution fell back to unscoped lookups. Route the cold
resume through `ResumeSessionWithOptions` with the new `RequireExecutorActor`
option so admission restores the session's durable executor actor, rechecks
task, executor and profile access for that user, and the resume fails closed
with `ErrExecutorActorUnavailable` when no identity can be established. An
unattributed session is then marked failed instead of launching as nobody.
Auth-disabled installs keep the synthetic identity, and the other
`ResumeSessionWithOptions` callers leave the flag unset and keep their
existing behavior.

## Scope

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_actor_test.go`
- `apps/backend/internal/orchestrator/cold_resume_admission_test.go`
- The associated ownership requirement and design.

## Validation

### Regression coverage

Eight focused scenarios cover collaborator- and owner-attributed relaunch,
the fail-closed unattributed session, the auth-disabled synthetic identity,
private-profile denial, duplicate and concurrent turn ends, cancelled turn
ends, and the active-session control that is not relaunched:

```bash
cd apps/backend
go test -trimpath ./internal/orchestrator -run '^TestColdResumeAdmission_' -count=1
```

An executor-level regression also verifies that `RequireExecutorActor` fails
closed when no admission callback is installed:

```bash
go test -trimpath ./internal/orchestrator/executor -run '^TestResumeSessionWithOptionsRequiresExecutorActorWithoutAdmission$' -count=1
```

The source session and an independent QA run both reported this suite
passing with exit 0. QA additionally passed mutation controls that undo only
the `task_operations.go` re-point or remove only the fail-closed check, and
confirmed the base revision fails the same scenarios. The identity resolver
and agent runtime in those runs were in-process test doubles: no container,
agentctl, or live multi-user install was exercised, and hosted CI has not
run. A pre-existing gap next to this seam, where a CREATED session with a
prepared workspace can relaunch without an actor, is tracked separately in
[#5181](https://github.com/kdlbs/kandev/issues/5181) and is out of scope
here.

The maintainer follow-up ran both focused commands above. Both passed.
