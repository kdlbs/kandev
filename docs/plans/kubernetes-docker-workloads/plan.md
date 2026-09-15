---
created: 2026-09-13
status: completed
requirements:
  - REQ-EXECUTORS-K8S-DOCKER-001
  - REQ-EXECUTORS-K8S-DOCKER-002
  - REQ-EXECUTORS-K8S-DOCKER-003
system_design:
  - ../../specs/executors/system-design/kubernetes-docker-workloads.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes Docker workloads

**Implemented and accepted in a bounded Linux/amd64 Kind environment.**

## Overview

Allow an administrator-selected ordinary sidecar to mount the same managed
workspace as an agent. Supply an opt-in full worker recipe with a per-Pod
Docker daemon, then verify source builds, Docker/Compose bind mounts and
workspace lifecycle through the existing Kubernetes executor.

The executor already supports kubeconfig authentication, Pod templates,
scheduling/resources, managed PVCs, credential injection and exact cleanup.
It currently rejects the reserved workspace mount in every sidecar. This plan
adds explicit companion access without changing storage ownership or exposing
the private runtime/auth volumes in companion specs.

The [requirements](../../specs/executors/requirements/kubernetes-docker-workloads.md)
and [system design](../../specs/executors/system-design/kubernetes-docker-workloads.md)
define the contract. No operator, Helm release or new control plane is needed.

## Scope

### In scope

- Explicit ordinary companion workspace grants with strict template and
  both-direction admitted-Pod validation.
- Snapshot replacement and identity-based cleanup regressions.
- An opt-in full worker image, explicit per-Pod daemon configuration, finite
  preparation and actual source/browser/container-build smoke.
- Isolated Kubernetes integration tests and accurate public documentation.

### Out of scope

- Installation-specific topology, hostnames, addresses, credentials, rollout
  plans, operating logs or production acceptance records. Keep these outside
  this repository and its PR titles, descriptions, comments and artifacts.
- Live deployment, service migration, new cluster-wide permissions, portable
  storage, automatic capacity management or a Docker sandbox guarantee.
- A new UI, API/profile field or implicit change to existing worker presets.

## Technical approach

In `apps/backend/internal/agent/kubernetes/{template,compose,admission}.go`,
permit one explicit ordinary sidecar mount of `kandev-workspace` at exactly
`/workspace`, with RO/RW access. Kandev retains the volume definition and main
mount. Reject redirected/overlapping grants, subpaths and runtime/auth access;
init/ephemeral rules stay unchanged. Compare desired and admitted grants by
container identity in both directions, preserving only proven neutral defaults.

The launch snapshot already stores the raw template. Replacement must reuse
its grant and recorded claim identity despite later profile edits. Existing
nonce, UID, ownership and existing-claim cleanup contracts remain intact.

Add a separate `k8s/worker-images/full/` recipe with immutable inputs and
repository-compatible language tools, linters and Playwright Chromium. The
non-root main agent connects to a separately configured privileged Docker
sidecar through a Pod-local Unix socket. Both mount the workspace; only the
agent gets Kandev runtime/auth mounts. Daemon data is explicitly disposable.

Provide explicit resources, writable caches and a finite daemon readiness gate
while preserving default clone/setup/agent-install/branch preparation. Run the
resolved preparation through the existing restricted Pod exec channel before
releasing the managed entrypoint, with a bounded sanitized failure diagnostic.
Use Docker's cgroupfs driver and a relative cgroup parent, then require runtime
evidence that nested scopes remain inside the Pod budget. Deployers must choose
their own namespace policy, placement, limits and network MTU. The recipe does
not grant privileges or install anything by itself.

## Tests

All new file/method names below are implementation outputs.

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1, 001.2 | New `workspace_grant_test.go`: `TestWorkspaceGrantTemplateValidation`, `TestWorkspaceGrantComposition`; RO/RW grants, all workspace modes, unchanged templates and rejected reserved access. |
| 001.3 | `TestWorkspaceGrantAdmission`; added/removed/transferred/duplicated/changed grants fail, unrelated additions pass; task 03 proves API defaulting. |
| 001.4 | New lifecycle `executor_kubernetes_workspace_grant_test.go`: `TestKubernetesWorkspaceGrantUsesLaunchSnapshot`; profile edits cannot change replacement grants or storage. |
| 002.1-002.4 | Task 02 pinned image/tool/browser checks, template contract and finite daemon preparation; task 03 actual Unix-only daemon operation. |
| 003.1 | Real source, browser, image build/run and Compose bind assertions with Kandev-visible results in task 03. |
| 003.2, 003.3 | Stop/Resume, snapshot replacement, exact managed cleanup, untouched existing claim and nested compute teardown. |
| 003.4 | Tested runtime/image inventory, independent daemons, nested resource accounting and two-containers/one-Pod inventory in task 03. |

Short IDs in the table use `AC-EXECUTORS-K8S-DOCKER-`.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/agent/kubernetes ./internal/agent/runtime/lifecycle -count=1)
```

## E2E tests

Task 03 adds `apps/web/e2e/tests/kubernetes/kubernetes-docker-workloads.spec.ts`
to the existing `containers` project. Reuse the real Kind/API/Docker fixtures
with a mock agent and no account credentials. Verify visible workspace results,
daemon failure, retained/replaced workspace, independent daemons and cleanup.
Existing UI is exercised without rendered layout changes.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/kubernetes/kubernetes-docker-workloads.spec.ts)
```

Select a bounded build/test host before these implementation-time commands.
Keep test evidence limited to generic fixtures, runtime versions and outcomes.

## Work orders

Run sequentially in the primary session after an implementation request.

- [x] [Task 01: Explicit companion workspace grants](task-01-workspace-grants.md)
- [x] [Task 02: Full worker image and daemon recipe](task-02-full-worker-image.md)
- [x] [Task 03: Real Kubernetes Docker lifecycle tests](task-03-docker-lifecycle.md)

## Verification results

- Required Kubernetes and lifecycle Go packages passed after workspace-grant implementation.
- The pinned Linux/amd64 image built and passed non-root source, browser and
  tool verification with image config digest
  `sha256:101d44047de5238b7846295a2e175c162c79328b42db9a2693d568a9ac59a0f6`.
- Template/readiness, clone/cache/retained-workspace, renderer and immutable-input
  tests passed.
- All four Docker lifecycle cases passed against Kubernetes v1.36.1 in Kind:
  full source/browser/Docker/Compose execution, finite daemon failure,
  Stop/Resume and lost-Pod replacement, and isolated-daemon accounting/cleanup.
- The focused browser run completed in 3.3 minutes. Required Go packages,
  documentation catalog, specification lint and public-doc validation passed.

These results establish the documented recipe on the recorded Linux/amd64,
Kubernetes and Docker versions. They do not establish portable enforcement on
every container runtime or remove the documented privileged-DinD boundary.

## Risks

- Privileged DinD weakens isolation; verify nested cgroup accounting before
  claiming resource enforcement. Kind results do not prove all runtimes.
- Arbitrary agent-only HOME/temp binds and callbacks are not covered by sharing
  `/workspace`; document tested compatibility without claiming complete parity.
- Image input versions, browser OS dependencies and writable paths must be
  verified in the built image, not inferred from installed client commands.
- An operator must upgrade to the verified backend before using the new grant.
  Old binaries reject it; production upgrade/recovery is outside this package.
