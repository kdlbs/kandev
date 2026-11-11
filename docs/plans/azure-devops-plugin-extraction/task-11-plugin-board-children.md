---
id: "11-plugin-board-children"
title: "Board card children"
status: done
wave: 3
depends_on:
  - "04-plugin-browse-screens"
  - "10-plugin-teams-sprints"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.6
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin-board-children.md
---

# Task 11: Board card children

## Summary

Requested by the owner on 2026-11-11 while testing the Offers Epics and Features boards on `contoso`. Board cards show a done/total count of their child work items and expand in place to list them (an epic's features, a feature's stories, a story's tasks), with detail and task actions on each child, and one expand/collapse-all control remembered per board. Ships in the plugin PR (P1) with tasks 01 to 07 and 10.

## In scope

- Backend (`internal/azure`, `internal/plugin`):
  - `ListChildren(ctx, project, parentIDs)`: WIQL link queries of at most 200 parents, run concurrently; child batch read with backlog order fields; state categories per work-item type (cached per call); `Removed` filtered; 1,000-child bound with `truncated`.
  - Mock client: parent links and per-type state categories in `mock_seed.json`, children for board-1 and board-2 cards, including a removed child and a mixed-type parent.
  - Action `boards.children` (`projectId`, `ids`) in `browse.go`, `client.go`, `manifest.yaml`.
- UI (`ui/src/browse`):
  - `board-children.ts`: pure helpers for the toggle label (one type or several), done counts, and lookup by parent.
  - `use-board-children.ts`: requests children after each snapshot, ignores stale responses.
  - Card footer toggle and child list (`board.tsx`, a new `board-card-children.tsx`), sharing the sprint card fields; events contained so the card neither opens nor drags.
  - Board header expand/collapse-all; `expandChildren` per board in the Board preference (`use-preferences.ts`).
  - Unavailable and truncated notices; locale keys in all seven plugin locales.
- Packaged E2E on desktop and phone.
- CHANGELOG, README feature list.

## Out of scope

- Grandchildren, adding or reordering children, editing a child from its line.
- Kandev host changes.

## Acceptance

- Every CHILDREN-001 criterion has a unit or packaged E2E test that failed before its change.
- Owner check on `contoso`: the Offers Epics board shows Customer Offers [R1] with its features, the Features board shows each feature's stories, and the Stories board shows each story's tasks, with the same done/total counts as Azure Boards.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && make check-format vet test)
(cd ../JnManso-kandev-plugin-azure-devops-main && make e2e)
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/internal/azure/` new `children.go`, `rest_client_children.go`, `mock_client_children.go`; `mock_client.go`, `mock_seed.json`
- `../JnManso-kandev-plugin-azure-devops-main/internal/plugin/browse.go`, `client.go`, `manifest.yaml`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/browse/` new `board-children.ts`, `use-board-children.ts`, `board-card-children.tsx`; `board.tsx`, `api.ts`, `types.ts`, `use-preferences.ts`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/locales/*.json`
- `../JnManso-kandev-plugin-azure-devops-main/ui/e2e/*.spec.ts`

## Dependencies

Tasks 04-plugin-browse-screens and 10-plugin-teams-sprints (shared card fields and task actions menu).

## Risks

- Board load time: the children read runs after the cards render, so it can only delay the counts. A Stories board with many tasks may take several batch reads; the 1,000-child bound caps it.
- Link queries across projects: children in another project are listed; their state categories come from that project's work-item types.
- Process templates name done states differently; using the `Completed` state category instead of state names keeps the count correct for custom processes.

## Results

- 2026-11-11: implemented in plugin commit `15852ce`.
  - Backend: `children.go` (link-query chunking, bounded child IDs, grouping in backlog order, `Removed` filtered, `StateCategories`), `rest_client_children.go` (concurrent link queries, batch read, one state read per project and type), `mock_client_children.go` with seeded links and categories, and the `boards.children` action (`browse_children.go`, at most 1,000 card IDs).
  - UI: `board-children.ts`, `use-board-children.ts`, `board-card-children.tsx`; the card footer toggle reads `Task: 1/2`, or `Child items: 0/2` for mixed types; child lines reuse the work-item task actions and open the detail without board context; **Expand all** / **Collapse all** stored as `expandChildren` per board key; unavailable and truncated notices. Locale keys in all seven locales (`childCountTyped` declared as a placeholder frame).
  - The mock's second board is now **Features** (a Feature card with a story and a bug), since task levels are no longer boards (AC 004.11).
- Tests: `children_test.go`, `rest_client_children_test.go` (three link queries for 450 parents, 1,000-child bound, one state read per type), `mock_client_children_test.go`, `browse_children_test.go` (summarized children, parent bound, failure), and `board-children.test.ts` (6 cases). Packaged E2E: "shows a card's child items and opens a child without opening the card", "counts children of several types without naming a type", "remembers expand all for the board after a reload", "shows the cards before their children and says when children are unavailable", and the phone case "expands a board card's children on a phone without horizontal scrolling". They fail on the previous code, which has no child toggle.
- Verification: `make check-format vet test` passes (148 Vitest tests, typecheck, Go); `go test -race ./internal/...` passes; `make e2e` passes 34/34. Desktop and phone captures with mock data checked: toggle, child lines, task actions, and **Collapse all** render without horizontal scrolling.
- 2026-11-11: owner check on `contoso` (Contoso, team Offers), on the instance at `localhost:21075` after reinstalling the package.
  - The owner saw the Stories board after a full page reload: #1002 shows `Task: 0/1` and expands to task #1003. The first view showed no counts because the open page still ran the previous bundle.
  - A headless check covered the rest. Every card's total matched a separate `[System.Parent] = <id>` work-item search that excludes removed items (0 mismatches):
    - Epics: #2001 Customer Offers [R1] shows `Feature: 1/4` (New, Closed, New, Active; Closed is the only `Completed` state).
    - Features: #2005 and #2006 have no children.
    - Stories: #2007 and #1002 show `Task: 0/1`; #1001 has no children.
  - **Expand all** on Epics survived a reload (`Collapse all`, list open), then was set back to collapsed.
  - A child's task actions opened the quick-action menu without a dialog, and the child line opened the detail of #2002 Offer Service Specs.
- 2026-11-11 (amendment, owner request after the check): child lines expand level by level down to the tasks (AC 001.2, 001.4 amended). Plugin commit `3e95134`.
  - `use-board-children.ts` keeps one children index per set of cards and reads, in one request, the items shown under expanded cards and lines whose children were never requested (`missingChildIds`, `mergeChildren` in `board-children.ts`). `board-card-children.tsx` renders lines recursively with their own toggle. **Expand all** still opens the first level only. No backend change.
  - Tests: `board-children.test.ts` adds 2 cases (8 in total). Packaged E2E adds "expands child lines down to their tasks, reading each level once" (Features card #102, story #101, tasks #301 and #302; exactly one read for the card level and one for the story level) and the phone case "expands nested child lines on a phone without horizontal scrolling". Both fail on the previous code, whose child lines have no toggle.
  - Verification: `make check-format vet test` passes; `make e2e` passes 36/36. Desktop and phone captures checked (indented level with a left border, no horizontal scrolling).
  - `contoso` data: the four features of Customer Offers [R1] have 3, 5, 27, and 1 stories (`boards.children` on the installed backend, which this amendment does not change). The UI check on `contoso` needs the rebuilt package installed.

