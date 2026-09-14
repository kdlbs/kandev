# ADR-2026-08-24-kubernetes-executor-resource-ownership: Kubernetes Executor Resource Ownership

**Status:** accepted
**Date:** 2026-08-24
**Area:** backend, frontend, infra, security

## Context

The Kubernetes executor must combine an administrator-authored Pod template
with Kandev-owned control-plane fields, survive backend and container restarts,
and delete remote resources safely. Kubernetes object names and labels alone
are reusable or forgeable, Pod owner references would delete persistent
workspace storage after an unexpected Pod loss, and template fields can grant
node-level privileges.

## Decision

A lifecycle instance maps to one namespaced Pod and optionally one per-session
managed PVC. `executors_running` remains the authoritative inventory.

Kandev owns and enforces:

- the namespace and generated resource names;
- standard `app.kubernetes.io/*` ownership labels and custom `kandev.ai/*`
  executor/profile/instance/task/session/environment identity labels;
- a fresh, cryptographically unpredictable 32-byte lowercase-hex
  `kandev.ai/create-nonce` annotation on every managed PVC, lifecycle Pod, and
  disposable streaming-probe Pod immediately before its API create request;
- the designated main-container command, args, working directory, agentctl
  port, reserved environment keys, and runtime/auth/workspace mounts;
- Linux OS and declared architecture scheduling constraints;
- `restartPolicy: Always` and default-disabled service-account token automount;
- exact Pod/PVC names and UIDs recorded after API creation.

The administrator owns compatible template policy, including images,
resources, security contexts, scheduling preferences, sidecars, init
containers, image-pull secrets, and workload service accounts. Structurally
compatible privileged/host settings are warned about and left to administrator
trust plus cluster admission policy; fields that collide with Kandev invariants
are rejected.

Managed PVCs do not receive a Pod owner reference. Kandev preserves them on
ordinary stop, backend shutdown, and unexpected Pod loss, then explicitly
deletes them after a terminal cleanup proves the stored UID and full ownership
label set. Existing claims are never deleted. Names alone never authorize
reconnect or deletion, and inventory/identity errors fail closed.

Create-error reconciliation is bound to one exact API request. When a create
result is ambiguous, Kandev may accept an exact-name GET only when the admitted
object preserves that request's create nonce exactly, in addition to passing
the existing owned-field admission validation. A missing or different nonce
fails closed before inventory checkpointing, Pod bootstrap, or deletion. A
successful create response must preserve the nonce under the same validator.

Kandev persists the validated workload launch snapshot (Pod template,
platform, main container, and storage configuration only) with the exact
runtime inventory. Kandev-resolved credentials, resolved profile environment
values, injected files, and scripts are excluded; administrators remain
responsible for literal values in the Pod template. Existing-Pod reconnect
uses only the recorded identity; replacement of a missing Pod uses that launch
snapshot, so a later profile edit cannot mutate a retained session.

Kandev injects agentctl and sensitive runtime material through `pods/exec` into
reserved Pod-scoped volumes, then reaches agentctl through a local
`127.0.0.1` port-forward. It does not put task credentials or kubeconfig
material in the Pod spec and does not expose agentctl through a cluster Service.

## Consequences

- Resume and cleanup can distinguish a retained Kandev session from a
  same-name replacement or foreign workload.
- A same-name object with copied ownership labels cannot be adopted after an
  ambiguous create unless it also carries the unpredictable nonce from that
  exact create request.
- A managed workspace survives Pod replacement, but terminal cleanup must
  delete Pod and PVC explicitly and idempotently.
- The raw-template surface remains powerful and requires administrator-only
  mutation, fixed namespace scope, prominent warnings, and cluster admission
  policy.
- Port-forward and exec streaming are required capabilities; API discovery
  alone is insufficient for connection validation.
- Workload images must contain the documented POSIX bootstrap tools and allow
  Kandev's reserved mounts.
- Profile edits do not alter existing or replacement Pods for retained
  sessions; new sessions receive the new template.

## Alternatives Considered

- **Kubernetes Job or Deployment per session:** rejected because these
  controllers add replacement/ownership semantics that conflict with the
  existing one-instance lifecycle and exact cleanup inventory.
- **Pod name or label-only recovery:** rejected because names can be reused and
  labels can be forged or copied.
- **Adopting a same-name object after owned-label validation alone:** rejected
  because an attacker can pre-create an object with copied stable identity
  labels during an ambiguous API outcome; reconciliation must prove continuity
  with the exact create request.
- **Pod owner reference on managed PVC:** rejected because unexpected Pod
  deletion would garbage-collect resumable workspace data.
- **A Kubernetes Secret per session:** rejected for the first version because
  exec delivery into a memory-backed volume avoids another credential-bearing
  API object and its cleanup lifecycle.
- **Kandev Service/Ingress for agentctl:** rejected because a loopback local
  port-forward keeps the control endpoint out of the cluster network surface.
- **Silently strip privileged template fields:** rejected because it obscures
  administrator intent and cannot replace Kubernetes admission controls.
