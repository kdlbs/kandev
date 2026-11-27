---
created: 2026-09-19
status: implemented
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-002
system_design:
  - ../../specs/executors/system-design/remote-docker-container-inputs.md
legacy_specs: []
---

# Implementation Plan: Remote Docker Container Inputs

## Overview

Stop the remote Docker executor from writing to the remote host's filesystem.
Every container input — the `agentctl` helper, the E2E mock-agent helper, the
per-instance agent session directory, and the credential and portable-config
files seeded into it — is delivered through the Docker Engine API into the
container that consumes it, so it is removed with that container.

The same change closes the reported defect: `RemoteDockerExecutor.StopInstance`
never removes `~/.kandev/agent-sessions/<instance-id>/`, so every task leaves
that agent's credential files on the remote host, including after archive and
delete.

## Scope

### In scope

- `CopyToContainer` on the Docker client wrapper, and a `FileUploader` that
  accumulates a tar for it.
- A seeding seam between `CreateContainer` and `StartContainer`, plus a
  helper-redelivery seam before a preserved container is restarted.
- Rewiring `remoteContainerHostFiles` to deliver rather than mount, and
  retiring `sshHostFileStore` and the remote Docker path's SFTP use.
- Best-effort removal of the pre-existing remote session directory on a
  terminal stop, for containers provisioned before this change.
- Go integration coverage against a real daemon, an E2E assertion that no
  `~/.kandev` tree appears on the fixture host, and the public-docs statement of
  the reduced remote-host prerequisites.

### Out of scope

- Any change to `local_docker`: mount set, session-directory location under the
  backend's Kandev home, and destructive-stop cleanup all stay as they are.
- The SSH executor, which keeps SFTP, `expandRemoteHome`, and its own cached
  `~/.kandev/bin/agentctl`. That is its contract, not shared code.
- Named volumes, a content-addressed helper cache, and any executor-profile
  storage setting. `AC-EXECUTORS-REMOTE-DOCKER-002.12` rules the last one out;
  the design records the first two as open questions.
- The daemon transport, endpoint resolver, profile surface, and executor enum.
- `AC-EXECUTORS-REMOTE-DOCKER-001.15` (local Git sources are still not
  rejected); that belongs to the shared `RequiresCloneURL` gap in
  [kdlbs/kandev#3778](https://github.com/kdlbs/kandev/issues/3778).

## Dependency order

```text
wave 1: task-01 (archive delivery + seeding seam)
wave 2: task-02 (remote inputs move into the container)
wave 3: task-03 (teardown)
wave 4: task-04 (integration, E2E, docs)
```

Nothing here is parallel-safe. Task 02 consumes the seam task 01 adds, task 03
removes the fallback task 02 leaves behind, and task 04 verifies the result end
to end. The chain is short because the change is one vertical slice through one
runtime.

## Work orders

| Task | Title | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | [Archive delivery and the container seeding seam](task-01-archive-delivery-seam.md) | 1 | none |
| 02 | [Remote container inputs move into the container](task-02-remote-inputs-in-container.md) | 2 | 01 |
| 03 | [Remote Docker teardown removes the session directory](task-03-teardown-session-dir.md) | 3 | 02 |
| 04 | [Integration, E2E, and documentation](task-04-integration-e2e-docs.md) | 4 | 03 |

## Risks

- **`PutArchive` into a created container.** The whole design rests on the
  daemon extracting a tar into a container that has been created but not
  started. Task 01's acceptance proves it against a real daemon before task 02
  depends on it; if it does not hold, the fallback is a short-lived helper
  container mounting the same paths, which is a change confined to task 01.
- **Helper transfer cost.** Dropping the remote content-hash cache means one
  helper-sized transfer per container start over the user's link. Accepted, and
  matched to the Sprites executor's existing behavior; the design records the
  cache restoration as an open question.
- **Non-root container users.** Archive entries are written with uid/gid 0,
  which is correct for every shipped image. An image that drops privileges would
  need the entries to follow the image's configured `User`.
- **Touching `ContainerManager` touches local Docker.** The existing mount-set
  characterization test is the guard, and task 01's acceptance requires it to be
  unchanged.
- **Version skew on resume.** A container preserved across a backend upgrade
  must not resume on a stale helper; the redelivery seam in task 01 and its use
  in task 02 exist for that case and are tested for it.

## Verification strategy

Each work order carries its own exact commands. The package as a whole is
verified by:

```bash
# From apps/backend:
go test ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/... -race -count=1
golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/... --timeout=5m
```

plus the daemon-backed integration test in task 01 and task 04, and the
`containers` Playwright scenario in task 04.

## ASCII UI previews

None. This package changes no rendered UI. The only user-visible text it
touches is public documentation, which task 04 owns.
