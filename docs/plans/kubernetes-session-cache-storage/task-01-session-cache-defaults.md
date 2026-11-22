---
id: "01-session-cache-defaults"
title: "Default Kubernetes session caches to disk"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.11
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 01: Default Kubernetes session caches to disk

## Summary

Task-owned Kubernetes sessions get npm and XDG cache locations on the
disk-backed `kandev-runtime` volume unless the operator has already set them.

## In scope

- Write the regression test first, in a new file
  `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_cache_test.go`,
  calling `kubernetesSessionEnvironment` directly:
  - `TestKubernetesSessionEnvironmentDefaultsCachesToRuntimeVolume`: a
    task-owned request with no cache env gets `NPM_CONFIG_CACHE` and
    `npm_config_cache` set to `/opt/kandev/cache/npm`, and `XDG_CACHE_HOME` set
    to `/opt/kandev/cache/xdg`. HOME stays the `/run/kandev/...` session home.
    Fails on the base because the keys are absent.
  - `TestKubernetesSessionEnvironmentKeepsProfileCacheLocations`, as a small
    table: profile env with uppercase, lowercase, or mixed-case npm keys;
    `XDG_CACHE_HOME`; and empty npm or XDG values. Non-empty profile values are
    kept, empty values receive the disk defaults, the other npm spelling is not
    added, and the unrelated variable is still defaulted.
- In `executor_kubernetes_session_env.go`, add two path constants and about ten
  lines in `kubernetesSessionEnvironment` after the `HOME` assignment. No new
  call sites; that function already feeds agentctl configuration, the session
  auth file and bootstrap `auth.env`.

## Out of scope

Pod/volume composition, HOME relocation, session-home cleanup, legacy
session-owned pods, extra tool-specific cache variables, and unrelated docs.

## Acceptance

1. The new tests fail on the base for the missing defaults, then pass.
2. Existing Kubernetes lifecycle tests pass unchanged, including per-session
   HOME/auth path, credential isolation and managed Git handoff.
3. Public docs: run `/docs-maintainer`. If `docs/public/**` describes Kubernetes
   pod volumes or a cache workaround, add one sentence on the default cache
   location and how to override it.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run 'KubernetesSession|KubernetesTaskPod' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agent/kubernetes -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Kubernetes' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_env.go apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_cache_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the first command before the fix and keep the expected failure output.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_cache_test.go` (new)

## Results

- RED confirmed: the focused lifecycle command failed on missing npm/XDG cache
  defaults before the production change.
- The focused lifecycle selector passed after the change:
  `go test -trimpath ./internal/agent/runtime/lifecycle -run 'KubernetesSession|KubernetesTaskPod' -count=1`.
- It passed again after adding mixed-case npm and empty npm/XDG cases.
- Lifecycle and Kubernetes packages passed:
  `go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agent/kubernetes -count=1`.
- Kubernetes lifecycle race tests passed:
  `go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Kubernetes' -count=1`.
- The package tests and race tests passed again after the additional cases.
- Changed-Go-file lint reported `0 issues` again.
- The Kubernetes public guide now documents cache defaults and profile overrides.
  `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs` passed.
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- `make -C apps/backend build` passed. It reported the expected absence of
  `codesign`/`rcodesign` and left macOS binaries unsigned.
