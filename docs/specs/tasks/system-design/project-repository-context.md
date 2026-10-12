---
status: draft
system: tasks
created: 2026-10-07
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

## Current evidence

`NewTaskDialog.handleCreate` sends `project_id` without `repositories`.
`Service.prepareTaskForCreation` inherits parent repositories and then performs
repository preflight. No project source lookup exists there.

A temporary Go reproduction used `setupOfficeTest`, real Office migrations,
a project, and a registered local Git repository containing `README.md`.
`CreateTask` succeeded but `ListTaskRepositories` returned nil.
The implementation plan retains the exact command and failure evidence.

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
| Plugin-owned or unknown URL | Existing preflight only, without forged trust | Refusal test | Creation error if unsupported |

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

Public documentation can describe project-source inheritance during implementation.
The design package alone does not advertise this behavior as shipped.

## Related decisions

- [Explicit local repository trust](../../../decisions/2026-07-20-explicit-local-repository-trust.md)
- [Provider-neutral remote repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)

These existing boundaries are reused. No new ADR is required for this correction.

## Implementation plans

- [Office project repository context](../../../plans/office-project-repository-context/plan.md)
