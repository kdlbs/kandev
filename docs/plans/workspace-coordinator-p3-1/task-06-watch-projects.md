---
id: "06-watch-projects"
title: "Watches by project"
status: draft
wave: 2
depends_on:
  - "phase 3 merged"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-PERMISSIONS-005
acceptance_criteria:
  - AC-COORDINATOR-PERMISSIONS-005.1
  - AC-COORDINATOR-PERMISSIONS-005.2
  - AC-COORDINATOR-PERMISSIONS-005.3
  - AC-COORDINATOR-PERMISSIONS-005.4
  - AC-COORDINATOR-PERMISSIONS-005.5
  - AC-COORDINATOR-PERMISSIONS-005.6
  - AC-COORDINATOR-PERMISSIONS-005.7
  - AC-COORDINATOR-PERMISSIONS-005.8
  - AC-COORDINATOR-PERMISSIONS-005.9
system_design:
  - ../../specs/coordinator/system-design/watch-projects.md
---

# Task 06: Watches by project (WP 3.1-6)

## Summary

Adds the Projects scope to Watches: storage, the filter module, enforcement in
every read, propose and count path, the settings and guided-setup UI, and the
copilot hint. Work order 01 and 04 call the same filter, so 06 may land before
01.

## In scope

- `internal/coordinator/watch/`: `Predicate.InProjects` (compiled in,
  enforcing stored scope whatever the flag), `Resolver`, `watch.Task`; every
  existing workflow-watch call site moves to `watch.Task` (read tools, propose
  tools, wake recorder, backstop, automatic counts, stall reads).
- Store: `project_scope` and `include_no_repository` on `coordinators`,
  `coordinator_watch_projects`; the subscription to repository and repository
  set deletion events; the `projects` member of the settings request with
  validation.
- Web: `watch-filter.ts` with the resolved `projects` input,
  `AttentionTask.repositoryIds`, the Projects part of the Watches section, the
  guided-setup step, the copilot scope hint, copy in six locales.
- If work order 01 has landed, the ledger snapshot and digest use `watch.Task`
  and the row stores `project_scope`.

## Out of scope

- Grouping by epic or label; Office projects.

## ASCII UI preview

Screens changed: UI-31-04 of [the plan](plan.md#ascii-ui-previews).

```text
Watches
  Boards: [ ] Watch every board, including new ones  (phase 2)
  Projects
   [ ] Watch every project, including new ones
       Include tasks with no repository [ ]
       Sets:  Payments (repo-a, repo-b) In scope [Take this project out of scope]
              Mobile   (repo-c)         Out      [Put this project in scope]
       Repositories not in a set:  repo-d  Out  [Put this project in scope]
  Save is disabled: Keep at least one project in scope or include tasks with no repository.
```

## Acceptance

- Predicate matrix: all, listed repository, repository in a listed set, no
  repository with the toggle on and off, multi-repository any-match.
- Each enforcement path treats a task outside as not found or refuses it
  naming the field; `create_task` refuses `repository_id` as `005.5` says.
- A repository added to a listed set widens the coordinator on the next read;
  a deleted set or repository is removed and an empty list watches nothing.
- Save validation matrix; a save that changes Projects archives the
  conversation and raises `policy_revision` once; a proposal made in scope
  stays approvable.
- Flag off is identical to phase 3; the UI covers the failed-load, reader and
  phone states.

## Validation

- `make -C apps/backend test` for the touched packages, with `synctest` for any timer and no `time.Sleep`; store conformance on SQLite and PostgreSQL for each new table or column.
- `cd apps && pnpm --filter @kandev/web` typecheck, lint and Vitest for the touched modules, `cd apps/web && pnpm run i18n:check`, and the Playwright spec of this work order (plan verification strategy) on desktop and `mobile-chrome`, with the `auth` project for reader cases.
- `python3 scripts/list-docs.py validate` if a specification changes.
