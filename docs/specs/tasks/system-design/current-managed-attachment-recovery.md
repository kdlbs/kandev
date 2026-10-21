---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008
created: 2026-10-21
owners:
  - kandev
---

# Current managed source attachment recovery

## Purpose and boundaries

The task system owns attachment admission, physical workspace inventory and
compensation. Workspace repository identity and shared clone placement remain
with their existing owners. This extends the [attachment design](attach-workspace-sources.md)
and [current-workspace design](current-workspace-sources.md) under the
[bounded recovery decision](../../../decisions/2026-10-21-current-managed-attachment-recovery.md).
It is a partial response to #4426; missing legacy recreation remains excluded.

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001 | Consumers, session selection and failure behavior |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008 | Identity, path admission, clone flow and compensation |

## Current consumers and defect

Registered `POST /api/v1/tasks/:id/workspace-sources` and task-surface MCP
`add_workspace_sources_kandev` enter `Service.AttachWorkspaceSources`, its idle
and authorization checks, batch persistence and `workspaceSourceMaterializer`.
`ensureHostRepositoryPaths` currently skips every nonempty saved path. A missing
current managed source therefore fails later root resolution with no clone call.
Empty-path source attachment already uses `RepositoryHostCloner` successfully.

Task-mode `add_branch_to_task_kandev` enters `Service.AddBranchToTask` and its
legacy `BranchMaterializer`. `prepareMaterializeRequest` skips empty paths
before selecting the live session; saved missing paths reach `worktree.Create`
without cloning. The live service rejects absent materialization and compensates
rather than accepting a deferred live result.

`Executor.EnsureRepositoryClonedForSession` also returns nonempty saved paths
unchecked. Change only this host-attachment seam; ordinary launch/resume uses
`ensureRepoLocalPathForSessionAndState` and keeps its existing policy.

## Identity and destination

Use the persisted repository in the task's workspace, never a request-supplied
filesystem destination. `Cloner.WorkspaceProviderRepositoryPath` is the sole
current-destination derivation. Its inputs are workspace ID, provider, provider
host, provider scope, immutable provider repository ID, owner and name.

With a nonempty scope, require its paired immutable repository ID. The path is
`<expanded clone base>/workspaces/<workspace>/<provider>/_scopes/<sha256(scope)>/<sha256(repository ID)>`.
Otherwise use the existing provider/host/owner/name layout below that workspace;
nondefault host encoding, nested owner validation and default-provider behavior
remain in that method. An unscoped built-in provider's immutable ID alone does
not select the scoped layout. Do not reconstruct these paths in backendapp.

Explicitly provider-managed sources acquire saved-missing-path repair authority.
`source_type=local` always follows exact local ownership even when provider
metadata is populated. A non-provider row with a saved local path retains the
existing explicit/local route; metadata alone does not convert it to managed.
A pathless non-local legacy row retains the existing provider-identity eligibility
for cloning only at the derived current destination. Do not change its source
type or infer a local grant from provider metadata. Missing/ambiguous provider
identity fails through existing validation. No source-type migration is added.

Use the existing `RemoteURL` or provider URL builder for the canonical
credential-free clone locator. Scoped identity remains workspace + provider +
scope + immutable ID; mutable owner/name are routing fields, not substitute
ownership. Missing host-sensitive identity must fail through existing provider
validation rather than infer authority from the active connection.

## Path admission and missing primitive

`repoclone` owns a small operation-specific path-admission primitive, proposed
as `InspectManagedAttachmentPath`; this symbol does not exist on base main.
It classifies the exact registered path as current-absent, current-existing or
existing-legacy and returns inspected filesystem evidence or a refusal. Empty
saved path means inspect the derived current destination, not skip inspection.

Reuse `ExpandedBasePath`, `WorkspaceProviderRepositoryPath`,
`ManagedCloneRelocationPaths` for supported legacy candidates, `CanonicalHTTPSCloneURL`,
origin-verification helpers and classified Git subprocesses. Do not reuse
`ShouldRecloneForWorkspace` as ownership proof: broad containment is insufficient.
The task service's private `resolveExplicitLocalRepositoryPath` supports user
linked worktrees and is not a managed-clone validator.

Admission requires:

1. Compare exact lexical absolute paths using existing platform-aware path rules.
   Saved missing paths must equal the exact current destination. No prefix,
   name-only, alternate-workspace or merely-under-managed-root match suffices.
   Propagate permission, I/O and cancellation errors; only confirmed absence
   allows cloning.
2. The expanded operator-configured clone base is the control-root boundary.
   Keep the exact lexical current/allowed-legacy derivation authoritative; do not
   canonicalize registration or clone placement. Permit aliases in parents above
   that base for compatibility. Inspect the base itself and its descendants with
   `Lstat`; reject links/reparse redirection there, files, occupied non-Git
   directories and changed identities before any clone/fetch/worktree command. Carry inspected
   evidence into revalidation rather than treating a second stat as the original
   proof. Retain observed physical base/object identity in the existing operation
   guard, including the resolved target of an observed outside-base parent alias
   when the base does not yet exist. Refuse alias-target/base replacement during
   authentication before mutation. This is bounded operation evidence, not a
   guarantee against arbitrary external filesystem writers.
3. For an existing managed clone, require a standalone `.git` directory, actual
   top level equal to the admitted checkout and Git directory/common directory
   equal to its `.git`. After the checkout passes path/control inspection, compare
   Git's physical results with `filepath.EvalSymlinks` of that inspected checkout
   to reconcile trusted outside-base parent aliases. Resolution grants no new
   candidate authority. Query each `rev-parse` path separately; remove only Git's
   terminal record newline, preserving embedded and trailing whitespace/newlines.
   Do not split multi-path output or trim legitimate path bytes.
   Reject `.git` file or link, `commondir`, redirected
   `core.worktree`, object alternates and symlinked Git control descendants
   (including nested refs/logs/worktrees/objects). Do not fetch to discover trust.
4. Read origin through the existing sanitized Git command path. Compare the
   complete credential-free repository locator using existing provider identity
   and SSH/HTTPS normalization; host alone is insufficient. Scoped origins
   must match the declared clone URL. Preserve established provider case rules.
5. Validate the clone output against the derived current path and the same
   origin/control-path checks before persistence or worktree creation. Reject
   empty output and a returned path to a foreign directory; do not repair an
   origin or delete occupied content to force acceptance.

Use the cloner's existing `repoMu` ownership when admission precedes a
clone/fetch mutation; inspect/revalidate at that mutation boundary without
recursive acquisition. Reuse existing subprocess environment sanitization.
Do not add a general filesystem capability or lock framework. The specific gap
on base is that `ensureClonedAtPathWithOriginVerification` acquires `repoMu` but
uses `.git` existence and optional origin verification, while the host seam's
prechecks have no operation-specific managed-path admission. Implement only the
small inspection/guard seam needed to combine them. If current helpers cannot
preserve inspected evidence through that operation, return the exact missing
primitive for review before expanding scope. No claim is made of protection
against arbitrary concurrent host filesystem writers beyond existing guards.

## Existing legacy reuse

Legacy candidates come only from `ManagedCloneRelocationPaths`: exact
provider/origin and older owner/name paths for its supported unscoped built-in
providers. Require the saved source to exist and pass the same standalone Git,
canonical path and full origin proof; keep its saved path and do not clone,
fetch, relocate or redirect it merely because a current destination differs.
Scoped/plugin identities cannot borrow an unscoped legacy layout. Enforce this
in attachment admission even though the global relocation path helper can return
built-in legacy candidates for a scoped row; do not alter that helper's contract. No directory
scan, fallback to another registration or legacy inference from a branch name.

A missing exact legacy source returns an actionable diagnostic: re-import the
provider repository to establish a current managed source, then retry attachment.
Do not create it or silently clear/replace its saved path. Existing selected
worktrees continue through `inspectRegisteredLegacyClone` and normal worktree
manager validation; do not weaken `REQ-TASKS-MANAGED-CLONE-RELOCATION-001.6`,
change its proof or use an attachment clone to relocate an existing task slot.

## Authenticated clone and verified persistence

Backendapp retains `RepositoryHostCloner.EnsureRepositoryClonedForSession`.
Executor retains `repositoryGitCredentialRequest` and existing GitHub/GitLab,
Azure basic-auth and scoped-plugin resolution. Every new clone request carries
the exact task, selected session, repository and workspace identities from the
admitted operation. Environment identity is checked by the materializer; it is
not a new credential-request field. Never use empty-ID `EnsureRepositoryCloned`
as a fallback, construct credentials in backendapp, or copy tokens into errors.
An unavailable inspection/authentication lane fails closed.

For a recoverable current-absent path, invoke existing authenticated cloning
without its existing early path-persistence side effect. Reuse/factor the
private clone/auth helper so the host-attachment path can validate output first;
do not change normal launch's best-effort persistence contract. Only then use the
attachment-specific `repoLocalPathUpdater.UpdateVerifiedRepositoryLocalPath`,
which calls the service-owned internal Go entrypoint
`Service.UpdateVerifiedManagedRepositoryPath` with the admitted snapshot/path.
Only this adapter may call it after repoclone/executor admission. It is not a
public editing option, HTTP/MCP request flag or generic validation bypass.

The service checks nonlocal/provider eligibility, complete identity, nonzero
version and a clean absolute existing directory path without canonicalizing
lexical bytes. Explicit-local sources and nonprovider rows with a saved local
path are refused. Eligible pathless nonlocal rows remain supported. Scoped
identity requires its paired immutable ID; built-ins retain host/owner/name
identity. Missing or invalid input fails before use.

Load the current persisted repository and authorize its workspace with existing
repository-manage scope. Match the admitted workspace/source/local-path and full
provider identity, including remote locator, and the exact observed version.
Use existing `UpdateRepositoryIfUnchanged` so a change between read and write is
also refused. Mutate only path/version on that loaded row; preserve all settings,
secret bindings and checkout values rather than copying caller snapshot fields.
Publish `RepositoryUpdated` only after the exact write, through the existing
publisher with its ordinary delivery semantics. Read/validation/cancellation and
write failures are fatal; unavailable version fencing cannot fall back to a
non-exact write. No schema, registry or separate persistence model is needed.

This internal entry preserves the already-admitted lexical path across parent
aliases. Ordinary `Service.UpdateRepository`, explicit-local canonicalization and
launch `RepoUpdater.UpdateRepositoryLocalPath` keep their existing behavior.
Set in-memory `LocalPath` only after successful validation and required
persistence. Unchanged existing current/legacy reuse needs no path rewrite. A
newly recreated current clone still performs the conditional verified-path write
even when its saved path string is unchanged. No opaque capability/token
infrastructure is introduced; trusted caller provenance is documented at the
narrow internal entrypoint.

## Session selection, branch wiring and environments

Batch source attachment retains `workspaceSourcesIdle`, task scope and
`loadMaterializationState`. Select `workspaceSourceCredentialSessionID` from
that task's canonical sessions and verify it still belongs to the admitted
host environment before clone side effects. Preserve the existing all-live-
sessions capability/root checks; one eligible session never authorizes ignoring
an incompatible live sibling. No session means no scoped clone.

Direct branch attachment must load its task/repository context, select the
existing `SelectBranchMaterializationSession`, resolve/validate its exact ready
Worktree environment (including inherited owner), and recheck the carried
`BranchMaterializationTarget` before managed-source recovery. Move the pathless
shortcut after that eligibility decision: a live eligible pathless provider
must clone; genuine prelaunch remains deferred. Keep
`verifyMaterializationTarget` before worktree creation and retain canonical
inventory write/session-binding checks. Revalidate a changed session/environment
before using clone output; a verified shared clone may survive refusal, but no
attachment/worktree success may be published for the stale target.

Wire the same host cloner into `branchMaterializer` and nested batch branches
from backendapp startup, including branches created by
`newWorkspaceSourceMaterializer`. One branch operation must not route through
the batch rebinder or change the running agent CWD. Return the existing exact
worktree/task-root response and materialized event only on completed success.

Source recovery runs only after `isHostWorkspaceExecutor` admission: Local,
legacy `local_pc`, Worktree. Task-Docker/LocalDocker, SSH, Sprites, Kubernetes
and other remote executors retain their environment/lifecycle materialization;
no host inspection or host clone fallback. Unknown executors fail as today.
An eligible branch remains Worktree-only. Creating/unprovisioned source
environments and actual branch prelaunch keep their current deferral contract.

## Failure, retries and resource lifetime

Use `commitWorkspaceSourceBatch`, existing worktree-manager rollback, owned-link
undo, environment inventory compensation and ordinary source error envelopes.
Mixed batches must restore every prior attachment and entry when any new source
fails. No success task event on failed materialization. Do not remove foreign
content or pre-existing clones, task worktrees or user roots.

The successfully verified shared clone and successfully persisted repository
path may remain after a later folder/rescan/worktree/inventory failure. Rollback
owns only this call's new task attachment/worktree/entry, not the shared source.
If verified-path persistence fails, no consumer may use that output and no new
task attachment remains; an already-created clone may remain for retry without
asserting a durable path change. Existing repository-creation cleanup rules
remain authoritative for a repository row created by this request.

Exact source retries reuse the current clone and do not duplicate links,
attachments or inventory. Legacy add-branch retains its existing duplicate-key
behavior; a failed call must be safely retryable without a stranded new row.
Do not change global association replacement or corruption-recovery policy.
A post-clone verification failure does not authorize deleting unverified
content. Task-owned compensation grants no `RemoveAll` authority over a shared
clone. Automatic invalid-clone deletion requires a demonstrated producer and
operation-owned deletion evidence; absent that proof, fail closed and preserve
the path instead of inventing a cleanup journal, registry or recovery policy.

## Evidence, compatibility and release limits

Current-main diagnostic: registered HTTP/MCP source handlers on Local and
Worktree and guarded MCP branch on Worktree, real SQLite/Git/cloner with exact
scope observer. Six missing/empty recovery assertions fail, five wrong-origin
foreign-path rejection assertions fail, 24 controls pass. Existing 14 session/
remote-boundary tests pass. These establish causal RED and controls, not repair
GREEN. Permanent tests were subsequently implemented after the explicit parent
implementation request; final receipts are in the work orders.

Use local transport for deterministic consumer regressions. Add focused provider
credential-request tests to prove exact scoping and denied credentials; local
transport alone does not prove GitHub/GitLab/Azure/plugin authentication. Run
path/reparse cases on native platforms when available; skip only demonstrably
unavailable link/permission capabilities and report that limit. Unit or cross-
compile success is not native Windows/macOS filesystem acceptance.

A server hosted in Docker operates on its own filesystem for host Local/
Worktree tasks. It is distinct from a task-Docker executor. Neither a local
consumer test nor remote executor stub is isolated server-in-Docker acceptance.
No live overlay removal, deployment, release availability or full #4426
resolution follows from this package. No new UI composition, error envelope,
public API arguments, metrics, flags or registry is introduced.

Contributor provenance: yattdev/kandev, ayattara's candidate
`5a4dd66a607e1453a569d46b01e4ffc6aa88ba59` (preceded by `20841b4e7d` and
`d9e2234d13`) was inspected. Its path-inspection helper was selectively adapted with
existing legacy derivation, absent-legacy refusal, platform path comparison
and operation-specific clone-lock checks. The whole series was not ported;
contributor candidate tests were not run or claimed. Its fork approval markers
are not canonical approval. Retain ayattara attribution in later delivery.
This design's absent-legacy refusal intentionally differs.

## Related contracts and delivery

- [Explicit local repository trust](../../../decisions/2026-07-20-explicit-local-repository-trust.md)
- [Provider-origin identity](../../../decisions/2026-07-20-repository-provider-origin-identity.md)
- [Remote selection](../../workspaces/system-design/remote-repository-resolution.md)
- [Managed relocation](managed-clone-relocation.md)
- [Implementation plan](../../../plans/current-managed-attachment-recovery/plan.md)
