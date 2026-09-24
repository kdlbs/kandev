---
status: current
system: tasks
requirements:
  - REQ-TASKS-REMOTE-OPTIONS-001
  - REQ-TASKS-REMOTE-OPTIONS-002
  - REQ-TASKS-REMOTE-OPTIONS-003
---

# Remote repository options design

## Boundaries and grounding

Task-repository attachments own the selected policy. Workspace repository rows
continue to own provider identity and reusable clones. Use existing
[launch repository resolution](launch-repository-resolution.md) and
[remote contribution](remote-contribution-tasks.md) authority; browser settings
cannot replace validated provider identity, contribution head, or branch policy.

Current source has three relevant preparation paths:

- `repoclone.Cloner.clone` in `internal/repoclone/clone.go` uses a five-minute
  execution budget and suppresses `blob:none` when an auth object is supplied.
  `TestAuthenticatedCloneDoesNotLeavePromisorCheckout` deliberately covers this.
- `internal/worktree/manager_lifecycle.go` already supports no-checkout creation
  for some paths. Sparse setup must precede the first checkout, including the
  contribution path, not be applied after a full tree is written.
- `internal/agent/runtime/lifecycle/default_scripts.go` prepares primary remote
  repositories, while `WorkspaceRepositoryMaterialization` and agentctl's
  `materializeRepositoryInternal` prepare sibling repositories. Updating the
  sibling API alone would leave the primary repository unaffected.

These paths are implementation targets, not evidence that options already work.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-REMOTE-OPTIONS-001` | Form and responsive presentation |
| `REQ-TASKS-REMOTE-OPTIONS-002` | Data and persistence; compatibility |
| `REQ-TASKS-REMOTE-OPTIONS-003` | Materialization; credentials; failure and tests |

## Data and persistence

Use a typed `RepositoryCheckoutOptions` contract with `version: 1`,
`download_mode: standard | on_demand`, and `sparse_directories: string[]`.
An empty list means All folders; UI Selected folders with no entries is invalid.
Omitted options mean existing behavior, not a new global clone default.

Expose `checkout_options` on task repository request/response types, including
`service.TaskRepositoryInput` and `pkg/api/v1.TaskRepository`. Internally store
its canonical representation under `TaskRepository.Metadata["checkout_options"]`.
Use typed parse/put helpers alongside the existing typed contribution metadata
helpers. Extend `buildTaskRepositoryMetadata`, all task-repository reconstruction
paths, and DTO mapping. Existing metadata storage needs no new table or column.
Old rows stay absent; round trips preserve unrelated metadata keys.

Validate before side effects: closed enums/version, at most 64 directories,
at most 4096 UTF-8 bytes per path and 16 KiB for the encoded options object.
Canonical paths use `/`; reject absolute paths, empty segments, `.`/`..`, NUL,
newlines, backslashes, and wildcard expressions. Preserve case and spaces.
Deduplicate exact normalized paths. Pass directories using stdin/argument arrays. Built-in shell scripts quote each
argument using the existing shell-quoting helper; never insert raw shell text. Validate existence as directories against the
provider-resolved commit during preparation. Do not follow filesystem symlinks
to validate paths outside the repository.

Resume reads the durable attachment, not browser state or the workspace repo.
Do not expose a post-materialization update endpoint in this package. Existing
generic edits must preserve the policy and reject attempts to change it once an
environment exists. Explicit task-copy flows may copy attachment options only
where their existing contract copies repository configuration; unrelated new
task defaults and repository-set saving must not capture them.

## Form and responsive presentation

Extend `TaskRemoteRepoRow`, `useRemoteReposState`, and remote payload builders in
`task-create-dialog-helpers.ts`. Applied state is keyed by row key. A panel owns
its temporary draft until Apply; repository identity change clears applied and
draft options using `applyRemoteRepoPatch`. Branch changes retain paths and
revalidate them at preparation. Removing a row closes its panel.

`RemoteRepoChip` gets a gear between `RemoteBranchPill` and `RemoveButton`.
Use an independent semantic button, never a nested button in the repo trigger.
Render it only after a repository URL is selected or submitted. A subtle dot and a short summary (for example, "On demand, 2 folders")
identify applied non-default choices. Existing repository and branch controls
remain in their current order. Shared create-dialog consumers receive the same
row behavior, including the new-subtask form; edit/session modes preserve saved
options without offering unsupported changes.

Desktop uses a bounded 320px `@kandev/ui` Popover, shared Select controls,
muted text-xs labels, and 28px control height. Phone uses the
inset Drawer pattern in `components/task/mobile/mobile-picker-sheet.tsx`:
repository title, one scrolling body, and a safe-area-aware fixed action footer.
This is a short temporary settings task, so it does not need a new route.
Use `useResponsiveBreakpoint`; coarse-pointer controls get 44px hit targets.
The phone form shares draft logic with desktop and does not stack a folder
picker on another drawer. Directory entry is a labelled multiline field, one directory per line, with
invalid line numbers reported alongside the field. Focus restoration and Escape use existing modal primitives.
Long names truncate visually but retain complete accessible names. No document
horizontal overflow; the form handles keyboard-reduced dynamic viewport height.

All product text uses localization, with en/pt-pt/zh-cn and generated zh-hk/zh-tw
catalogs. The public how-to is in the
task-creation section of `docs/public/tasks-and-workflows.md`.

## Materialization

Propagate options by attachment identity through launch/resume repository specs,
`worktree.CreateRequest`, primary remote preparation, and sibling materialization
requests in lifecycle, agentctl client, and agentctl server. The runtime validates
again at the boundary. The first repository and every sibling use the same
effective policy; never read only `repositories[0]` for options.

For on-demand mode, use `git clone --filter=blob:none --no-checkout` with existing
tag policy and provider-validated origin. Do not introduce `--depth=1` or silently
fall back to a full transfer when the server refuses filtering. Standard preserves
existing clone behavior, but Selected folders still requires delaying checkout.
Configure cone-mode sparse checkout in the task worktree before its first checkout,
then check out the resolved branch/head. Keep root and ancestor files per Git's
cone semantics. Include only selected submodules in initialization; preserve the
existing git-crypt unlock order and contribution binding behavior.

Shared host caches must not contain a task's sparse pattern or task credentials.
Add a versioned on-demand cache variant beneath the existing workspace/provider
repository namespace, keyed by download mode rather than directory choices.
Leave existing standard cache locations and recorded repository paths intact;
carry the effective variant path on the launch request, not by overwriting the
shared `Repository.LocalPath` for another task. Worktrees use worktree-specific
sparse configuration. Task-owned remote checkouts can apply scope directly.
Never convert a user-managed local checkout.

Record a credential-free preparation marker beside the managed checkout with
repository identity, option version/mode, and completion state. Sparse scope
belongs to the environment/attachment's worktree record, not shared cache state.
Use existing per-path admission/locking and temporary-directory publication.
For existing standard clones with no marker, validate Git state and preserve the
existing reuse behavior; an unmarked directory is not permission to delete it.

## Credentials and capability admission

On-demand mode is enabled only after post-clone lazy fetch works for the selected
provider/executor. Follow the existing
[GitHub managed helper](../../integrations/system-design/github-authentication-02.md)
and [lease reissue](../../platform/requirements/git-credential-lease-reissue.md)
contracts. Each Kandev-owned Git command obtains the current scoped environment;
the agent and tools receive the existing execution-scoped helper. Never persist
the transient clone password in Git config or reuse another task's lease.
Managed credential failure never falls through to the optional personal host bridge.

Introduce one backend capability evaluator over trusted provider identity and
executor preparation support; both submit validation and the UI capability
response consume it. The endpoint is a workspace-authorized POST to
`/api/v1/workspaces/:id/repository-checkout-capabilities`, accepting the existing
remote locator shape plus executor profile ID. It performs no clone and returns
supported modes, sparse support, and stable reason codes. Browser responses are
advisory; submission recomputes eligibility. Resolve identities through the
existing provider inspection seam, not browser-supplied permission claims.

GitHub with Kandev-managed built-in preparation is the required first supported
path. Other providers and custom preparation scripts retain Standard unless
their existing credentials/preparation satisfy the same tests. Sparse options
require task-isolated checkout ownership; executors that directly reuse a shared
working directory cannot advertise it. Switching executor invalidates capability
results but preserves the user's draft, with an error until incompatible choices
are reset. Capability gating is part of this draft release scope, not an assertion
that every current executor is compatible.

## Failure, observability, and tests

Keep finite Git execution budgets and caller cancellation through shared
`subproc` helpers. This package does not extend the five-minute host clone limit.
Distinct error codes cover invalid directory, unsupported mode, unavailable
lazy-fetch credentials, and incomplete checkout. Preserve task policy on failure.
Do not start the agent if required preparation fails. Retry creates or validates
only Kandev-owned preparation state; never reset an existing dirty worktree.

Log mode, preparation stage, and cache reuse without credential-bearing URLs or
unbounded per-file output. UI errors use translated reason codes. Do not claim
speedups without a measured fixture; network/server costs can remain substantial.

Required tests combine a local filter-capable Git server with a large omitted
blob, token expiry/revocation simulation, and real isolated worktrees. Assert
omitted blobs are absent before demand (disable lazy fetch for object inspection),
then readable with the correct credential after clone auth cleanup. Prove two
tasks with different sparse paths stay independent, including retry and resume.
Exercise primary remote preparation and siblings separately. Browser tests prove
row isolation, payload persistence, dismissal, validation, and phone geometry.

No separate ADR is needed: this feature-specific policy and its rationale are
preserved here. Reusing a workspace-global sparse setting was rejected because
it would change other task worktrees. A full directory-browser service is deferred
because entering known directories provides the core outcome without cloning during selection.

## Implementation plan

- [Remote repository options](../../../plans/remote-repository-options/plan.md)

## Implemented preparation coverage

The initial capability evaluator enables GitHub with built-in Worktree and Local
Docker preparation. Remote Docker is not implemented by the runtime; SSH,
Kubernetes, Sprites, other providers, and custom preparation scripts remain
unsupported for non-default options. The task picker displays the backend reason.

Shared caches use `_checkout_modes/<repository-path-hash>/<mode>-v1`, with a
credential-free readiness marker inside `.git`. Task worktrees keep a separate
options marker in their private Git directory. Reuse compares that marker and
preserves uncommitted work. Remote siblings use staged agentctl materialization.
Local Docker prepares its provisional container before publishing readiness.
Folder validation follows target-revision checkout and precedes repository setup
and contribution-destination configuration. Host checkout commands receive only
managed credential environment and scoped credential configuration, separately
from the profile environment passed to setup scripts. Ambiguous duplicate
repository updates without branch identity are rejected rather than losing policy.
Single-attachment branch changes preserve the policy. Exact submodule paths are
valid sparse targets; directory text preserves leading and trailing spaces.
Option-specific host caches retain normal managed clones as the owner of git-crypt
unlock keys through a verified-origin link, and detect filters in committed
attributes before materialization. Clone diagnostics are withheld from task logs
because Git tracing can expose credential-bearing URLs.

Container sink validation runs the generated built-in preparation script in the
existing `kandev-agent:e2e` image. Its local authenticated Git server tests sparse
files, omitted historical blobs, lazy reads after initial-helper disposal, and
revocation. API materialization and launch propagation have separate Go tests;
desktop/mobile Playwright tests cover picker interactions and task persistence.
