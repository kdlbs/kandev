---
created: 2026-09-16
status: planned
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
legacy_specs: []
---

# Implementation Plan: Remote Docker Executor

## Overview

Replace the `remote_docker` stub with a working runtime that provisions task
containers on a Docker daemon reached over SSH. The work composes the existing
Docker container model with the existing SSH transport rather than writing a
third remote runtime.

## Scope

### In scope

- An SSH-dialed Docker client and profile-owned daemon connection.
- Removal of the three backend-host couplings: bind-mount sources, `agentctl`
  delivery, and published-port reachability.
- A test-then-trust profile create flow with distinct probe steps.
- Resume, stop, reset, archive, and delete against a remote daemon.
- A `containers`-project E2E scenario over the existing sshd fixture.
- Public executor documentation replacing the "Not implemented" row.

### Out of scope

- A `tcp://` transport, per `AC-EXECUTORS-REMOTE-DOCKER-001.3`.
- Any change to `local_docker` behavior or the executor enum.
- Docker context files, daemon provisioning, or remote host bootstrap beyond
  the `agentctl` helper.
- Sharing one pooled SSH connection between `ssh` and `remote_docker` profiles.

## Dependency order

```text
wave 1: task-01 (client)  task-02 (host files)  task-03 (endpoints)
wave 2: task-04 (runtime)
wave 3: task-05 (profile UI + test endpoint)
wave 4: task-06 (E2E + docs)
```

Tasks 01, 02, and 03 touch disjoint files and are parallel-safe. Task 04
composes all three and cannot start before they land. Task 05 depends on the
probe surface task 04 exposes. Task 06 validates the whole.

## Work orders

| Task | Title | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | SSH-dialed Docker client | 1 | none |
| 02 | Container host-file provider | 1 | none |
| 03 | Container endpoint resolver | 1 | none |
| 04 | Remote Docker runtime | 2 | 01, 02, 03 |
| 05 | Profile create flow and connection test | 3 | 04 |
| 06 | E2E scenario and public documentation | 4 | 05 |

## Verification

Per work order, plus a final package check:

```bash
# From apps/backend:
rtk make test lint
# From apps/web:
rtk pnpm run typecheck && rtk pnpm --filter @kandev/web lint
KANDEV_E2E_CONTAINERS=1 rtk pnpm run e2e --project=containers --grep "remote docker"
```

## Risks

- The `WithHost` then `WithDialContext` option order is load-bearing;
  `sockets.ConfigureTransport` installs a TCP dialer that silently wins if the
  order is reversed. Task 01 pins this with a test.
- `docker system dial-stdio` requires the SSH user to reach the Docker socket.
  This is the most likely first-run failure and must be a named cause, not a
  generic daemon error.
- The published instance-port range is 100 ports; a forward is created per
  instance port actually used, not eagerly for the range.
- The current container manager always selects the `linux/amd64` `agentctl`
  helper. Remote hosts make this visible; task 02 must select by probed
  platform.
- E2E needs a Docker daemon reachable over the sshd fixture. If nesting a
  daemon in the existing container proves unstable, the scenario may need a
  separate compose fixture; this is the least certain estimate in the plan.

## ASCII UI previews

Profile create, desktop:

```text
┌─ New executor profile ── Docker (remote) ───────────────┐
│ Name       [ build-box                              ]   │
│ Host       [ build-box                              ]   │
│            alias from ~/.ssh/config, or user@host       │
│                                                         │
│ [ Test connection ]                                     │
│                                                         │
│  ✓ SSH reachable            build-box:22 as dev         │
│  ✓ Platform                 linux/arm64                 │
│  ✓ Docker daemon            reachable                   │
│  ✓ API version              1.51                        │
│                                                         │
│ ┌ Host key ───────────────────────────────────────────┐ │
│ │ SHA256:9f2c…8ab1                                    │ │
│ │ [x] Trust this host                                 │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│ ⚠ This profile grants effective root on the remote      │
│   host. The SSH user reaches the Docker socket and      │
│   Dockerfile instructions run with the daemon's         │
│   authority.                                            │
│                                                         │
│ Image tag  [ kandev/multi-agent:latest              ]   │
│ Dockerfile [ FROM …                                 ]   │
│ [ Build image ]                     [ Cancel ] [ Save ] │
└─────────────────────────────────────────────────────────┘
```

Phone, same flow stacked; the probe list and host-key block keep full width and
the action row pins to the bottom:

```text
┌─────────────────────────┐
│ ‹ New profile           │
│ Docker (remote)         │
│                         │
│ Name                    │
│ [ build-box           ] │
│ Host                    │
│ [ build-box           ] │
│                         │
│ [   Test connection   ] │
│                         │
│ ✓ SSH reachable         │
│ ✓ linux/arm64           │
│ ✓ Docker daemon         │
│ ✓ API 1.51              │
│                         │
│ SHA256:9f2c…8ab1        │
│ [x] Trust this host     │
│                         │
│ ⚠ Effective root on the │
│   remote host.          │
│                         │
│ ┌─────────────────────┐ │
│ │       Save          │ │
│ └─────────────────────┘ │
└─────────────────────────┘
```
