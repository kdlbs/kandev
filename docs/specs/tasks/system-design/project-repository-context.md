---
status: draft
system: tasks
created: 2026-10-07
updated: 2026-10-29
owners:
  - kandev
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
---

# Project repository context design

## Purpose and boundaries

The task service selects and persists task attachments. Office supplies project
configuration through a narrow read adapter. The existing workspace resolver and
executor pipeline retain repository admission, branch policies, and materialization.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-PROJECT-REPOSITORIES-001` | Selection, resolution, persistence, launch, and failure |
| `AC-TASKS-PROJECT-REPOSITORIES-001.10` through `.13` | Workspace-verified remote admission |

## Creation boundary

`NewTaskDialog.handleCreate` sends `project_id` without `repositories`.
`Service.prepareTaskForCreation` inherits parent repositories and then performs
`prepareProjectRepositorySources` before repository preflight.
All generated remote inputs pass through `preflightRepositoryInputs` before resolution or persistence.
The project stores source strings, so provider inference cannot depend on picker metadata.

## Project read adapter

Add `ProjectRepositorySourceReader` to `internal/task/service`, with a read method
accepting a project ID. Its neutral result contains the owning workspace ID and
the ordered source strings. The task service must not import Office service packages.

Add `officeProjectRepositorySourceAdapter` in `internal/backendapp`.
It reads through `office/repository/sqlite.Repository.GetProject` and decodes
`Project.Repositories` with `office/models.DecodeRepositories`.
Use exact project IDs, not name fallback.

Wire the adapter into the shared task service in `registerRoutes`, alongside
existing Office workspace-policy wiring. REST, WebSocket, MCP, onboarding, and
routine task creators then share one creation boundary.
Cover the production wiring as well as the adapter method.

## Selection

Add a focused project-source preparation helper beside `service_tasks.go`.
Call it after `inheritParentRepositories` and before `preflightRepositorySelections`.
Parent project inheritance and workspace policy preparation continue to run first.

Automatic project selection applies only when all these conditions hold:

- `ProjectID` is nonempty and `ParentID` is empty.
- `Repositories` is nil. An explicit empty slice bypasses automatic selection.
- `WorkspacePath` is empty.
- The effective workspace policy does not select an existing shared workspace.

No project read is needed outside this selection path.
Within it, a missing reader is a configuration error. A project from another
workspace is refused before any source resolution.
An empty decoded source list leaves the request repositoryless.

## Resolution

Convert local paths to `TaskRepositoryInput.LocalPath` and supported remote URLs
to `TaskRepositoryInput.RemoteURL`. Accept supported host/path forms such as
`github.com/owner/repo` without a URL scheme through the existing provider
parser. Do not interpret a Windows drive path as a remote URL.
Do not set `TrustedProviderDescriptor` from project data.

Validate and canonicalize local project entries with the existing explicit-local
repository boundary. Plain folders, missing paths, and invalid Git metadata are
errors. Project entries represent explicitly configured sources, not scan roots.

Pass candidates through normal preflight and repository resolution.
Reuse `resolveRepoInputLocal`, `resolveRepoInputRemote`, and workspace ownership
checks. Do not add provider-specific clone or credential code.
Leave base and checkout branches unset so normal defaults and policies apply.

Deduplicate automatically selected sources by canonical repository identity.
Preserve first occurrence order. Apply this only to generated project selections
so explicit multi-branch inputs retain their current duplicate-slot rules.
Resolve all selected entries. A mixture of valid and invalid entries must fail.

| Source | Identity and behavior | Evidence | Unsupported result |
| --- | --- | --- | --- |
| Registered local Git path | Reuse workspace repository ID | Service regression with real Git fixture | Creation error |
| Unregistered local Git path | Normal validated find-or-create | Service integration test | Creation error |
| Built-in remote URL | Existing provider origin and namespace resolver | Table tests without network access | Existing provider validation error |
| Plugin-owned remote URL | Server discovery through active workspace inspection | Project and plugin-discovery tests | Typed refusal without writes |
| Unknown or ambiguous URL | No unique verified candidate | Refusal tests | Creation error before writes |

## Workspace-verified remote admission

`preflightRepositoryInputs` owns shared admission for generated project URLs, explicit task inputs, replacement, and standalone remote registration.
Keep `parseRemoteRepositoryURL` strict for unknown hosts without a provider hint.
Provider recognition comes from a verified workspace connection or the
[plugin discovery contract](../../plugins/system-design/repository-url-discovery.md).

Apply these routes before any repository resolution:

1. Saved IDs, local inputs, and trusted Host descriptors retain their existing admission paths. A saved repository ID short-circuits URL and provider admission even when the request also carries remote metadata; `resolveRepoInputID` still enforces workspace ownership and refuses a foreign ID.
2. Explicit built-in providers use their current parser and origin verifier. Never reinterpret a failed explicit selection.
3. Explicit plugin providers use `ResolveRepositorySelection` for that manifest owner only.
4. Unhinted public built-in URLs retain their existing provider meaning and verification.
5. Other unhinted URLs use `resolveWorkspaceRemoteProvider` to collect verified candidates before choosing one.

The new helper lives in `internal/task/service/workspace_remote_provider.go`.
For GitHub and GitLab candidates, parse with the existing explicit provider hint and call the workspace-owned origin verifier.
An invalid parse or origin mismatch contributes no candidate.
Preserve genuine unavailable failures for final error selection.
GitLab compares exact scheme, host, and port and requires its configured authentication method.
Only this verifier sets `TrustedRemote`. It reads connection metadata without credentials or provider network calls.

Also call `RepositorySelectionResolver.DiscoverRepositorySelections` for plugin candidates.
Its result separates candidates and ordinary provider failure from fatal operation errors.
An operation error aborts admission before candidate selection. A candidate failure participates in final error selection only without a unique match.
The backend adapter maps every validated plugin descriptor without rebuilding its clone URL.
Validate each result through a discovery-specific wrapper around `trustResolvedRepositorySelection`.
That wrapper supplies the discovered provider from server authority before applying the existing scope and immutable-ID pinning checks.
Do not weaken the explicit-selection provider equality check.
Task branches, checkout selectors, and policies remain caller-owned.

After the candidate pass completes, accept exactly one valid candidate.
Reject two or more as an invalid, ambiguous selection, even if a candidate is built-in.
One completed valid candidate can succeed despite an unrelated candidate's ordinary inspection error.
With no candidate, prefer unavailable over invalid, then not-found. All ordinary no-matches produce invalid selection.
Caller cancellation, a discovery deadline, or changed plugin eligibility invalidates the entire discovery attempt.
Do not accept an incomplete candidate set in those cases.

This replaces GitHub-first precedence only for ambiguous, unhinted custom-host URLs.
Explicit provider selections keep their selected provider.
Carry the verified input back into `inputs[index]` before resolution.
Keep origin proof scoped to that input and preflight call. Do not transfer trust across aliases or later requests.

Explicit GitLab hints require origin verification even when parsing succeeds.
Skip URL verification for saved IDs and local inputs.
Registration consumes the same verified preflight result without its former second standalone origin call.
The lower resolver retains `untrusted GitLab origin` refusal for direct unverified inputs.

All inputs complete preflight before any repository resolution starts.
A rejected source fails the complete list in either source order.
Later resolution errors retain the existing repository-row lifecycle described below.
This adds no transaction, schema, clone path, credential policy, or provider-default lookup.
Plugin discovery can perform the existing bounded provider inspection calls.

Public provider behavior, GitHub Enterprise host identity, and configured GitLab HTTP origins remain supported.
No new built-in custom-host GitLab SSH/SCP parser support is added.
Plugin input syntax stays provider-owned, and returned clone descriptors retain the existing HTTPS contract.
See the [discovery decision](../../../decisions/2026-10-29-provider-owned-repository-url-discovery.md)
and [Enterprise host design](../../integrations/system-design/github-enterprise-hosts.md#workflow-consumers-and-capabilities).

## Persistence and publication

Retain `resolveTaskCreationReferences` and `finalizeCreatedTask` ordering.
Resolve attachment rows before the task insert. Persist them before `task.created`
and assignee admission can schedule the first run.
An event-boundary regression reads persisted attachments when task creation is published.

No schema change or migration is needed. Existing resolver-created workspace
repository rows retain their current lifecycle after later creation errors.
Do not claim task creation is a new cross-table atomic transaction.
External-ID found-task retries still return before project resolution.

## Launch and children

The existing executor reads persisted task attachments.
`Executor.applyRepositoryConfig` supplies repository identity and source path.
`shouldUseWorktree` is true only for the Worktree executor.
This correction therefore supplies source context without changing `local_pc`
into a worktree executor.

First-launch evidence must read a source sentinel through the prepared environment.
It must also verify repository inventory and inherited child context.
No executor rewrite is planned. If attachment alone fails this proof, stop and
update the package with the newly observed cause before widening the fix.

## Failure and security

Project read, decoding, ownership, and source validation errors return before
the task row or creation event. They do not fall back to a scratch launch.
Existing frontend error presentation remains responsible for these errors.
Preserve workspace authorization and credential-free repository URLs.

## Surface and documentation

Office desktop and phone forms keep their existing composition and payload.
Backend source selection supplies parity without a new picker or new copy.
Browser evidence covers persisted attachments on both viewports.

`ProjectRepositoryPicker` already offers literal custom URLs through `shouldShowCustomEntry`.
It does not call `looksLikeSupportedRemoteURL` from the separate workspace-source picker.
Keep both frontend paths unchanged for this correction.
Use the current project editor and New Task form in desktop and phone regressions.
Configure GitLab in the Office workspace, which differs from the base E2E seed workspace.
Browser success proves admission and persistence, not live self-managed checkout or authentication.
Use the real packaged repository fixture to prove provider discovery from a bare source URL.
No browser provider ID or inspected descriptor is submitted by the project form.

Public documentation can describe project-source inheritance during implementation.
The design package alone does not advertise this behavior as shipped.

## Related decisions

- [Explicit local repository trust](../../../decisions/2026-07-20-explicit-local-repository-trust.md)
- [Provider-neutral remote repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)

Provider trust remains unchanged. Bare-URL discovery follows
[provider-owned discovery](../../../decisions/2026-10-29-provider-owned-repository-url-discovery.md).

## Implementation plans

- [Office project repository context](../../../plans/office-project-repository-context/plan.md)
- [Provider-neutral project-source admission](../../../plans/office-project-self-managed-gitlab/plan.md)
