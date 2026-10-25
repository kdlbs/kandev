---
id: "01-recover-dead-ssh-runtime"
title: "Recover from a dead SSH runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.9
  - AC-EXECUTORS-SSH-EXECUTOR-001.10
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
---

# Task 01: Recover from a Dead SSH Runtime

## Summary

Make SSH session resume replace a confirmed-dead remote controller while
preserving the task workspace and ACP conversation. Keep indeterminate SSH
probe failures fail-closed.

## In scope

- Classify remote controller liveness probe outcomes.
- Clear only stale session-runtime metadata after confirmed absence.
- Continue through normal SSH controller creation with the preserved resume
  token and task workspace.
- Add focused lifecycle regression tests.

## Out of scope

- Automatic runtime eviction.
- Provider retry-policy changes.
- Other executor implementations or user-interface changes.

## Acceptance

- A completed remote liveness probe that reports the old PID absent permits
  normal fresh controller creation and retains task-directory metadata.
- An SSH session or transport error aborts resume before controller creation.
- Existing live-controller reattachment behavior remains unchanged.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(SSHExecutorResumeRemoteInstance|ProbeRemoteAgentctlLiveness)$' -count=1
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_stop_resume_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations_remote_test.go`
- `docs/specs/executors/requirements/ssh-executor.md`
- `docs/specs/executors/system-design/ssh-executor.md`
- `docs/plans/ssh-dead-runtime-resume/plan.md`
- `docs/plans/ssh-dead-runtime-resume/task-01-recover-dead-ssh-runtime.md`

## Dependencies

None.

## Risks

- Misclassifying a transport failure as a dead process can create a competing
  controller.

## Parallelism

`sequential`

## Inputs

- Issue [#3330](https://github.com/kdlbs/kandev/issues/3330).
- `REQ-EXECUTORS-SSH-EXECUTOR-001` and its SSH recovery criteria.
- The recovery section in the SSH executor system design.
- Existing SSH stop, resume, process-probe, and metadata-clear tests.

## Results

- RED: the dead-runtime regression failed because
  `ResumeRemoteInstance` returned `ssh resume: agentctl pid 4242 not alive on
  remote` instead of yielding to fresh instance creation.
- RED: the malformed-PID regression caught an intermediate implementation that
  treated invalid persisted identity as confirmed process absence.
- GREEN: the SSH resume preflight now treats only a completed remote non-zero
  process probe as confirmed absence. It closes the stale connection, clears
  session-runtime metadata, preserves the task directory and resume intent,
  and returns to normal instance creation.
- SSH connection, session, and malformed-PID failures retain their metadata and
  fail closed.
- `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run
  'Test(SSHExecutorResumeRemoteInstance|ProbeRemoteAgentctlLiveness)$'
  -count=1` passed with 12 tests.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run
  'Test(IsRemoteAgentctlAlive|CreateExecution.*RemoteResumePreflight)$'
  -count=1` passed with 4 tests.
- Both specification lint commands, `git diff --check`, and `gofmt -l` passed.
