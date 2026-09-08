---
id: "02-truncate-generated-titles"
title: "Truncate generated review/issue task titles"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/title-length-limit.md"
parallel-safe: true
---

# Task 02: Truncate Generated Review/Issue Task Titles

## Root Cause

`buildReviewTaskRequest` (`orchestrator/event_handlers_github.go:451`) builds
`fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title)` and the issue watcher
(`event_handlers_github.go:1368`) builds `fmt.Sprintf("Issue #%d: %s", ...)`,
neither capped. Both feed `Service.CreateTask`, whose
`validateTaskTitle(req.Title)` (`task/service/service_tasks.go:341`) rejects
titles over 60 runes. `TruncateTaskTitle` is only applied to the auto-title
derivation path, not caller-supplied titles, so long PR/issue titles drop the
review/issue task entirely.

## Acceptance

- The complete generated review title and issue title are passed through
  `taskservice.TruncateTaskTitle` before leaving the orchestrator.
- Over-limit titles keep the `PR #<n>:` / `Issue #<n>:` prefix, are ≤60 runes,
  and end with a single `…`. Multibyte titles are counted by code point.
- Titles at or under 60 runes are unchanged.

## Audit (in scope, do not broaden)

`buildReviewTaskRequest` and the GitHub issue builder (`event_handlers_github.go`)
are the two builders that feed `CreateTask` without truncation. Confirm; fix both.
Adjacent builders are already safe and are OUT of scope unless a test shows
otherwise: `event_handlers_automation.go` already calls `TruncateTaskTitle`
(lines 314/321/324); `source_sentry.go` truncates via `truncateSentryTitle`;
`office/routines/service.go:503` already truncates. `source_jira.go`,
`source_linear.go`, `source_gitlab.go`, `source_azuredevops.go` build
`IssueTaskRequest` titles — note them, but only fix if a regression test
demonstrates they reach `CreateTask` without truncation.

## Regression Test (RED first)

- In `orchestrator/event_handlers_github_review_test.go`: table-driven
  `buildReviewTaskRequest` cases — short ASCII (unchanged), long ASCII
  (prefix kept, ≤60 runes, ends `…`), long multibyte (no split, ≤60 runes). Add
  the equivalent for the issue builder.

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'TestBuildReviewTaskRequest|TestBuildIssueTaskRequest|Title' ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`
- `apps/backend/internal/task/service/task_title.go` (read-only reference)

## Dependencies

None.

## Parallelism

`sequential` within itself; `parallel-safe` against Tasks 01 and 03.

## Inputs

- Amended spec: `docs/specs/tasks/requirements/title-length-limit.md` (watcher-title bullet
  and scenarios).
- Confirmed 5 "task title is too long" review-task failures in the logs
  (PRs #12096, #12057, #12054).

## Output contract

Report the RED failures, the truncation change, the audit outcome for adjacent
builders, files changed, exact verification result, residual risks, and update
this task plus `plan.md` status.
