---
id: "05-task-panels-e2e"
title: "Task panels and end-to-end flow"
status: done
wave: 5
depends_on:
  - "02-context-workspaces"
  - "03-coordinator-mcp"
  - "04-navigation-forms"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-002
  - REQ-PROJECTS-AGENT-PROJECTS-003
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-001.2
  - AC-PROJECTS-AGENT-PROJECTS-002.2
  - AC-PROJECTS-AGENT-PROJECTS-003.1
  - AC-PROJECTS-AGENT-PROJECTS-003.2
  - AC-PROJECTS-AGENT-PROJECTS-003.3
  - AC-PROJECTS-AGENT-PROJECTS-003.4
  - AC-PROJECTS-AGENT-PROJECTS-004.1
  - AC-PROJECTS-AGENT-PROJECTS-004.4
  - AC-PROJECTS-AGENT-PROJECTS-005.2
  - AC-PROJECTS-AGENT-PROJECTS-005.3
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 05: Task panels and end-to-end flow

## Summary

Render the coordinator's virtual Context/Workspace Files root and preserve
repository-only Changes. Complete the desktop and mobile user flows with
targeted Playwright evidence and disabled-path checks.

## In scope

- Main task virtual Files tree, worker Context entry, and phone file navigation.
- Changes repository scoping, project task headers, and empty/error states.
- Desktop and mobile Playwright tests plus backend disabled-path integration.
- A mock-agent script that calls `create_project_worker` through the real MCP
  surface so E2E can observe an actual child task.
- Public documentation for creating, navigating, and managing Agent Projects
  and the initial local-executor limit; state that current database backups
  do not include filesystem context.

## Out of scope

- Remote context transport, new Git status semantics, subscriptions.

## Acceptance

- Main Files shows Context and Workspace; worker Files starts at its
  repository workspace and can open Context. A context edit is visible to
  another project task and never appears in Changes.
- Desktop and phone tests complete project creation, coordinator/worker
  navigation, context edit, repo Changes inspection, archive/restore, and
  delete confirmation with the context choice.
- Disabled flag tests show no project creation, MCP worker creation, or agent
  launch side effects; ordinary task and Office paths remain usable.

## ASCII UI preview

See [UI-03 and UI-04 in the plan](plan.md#ascii-ui-preview).

### UI-03: Coordinator Files and Changes

```text
Files                      Changes
v Context                  Repo: owner/api v
  notes.md                 (repo changes only)
v Workspace
  owner-api/
  owner-web/
```

### UI-04: Phone task panels

```text
Project / Coordinator
[Chat] [Files] [Changes]
Files: [Context] [Workspace]
(one focused panel and one scroll owner)
```

The panel separation is structural; labels and spacing are illustrative.
Reuse the existing mobile file-viewer panel and bottom navigation pattern.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/task/file-browser-path.test.ts components/task/file-tab-content.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/projects/agent-projects.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/projects/mobile-agent-projects.spec.ts)
(cd apps/backend && go test ./internal/projects/... ./internal/mcp/handlers/...)
```

## Files likely touched

- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/file-browser-tree-loader.ts`
- `apps/web/components/task/mobile/`
- `apps/web/e2e/tests/projects/agent-projects.spec.ts`
- `apps/web/e2e/tests/projects/mobile-agent-projects.spec.ts`
- `apps/backend/internal/projects/`
- `apps/backend/internal/mcp/handlers/`
- `apps/backend/cmd/mock-agent/script.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `docs/public/agent-projects.md`

## Dependencies

Tasks 02, 03, and 04 supply context API, worker tools, and navigation.

## Risks

- Existing file browser requests use the session worktree path; the virtual
  root must never turn arbitrary client paths into host filesystem reads.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/agent-projects.md)
- [System design](../../specs/projects/system-design/agent-projects.md)
- [Full preview](plan.md#ascii-ui-preview)

## Results

Passed:

- `pnpm test -- --run components/task/file-browser-path.test.ts components/task/file-tab-content.test.tsx`: 2 files, 7 tests.
- `pnpm e2e:run --host --project chromium tests/projects/agent-projects.spec.ts`: desktop flow passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/projects/mobile-agent-projects.spec.ts`: phone flow passed against the fresh build.
- The managed desktop E2E run rebuilt the backend, production E2E web bundle,
  and fixture plugin package.
