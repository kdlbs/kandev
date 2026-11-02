---
id: "01-select-eligible-tasks"
title: "Select eligible tasks before paging"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-SEARCH-002
acceptance_criteria:
  - AC-TASKS-WORKSPACE-SEARCH-002.1
  - AC-TASKS-WORKSPACE-SEARCH-002.2
  - AC-TASKS-WORKSPACE-SEARCH-002.3
  - AC-TASKS-WORKSPACE-SEARCH-002.4
system_design:
  - ../../specs/tasks/system-design/workspace-search.md
---

# Task 01: Select eligible tasks before paging

## Summary and scope

In one sequential vertical change, filter the settled selected workflow's
command-panel-visible steps in the authorized workspace list before pagination,
then use that bounded page in the empty-query panel. Follow the paired design
for the opt-in contract, missing-identity fallback, SQL, and ordering. Retain
default listing and nonempty search semantics. Do not modify private
qualification tests, other workflows, UI layout, or production code during the
design turn.

## Acceptance

1. An independently authored rendered regression fails before the correction
   with twenty newer hidden rows and an older eligible row, and passes after it:
   once settled, selecting the task invokes navigation. A first-page control
   passes. The actual hook, `CommandsListContent`, real command primitives,
   and English catalogs are exercised at a valid paginated API boundary.
2. Real SQLite repository and authorized HTTP/service tests prove the predicate
   precedes count/page limits, respects workspace, archive, ephemeral, config
   and automation restrictions, rejects invalid opt-in combinations, and leaves
   ordinary lists and text search unchanged. Repeat dialect-sensitive behavior
   with a real environment-gated PostgreSQL database.
3. Focused hook tests cover five/twenty caps, step order, all-hidden, no-step
   fallback, search active/archive behavior, and abort guards. Desktop and
   phone share these results without a presentation or interaction change. The
   API client test verifies `command_panel_visible` and `workflow_id` query
   serialization and that ordinary requests omit the opt-in.

## Likely files and dependencies

- `apps/backend/internal/task/repository/sqlite/task.go` and
  `command_panel_eligible_test.go`; `apps/backend/internal/task/service/service_tasks.go`
  and `command_panel_eligible_test.go`; `apps/backend/internal/task/handlers/task_http_handlers.go`
  and `command_panel_eligible_test.go`. Preserve shared hydration and authorization.
- `apps/web/lib/api/domains/kanban-api.ts`, `apps/web/components/command-panel.tsx`,
  `apps/web/lib/api/domains/kanban-command-panel-api.test.ts`,
  `apps/web/hooks/use-command-panel-task-results.ts` and its tests, and the new
  `apps/web/components/command-panel-eligible-tasks.test.tsx` rendered test.
- No dependencies on other work orders. Check the current PR and base ownership
  before edits; no native or persistent workers are authorized.

Public documentation assessment: `GET /api/v1/workspaces/:id/tasks` is internal
UI transport and is not documented as a supported public API in `docs/public`,
the root README, or the screenshot catalog. Other documented REST routes are
separate. The additive option needs no public parameter reference; existing
workflow guidance describes the visible setting.

## Verification

After a later explicit implementation instruction and exclusive heavy lease,
run the following from the repository root, serially. Write the new regression
first and run it red for the stated reason before changing production code.
Use a unique owned `KANDEV_RUN_QUIET_DIR` for each run-quiet invocation when
used; retain and join any native process handles.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -v -run '^TestCommandPanelEligible' ./internal/task/repository/sqlite)
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p=1 -count=1 -v -run '^TestCommandPanelEligible' ./internal/task/handlers ./internal/task/service)
(cd apps/web && pnpm exec vitest run --maxWorkers=1 lib/api/domains/kanban-api.test.ts lib/api/domains/kanban-command-panel-api.test.ts hooks/use-command-panel-task-results.test.ts components/command-panel-eligible-tasks.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/api/domains/kanban-api.ts lib/api/domains/kanban-command-panel-api.test.ts components/command-panel.tsx hooks/use-command-panel-task-results.ts hooks/use-command-panel-task-results.test.ts components/command-panel-eligible-tasks.test.tsx)
(cd apps/web && env NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
env -u GOMEMLIMIT PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/task.go apps/backend/internal/task/repository/sqlite/command_panel_eligible_test.go apps/backend/internal/task/service/service_tasks.go apps/backend/internal/task/service/command_panel_eligible_test.go apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/command_panel_eligible_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

With `KANDEV_TEST_POSTGRES_DSN` set, rerun the repository command and record
the PostgreSQL subtest result; otherwise report its skip. Hosted Backend and
PostgreSQL CI cohorts remain required; do not start a container or add a broad
local suite as a gate. Name new backend tests with the `TestCommandPanelEligible`
prefix so the commands select them. Include every additional changed Go and
TS/TSX file in the respective lint commands. Go test limits apply only to tests;
the Go lint helper receives its separate default 3 GiB memory limit. Record the
actual selected tests and commands, and avoid broad reruns without a new failure
or changed code.

## Results

- Independently authored rendered regression: one hidden-first-page failure and
  one ordinary-page control pass before the fix; both pass after the fix. API
  serialization, SQLite capability and invalid HTTP request tests also failed
  for their intended missing behavior before implementation. The service test
  was added after the implementation and passed against the real repository.
- Frozen pnpm install passed once. The final four frontend files passed 37
  tests; after test literal consolidation the changed hook suite passed its
  13 tests again. Scoped ESLint passed for all changed TS/TSX files. The staged
  i18n ratchet passed.
- The Go 1.26 local `-race -trimpath -tags=fts5 -p=1` SQLite and HTTP/service
  tests passed under `GOMAXPROCS=2 GOMEMLIMIT=512MiB`. The existing literal-search
  pagination and filter controls also passed with those settings. The real
  PostgreSQL test skipped because `KANDEV_TEST_POSTGRES_DSN` was unset; hosted
  Backend and PostgreSQL checks remain required. The canonical changed-Go lint
  helper passed with its separate default 3 GiB limit after test files were
  staged.
- Typecheck passed twice at `NODE_OPTIONS=--max-old-space-size=6144`: the first
  pass preceded the all-hidden and abort test additions; the second covered
  the final TS inputs. This is not a default-heap pass. The cgroup limit was
  not established. No other local heap escalation was attempted.
