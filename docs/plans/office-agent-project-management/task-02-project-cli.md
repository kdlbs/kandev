---
id: "02-project-cli"
title: "Add Office project CLI commands"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 02: Add Office project CLI commands

## Acceptance

- `$KANDEV_CLI kandev projects list` and `projects create` call the run-scoped project endpoints and preserve structured JSON responses.
- `projects create` rejects an empty name before contacting the server and supports repeated `--repository` flags.
- `task create --project PROJECT_ID` sends `project_id` through the run-scoped Office endpoint; parent and assignee remain supported, while fields the Office creator cannot persist are rejected explicitly.

## Verification

```bash
cd apps/backend && rtk go test ./cmd/agentctl -run 'Test(Project|TaskCreate)' -count=1
```

## Files Likely Touched

- `apps/backend/cmd/agentctl/kandev.go`
- `apps/backend/cmd/agentctl/kandev_projects.go`
- `apps/backend/cmd/agentctl/kandev_task.go`
- `apps/backend/cmd/agentctl/kandev_test.go`

## Inputs

- Runtime paths from Task 01: `GET/POST /api/v1/office/runtime/projects`.
- Pattern: `kandev_agents.go`, `kandev_routines.go`, and request-capture tests.

## Output Contract

Report command syntax, payloads, files changed, targeted tests, blockers, risks, and task status. Use TDD and do not edit `plan.md`.

## Completion Evidence

- Added `projects list` and `projects create` dispatch against `/api/v1/office/runtime/projects`.
- Added repeatable `--repository` plus description, lead profile, color, budget, and executor config create flags with pre-request blank-name validation.
- Added `task create --project`, serialized as `project_id` without changing existing task fields.
- `cd apps/backend && rtk go test ./cmd/agentctl -run 'Test(Project|TaskCreate)' -count=1` passed: 12 tests.
- `cd apps/backend && rtk go test ./cmd/agentctl -count=1` passed: 39 tests.

## Corrective Evidence

- `task create` now posts to `/api/v1/office/runtime/tasks`; it neither requires nor serializes mutable `KANDEV_WORKSPACE_ID`, so callers cannot select the authorization workspace.
- `--parent`, `--assignee`, and `--project` remain supported. Priority, blocker, and workspace-policy flags now return explicit unsupported errors before any HTTP request because the current Office creator cannot persist them faithfully.
- Socket-free CLI contract suite passed: 11 tests covering the runtime path, absent/spoofed workspace environment, project payload, setup workflow, and every unsupported flag.
- Corrected the bundled `kandev-protocol` task-create reference to advertise only `--title`, `--parent`, `--assignee`, and `--project`; both the protocol and project skills are now pinned by a content regression test against unsupported task-create flags.
- Added whitespace-only task-title rejection before client construction or HTTP
  dispatch, and normalized accepted task titles before serialization. The
  socket-free focused task-create suite passed: 11 tests.
- Added the runtime-supported `task create --description TEXT` flag and pinned its JSON serialization with the socket-free transport harness, so the bundled `kandev-escalation` command now succeeds contractually.
- Expanded the bundled-skill regression to discover every system skill containing `task create`; it now covers escalation, projects, and protocol examples while rejecting unsupported priority, blocker, and workspace-policy flags.
- Focused verification passed: 10 socket-free task-create tests, compile-only `cmd/agentctl`, all 90 `internal/office/skills` tests, and backend lint with 0 issues.
- Repaired `kandev-escalation` to parse the runtime's `.task_id` response, removed the nonexistent `--add-blocker` mutation, and documented comment/task-update recovery instead of an automatic blocker-resolution wakeup.
- Expanded bundled-skill contracts to require protocol `--description`, pin the supported escalation sequence, and reject known unsupported flags and subcommands across every bundled skill.
- Routed `tasks message` through `POST /api/v1/office/runtime/comments` with only `task_id` and `body`; the CLI no longer selects author fields or writes through the administrative task-comments endpoint.
- Implemented the documented `--prompt -` stdin behavior and pinned HTTP 403 propagation from runtime scope denial with socket-free transport tests.
- Updated the protocol and escalation skills to describe signed runtime scope and server-derived attribution. Escalation now posts only on the current blocked task because the newly created human task is not automatically in the run's mutation scope.
- Security verification passed: 3 socket-free `TestTasksMessage_` tests, all 91 Office skill tests, 4 existing runtime scope/attribution tests, and targeted golangci-lint with 0 issues.
- Routed `task update` through signed
  `POST /api/v1/office/runtime/tasks/:id/status`; it no longer reaches the
  administrative `PATCH /api/v1/office/tasks/:id` route. Status plus optional
  comment remains supported, while comment-only calls fail locally and direct
  callers to `tasks message`.
- Four socket-free CLI regressions pin the runtime method/path/payload, HTTP 403
  propagation, comment-only guidance, and no-request empty-input guard.
