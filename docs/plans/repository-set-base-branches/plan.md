---
created: 2026-09-01
status: complete
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
  - REQ-WORKSPACES-REPOSITORY-SETS-002
  - REQ-WORKSPACES-REPOSITORY-SETS-003
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
legacy_specs: []
---

# Implementation Plan: Repository Set Base Branches

## Overview

Add an optional base branch to each repository-set member. Then expose that
value in the approved inline editor and copy it into new task drafts.

The backend contract comes first because every later task consumes the member
shape. Task-draft behavior comes next because it defines local and worktree
semantics. The responsive editor, E2E evidence, and public documentation follow.

## Scope

### In scope

- Persist one optional base branch for each set member.
- Preserve `repository_ids` as a compatibility input.
- Show selected members and base selectors in one searchable editor.
- Load a member branch list only when its selector opens.
- Copy saved bases into new task rows without changing existing rows.
- Keep local base and checkout branches separate.
- Capture effective bases through `Save as set`.
- Cover desktop and phone workflows.

### Out of scope

- One bulk branch value for all members.
- Branch policies, generated branch templates, and pull-request targets in sets.
- Remote URLs, local folder sources, and Quick Chat.
- Automatic replacement of unavailable saved bases.

## Technical approach

### Persistence and service contract

Add `base_branch` to `repository_set_items` in the base schema and replayable
migrations. Extend models, repository scans, writes, DTOs, and event payloads.

Replace ordered ID slices inside the service with ordered member inputs. Keep
`repository_ids` at the handler boundary for compatibility. Reject requests
that contain both member fields.

Use `securityutil.IsValidBranchName` for each non-empty base. Do not query Git
inside set mutations.

### Task-draft behavior

Extend `TaskRepoRow` with explicit base state. Update `applyRepositorySet` to
copy a saved base only into a new row. Keep the idempotent skip rule for a row
that is already present.

Update task payload construction so worktree execution uses the effective base.
For local execution, send the effective base and checkout branch separately.

Update `Save as set` to send ordered member objects. The dialog summary states
that repository bases are included.

### Responsive settings editor

Replace the all-repository checkbox list with a selected-member list and an Add
repository picker. Keep explicit move actions for order.

Each row shows `Task default` or its saved base. Enable `useBranches` only
while that row selector is open. Keep an unavailable saved value visible.

Reuse New Task `BranchSelector`, `sortBranches`, and `branchToOption` without a
settings-only fork. Preserve branch search, refresh, remote-qualified names,
and origin badges.

Use a bounded desktop dialog. Use the repository branch-policy full-height
drawer pattern on phones. The phone drawer owns one scroll region and a
safe-area footer.

### State, events, and localization

Extend HTTP, WebSocket, store, and boot-state types with `base_branch`. Add all
new copy to English, Portuguese, Simplified Chinese, and Traditional Chinese
catalogs. Generate both Traditional Chinese variants with the current script.

### Public documentation

Update the repository-set section in `docs/public/tasks-and-workflows.md`. State
how saved bases, task defaults, repository defaults, and unavailable branches
work.

## Tests

| Acceptance criteria                                                          | Evidence                                                                                                     |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `AC-WORKSPACES-REPOSITORY-SETS-001.8`, `AC-WORKSPACES-REPOSITORY-SETS-002.1` | Repository and service tests for migration, ordered persistence, validation, and atomic replacement.         |
| `AC-WORKSPACES-REPOSITORY-SETS-003.2` through `.8`                           | Pure apply tests and task-payload tests for idempotence, unavailable values, and local semantics.            |
| `AC-WORKSPACES-REPOSITORY-SETS-002.2` through `.9`                           | Component tests for rows, shared branch search, origin labels, reset, lazy loading, and responsive surfaces. |
| `AC-WORKSPACES-REPOSITORY-SETS-003.9`                                        | Save-as-set component and API tests for ordered member objects.                                              |

## E2E tests

- Desktop settings creates and edits saved bases in
  `apps/web/e2e/tests/settings/workspace-repository-sets.spec.ts`. The test also
  proves search, remote-qualified names, origin badges, and refresh.
- Desktop task creation applies those bases in
  `apps/web/e2e/tests/task/create-task-repository-sets.spec.ts`.
- Phone settings proves the full-height editor, internal scroll, safe footer,
  touch targets, and selector containment in
  `mobile-workspace-repository-sets.spec.ts`.
- Phone task creation proves the same saved-base outcome in
  `mobile-create-task-repository-sets.spec.ts`.

## Work orders

- [x] [Task 01: Persist member base branches](task-01-persist-member-base-branches.md)
- [x] [Task 02: Apply saved bases to task drafts](task-02-apply-saved-bases.md)
- [x] [Task 03: Build the responsive inline editor](task-03-responsive-inline-editor.md)
- [x] [Task 04: Prove saved-base workflows](task-04-saved-base-e2e.md)
- [x] [Task 05: Document repository-set bases](task-05-public-documentation.md)

## Verification results

- Backend focused packages pass: `go test ./internal/task/repository/sqlite
./internal/task/service ./internal/task/dto ./internal/task/handlers
./internal/backendapp`.
- Frontend focused tests pass: 8 files, 103 tests. The task-draft and editor
  work-order commands also pass independently with 82 and 46 tests.
- `pnpm run typecheck`, `pnpm --filter @kandev/web lint`, `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, and the changed-file E2E lint pass.
- `make build-web` passes. Desktop repository-set E2E passes 7 tests, and mobile
  repository-set E2E passes 3 tests.
- Public documentation validation passes 61 tests and validates 41 published
  pages. Specification lint passes all files.
- The full-repository `lint:e2e-sleeps` command still reports unrelated existing
  errors in other files; the five changed E2E files pass its rule directly.

## Risks

- The current task row overloads one branch value across executor types. The
  implementation must not map a saved base to a local checkout.
- Large sets can cause many Git requests if branch hooks become active during
  row render. Selector-open state must gate each request.
- SQLite and PostgreSQL both use the repository-set schema path. The migration
  and scans must change together.
- Old clients send `repository_ids`. Removing this input can break scripts and
  existing E2E helpers.
- An unavailable branch can persist after a remote deletion. The UI must keep
  the value visible and block unintended substitution.
