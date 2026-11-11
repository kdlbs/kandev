---
id: "10-plugin-teams-sprints"
title: "Teams and sprints"
status: in_progress
wave: 3
depends_on:
  - "04-plugin-browse-screens"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.6
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.7
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.8
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin-sprints.md
---

# Task 10: Teams and sprints

## Summary

Requested by the owner on 2026-11-10 while testing on `contoso` (the project has hundreds of teams). Team selectors list the user's teams first and remember the last team per project; work-item mode gains Team and Sprint filters; a new Sprints mode shows a sprint as a read-only taskboard with team, sprint, and person selectors, modelled on Azure Boards > Sprints > Taskboard. Ships in the plugin PR (P1) with tasks 01 to 07.

## In scope

- Backend (`internal/azure`, `internal/plugin`):
  - `ListMyTeams` (`$mine=true`, paged), `GetTeamFieldValues`, `ListTeamIterations`, `GetTeamSettings` (working days), `ListTeamDaysOff`, `GetIterationWorkItems`, `ListTaskboardColumns`; mock client and seed for each.
  - Actions: `teams.list` returns `mineIds`; new `teams.settings` and `sprints.board` per the design table, with the 1,000-card bound and `truncated` flag.
  - Pure helpers with unit tests: taskboard column mapping, row grouping and ordering, unparented row, working days left.
- UI (`ui/src/browse`):
  - Team selector with **My teams** group, shared by Board, Work items, and Sprints; `teamByProject` preference.
  - Work-item Team and Sprint filters, WIQL area and iteration conditions (pure helper next to `workItemSearchWiql`), saved views with `teamId` and `iterationId`.
  - Sprints mode tab: sprint selector (filter, past/current/future tags, dates, working days left), person selector, taskboard rows with collapse and collapse all, cards opening the detail and task actions; phone composition.
  - Locale keys in all seven plugin locales.
- Packaged E2E on desktop and phone with mock data for my-teams ordering, filters, sprint default, person filter, collapse, empty states.
- CHANGELOG, README feature list.

## Out of scope

- Moving cards, sprint planning, capacity, burndown.
- Kandev host changes.

## Acceptance

- Every SPRINTS-001 criterion has a unit or packaged E2E test that failed before its change.
- Owner check on `contoso`: team Offers listed under My teams, sprint Sprint 23 opens as current with the same rows and tasks as Azure Boards, and the person filter matches Azure's.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && make check-format vet test)
(cd ../JnManso-kandev-plugin-azure-devops-main && make e2e)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/internal/azure/rest_client.go`, `mock_client.go`, `mock_seed.json`, new `sprints.go`
- `../JnManso-kandev-plugin-azure-devops-main/internal/plugin/actions_browse.go`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/browse/` (new `sprints.tsx`, `team-picker.tsx`, `sprint-board.ts`), `filters.tsx`, `page.tsx`, `search-query.ts`, `use-preferences.ts`, `mode-tabs.tsx`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/locales/*.json`
- `../JnManso-kandev-plugin-azure-devops-main/ui/e2e/*.spec.ts`

## Dependencies

Task 04-plugin-browse-screens.

## Risks

- `$mine=true` reflects the identity behind the PAT or Microsoft account; with a shared PAT, "my teams" are the PAT owner's. Documented in the README.
- Large sprints: the 1,000-card bound keeps the action response small; the UI says when it is truncated.
- Customized taskboard columns can map one state to several work-item types; the mapping helper follows Azure's column mappings exactly and is unit-tested with a custom process.

## Parallelism

Backend actions and UI helpers can proceed in parallel; the Sprints UI needs the action contract.

## Inputs

- Owner screenshots of 2026-11-10: Azure Boards board selector, Sprints taskboard for Offers Sprint 23 with the sprint selector open.
- Test notes `/tmp/ado-test/test-notes.md` items 13 to 15.

## Results

- 2026-11-11: implemented on the plugin branch (`feat/bootstrap`); PR P1 opens with tasks 01 to 07.
  - Backend: `internal/azure/sprints.go` (column mapping, row grouping and order, working days left, people), `rest_client_sprints.go` (`ListMyTeamIDs`, `GetTeamSettings`, `GetSprintBoard`, six concurrent reads, 1,000-card bound), mock client and seed (team-2 with three sprints, sprint items kept apart from the work-item search set). Actions `teams.list` (`mineIds`, degrading to none when Azure refuses), `teams.settings`, `sprints.board`; saved views keep `teamId` and `iterationId`.
  - UI: Sprints mode (`sprints.tsx`, `use-sprints.ts`), team selector with **My teams** first in Board, Work items, and Sprints (`teamPickerOptions`), `teamByProject` and Sprints preferences, work-item Team and Sprint filters with WIQL area and sprint conditions (`sprints-model.ts`), saved views (`saved-view-filters.ts`), card task actions shared with the Board (task 04, AC 004.10), keys in all seven locales (`sprint`/`sprints` declared verbatim for pt-PT).
  - Tests: Go `sprints_test.go`, `rest_client_sprints_test.go`, `mock_client_sprints_test.go`, `browse_sprints_test.go` (each failed before its code); Vitest `sprints-model.test.ts` (14), `saved-view-filters.test.ts`, `board-discovery.test.ts`, `search-query.test.ts`; packaged E2E on desktop (my teams first, board card actions, no-sprints and empty states, current sprint taskboard, person filter and collapse, card detail and actions, work-item team and sprint filters) and phone (stacked selectors, column groups, no horizontal scrolling).
  - Verification: `make check-format vet test` passes (142 Vitest tests and typecheck), `go test -race ./internal/...` passes, `make e2e` passes 28/28.
  - Pending: the owner check on `contoso` (Offers under My teams, Sprint 23 as current with the same rows and person filter as Azure Boards).
- 2026-11-11: owner testing on `contoso` found the Sprints view without columns and without tasks for Offers sprint Sprint 23 (rows showed task counts but no cards).
  - Cause: `taskboardcolumns` returns an empty list for a team that never customized its taskboard, so no column existed, every task had an empty `columnId`, and the UI rendered none.
  - Fix (plugin commit `f88be7d`): `DefaultTaskboardColumns` builds Azure's default columns from the task level's types (`backlogs`) and their states, and `tasksInColumn` shows a task without a known column in the first column.
  - Tests: `TestDefaultTaskboardColumnsFollowTheTaskStates`, `TestBuildSprintRowsWithDefaultColumnsKeepsUnparentedTasksAsTasks`, `TestRESTClientUsesTheTaskStatesWhenTheTaskboardIsNotCustomized` (failed before: no columns), and the `tasksInColumn` case in `sprints-model.test.ts` (failed before: missing helper). `go test -race ./internal/...`, `make check-format vet test`, and `make e2e` (36/36) pass.
  - Checked on `contoso` after reinstalling: Offers Sprint 23 shows columns New, Active, and Closed. Task #1007 (Closed) is in Closed, and #1003 and #1005 (Active) are in Active, under their stories.
