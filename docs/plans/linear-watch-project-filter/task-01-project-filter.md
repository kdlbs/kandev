---
id: "01-project-filter"
title: "Filter Linear watches by project"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-LINEAR-PROJECTS-001
acceptance_criteria:
  - AC-INTEGRATIONS-LINEAR-PROJECTS-001.1
  - AC-INTEGRATIONS-LINEAR-PROJECTS-001.2
  - AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
system_design:
  - ../../specs/integrations/system-design/linear-watch-projects.md
---

# Task 01: Filter Linear Watches by Project

## Summary

Add `projectIds` to the Linear watch filter, a team project inventory, and a
project picker in the watch dialog that keeps the stored selection on save.

## In scope

- `SearchFilter.ProjectIDs`, `buildIssueFilter`, `normalizeFilter`,
  `filterIsEmpty`, and the validation message.
- `Client.ListProjects`, `GET /api/v1/linear/projects`, the mock client and its
  `POST /api/v1/linear/mock/projects` control route, and a shared
  `collectPages` helper reused by `ListUsers`.
- `LinearSearchFilter.projectIds`, `listLinearProjects`, form state
  build/parse, `ProjectMultiSelect`, team-change reset, table summary, locales.

## Out of scope

- Prompt placeholders (Task 02) and the `/linear` browser filters.

## ASCII UI preview

See `UI-01` in the [plan](plan.md#ascii-ui-preview): a Projects chip row under
Labels, hidden until a team is selected.

## Acceptance

1. Search requests carry `project.id.in` only when projects are selected; a
   project-only filter is valid.
2. The dialog lists all team projects, saves the selection, and keeps stored
   `projectIds` when an existing watch is saved, on desktop and phone.

## Verification

```bash
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/linear/ -run 'Project|ListUsers|BuildIssueFilter|HTTPList')
(cd apps/web && pnpm exec vitest run components/linear/linear-issue-watch-form.test.ts)
(cd apps/web && e2e/scripts/run-e2e.sh --host --no-build --shards 1 --project chromium -- tests/integrations/linear-watcher-project-filter.spec.ts)
(cd apps/web && e2e/scripts/run-e2e.sh --host --no-build --shards 1 --project mobile-chrome -- tests/integrations/mobile-linear-watcher-project-filter.spec.ts)
```

## Files likely touched

- `apps/backend/internal/linear/{models,client,graphql_client,graphql_pagination,graphql_projects,graphql_users,handlers,mock_client,mock_controller,service_issue_watch}.go` and tests
- `apps/web/components/linear/linear-issue-watch-{fields,filter-fields,form,table}.ts(x)`
- `apps/web/lib/{types/linear,api/domains/linear-api}.ts`, `apps/web/src/locales/*/linear.json`
- `apps/web/e2e/helpers/api-client.ts`, `apps/web/e2e/tests/integrations/*project-filter*`

## Dependencies

None.

## Parallelism

`sequential`

## Results

Implemented. `TestBuildIssueFilter_Projects`,
`TestGraphQLClient_ListProjects_*`, `TestService_IssueWatch_ProjectIDsFilter`,
`TestHTTPListProjects_*`, and the existing `ListUsers` pagination suite pass
with `-race`. The form round-trip unit tests pass. Both Playwright specs pass
locally on `chromium` and `mobile-chrome`, together with the existing Linear
settings, dispatch-order, watcher-delete, and agent-profile-delete specs.
