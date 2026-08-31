---
status: shipped
created: 2026-08-24
updated: 2026-08-26
owner: Kandev
---

# Kubernetes Executor

## Why

Teams that already operate Kubernetes need Kandev sessions to run inside their
cluster policies, networks, storage, and workload identity boundary. Today they
must use a host-local, Docker, Sprites, or SSH executor instead of scheduling a
first-class Pod with an administrator-reviewed workload template.

## What

- Administrators can create a Kubernetes executor for one API server context
  and one namespace by using either an explicit kubeconfig file on the Kandev
  host or Kandev's in-cluster service-account credentials.
- Administrators can test authentication, API discovery, namespace access,
  required RBAC subresources, and optional Pod/PVC admission before selecting
  the executor for work.
- Administrators can create Kubernetes profiles from one strict
  `core/v1 PodTemplate` YAML document. The profile chooses the main container,
  Linux architecture, and workspace storage policy.
- One task session maps to one Pod. Kandev starts its existing `agentctl`
  control process in the designated container and reaches it through a
  loopback-only Kubernetes port-forward.
- The template can define images, sidecars, init containers, resource and
  security settings, scheduling policy, image-pull secrets, and workload
  service accounts. Kandev retains exclusive control of the fields needed for
  runtime identity, bootstrap, transport, workspace access, recovery, and
  cleanup.
- Profiles support a Kandev-managed per-session PVC, a Pod-scoped `emptyDir`,
  or an existing namespaced PVC. Kandev never deletes an existing PVC.
- Ordinary Stop and backend shutdown preserve the Pod and workspace for
  resume. Archive, delete, and explicit force cleanup remove only resources
  proven to belong to the recorded session.
- Members can discover and use administrator-configured Kubernetes profiles.
  They cannot create, edit, delete, or test Kubernetes executor/profile
  configuration.
- Linux `amd64` and `arm64` are supported. The profile fixes the platform and
  Kandev ensures the Pod is scheduled to a matching node before injecting the
  matching helper binary.
- The settings experience exposes the same capabilities on desktop and mobile,
  with touch-visible actions and no page-level horizontal overflow.
- Kubernetes is represented by a Pod-shaped cube glyph wherever an executor or
  runtime icon is shown. It does not reuse the generic cloud-computing glyph.
- A Kubernetes task's executor disclosure shows the exact authorized session
  Pod status rather than falling back to the recorded task-environment
  `ready` value. It exposes the Pod identity, phase/container state, restart
  count, workspace mode, and sanitized failure reason when present.
- The task disclosure provides explicit Refresh and Executor settings actions.
  On fine-pointer desktop it may use the existing compact popover; on a coarse
  pointer it is a named button that opens a Drawer with the same information and
  actions. Kubernetes does not expose the generic Reset environment action,
  because Pod/PVC cleanup remains owned by the session Stop/Resume and terminal
  Archive/Delete lifecycle.

Decision: [ADR-2026-08-24-kubernetes-executor-resource-ownership](../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md).

## Data model

No new database table is required. Existing entities carry the contract:

### `executors`

- `type`: `"k8s"`.
- `config.auth_mode`: `"kubeconfig" | "in_cluster"`.
- `config.kubeconfig_path`: absolute path, required only for `kubeconfig`.
- `config.kube_context`: optional context name.
- `config.namespace`: required namespace; profile/task metadata cannot
  override it.
- `config.request_timeout_seconds`: bounded positive timeout.

Kubeconfig bytes and resolved credentials are never copied into this JSON.

### `executor_profiles`

- `config.platform`: `"linux/amd64" | "linux/arm64"`.
- `config.main_container`: container name, default `kandev-agent`.
- `config.pod_template_yaml`: one strict `v1/PodTemplate` YAML document,
  limited to 256 KiB.
- `config.workspace.mode`: `"managed_pvc" | "empty_dir" | "existing_claim"`.
- `config.workspace.size`, `storage_class`, and `access_modes`: managed-PVC
  settings.
- `config.workspace.claim_name`: required for `existing_claim`.

### `executors_running.metadata`

The authoritative runtime inventory records namespace, Pod name and UID, main
container, platform, workspace mode, PVC name and UID, whether Kandev created
the PVC, agentctl remote port, sanitized template/config hashes, and the
validated workload profile snapshot used for that launch: Pod template,
platform, main container, and storage configuration only. Kandev does not add
resolved credentials, resolved profile environment values, injected files, or
scripts to the snapshot; administrators remain responsible for literal values
they place in the Pod template itself. The local port-forward port is
process-local and is not durable.

## API surface

Existing executor/profile CRUD routes accept `type: "k8s"` and the configuration
above. Validation errors identify a stable field path.

### `POST /api/v1/kubernetes/test`

Administrator-only. Accepts unsaved executor config and an optional profile
config. Returns `success`, `server_version`, `namespace`, `steps`, `warnings`,
and an optional sanitized error. Each step contains a stable key, success
state, duration, and human-readable detail. Dry-run Pod/PVC admission may be
performed when profile config is supplied; no retained workload is created.

### `GET /api/v1/kubernetes/executors/:id/sessions`

Returns sanitized active session rows derived from `executors_running`, then
verified against exact recorded Pods. It does not enumerate arbitrary
namespace workloads. Rows include task/session IDs, Pod name/phase, container
state/restarts, workspace kind, created time, and sanitized failure reason.
Callers may supply `task_id` and, with it, an optional `session_id` to select
only the authorized task/session rows needed by task chrome. Filtering happens
before any Pod lookup, so refreshing one task disclosure never probes every Pod
using the executor. A session filter without its task identity is invalid.

### `GET /api/v1/kubernetes/executors/:id/session-impact`

Administrator-only. Returns an authoritative `active_session_count` for save
confirmation by counting every Kubernetes `executors_running` row with the
exact executor ID. Unlike the detailed session list, this count is not filtered
through task ownership, inventory completeness, or live Pod status, so an
administrator cannot unknowingly change shared reconnect or workload settings
while another user's retained session is hidden. It returns no task, session,
or cluster-resource identity.

## State machine

1. **Configured:** the executor/profile are persisted but no cluster resource
   exists.
2. **Provisioning storage:** Kandev verifies an existing claim or creates a
   managed PVC.
3. **Scheduling:** Kandev creates the merged Pod and waits for the main
   container to run.
4. **Bootstrapping:** Kandev uploads helper/config/credentials through
   `pods/exec`, signals the managed entrypoint, and establishes port-forward.
5. **Running:** the existing nonce handshake succeeds and agentctl owns the
   agent subprocess.
6. **Preserved:** ordinary stop or backend shutdown closes local forwarding but
   retains Pod/workspace for resume.
7. **Cleaning:** terminal/forced cleanup validates exact identities, deletes
   the Pod, then deletes only a Kandev-created PVC.
8. **Cleaned:** resources are absent and runtime inventory can be removed.

A failed transition cleans only resources created by that attempt. A failed
identity check or inventory lookup stops cleanup without deleting anything.

## Permissions

- Administrators can create, edit, delete, validate, and test Kubernetes
  executors and profiles.
- Members can list executors/profiles, select an enabled profile, start tasks,
  resume their sessions, and view sanitized session status.
- The Kubernetes identity requires namespaced Pod and streaming-subresource
  access. PVC permissions are conditional on workspace mode. Optional Event or
  Log read access improves diagnostics but is not required to launch.
- The workload Pod's service account is separate from Kandev's API identity.
  Token automount defaults off when the template does not explicitly opt in.

## Failure modes

- Missing/invalid Kubernetes configuration fails closed; Kandev never falls
  back to the standalone executor.
- Unknown template fields, multiple YAML documents, reserved-field conflicts,
  platform conflicts, or missing main-container image fail before an API write.
- Kubernetes admission, quota, scheduling, image-pull, PVC-binding, exec, and
  port-forward failures surface the causal step and a sanitized error.
- If an admission webhook mutates a Kandev-owned invariant, Kandev rejects the
  returned Pod and removes that exact newly created object.
- Every managed resource create carries a fresh Kandev-owned 256-bit nonce.
  After an ambiguous create result, Kandev accepts an exact-name lookup only if
  the admitted object preserves that request's nonce and every other owned
  field; otherwise it performs no checkpoint, bootstrap, or deletion.
- A proxy that cannot carry Kubernetes streaming subresources is reported as
  incompatible even when API discovery succeeds.
- Recovery rejects a Pod or PVC whose UID or required ownership labels differ
  from `executors_running`.
- If a Pod disappears while a managed PVC remains, resume can create a
  replacement Pod against the verified PVC. Lost `emptyDir` workspace is
  reported as unrecoverable.
- Cleanup is idempotent for NotFound but fails closed on identity mismatch,
  inventory failure, or ambiguous ownership.

## Persistence guarantees

- Pod/PVC identities, auth token references, and bootstrap nonce references
  survive backend restart through existing `executors_running` metadata and
  runtime secret storage.
- A local port-forward never survives process restart; recovery creates a new
  loopback forward.
- A main-container restart preserves the Pod's runtime/auth `emptyDir` data and
  starts a new agentctl process. That process generates a new token and accepts
  one handshake with the persisted bootstrap nonce.
- Managed PVCs and existing claims preserve workspace data across Pod
  replacement. `emptyDir` preserves data only for the lifetime of its Pod.
- Workload-profile edits (Pod template, platform, main container, and storage)
  affect new sessions only. Running Pods retain their created spec and recorded
  identities; recovery that must replace a missing Pod uses the recorded
  workload launch snapshot rather than the profile's current contents.

## Scenarios

- **GIVEN** valid kubeconfig credentials and namespace RBAC, **WHEN** an
  administrator tests the executor, **THEN** every mandatory permission and
  the API server version are shown without a retained workload.
- **GIVEN** Kandev runs in a Pod with the documented service account, **WHEN**
  an administrator selects in-cluster auth, **THEN** connection testing and
  task launch use that service-account identity without a kubeconfig file.
- **GIVEN** a valid profile with a sidecar, affinity, resource limits, and
  workload service account, **WHEN** a task starts, **THEN** those operator
  fields remain in the created Pod while Kandev-owned bootstrap fields match
  the documented invariants.
- **GIVEN** a profile attempts to replace a reserved command, mount, env key,
  identity label, namespace, or architecture selector, **WHEN** it is saved or
  tested, **THEN** validation identifies the conflicting field before creating
  a Pod.
- **GIVEN** a member can see a configured Kubernetes profile, **WHEN** they
  start a task, **THEN** the task may use the profile, but Kubernetes settings
  remain read-only and mutation APIs return forbidden.
- **GIVEN** a running Kubernetes session, **WHEN** the backend restarts,
  **THEN** recovery verifies the recorded Pod UID/labels, creates a new local
  forward, and reconnects without losing the workspace.
- **GIVEN** a running main container, **WHEN** Kubernetes restarts it, **THEN**
  agentctl starts again from the Pod runtime volume and Kandev re-handshakes
  with a newly generated token.
- **GIVEN** a running session, **WHEN** the user stops and later resumes it,
  **THEN** its Pod and workspace remain available and no terminal cleanup ran.
- **GIVEN** a terminally archived session using a managed PVC, **WHEN** cleanup
  runs, **THEN** Kandev deletes only the exact recorded Pod and PVC after UID
  and ownership-label validation.
- **GIVEN** a session uses an existing claim, **WHEN** the session is deleted,
  **THEN** Kandev deletes the Pod but leaves the claim intact.
- **GIVEN** a same-name object has a different UID or ownership labels,
  **WHEN** recovery or cleanup runs, **THEN** Kandev fails closed and leaves the
  object untouched.
- **GIVEN** a create result is ambiguous and a same-name object copies the
  expected labels and specification but lacks the exact per-request create
  nonce, **WHEN** Kandev reconciles the request, **THEN** it does not adopt,
  bootstrap, checkpoint, or delete that object.
- **GIVEN** retained Kubernetes inventory belongs to several users or contains
  a malformed row, **WHEN** an administrator saves executor or workload
  settings, **THEN** the confirmation count includes every exact executor row
  without exposing another user's session identity.
- **GIVEN** the settings page is opened on a phone, **WHEN** an administrator
  configures auth, template, and storage, **THEN** all required actions remain
  touch-visible, scroll within one vertical owner, and produce no document
  horizontal overflow.
- **GIVEN** a task uses a running Kubernetes session, **WHEN** its executor
  disclosure opens, **THEN** the Pod-shaped Kubernetes icon, exact Pod name,
  running state, restart count, and workspace mode are shown, and Refresh plus
  Executor settings are visible controls instead of a generic `ready` badge and
  empty-resource message.
- **GIVEN** the recorded Kubernetes Pod is pending, failed, missing, or cannot
  be inspected, **WHEN** the task disclosure refreshes, **THEN** it shows the
  sanitized live state or unavailable/error state and never reports `ready`
  solely from the task-environment row.
- **GIVEN** the Kubernetes task is opened with a coarse pointer, **WHEN** the
  user taps the named executor button, **THEN** a Drawer exposes the same Pod
  status and controls as desktop without depending on hover.

## Out of scope

- Kubernetes operators, CRDs, Helm packaging, Deployments, Jobs, multi-Pod
  sessions, autoscaling, and pooled/multi-cluster scheduling.
- Cross-namespace profiles inside one executor.
- Windows Pods and automatic node-architecture discovery.
- Sharing one Pod or one managed PVC among concurrent sessions.
- Live mutation of running Pods after profile edits.
- Raw kubeconfig storage in executor JSON or the current user-scoped secret
  store.
- Exposing agentctl through a Service, Ingress, NodePort, or non-loopback
  listener.
- Direct Pod restart/delete controls in the task disclosure. Existing
  Stop/Resume and terminal Archive/Delete flows remain the lifecycle authority.

## Implementation plan

See [the Kubernetes executor implementation plan](../../plans/kubernetes-executor/plan.md)
and the [task runtime disclosure repair plan](../../plans/kubernetes-executor-runtime-disclosure/plan.md).
