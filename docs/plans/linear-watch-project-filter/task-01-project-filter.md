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
Labels. Without a team, saved selected IDs remain removable; an empty
selection shows only the no-team helper.

## Acceptance

1. Search requests carry `project.id.in` only when projects are selected; a
   project-only filter is valid.
2. The dialog lists all team projects, saves the selection, and keeps stored
   `projectIds` when an existing watch is saved, on desktop and phone.

## Verification

```bash
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/linear/ -run 'Project|ListUsers|BuildIssueFilter|HTTPList')
(cd apps/web && pnpm exec vitest run components/linear/linear-issue-watch-form.test.ts components/linear/linear-issue-watch-fields.test.tsx)
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

Original author-reported verification: `TestBuildIssueFilter_Projects`,
`TestGraphQLClient_ListProjects_*`, `TestService_IssueWatch_ProjectIDsFilter`,
`TestHTTPListProjects_*`, and the existing `ListUsers` pagination suite pass
with `-race`. The form round-trip unit tests pass. Both Playwright specs pass
locally on `chromium` and `mobile-chrome`, together with the existing Linear
settings, dispatch-order, watcher-delete, and agent-profile-delete specs.


Review remediation preserves project-only saved selections as removable chips
through the real dialog `FilterFields` caller, without fetching unscoped
inventory or clearing other filters. New unit controls cover no-team empty
state, team loading, missing-ID removal, and the existing workspace/team cache.
Desktop and phone regressions remove an unavailable ID, save, reload, and check
the surviving project-only filter; phone coverage measures its 44 px target and
checks actual center-point hit testing. This reuses the shipped Linear dialog's
single-column Projects row and wrapping chips; the dialog owns scrolling and
Update remains the primary action. Hosted caller RED/controls/GREEN is recorded
below; exact-built-source browser verification is pending.


Hosted review proof: five shared controls passed with the original caller,
then the project-only selection assertion failed because the selected chip was
absent. Restoring the corrected caller passed all 26 selected form/component
tests. The combined proof workflow subsequently failed its standalone typecheck
with the default Node heap exhausted and reported required test formatting.
The hosted formatter output was applied with only wrapping and trailing commas.
This is behavioral RED/controls/GREEN evidence, not a successful typecheck or
full-CI claim. Actual final full frontend typecheck and desktop/phone browser
results remain pending in retained PR evidence.


The desktop/phone selection flow arms the shared `waitForHttp` observer before
choosing Team and requires the projects GET to succeed before using its chips.
This addresses the review's causal-wait finding without changing timeout or
retry budgets; final rendered verification remains pending.

The full frontend workflow `39155688189` and owning Linear backend package in
`39155688188` passed at hosted merge source `5d27bfe0da` for head `0e89cdce29`.
The phone project-only case in `39155688165` stopped before opening the dialog:
the API fixture supplies team `ENG` when `teamKey` is omitted. This was a fixture
setup failure, not assertion RED for the product defect. The regression now
supplies an explicit empty team and reads the saved filter back before opening
the dialog, verifying both the empty team and the two project IDs. Final browser
and full-family verification of this correction remains pending in PR evidence.
