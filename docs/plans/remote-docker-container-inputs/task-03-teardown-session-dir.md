---
id: "03-teardown-session-dir"
title: "Remote Docker teardown removes the session directory"
status: done
wave: 3
depends_on:
  - "02-remote-inputs-in-container"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-002
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-002.8
  - AC-EXECUTORS-REMOTE-DOCKER-002.9
  - AC-EXECUTORS-REMOTE-DOCKER-002.10
system_design:
  - ../../specs/executors/system-design/remote-docker-container-inputs.md
---

# Task 03: Remote Docker Teardown Removes the Session Directory

## Summary

Fix the reported defect. `RemoteDockerExecutor.StopInstance` never removes
`<remote-kandev-home>/agent-sessions/<instance-id>/`, so a container provisioned
before task 02 leaves that agent's credential files on the remote host forever,
including after archive and delete.

## In scope

- A best-effort `rm -rf` of exactly
  `<remote-kandev-home>/agent-sessions/<instance-id>/` over the live SSH
  connection in `RemoteDockerExecutor.StopInstance`, run on a terminal stop
  before the session is released.
- The gate is `shouldRunExecutorCleanup(instance.StopReason)`, the same
  predicate `DockerExecutor.StopInstance` uses, plus a non-empty instance ID.
- The path is composed from the runtime's own `remoteKandevHomeDir` constant and
  the instance ID, shell-quoted, never read from stored metadata.
- A removal failure is logged and does not fail the stop, because stop runs
  inside archive and delete.

## Out of scope

- Removing the remote `~/.kandev/bin/agentctl` cache. It is shared with the SSH
  executor's own cache at the same path and is not instance-scoped.
- Any sweep of directories belonging to instances this backend does not know
  about.
- Changing which stop reasons preserve the container.

## Acceptance

- A terminal stop issues exactly one removal, for the composed per-instance path
  and no other, and the stop still reports the container result it reported
  before.
- A resumable stop issues no removal, and the following resume reaches the same
  container and its seeded session directory.
- A removal that fails is logged and the stop still succeeds.

## Verification

Write the failing test first: a terminal stop against a fake SSH connection must
record the per-instance removal. Confirm it fails before the production change.
Then, from `apps/backend`:

```bash
go test ./internal/agent/runtime/lifecycle/... -run 'RemoteDocker.*Stop|Cleanup|SessionDir' -race -count=1
go test ./internal/agent/runtime/lifecycle/... -race -count=1
golangci-lint run ./internal/agent/runtime/lifecycle/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_lifecycle_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_docker_host_files.go`
  (`remoteKandevHomeDir` is the only survivor of task 02 in this file)

## Dependencies

Task 02, which decides what remains of the remote host-file code.

## Risks

- This is a recursive delete on a machine Kandev does not own. The path must be
  composed, not stored, and shell-quoted; a stored or interpolated path is the
  failure mode that turns a cleanup into data loss.
- `StopInstance` already has a branch that returns an error when no live session
  exists. The removal must not change that outcome or mask the container result.
- The teardown path runs inside archive and delete, so it must not add a new way
  for those to fail.

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-002`, acceptance criteria `.8`, `.9`, `.10`.
- `executor_docker.go` lines around the existing `CleanupAgentSessionDir` call,
  which is the symmetry being restored.
- `executor_sprites_lifecycle.go` `shouldRunExecutorCleanup`.
- `executor_ssh_operations.go` `runSSHCommand`, `shellQuote`.

## Results

- `RemoteDockerExecutor.removeLegacySessionDir` removes
  `<remote-home>/.kandev/agent-sessions/<instance-id>/` on a terminal stop,
  gated by `shouldRunExecutorCleanup` and a non-empty instance ID. It runs
  before the existing `ContainerID == ""` early return, so a task whose
  container is already gone still has its credentials removed.
- The path is composed from `remoteKandevHomeDir` and the instance ID, then
  shell-quoted. `TestRemoteDockerRemovalNamesOnlyThePerInstanceDir` pins the
  exact command and proves caller-influenced metadata cannot steer it.
- The removal is bounded at 30s and best-effort: a failure is logged with the
  remote's own stderr and the stop still succeeds, because stop runs inside
  archive and delete.
- `remoteKandevHomeDir` now lives beside its only use, with a comment saying
  why a constant naming the remote home survives a change that stopped writing
  there.

### Corrected during implementation

The first version of the test expected `<home>/agent-sessions/...`. The real
path is `<home>/.kandev/agent-sessions/...`, because `remoteKandevHomeDir` is
`~/.kandev`. The production code was right and the test was wrong; the test now
pins the full expanded path.

### Verified with

- `go test ./internal/agent/runtime/lifecycle/... -race -count=1` — ok (54s, goleak clean)
- `golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/... --timeout=5m` — 0 issues
