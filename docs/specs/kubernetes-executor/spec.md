---
status: implemented
created: 2026-08-24
updated: 2026-09-01
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
- Kubernetes is represented by a package-style Pod glyph wherever an executor
  or runtime icon is shown. It does not reuse the generic cloud-computing or
  plain cube glyph.
- A Kubernetes task's executor disclosure shows the exact authorized session
  Pod status rather than falling back to the recorded task-environment
  `ready` value. It exposes the Pod identity, phase/container state, restart
  count, workspace mode, and sanitized failure reason when present.
- The task disclosure provides compact icon-only Refresh and Profile settings
  actions in its summary header. The controls remain keyboard accessible,
  labelled, and at least 44 px on touch layouts without becoming the visual
  focus of the disclosure.
  On fine-pointer desktop it may use the existing compact popover; on a coarse
  pointer it is a named button that opens a Drawer with the same information and
  actions. Kubernetes does not expose the generic Reset environment action,
  because Pod/PVC cleanup remains owned by the session Stop/Resume and terminal
  Archive/Delete lifecycle.
- The Kubernetes task disclosure presents one deliberate Pod summary: a
  Pod-shaped identity header, a human-readable live status, grouped runtime
  facts, and quiet corner controls for Refresh and Profile settings. Technical
  identities use monospace typography; state, dates, labels, and actions do not.
- Refreshing a task disclosure always has visible in-progress feedback. A click
  made while background polling already owns the same request joins that request
  instead of appearing to succeed without waiting or issuing a duplicate read.
- Every Kubernetes profile page begins with the shared executor's connection
  configuration, connection and permissions test, and executor-wide active
  sessions before profile details, workload, workspace, credentials, or scripts.
  Connection fields are edited inline on that profile page instead of through a
  nested connection destination. The page states that connection values are
  shared by every profile under the executor. Testing uses the current unsaved
  connection, workload, and workspace values.
- Kubernetes connection settings and profile settings remain separate persisted
  resources, but one profile page coordinates their dirty state, validation,
  active-session confirmation, save, and reset behavior as one settings
  hierarchy. A partial API failure leaves the successfully saved resource clean
  and the failed resource visibly dirty for retry.
- The settings tree treats a configured Kubernetes executor as a profile group,
  not as a second connection subpage. Task disclosure settings links target the
  exact `executor_profile_id`. Legacy executor-level URLs redirect to a profile
  when one exists and remain only as an orphan-executor recovery surface when no
  profile exists.
- Sidebar, Kanban, and task-page Kubernetes status surfaces converge on the same
  current credential view. Rotating or repairing kubeconfig credentials must not
  leave list/card chrome pinned to a launch-time `Unauthorized` result after the
  current executor configuration can inspect the recorded Pod successfully.
- Sidebar and Kanban executor indicators hydrate their exact task/session status
  as soon as a valid indicator is rendered. The user does not need to hover the
  icon before its healthy, failed, or unavailable tone becomes truthful. Exact
  duplicate scopes share one in-flight read and one recent result; a later
  hover or keyboard focus refreshes that scope without clearing the last known
  facts while the refresh is pending.
- A fine-pointer task-list or Kanban executor indicator uses the same compact,
  structured disclosure language as the task Pull Request indicator: a
  focusable icon trigger, a bounded summary surface, a clear identity header,
  and aligned labelled status rows. It does not fall back to an unstructured
  stack of sentences. Kubernetes rows include state, restart count, workspace
  mode, creation time, last check, and a sanitized failure when present.
- On a coarse pointer, the same compact executor indicator is tappable and
  opens a bottom Drawer containing the same status facts. Its touch target is
  at least 44 px without making dense task rows visually taller, and the Drawer
  owns bounded scrolling plus safe-area padding. Hover is never the only way to
  discover the status.
- The raw PodTemplate YAML field grows and shrinks with its content from a compact
  minimum height. It has no independent vertical scrollbar; the settings page is
  the vertical scroll owner, while unwrapped YAML retains contained horizontal
  scrolling without widening the document.

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
- **GIVEN** an administrator opens a Kubernetes profile from the Executors tree,
  **WHEN** the profile page renders, **THEN** editable shared connection fields,
  cluster diagnostics, and executor-wide active sessions are visible before the
  editable profile sections without navigating to another settings page.
- **GIVEN** a member opens that Kubernetes profile, **WHEN** cluster status
  renders, **THEN** active sessions and the inline read-only connection fields
  remain visible, while the connection test is visible, disabled, and explains
  that administrator access is required.
- **GIVEN** connection and workload values on one Kubernetes profile page are
  both dirty, **WHEN** an administrator saves, **THEN** the page requests one
  active-session confirmation, persists each resource in dependency order, and
  leaves only a failed resource dirty if one API call fails.
- **GIVEN** a configured Kubernetes executor has at least one profile, **WHEN**
  the user selects the executor group or opens its legacy connection URL,
  **THEN** settings resolve to a profile root rather than mounting a separate
  connection editor.
- **GIVEN** a Kubernetes profile has short or long PodTemplate YAML, **WHEN** the
  value loads, is edited, or is reset, **THEN** the field shrinks or grows to the
  content, the page remains the only vertical scroll owner, and long unwrapped
  lines do not create document-level horizontal overflow.
- **GIVEN** a task uses a running Kubernetes session, **WHEN** its executor
  disclosure opens, **THEN** the Pod-shaped Kubernetes icon, exact Pod name,
  running state, restart count, and workspace mode are shown, and compact
  Refresh plus Profile settings icons are available in the summary header
  instead of a generic `ready` badge and empty-resource message.
- **GIVEN** that disclosure belongs to a Kubernetes profile, **WHEN** the user
  selects its settings icon, **THEN** navigation opens the exact profile root
  identified by `executor_profile_id`, not the executor-level connection route.
- **GIVEN** a loaded Kubernetes task disclosure, **WHEN** the user selects
  Refresh, **THEN** the control immediately reports an in-progress state, waits
  for the exact task/session status read, and then publishes the refreshed Pod
  facts without clearing the last known values during the request.
- **GIVEN** background polling has already started that exact status read,
  **WHEN** the user selects Refresh, **THEN** the visible refresh state remains
  active until the shared request settles and no duplicate read is issued.
- **GIVEN** a sidebar or Kanban executor hover previously observed an
  `Unauthorized` status from launch-time credentials, **WHEN** current
  kubeconfig credentials can inspect the exact recorded Pod and the indicator
  mounts or its disclosure opens again, **THEN** the status is re-read, the
  error clears, and the Pod icon uses the current healthy tone.
- **GIVEN** Kanban renders a Kubernetes card with an executor, task, and primary
  session identity, **WHEN** the card enters the document, **THEN** its exact Pod
  status is requested before any hover and the Pod icon updates to the returned
  semantic tone. Another mounted indicator for that exact scope joins the same
  request instead of issuing a duplicate.
- **GIVEN** an eager status read is already in flight, **WHEN** the user hovers
  or keyboard-focuses the executor indicator, **THEN** the disclosure opens with
  visible loading feedback, retains any last known facts, and joins the causal
  read. A later open after settlement performs a fresh exact read.
- **GIVEN** the rendered task has no primary session or lacks the executor
  identity required by its status source, **WHEN** its indicator renders,
  **THEN** no malformed request is sent and the icon plus disclosure report an
  unavailable state rather than `ready`.
- **GIVEN** a fine-pointer user opens a Kubernetes executor indicator in Kanban
  or the task sidebar, **WHEN** the summary appears, **THEN** it uses a bounded
  Pull Request-style identity header and aligned labelled rows for state,
  restarts, workspace, created time, and last check, with the failure reason as
  a distinct sanitized status row when present.
- **GIVEN** the same indicator is rendered for a coarse pointer, **WHEN** the
  user taps its 44 px target, **THEN** a safe-area-aware bottom Drawer presents
  the same facts and can be dismissed without selecting or opening the task row.
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
- Merging executor connection settings and profile settings into one database
  entity or API transaction. They remain separate resources even though one
  profile page composes and coordinates both settings surfaces.
- Profile-scoped active-session filtering. The status section deliberately shows
  every sanitized active session using the shared executor.
- Replacing the raw PodTemplate field with a syntax-highlighting YAML editor.
- Persisting live Pod status in `TaskStatusSummary`, adding a Kanban-specific
  backend batch endpoint, or changing the existing exact Kubernetes session and
  non-Kubernetes `task.session.status` wire contracts.

## Implementation plan

See [the Kubernetes executor implementation plan](../../plans/kubernetes-executor/plan.md),
the [task runtime disclosure repair plan](../../plans/kubernetes-executor-runtime-disclosure/plan.md),
and the [Kubernetes executor UX repair plan](../../plans/kubernetes-executor-ux-polish/plan.md).
The follow-up status and settings hierarchy repair is tracked in the
[Kubernetes executor status and settings repair plan](../../plans/kubernetes-executor-status-settings-repair/plan.md).
The eager task-icon status and disclosure repair is tracked in the
[Kubernetes executor task status preview repair plan](../../plans/kubernetes-executor-task-status-preview-repair/plan.md).
