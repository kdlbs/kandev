---
id: "07-environment-layer-executor-dispatch"
title: "Route task-environment status and teardown to the owning executor"
status: done
wave: 4
depends_on:
  - "04-remote-docker-runtime"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-EXECUTORS-REMOTE-DOCKER-001.13
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 07: Route Task-Environment Status and Teardown to the Owning Executor

## Summary

The task-environment layer asks the install-wide Docker daemon about every
container, whatever executor created it. For a `remote_docker` session the
container is on another host, so the inspect always fails, the failure is
converted to "missing", and a live session becomes unresumable. Reset
Environment has the same defect and would leave the remote container running.
Dispatch both operations to the executor that owns the environment.

## Evidence

Observed against a live session (container `326694a5666a`, `Up 21 minutes` on
the remote host, streams connected):

```
ERROR Failed to inspect container
  {"runtime": "docker", "container_id": "326694a5666a…",
   "error": "No such container: 326694a5666a…"}
```

Logged every 2-5 seconds, tagged `runtime: docker` while the environment's
executor type is `remote_docker`.

Root cause: `lifecycle.Manager.GetContainerLiveStatus` and
`lifecycle.Manager.DestroyContainer` both resolve their backend with a
hardcoded `executor.NameDocker`, and the status path converts *any* inspect
error into `Missing: true`. `DestroyContainer` has exactly one caller, Reset
Environment.

## Scope

- Change `taskservice.EnvironmentDestroyer.DestroyContainer` and
  `GetContainerLiveStatus` to take the `*models.TaskEnvironment`, matching the
  interface's existing `PushEnvironmentBranch` shape. The environment already
  carries `ExecutorType` and `ExecutorID`; the container ID alone is not enough
  to identify a daemon.
- Add `internal/agent/runtime/remote_docker_containers.go`: inspect and remove
  a container on a remote executor's own daemon, resolving the target through
  the existing `RemoteDockerTargetFromConfig` and a `Connect` closure, mirroring
  `RemoteDockerBuilder` rather than introducing a second connection shape.
- Dispatch in `environmentDestroyerAdapter` on
  `executor.ExecutorTypeToBackend(env.ExecutorType)`: remote Docker goes to the
  new helper with the executor row's config; every other type keeps today's
  path unchanged.
- Stop converting an unreachable daemon into `missing`. A transport failure is
  reported as such, per AC-EXECUTORS-REMOTE-DOCKER-001.13; only a daemon that
  answers and does not know the container is `missing`.
- Frontend: the environment popover's shell hint is hardcoded
  `docker exec -it <id> sh`, which does not work for a remote container. Render
  the SSH-prefixed form for a remote Docker environment.

## Exclusions

- No change to `RemoteDockerExecutor.StopInstance`. The stop/archive/delete
  path is already remote-aware and correct; this defect is confined to the
  task-environment layer.
- No change to local Docker, Sprites, SSH, or Kubernetes behavior.
- No new recovery of remote sessions across a backend restart.
  `RecoverInstances` returning empty is existing behavior and out of scope.

## Implementation acceptance conditions

1. For an environment whose `ExecutorType` is `remote_docker`, status and
   destroy both resolve the remote executor's daemon; a test asserts the local
   Docker backend is never consulted for that environment, and that a local
   Docker environment still is.
2. A container the remote daemon reports as running is surfaced as running, not
   `missing`; a daemon that cannot be reached surfaces a transport failure
   distinct from `missing`.
3. Reset Environment on a remote Docker environment issues its removal against
   the remote daemon, and reports an error rather than success when that daemon
   is unreachable.

## Verification commands

```
make -C apps/backend test ARGS='-run TestEnvironment ./internal/task/service/...'
make -C apps/backend test ARGS='-run TestRemoteDockerContainers ./internal/agent/runtime/...'
make -C apps/backend lint
cd apps/web && pnpm vitest run components/task/executor-environment-info.test.tsx
```

## Likely files

- `apps/backend/internal/agent/runtime/remote_docker_containers.go` (new)
- `apps/backend/internal/agent/runtime/remote_docker_containers_test.go` (new)
- `apps/backend/internal/task/service/service_task_environments.go`
- `apps/backend/internal/backendapp/worktree.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_environment.go`
- `apps/web/components/task/executor-environment-info.tsx`

## Dependencies and risks

- The signature change touches every `EnvironmentDestroyer` implementation,
  including test fakes. Compile errors are the intended guard: a call site that
  still passes a bare container ID cannot identify a daemon.
- Opening an SSH connection per status poll is wasteful. The poll interval is
  seconds, so reuse or short-lived caching may be needed; measure before adding
  a cache, and do not hold a connection open for an idle environment.
- Distinguishing "daemon unreachable" from "container gone" is the behavioral
  core of this fix. Collapsing them again would reintroduce the defect in a new
  place.

## Results

Implemented. `containerOpsDispatch` (`internal/backendapp/environment_containers.go`)
routes a task environment's status and destroy calls by executor type;
`RemoteDockerContainers` (`internal/agent/runtime/remote_docker_containers.go`)
reaches the executor's own daemon through the same target resolution the build
endpoint uses. `EnvironmentDestroyer` now takes the `*models.TaskEnvironment`,
because a container ID identifies a container only relative to a daemon.
`GetSSHLiveStatus` was extended to remote Docker so the popover has a host,
which the shell hint needs to be correct.

- `go test ./internal/agent/runtime/` — ok (4 container tests)
- `go test ./internal/backendapp/...` — ok (3 dispatch tests, 61.9s)
- `go test ./internal/task/service/...` — ok (242.7s)
- `golangci-lint run` over all three packages — 0 issues
- `pnpm vitest run components/task/executor-environment-info.remote-docker.test.tsx` — 5 passed
- `pnpm run typecheck`, `pnpm --filter @kandev/web lint` — clean

Verified against the container that exhibited the defect. Before the fix the
popover showed "missing" and the backend logged a failed local inspect every
2-5 seconds. After it, with the same container `Up 2 hours` on the remote host:

```
GET /api/v1/tasks/<id>/environment/live
{"container":{"container_id":"326694a5666a…","state":"running",
  "status":"running","started_at":"2026-09-17T17:55:45Z"}}
```

No `missing` flag, `started_at` read from the remote daemon, and zero
`Failed to inspect container` lines since restart.
