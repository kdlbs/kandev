---
id: "04-navigation-forms"
title: "Project navigation and forms"
status: done
wave: 4
depends_on:
  - "01-project-identity"
  - "03-coordinator-mcp"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-001.1
  - AC-PROJECTS-AGENT-PROJECTS-001.2
  - AC-PROJECTS-AGENT-PROJECTS-001.3
  - AC-PROJECTS-AGENT-PROJECTS-001.5
  - AC-PROJECTS-AGENT-PROJECTS-004.1
  - AC-PROJECTS-AGENT-PROJECTS-004.4
  - AC-PROJECTS-AGENT-PROJECTS-005.1
  - AC-PROJECTS-AGENT-PROJECTS-005.2
  - AC-PROJECTS-AGENT-PROJECTS-005.3
  - AC-PROJECTS-AGENT-PROJECTS-005.4
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 04: Project navigation and forms

## Summary

Extend the single Projects section and add responsive create/edit experience.
Display one project-name row and direct worker rows without duplicating them
in ordinary task lists, and remove workflow controls for project tasks.

## In scope

- Project API client, state/hydration, status updates, sidebar tree and routes.
- Desktop create/edit dialog and phone full-height creation surface.
- Desktop and phone archive/delete confirmations with worker count and
  context-retention choice for delete, plus an Archived Projects view with
  restore action.
- Project task workflow chrome/menu suppression and unavailable-dependency UI.
- Localization in all required catalogs.

## Out of scope

- Files/Changes root presentation and final integrated E2E flow.

## Acceptance

- Desktop/phone users can create, open, expand, and edit Agent Projects; each
  project-name row opens its coordinator, while worker rows appear directly
  beneath it only when workers exist. Office entries remain in Office
  navigation of the same section.
- Project archive/delete has a project-level confirmation on desktop and
  phone; archived projects can be restored; the coordinator's generic task
  menu cannot bypass these actions.
- Neither coordinator nor worker shows workflow selectors in task chrome or
  menus; ordinary tasks keep them.
- The phone form has one internal scroll area, safe-area footer, reachable
  44px controls, and a usable failure/invalid-dependency state.

## ASCII UI preview

See [UI-01, UI-02, UI-04, and UI-05 in the plan](plan.md#ascii-ui-preview).

### UI-01: Desktop project tree

```text
Projects                               [+]
v Release migration                 active
  |- Update API adapters            done
  `- Verify mobile flows           running
  New project                       idle
```

### UI-02: Desktop create/edit fields

```text
Name [              ]  Remote repositories [Select...]
Primary repo [       ]  Coordinator [      ]
Execution: Workspace default (Local worktree)
Economy [            ]  Frontier [         ]
                       [Cancel] [Create project]
```

### UI-04: Phone creation

```text
[Back] Create project
Name [                     ]
Repositories [Select        ]
Primary [Select             ]
Execution: Local worktree
Coordinator [Select         ]
Economy [Select             ]
Frontier [Select            ]
(scrolling body)
[Create project] (fixed safe-area footer)
```

### UI-05: Project delete confirmation

```text
Delete Release migration?
Coordinator and 2 worker tasks will be deleted.
Context: (o) Keep files  ( ) Delete files
                      [Cancel] [Delete project]
```

Phone uses a full-height confirmation surface with the same count and choice.

The tree and fields are structural; exact text and spacing are illustrative.
The mobile navigation sheet is the nearest shipped navigation exemplar.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/app-sidebar components/task lib/state/slices/features)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
```

## Files likely touched

- `apps/web/components/app-sidebar/`
- `apps/web/components/navigation/mobile-task-navigation-provider.tsx`
- `apps/web/components/task/TaskHeader.tsx`
- `apps/web/components/task/task-actions-menu-dialogs.tsx`
- `apps/web/lib/state/`
- `apps/web/lib/api/`
- `apps/web/src/locales/`
- `apps/web/AGENTS.md`

## Dependencies

Tasks 01 and 03 supply project APIs and worker status.

## Risks

- Keep one `ProjectsSection` slot and mode-specific data so Agent Projects
  cannot appear as Office projects or create a duplicate Projects header.
- The current section returns early outside Office and reads only
  `selectOfficeProjects`; replace both assumptions with mode-specific
  project API/store hydration.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/agent-projects.md)
- [System design](../../specs/projects/system-design/agent-projects.md)
- [Full preview](plan.md#ascii-ui-preview)

## Results

Passed:

- Focused Agent Projects API, store, feature contract, sidebar, and task chrome
  tests: 5 files, 32 tests.
- Agent Projects state hydration tests: 5 tests.
- `pnpm run lint`, `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`.
