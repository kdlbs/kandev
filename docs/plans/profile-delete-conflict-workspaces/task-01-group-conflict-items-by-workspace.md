---
id: "01-group-conflict-items-by-workspace"
title: "Group delete-conflict items by workspace"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DELETE-WORKSPACES-001
acceptance_criteria:
  - AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.1
  - AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.2
  - AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.3
  - AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.4
  - AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.5
  - AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.6
system_design:
  - ../../specs/agents/system-design/profile-delete-conflict-workspaces.md
---

# Task 01: Group delete-conflict items by workspace

## Summary

Return workspace ID and name for each affected task in the profile delete 409
body, and render the conflict dialog's Tasks and Quick Chats sections grouped
under workspace headings, with a localized "Unknown workspace" fallback.

## In scope

- `ActiveTaskInfo` fields and the `GetActiveTaskInfoByAgentProfile` query.
- `ActiveSessionInfo` type, `groupSessionsByWorkspace` helper, and
  `SessionConflictSection` rendering.
- `agents:conflictUnknownWorkspace` in all locales.
- Go, Vitest, and Playwright tests listed in the plan.

## Out of scope

- Other conflict sections, task links, Office agent deletion, and the
  blocking-session predicate.

## Acceptance

- The 409 body's `active_sessions` rows carry `workspace_id` and
  `workspace_name`; a task whose workspace row is missing returns `""` name.
- The dialog groups tasks and quick chats by workspace with the ordering and
  fallback rules of AC .3 and .4.
- All listed verification commands pass.

## ASCII UI preview

`UI-01` from [the plan](plan.md#ascii-ui-preview), desktop excerpt:

```text
| Tasks:                                       |
|   Backend                                    |
|    - Fix login                               |
|    - Refactor auth                           |
|   Frontend                                   |
|    - Add billing page                        |
| Quick Chats:                                 |
|   Unknown workspace                          |
|    - Untitled quick chat                     |
```

Applies to AC .1, .2, .4, .5.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite/ -run 'TestAgentProfileBlockers' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/settings/... -count=1)
(cd apps/web && pnpm exec vitest run lib/settings/agent-profile-delete-conflict.test.ts components/settings/agent-profile-delete-dialog.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --docker --shards 1 --project chromium tests/settings/agent-profile-delete.spec.ts)
```

## Files likely touched

- `apps/backend/internal/agent/dto/dto.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/agent_profile_blockers_workspace_test.go` (new)
- `apps/web/lib/types/agent-profile-errors.ts`
- `apps/web/lib/settings/agent-profile-delete-conflict.ts` (new)
- `apps/web/lib/settings/agent-profile-delete-conflict.test.ts` (new)
- `apps/web/components/settings/agent-profile-delete-dialog.tsx`
- `apps/web/components/settings/agent-profile-delete-dialog.test.tsx`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/agent-profile-delete.spec.ts`

## Dependencies

None.

## Risks

- The dialog function already carries a complexity lint suppression; keep the
  grouping in the helper and `SessionConflictSection` rather than the dialog.
- Fresh worktree needs `pnpm install --frozen-lockfile` in `apps/` first.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/profile-delete-conflict-workspaces.md)
- [System design](../../specs/agents/system-design/profile-delete-conflict-workspaces.md)
- Existing `AutomationConflictSection` workspace label pattern in
  `agent-profile-delete-dialog.tsx`.

## Results

- `go test -tags fts5 ./internal/task/repository/sqlite/ -run 'TestAgentProfileBlockers' -count=1`: pass (new
  `TestAgentProfileBlockersIncludeWorkspace`, `TestAgentProfileBlockersNameEachTasksWorkspace`, and
  `TestAgentProfileBlockersKeepTaskWithMissingWorkspace` failed before the query change).
- `go test -tags fts5 ./internal/agent/settings/... -count=1`: pass.
- `vitest run lib/settings/agent-profile-delete-conflict.test.ts components/settings/agent-profile-delete-dialog.test.tsx`:
  22 passed (new grouping tests failed before implementation).
- `pnpm run typecheck` (with `NODE_OPTIONS=--max-old-space-size=12288`) and `pnpm run i18n:check`: pass.
- `pnpm e2e:run --docker --shards 1 --project chromium tests/settings/agent-profile-delete.spec.ts`:
  6 passed. Host mode was unavailable locally (no Playwright browser on NixOS).
- `bash scripts/lint-go-changed` on changed Go files: 0 issues; ESLint on changed TS files: clean.
