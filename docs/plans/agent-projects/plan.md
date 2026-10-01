---
created: 2026-09-24
status: complete
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-002
  - REQ-PROJECTS-AGENT-PROJECTS-003
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
system_design:
  - ../../specs/projects/system-design/agent-projects.md
legacy_specs: []
---

# Implementation Plan: Agent Projects

## Overview

Add a gated Agent Projects domain, then wire context and coordinator tools,
then expose the desktop/phone experience. Task identity and flag admission
come first so no UI or MCP route can create a partially classified project
task. All five work orders are implemented and verified.

## Scope

### In scope

- Named projects with selected remote repositories and three profile choices.
- A workflow-free coordinator task and direct worker tasks.
- Host-local shared context, project-specific tools, desktop/phone navigation,
  and separate Files/Changes roots.

### Out of scope

- Cross-host execution/context sync, schedules/subscriptions, local source
  repositories, automatic preference learning, existing-task conversion.

## Technical approach

1. Add `features.agentProjects` across typed config, registry, profiles, and
   frontend. Add `agent_projects` and `tasks.agent_project_id` without touching
   Office `tasks.project_id`. Create a service-owned workflow-free task path.
2. Add stable context storage and task-local safe links. Keep
   `worktree.Config.TaskWorktreePath` direct-child behavior; start in the primary
   repo worktree. Grant context and sibling repo worktrees as writable roots
   through each supported agent adapter. Add a project-authorized virtual
   Files root; leave Git status bound to task worktrees.
3. Add a `project-coordinator` MCP surface and server-validated worker creation
   with a tier enum and eight-active-worker cap. Exclude generic task/workflow
   tools from project MCP surfaces. Inject project instructions at launch.
4. Extend the one Projects sidebar section with Agent Project rows and direct
   worker children, create/edit surfaces, and project-aware task chrome. Keep
   its existing Office entries in Office navigation.
5. Cover enabled/disabled APIs, task lifecycle, shared context, desktop and
   phone flows with focused tests, including project archive/delete.

## ASCII UI preview

Structural requirements: one Projects section, one project-name row that opens
its coordinator, direct worker children only when present, three profile
fields, primary repo selection, and context/workspace file separation. Labels
and spacing are illustrative. New
copy must use localization catalogs.

### UI-01: Desktop sidebar, expanded project

Maps to `AC-PROJECTS-AGENT-PROJECTS-005.1` and
`AC-PROJECTS-AGENT-PROJECTS-004.1`.

```text
Projects                               [+]
v  Release migration                active  ...
   |- Update API adapters           done
   `- Verify mobile flows          running

   New project                       idle  ...

Tasks
   Ordinary workflow tasks...
```

The project-name row opens the coordinator conversation; its separate chevron
expands worker rows. A project without workers has no chevron or child row.
Child rows open their own task views. Status updates do not reorder the tree
unexpectedly.

### UI-02: Create/edit project, desktop

Maps to `AC-PROJECTS-AGENT-PROJECTS-001.1`,
`AC-PROJECTS-AGENT-PROJECTS-001.3`, and
`AC-PROJECTS-AGENT-PROJECTS-001.5`.

```text
Create Agent Project
Name                 [ Release migration       ]
Remote repositories  [ Search and select...    ]
Selected             [owner/api] [owner/web]
Primary repository   [owner/api              v]
Execution            Workspace default: Local worktree
Coordinator profile  [Coordinator            v]
Economy profile      [Economy                v]
Frontier profile     [Frontier               v]
                     [Cancel] [Create project]
```

No workflow field. Editing shows unavailable dependencies without silently
substituting a default. The resolved workspace-default worktree executor is
shown as read-only. The form body scrolls inside a bounded dialog.

### UI-03: Main task Files and Changes

Maps to `AC-PROJECTS-AGENT-PROJECTS-003.2` and
`AC-PROJECTS-AGENT-PROJECTS-003.3`.

```text
Files                         Changes
v Context                     Repo: owner/api v
  notes.md                    Modified files in owner/api
  docs/                       only
v Workspace
  owner-api/
  owner-web/
```

`Context` is outside Git; the Changes selector remains repository scoped.
Worker Files opens at its checkout and offers a Context entry.

### UI-04: Phone navigation and create surface

Maps to `AC-PROJECTS-AGENT-PROJECTS-005.1` and
`AC-PROJECTS-AGENT-PROJECTS-005.2`.

```text
Navigation sheet                 Create Agent Project
Projects                    [+]   [Back]  Create project
v Release migration              Name [                   ]
  Update API adapters            Repositories [Select     ]
  Verify mobile flows            Primary [owner/api      ]
  New project                    Execution: Local worktree
Tasks                             Coordinator [Select     ]
                                  Economy [Select         ]
                                  Frontier [Select        ]
                                  (scrolling form)
                                  [Create project] (fixed)
```

Use the current mobile task-navigation sheet for entry. The creation surface
is full height, has one internal scroll owner and safe-area footer, and keeps
all controls at least 44px for touch. The task view uses its existing focused
mobile Files and Changes panels.

### UI-05: Project archive/delete confirmation

Maps to `AC-PROJECTS-AGENT-PROJECTS-004.4`.

```text
Archive Release migration?
This archives the coordinator and 2 worker tasks.
Shared context stays available for restore.
                                [Cancel] [Archive project]

Delete Release migration?
This deletes the coordinator and 2 worker tasks.
Context: (o) Keep files  ( ) Delete files
                                [Cancel] [Delete project]
```

The phone version uses a full-height confirmation surface with the same task
count and context choice, internal scrolling, and a safe-area primary action.
The action is unavailable while a project task is running; the failure names
the running task and leaves the project intact.

## Tests

| Acceptance | Evidence |
| --- | --- |
| `REQ-PROJECTS-AGENT-PROJECTS-001` | Project service/handler tests for validation, atomic create, edit revision, resolved executor, and missing dependencies |
| `REQ-PROJECTS-AGENT-PROJECTS-002` | MCP admission, worker cap, result projection, and runtime profile tests |
| `REQ-PROJECTS-AGENT-PROJECTS-003` | Writable-root, context path/link/containment, and Git isolation tests |
| `REQ-PROJECTS-AGENT-PROJECTS-004` | Workflow-free task service, project archive/delete, board exclusion, and menu tests |
| `REQ-PROJECTS-AGENT-PROJECTS-005` | Flag registry, sidebar, desktop/mobile, and missing-dependency tests |

## E2E tests

- `apps/web/e2e/tests/projects/agent-projects.spec.ts` (`chromium`): create,
  open, edit profiles, create/open worker, inspect Context/Workspace/Changes,
  confirm archive/restore/delete, and verify ordinary task/Office separation
  (`AC-PROJECTS-AGENT-PROJECTS-001.2`,
  `AC-PROJECTS-AGENT-PROJECTS-002.2`,
  `AC-PROJECTS-AGENT-PROJECTS-003.2`,
  `AC-PROJECTS-AGENT-PROJECTS-003.3`,
  `AC-PROJECTS-AGENT-PROJECTS-004.4`).
- `apps/web/e2e/tests/projects/mobile-agent-projects.spec.ts`
  (`mobile-chrome`): create from navigation sheet, open child, edit project,
  use Files/Context/Changes, confirm archive/restore/delete, and assert
  viewport/scroll/touch behavior (`AC-PROJECTS-AGENT-PROJECTS-005.2`,
  `AC-PROJECTS-AGENT-PROJECTS-003.2`,
  `AC-PROJECTS-AGENT-PROJECTS-004.4`).
- The mock-agent scenario calls `create_agent_project_worker_kandev` through
  the real MCP endpoint, and both browser tests observe the created worker.
- Backend tests verify disabled-flag API and MCP requests reject before task
  creation or agent launch (`AC-PROJECTS-AGENT-PROJECTS-005.3`).

## Work orders

- [x] [Task 01: Project identity and flag](task-01-project-identity.md)
- [x] [Task 02: Context and task workspaces](task-02-context-workspaces.md)
- [x] [Task 03: Coordinator instructions and MCP](task-03-coordinator-mcp.md)
- [x] [Task 04: Project navigation and forms](task-04-navigation-forms.md)
- [x] [Task 05: Task panels and end-to-end flow](task-05-task-panels-e2e.md)

## Verification results

All five work orders are complete. Backend package tests, desktop and phone E2E,
focused frontend tests, typecheck, lint, localization checks, public documentation
validation, and specification validation passed. Exact commands are recorded
in each work order's Results section.

## Risks

- Workflow-free persistent tasks touch existing assumptions in task creation,
  board projections, session launch, and task actions; a narrow project origin
  must be audited end to end.
- A task-local symlink cannot share context with a remote executor. The first
  release blocks those executors until a transport contract exists.
- UI/API context writes use content-hash comparison; direct agent edits are
  last-writer-wins and can race with each other.
- Existing database snapshots exclude filesystem context; project context
  needs an explicit operator backup until export/restore is designed.
- The existing Office Projects section owns the same sidebar slot; render
  Agent Project rows in ordinary navigation and Office rows in Office
  navigation without mixing their data or showing duplicate sections.

## Settled first-release boundaries

- Projects use host-local worktree execution with provider-backed remote
  repositories. Cross-machine context sync is deferred.
- One Projects sidebar section shows Agent Projects in ordinary navigation and
  Office projects in Office navigation.
- PR, CI, and timer subscriptions are deferred.
