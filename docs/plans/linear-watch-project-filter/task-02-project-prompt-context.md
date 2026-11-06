---
id: "02-project-prompt-context"
title: "Project context in Linear watch prompts"
status: done
wave: 2
depends_on:
  - "01-project-filter"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-LINEAR-PROJECTS-002
acceptance_criteria:
  - AC-INTEGRATIONS-LINEAR-PROJECTS-002.1
  - AC-INTEGRATIONS-LINEAR-PROJECTS-002.2
  - AC-INTEGRATIONS-LINEAR-PROJECTS-002.3
  - AC-INTEGRATIONS-LINEAR-PROJECTS-002.4
system_design:
  - ../../specs/integrations/system-design/linear-watch-projects.md
---

# Task 02: Project Context in Linear Watch Prompts

## Summary

Fetch each issue's project with the issue, fetch project descriptions only for
watches whose prompt uses them, and render four project placeholders with a
bounded description.

## In scope

- `project { id name url description }` in `issueFragment` and the
  `LinearIssue` project fields.
- `Client.GetProjectContents` and the placeholder-gated lookup in
  `CheckIssueWatch`.
- Project placeholders and description truncation in `interpolateLinearPrompt`.
- Prompt editor placeholder entries and locales.

## Out of scope

- Changing the default prompt or refreshing running tasks.

## Acceptance

1. The four placeholders render project data, are empty without a project, and
   truncate the description at 4,000 characters with a notice.
2. Only prompts with `{{issue.project.description}}` trigger the description
   lookup; its failure fails the poll.

## Verification

```bash
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/linear/ -run 'Project')
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/orchestrator/ -run 'InterpolateLinearPrompt')
```

## Files likely touched

- `apps/backend/internal/linear/{graphql_client,graphql_projects,models,service_issue_watch}.go` and tests
- `apps/backend/internal/orchestrator/event_handlers_linear.go` and its test
- `apps/web/components/linear/linear-issue-watch-placeholders.ts`, `apps/web/src/locales/*/linear.json`

## Dependencies

Task 01.

## Parallelism

`sequential`

## Results

Original author-reported verification: `TestGraphQLClient_SearchIssues_DecodesProject`,
`TestGraphQLClient_GetProjectContents_BatchesByID`,
`TestService_CheckIssueWatch_AttachesProjectDescriptions`,
`TestService_CheckIssueWatch_SkipsProjectLookupWithoutPlaceholder`,
`TestService_CheckIssueWatch_ProjectLookupFailureFailsPoll`, and the
`TestInterpolateLinearPrompt_*` tests pass with `-race`.


Review clarifications preserve the literal-placeholder gate and existing saved
prompt expansion order. Content requests contain at most 25 IDs, without a byte
size guarantee; oversized responses fail the poll. Successful omitted projects
render empty content. Hosted focused and full-family follow-up results remain
pending in the PR review evidence.
