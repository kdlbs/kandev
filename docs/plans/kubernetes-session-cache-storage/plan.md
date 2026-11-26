---
created: 2026-11-22
status: done
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
legacy_specs: []
---

# Disk-backed Kubernetes session caches

## Overview

Fix [issue #5417](https://github.com/kdlbs/kandev/issues/5417). On a Kubernetes
task pod, npm writes its cache, including what `npx` downloads, into each
session's memory-backed HOME. Point package-manager caches at the task pod's
existing disk-backed runtime volume. HOME, credentials and CLI state stay where
they are now.

## Evidence and root cause

- `kubeexecutor.managedVolumes` (`apps/backend/internal/agent/kubernetes/compose.go`)
  mounts `kandev-auth` at `/run/kandev` as `emptyDir{medium: Memory}`.
- `kubernetesSessionHome` (`executor_kubernetes_session_env.go`) sets each
  task-owned session's HOME to `/run/kandev/sessions/<hash>/home`. Kandev sets
  neither `NPM_CONFIG_CACHE` nor `XDG_CACHE_HOME`, so npm uses `$HOME/.npm` (and
  `$HOME/.npm/_npx` for `npx`) on tmpfs. Every session gets its own copy, and
  the copy survives session stop while the pod is retained.
- Code paths: `taskKubernetesCreateRequest` stores `kubernetesSessionEnvironment`
  in `req.Env`. That environment is what agentctl configuration sends to the
  agent subprocess. It also goes into the session auth file and the bootstrap
  `auth.env`, which the prepare script sources. agentctl starts children from
  `os.Environ()` (`agentctl/server/process/runner.go`), so env vars from the
  Pod template are inherited unless `req.Env` overrides them.
- The issue reports measurements consistent with this: about 816 MiB of tmpfs
  per fresh pod, and 613 MiB reclaimed by deleting one session's npm cache.
  With the reporter's `NPM_CONFIG_CACHE` workaround on a disk volume, tmpfs use
  dropped to 37–38 MiB.
- `kandev-runtime` (`/opt/kandev`) is a disk-backed `emptyDir` that every
  task pod mounts. Pod templates cannot mount over it (`isReservedMountPath`),
  and bootstrap already writes into it. Retained pods have it too, so the fix
  takes effect without changing or recreating any pod.

Regression level: a unit test on the lifecycle session-environment builder
(fresh and attach request paths). No pod or volume change, so no Kind E2E.

## Requirement conformance

Classification 2, missing behavior. Volume medium and cache placement were never
specified. New criterion `AC-EXECUTORS-KUBERNETES-TASK-POD-001.11` in
[the requirement](../../specs/executors/requirements/kubernetes-task-pod.md).
The design is the new "Session cache storage" section of
[the system design](../../specs/executors/system-design/kubernetes-task-pod.md).
No ADR: this does not change the trust boundary
(`2026-09-23-kubernetes-task-trust-boundary`), because the task is already one
trust boundary.

## Technical approach

In `kubernetesSessionEnvironment`, next to the existing `HOME` assignment, set
`NPM_CONFIG_CACHE` and `npm_config_cache` to `/opt/kandev/cache/npm`, and
`XDG_CACHE_HOME` to `/opt/kandev/cache/xdg`. Skip a default for a non-empty
profile value; match npm cache keys case-insensitively, and use the default for
empty values. That one function already feeds agentctl configuration, the
session auth file and bootstrap `auth.env`, so no other call site changes. Pod
template `env` is not parsed: a template-level cache path is overridden by
another disk-backed path, which is harmless.

No pod spec, volume, PodTemplate validation, schema, UI or runtime-flag change.

## Exclusions

- Moving HOME or CLI config off tmpfs. The issue raises this as a broader
  option, but it would put credential files on disk and needs compatibility work
  for every CLI. That should be a separate, deliberate design.
- Deleting session homes or old tmpfs caches on stop. CLI state under HOME is
  needed to resume. Caches already written to tmpfs are reclaimed when the pod
  is recreated or the task is cleaned up.
- Cache variables for other tools (bun, pnpm store, cargo). `XDG_CACHE_HOME`
  covers pip, uv, go-build and other XDG-aware tools. Extend only when there is
  evidence.
- Legacy session-owned pods (single session per pod, recovery-only path).
- A size limit on `kandev-runtime`. Ephemeral-storage policy stays with the
  operator's template and cluster.

## Work orders

- [x] [Task 01: Default session caches to disk](task-01-session-cache-defaults.md)

## Risks

- A cache path set only in the Pod template is overridden. It still lands on
  disk; profile env is the documented override.
- Concurrent sessions sharing one npm cache. npm's cacache is content-addressed
  and safe for concurrent use. Sharing across sessions is within the task trust
  boundary.
- A runtime user that cannot write `/opt/kandev`. That user already must write
  there for bootstrap, so this adds no new requirement.

## Verification results

Design checkpoint: root cause confirmed by code reading against `origin/main`
`c68b3340d72`. Implementation and verification results are recorded in the
completed work order.
