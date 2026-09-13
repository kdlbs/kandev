---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-K8S-DOCKER-001
  - REQ-EXECUTORS-K8S-DOCKER-002
  - REQ-EXECUTORS-K8S-DOCKER-003
---

# Kubernetes Docker Workloads System Design

## Purpose and boundaries

The executor owns workspace composition, workload snapshots and exact resource
cleanup. The [requirements](../requirements/kubernetes-docker-workloads.md)
introduce explicit companion workspace access plus an opt-in Docker workload
recipe. Operators supply deployment-specific placement, credentials and capacity.
The [implementation plan](../../../plans/kubernetes-docker-workloads/plan.md)
contains only the reusable executor change, recipe and integration evidence.

The existing [resource-ownership decision](../../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md)
continues to govern names, UIDs, nonces, storage identity and cleanup. The new
mount exception must be reconciled with the foundation and public documentation
when implemented. Existing release behavior still rejects the exception until
that change ships. This design contains its rationale; no independent ADR is
needed for a parallel copy of the same contract.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-EXECUTORS-K8S-DOCKER-001 | Workspace grant and admission |
| REQ-EXECUTORS-K8S-DOCKER-002 | Worker and daemon configuration |
| REQ-EXECUTORS-K8S-DOCKER-003 | Lifecycle and evidence |

## Workspace grant and admission

Use existing `pod_template_yaml`; add no profile/API field, migration or UI.
An ordinary container other than `ProfileConfig.MainContainer` may explicitly
reference `kandev-workspace` at exactly `/workspace` once. `readOnly` selects
access mode. Reject subpaths, subpath expressions, mount propagation, recursive
mount options, alternate paths/devices and additional overlapping mounts. The
template still cannot define the volume itself. Main, init and ephemeral
container rules remain unchanged, including all runtime/auth mount prohibitions.

`ValidatePodTemplate` and `validateContainerReservedFields` in
`internal/agent/kubernetes/template.go` need container-role context for this
narrow exception. `ComposePod` preserves the requested sidecar reference and
continues to inject/own the actual workspace volume and the main-container
mount. All three workspace modes use the same grant rule.

`ValidateAdmittedPod` in `internal/agent/kubernetes/admission.go` must carry the
desired Pod's ordinary-container identities into companion mount validation.
Compare grants by container name and full permitted mount shape, not by array
position, volume name alone or the main container's mount. Verify both directions:
every admitted grant must have been requested, and every requested grant must
remain exactly once on the same named ordinary container. Reject added/removed
grants, missing/renamed recipients, duplicate grants, changed access modes and
attempts to move a grant into init or ephemeral containers.

Normalize only documented neutral API defaults before comparison; real API
integration must prove any such normalization. Continue accepting unrelated
labels and compatible additions that carry no reserved access. Do not weaken
existing main-container, control-port, env, volume, nonce or metadata validation.
Existing Kandev-owned volumes retain exact-source validation.

The grant is an administrative data-access decision, not a security barrier
against a privileged companion. A writable companion can alter repository
contents. Runtime/auth mounts are not explicitly shared with it.

## Worker and daemon configuration

Add a separate recipe under `k8s/worker-images/full/`; preserve the existing
minimal/node-pnpm/python recipe and its checks. Derive from an immutable
universal digest and pin all added inputs in the new recipe's `pins.env`.
Resolve actual registry digests during implementation; source examples may use
clearly marked substitutions, but deployable configuration must reject them.

Use `mise.toml`, the Go module and pnpm lockfile for repository-compatible tools.
The inspected universal defaults need golangci-lint v2 instead of v1.62.0 and
Playwright libraries/browser matching the resolved current package. Include
Go, Rust, Node, pnpm, Python/venv, native compilers, Docker CLI, Buildx and Compose.
Baked tools/browser binaries live outside mounted workspace/auth paths.

Set writable npm, pnpm, Python and Go caches under `/workspace`, but create
them after initial repository clone: the default Kubernetes prepare script
correctly refuses a fresh nonempty non-repository workspace. Kandev owns HOME
and injects it as `/run/kandev/home`; do not freeze host-home paths into config.
Agent CLIs continue through Kandev's selected-agent installation path.

The advanced template has ordinary containers `kandev-agent` and `docker-engine`.
The agent uses UID/GID 1000, dropped capabilities, no escalation and default
seccomp. Pod fsGroup is 1000. Put non-root constraints on the main container;
a Pod-wide `runAsNonRoot` would incorrectly conflict with the chosen daemon.
The daemon is an explicitly selected pinned DinD image, privileged and root,
without hostPath, host namespaces or any service-account token.

Use a shared Pod-scoped `emptyDir` mounted at `/run/docker` for the Unix socket;
run the daemon with an explicit socket group compatible with GID 1000. The
client's `DOCKER_HOST` targets that socket. Do not expose a TCP daemon listener,
even through the sidecar's inherited entrypoint defaults. Both containers mount
the managed workspace at `/workspace`; only the agent receives runtime/auth
mounts. Docker data uses its own `emptyDir` at `/var/lib/docker` with an explicit
size budget, separate from the durable checkout.

Docker bridges run inside an already reduced-MTU Pod network. Set their MTU no
higher than the actual Pod interface MTU,
and test HTTPS/large transfers from nested containers. Do not change Flannel or
the cluster API endpoint to fix a nested bridge problem.

An administrator prepare script prefixes a bounded `docker info` wait before
the existing Kubernetes preparation flow. Preserve clone/origin verification,
repository setup, agent installation and Kandev's branch-selection postlude;
do not use a wait-only replacement script. A startup probe or running main
container is not a substitute for this preparation gate.

Nested bind sources must exist in the daemon's mount namespace. `/workspace`
is deliberately shared; arbitrary agent paths such as its private runtime home
are not. Repository tests requiring callbacks, port forwarding, temporary
directories or SSH fixtures must either use supported shared workspace paths
or be reported as incompatible. Do not describe a basic Compose smoke as full
Kandev Docker-executor parity.

## Lifecycle and evidence

`executors_running` remains authoritative. The raw template already persists
in the workload snapshot, so grants need no new persistence schema. Replacement
through `executor_kubernetes_reconnect.go` composes the recorded template with
the recorded PVC. Tests must prove that edited profiles cannot add grants to
old snapshots and that existing identity-based cleanup remains intact.

Ordinary Stop retains the daemon and nested containers because it retains the
Pod. Source tests clean their own disposable Compose project. Terminal Pod
deletion must remove the nested processes; managed-PVC deletion remains Kandev's
existing UID/nonce-checked operation. Docker cache loss on replacement is expected.
Never mount a retained Docker cache across separate sessions.

Provide a bounded smoke script that tests source tools, browser launch, image
build/run, workspace bind mounts and Compose cleanup. Use real filesystem
assertions and emitted results, not method-call counts. Automated Kubernetes
coverage uses the existing Kind fixture and mock agent, then verifies visible
terminal/results and lifecycle through the existing UI. This changes no rendered
UI layout; desktop/mobile settings retain their current shared raw-template flow.

Deployment acceptance additionally uses an operator-selected agent credential,
tests each selected worker, and verifies nested cgroup accounting, cross-session
daemon isolation, results, retained compute and exact final cleanup. Sidecar resource
fields alone are not proof of nested resource enforcement. Refuse acceptance if
nested processes escape the measured Pod budget; record the result before any
design revision. Privileged operation does not promise adversarial isolation.

## Deployment and compatibility

This feature is opt-in through a privileged administrator-authored template,
not a rollout to existing users. Only a separately reviewed namespace policy
may permit the chosen companion privilege. Operators still configure placement,
tolerations, quotas, network access, workload identity and image distribution.
The backend uses a restricted kubeconfig; it does not install cluster RBAC or
copy cluster credentials into task Pods.

The existing service must run a binary containing the workspace-grant change
before it can accept the advanced template. Building the branch alone does not
update the active service. Stage a compatible build and backup, then schedule
the existing service update explicitly around active sessions; no second control
plane is a production fallback. Development/Kind fixtures remain disposable.

Public docs must distinguish experimental evidence from universal runtime support
and remove the conflicting bulk-apply guidance that could deploy another control
plane. The ordinary executor's existing behavior and image presets remain valid.

## Alternatives

Running a daemon in the main container would require changing the agent's
privilege/process model. A shared host socket would merge session trust and
daemon lifecycle. An existing claim duplicated under another mount path would
couple profiles to fixed storage and bypass the intended per-session ownership.
An explicit sidecar grant is the smallest durable change that preserves the
existing managed-workspace model. Rootless BuildKit alone does not run general
Docker/Compose workloads; a different runtime would require separate evidence.

## Delivery

See [implementation work orders](../../../plans/kubernetes-docker-workloads/plan.md).
All named new recipe, smoke and test files are implementation outputs, not
claims that they already exist.
