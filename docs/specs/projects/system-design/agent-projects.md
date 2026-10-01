---
status: current
system: projects
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-002
  - REQ-PROJECTS-AGENT-PROJECTS-003
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
created: 2026-09-24
owners:
  - kandev
---

# Agent Projects System Design

## Purpose and boundaries

The Projects service owns project identity, coordinator membership, profile
selection policy, and context lifetime. It composes the task service,
worktree manager, agent runtime, MCP server, and web navigation. It does not
change the meaning of Office `project_id` or ordinary workflows. See the
[identity and context decision](../../../decisions/2026-09-24-agent-project-identity-and-context.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PROJECTS-AGENT-PROJECTS-001` | [Project model and API](#project-model-and-api), [Creation and editing](#creation-and-editing) |
| `REQ-PROJECTS-AGENT-PROJECTS-002` | [Agent and MCP boundaries](#agent-and-mcp-boundaries) |
| `REQ-PROJECTS-AGENT-PROJECTS-003` | [Context and execution layout](#context-and-execution-layout), [File and Changes views](#file-and-changes-views) |
| `REQ-PROJECTS-AGENT-PROJECTS-004` | [Workflow-free task boundary](#workflow-free-task-boundary) |
| `REQ-PROJECTS-AGENT-PROJECTS-005` | [Navigation](#navigation), [Flag and failure behavior](#flag-and-failure-behavior) |

## Project model and API

Add a project domain under `apps/backend/internal/projects/` with workspace
authorization, HTTP handlers, service, and SQLite persistence. An
`agent_projects` row stores UUID, workspace ID, name, ordered selected remote
repository IDs, primary repository ID, coordinator/economy/frontier agent
profile IDs, resolved compatible local worktree executor profile ID, main task
ID, timestamps, and archived state. A workspace may contain many Agent
Projects; the name is display data, never a filesystem identity.
Each worker task also persists its selected tier and the profile ID resolved
at creation; later project profile edits do not retarget that worker.

Add `agent_project_id` to tasks with a foreign key/index. The main task is
identified by `agent_projects.main_task_id`; every worker has that task as
`parent_id`. Reject mismatched membership and parent links at the project
service boundary. Do not set `tasks.project_id` (Office), and do not infer
Agent Project identity from `parent_id` alone. Project DTOs expose IDs,
display fields, profile names/availability, selected repo labels, and a
bounded task-status projection. Use workspace-scoped list/detail/create/edit
routes. Project task creation uses a separate server path, not the generic
task-create form or an Office API. Project updates use a version/revision so
an outdated edit cannot overwrite a newer one.

## Creation and editing

The create service validates current workspace access, remote-backed repo
membership, valid agent profiles, and worktree executor compatibility before
writing. Persist project and workflow-free main task together, then provision
the context directory and coordinator session. If filesystem provisioning
fails, reconcile or roll back the new project rather than exposing a partly
ready row. Repeated create requests use a client request identity to avoid
duplicate coordinators. Opening the project selects/ensures a session but
does not dispatch a turn; the first user message starts it.

The create form has no executor selector. The workspace default stores an
executor ID. The service requires that executor to be active and host-local
worktree, then resolves its first available profile (the task service returns
profiles by name) and stores that profile ID. If the default executor or its
profile is missing or incompatible, creation fails with a link to executor
settings; it never silently chooses another executor. The edit form shows the
stored executor profile as read-only and reports when it is unavailable.
Repairing it requires the user to choose a compatible workspace default in
settings and explicitly apply that default to the project.

Profile edits affect new coordinator sessions and workers created afterward.
An existing coordinator session, whether running or stopped, resumes with its
session-pinned profile; each existing worker retains its tier and pinned
profile for all later sessions. Repository edits are admitted only when project tasks have
no running agent. They update the set used for future worker tasks; they do
not mutate existing worktree attachments. A removed repo remains visible on
older tasks. If a profile or repo was deleted, detail reads continue with an
unavailable marker and launches fail until corrected.

## Context and execution layout

Use UUID-based roots under Kandev's configured data directory, for example:

```text
~/.kandev/agent-projects/<project-id>/context/
  notes.md
  docs/
  internal/
~/.kandev/tasks/<main-task-dir>/
  context -> ~/.kandev/agent-projects/<project-id>/context
  <primary-repo-worktree>/
  <other-repo-worktree>/
~/.kandev/tasks/<worker-task-dir>/
  context -> ~/.kandev/agent-projects/<project-id>/context
  <primary-repo-worktree>/
  <other-repo-worktree>/
```

This preserves `worktree.Config.TaskWorktreePath` and the worktree manager's
ownership marker/cleanup rules. The symlink is a convenience for shell agents;
the canonical path is the project context root. The backend creates links
atomically after checking the task root is owned by the task and the link
resolves to the expected canonical project path. It never follows a
user-replaced link when writing project context. Startup repairs a missing
safe link; a wrong or unsafe link blocks launch and reports an actionable
error. Context files are ordinary durable files and remain after task/session
cleanup. The project Files API compares a content hash before an edit and
uses atomic replacement, so its stale UI/API writes receive a conflict even
after a direct agent edit. Direct shell/editor writes through the symlink
bypass that API and are last-writer-wins; the prompt asks agents to write a
temporary file and rename it, but this does not prevent concurrent edits.
Archive retains context. Delete offers an explicit retain/remove-context
choice. Ordinary task archive and worktree cleanup never delete the canonical
context root. Project task cleanup unlinks the task-local `context` entry
without following it, then lets worktree cleanup remove an empty task root.
Tests must exercise the ownership marker, unexpected-entry/orphan scan, and
cleanup behavior with that extra symlink present.

Both task types start in their primary repository worktree. The server-authored
project instruction includes canonical context path and task-relative
`../context`, identifies all repo checkout paths, and explains the Notes
contract. Repository `AGENTS.md` and skills retain normal discovery from cwd.
The launch adapter must add the canonical context root and every sibling
repository worktree to that agent type's allowed read/write roots, beyond its
cwd. Project eligibility tests use each configured agent family and its
managed launch arguments/configuration, rather than trusting user-entered
`AgentProfile.CLIFlags`. If an agent type cannot grant those roots, project
creation/edit rejects that profile and a launch recheck fails before agent
startup. This applies to coordinator, economy, and frontier profiles.
For the initial release, only a host-local worktree executor is admitted.
Docker, SSH, Kubernetes, Sprites, or another host cannot use a host symlink as
shared context; a later design must add a versioned context sync/mount contract.

## Agent and MCP boundaries

At each coordinator launch, resolve the stored coordinator profile and add a
server-owned Projects instruction to the ordinary profile instructions. Keep
user/repository/context content lower trust than that instruction. The
instruction describes coordinator behavior, task vs native subagent roles,
`notes.md` status updates, and the context path. Do not claim that agent
instructions alone enforce delegation or profile policy.

Extend `internal/mcp/profile.Context` with a `project-coordinator` surface and
small, explicit tool catalog: read project, list/get workers, create worker,
message/stop worker, and ask the user. Project Notes in context replace task
plan MCP tools for the coordinator. Add a separate `project-worker` surface
with only task-local read/status and parent-question tools. Both project
surfaces exclude generic `create_task_kandev`, `spawn_session_kandev`,
`move_task_kandev`, workflow-step tools, and `step_complete_kandev`; no project
task has a workflow step to complete. Keep the existing Kanban and Office
catalogs unchanged. The MCP
principal binds project ID, main task ID, session ID, and workspace server-side.
`create_project_worker` accepts a tier enum (`economy` or `frontier`), title,
prompt, and optional selected project repository IDs and per-repository remote
base branches. Omitted repository IDs mean all currently selected project
repositories. Omitted bases mean each repository's configured default branch.
An explicit base must be an available remote ref in the selected project repo;
to build on another worker's work, that worker first publishes its branch.
The service validates the ref before creating a worktree and derives a fresh
task branch from the existing branch-naming policy. The service resolves
the configured agent profile and compatible executor itself, creates a direct
child with `new_workspace`, and checks membership and the flag again before
dispatch. It rejects arbitrary profile IDs, parent IDs, workspace IDs, and
repo URLs even if supplied through a raw MCP request. No project coordinator
capability is granted merely because a normal task has a child.

Limit a project to eight worker sessions in `STARTING` or `RUNNING`. The
service counts live or reserved starts atomically with create-and-start
admission and returns a typed capacity error with no task or agent side
effect when full. Stopped and completed workers remain visible but do not
consume capacity. `list/get worker` returns task and
session state, latest user-visible agent result or error, repository/branch
identity, linked PR/MR summary, and update time; it never exposes private
runtime payloads from another workspace. A worker result does not wake the
coordinator automatically in this release.

## Workflow-free task boundary

Extend task creation with a typed Agent Project origin/membership admitted
only by the project service. `workflow_id` and `workflow_step_id` remain empty.
Narrowly relax `Service.validateCreateTaskRequest` for validated project
requests; do not reuse the Office `isOfficeRequest` shortcut. Generic
task creation with a project task as parent fails at the task service layer,
even when called through a raw HTTP/WS/MCP route; only the project service's
typed worker path can create project children. Audit all task
create, launch, session ensure, update, reparent, board query, status, and
archive paths that currently assume a workflow for persistent tasks. Reject
generic workflow move/reparent operations on project tasks. Preserve session
conversation, stop, archive, and delete without workflow transitions.

Archiving/deleting a coordinator is a project-level action. The desktop/phone
confirmation names the project and count of direct workers. Archive applies
to coordinator and all workers, retains context, and removes the active
sidebar row; an Archived Projects view can restore the project and its tasks.
Delete applies to coordinator and all workers and offers a
separate retain/remove-context choice. Both reject while any project task is
running and report a partial failure with remaining resources; retries are
idempotent. The generic task archive/delete handler rejects the coordinator
so it cannot bypass this contract. Ordinary task deletion cannot orphan its project.
Individual workers can be archived/deleted without touching the coordinator
or shared context. Existing `tasks.project_id` and `IsFromOffice` keep Office
semantics; project costs, if surfaced later, require a separate attribution
dimension.

## File and Changes views

The main Files panel has a project-aware virtual root with `Context` and
`Workspace` nodes. `Context` maps to a project-authorized context service;
`Workspace` maps to the task's existing agentctl worktree file API. The child
Files panel keeps the existing repository-root view and adds a clear Context
entry. The backend canonicalizes paths and checks containment within the
selected backing root for reads, writes, uploads, rename, and delete; symlinks
cannot escape either root or cross into another project's context. The
frontend cannot choose an arbitrary host path. Existing Git/Changes feeds
remain keyed to task environment and repository. They never scan the project
context directory or its task-local link. Multi-repo Changes retains its
current per-repository selection behavior.

## Navigation

Extend the existing `ProjectsSection` in
`components/app-sidebar/sections/projects-section.tsx` so the sidebar has one
section labeled Projects. In ordinary workspace navigation, it lists Agent
Projects; in Office navigation, it retains the existing Office project entries.
Replace the current `useInOffice` early return and `selectOfficeProjects`-only
read with a mode-specific projection. Agent Project list/detail and child
status load from the project API/WS projection into a dedicated frontend
state slice; they do not depend on ordinary board hydration.
Never render two Projects sections at once. An Agent Project has one row named
for the project that opens its coordinator task. A separate chevron expands
direct worker rows when workers exist; no coordinator child row or empty child
placeholder is rendered. Child rows open their own task view with project
context. Exclude project tasks from ordinary task sidebar/board lists to avoid
duplicate rows. The section has a create action, status, and project edit menu.
Desktop uses a dialog for create/edit. Phone uses
a full-height drawer or route using the existing mobile navigation sheet and
repository picker patterns; the body scrolls, the footer stays visible above
the safe area, and every action has a 44px touch target. The phone task view
provides Files/Context/Changes through existing focused panels. Hiding the
workflow control is based on authoritative task membership, not a route-only
check. Localize all new copy in every required catalog.

## Flag and failure behavior

Register `features.agentProjects` / `KANDEV_FEATURES_AGENT_PROJECTS` in the
typed runtime flag registry, config, `profiles.yaml`, and frontend default.
All shipped profile defaults are false. Mark this flag restart-required because
MCP tool registration and project service wiring are composed at startup.
The backend gates project HTTP/WS/MCP
routes, project task creation and launch, and background reconciliation before
side effects. The frontend hides Agent Project rows, create controls, and
routes when disabled; the existing Office Projects view remains available. A disabled
instance retains stored projects without creating worker tasks or runs. Flag
changes that require runtime reconstruction use the registry restart metadata.

Emit structured project create/edit, context repair failure, worker admission,
and launch failure logs with project/task IDs. Keep user-visible errors typed
for invalid dependency, unsupported executor, stale edit, missing context,
and disabled feature. No secrets or context file bodies enter logs.

## Verification strategy

Backend tests cover fresh/migrated schema, Office-project isolation,
authorization, flag-off side-effect prevention, project create/edit recovery,
profile tier admission, workflow-free lifecycle, context containment and
symlink replacement, and Git scope. Frontend unit tests cover projection,
missing dependencies, sidebar membership, and workflow control suppression.
Desktop and `mobile-chrome` Playwright flows create a project, start a
coordinator, create/open a worker, edit context, and verify file/Changes roots.
Use a disposable remote-backed repository fixture and host-local worktree
executor. Validate disabled route/API behavior separately.

## Related decisions

- [Separate Agent Projects and context ownership](../../../decisions/2026-09-24-agent-project-identity-and-context.md)
