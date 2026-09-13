---
id: "01-auto-focus-preference"
title: "Implement saved task creation auto-focus preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATION-AUTO-FOCUS-001
acceptance_criteria:
  - AC-TASKS-CREATION-AUTO-FOCUS-001.1
  - AC-TASKS-CREATION-AUTO-FOCUS-001.2
  - AC-TASKS-CREATION-AUTO-FOCUS-001.3
  - AC-TASKS-CREATION-AUTO-FOCUS-001.4
  - AC-TASKS-CREATION-AUTO-FOCUS-001.5
system_design:
  - ../../specs/tasks/system-design/creation-auto-focus.md
---

# Task 01: Saved task creation auto-focus preference

## Summary

Implement the saved default-on preference end to end. Gate only task-opening
side effects, preserving successful task creation and launch work.

## In scope

User settings default/persistence/projection, settings UI and discovery,
localized copy, dialog success metadata and all focus-owning consumers,
desktop/mobile regression evidence, and public task documentation.

## Out of scope

New per-task controls, runtime flags, changed agent-start rules, Quick Chat,
edit/additional-session navigation changes, and background task-event focus.

## Acceptance

1. The setting survives save/reload and partial updates with a true missing-value default, preserving false; the shared save/discard/error flow owns persistence.
2. Every creation completion consumer honors suppression without losing cache updates, dialog dismissal, plan setup, or requested launch. Enabled/absent policy preserves current behavior.
3. Localized desktop and phone settings and creation flows pass the mapped tests, including switch hitbox/overflow checks and manual opening after background creation.

## ASCII UI preview

UI-01, Task Actions; [full preview and state notes](plan.md#ascii-ui-preview):

```text
Auto-focus new tasks                 [ ON ]
Open newly created tasks automatically.
Turn this off to stay on your current view.
Tasks and agents still start as requested.

Changed to OFF -> shared [ Save changes ]
```

Shared desktop/phone hierarchy; phone text wraps, the control has a >=44px
touch target, and the existing page owns scrolling. UI-02: after successful
creation with saved OFF, close the dialog and retain the current view. Covers
AC-TASKS-CREATION-AUTO-FOCUS-001.1–001.5.

## Verification

Read `/tdd`, `/e2e`, scoped AGENTS files, and the assigned design before coding.
Write failing behavior tests first, then implement. Run from the repository
root; each command roots itself. In a fresh worktree, first install dependencies
with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test ./internal/user/... ./internal/settingscatalog/...)
(cd apps/backend && go test ./internal/backendapp -run AutoFocusNewTasks -count=1)
(cd apps/backend && go run ./cmd/settings-catalog)
(cd apps/backend && go run ./cmd/settings-catalog --check)
(cd apps/web && pnpm exec vitest run creation-auto-focus task-create-dialog-submit task-create-dialog-helpers task-create-dialog.test.tsx app-sidebar-new-task-item canvas-task-create-launcher dockview-header-actions session-task-switcher-sheet kanban-board use-kanban-actions user-settings quick-task-launcher azure-devops-task-launcher improve-kandev-dialog app-sidebar-footer)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
make build-web
GOCACHE=/tmp/kandev-auto-focus-go-cache make -C apps/backend build-agentctl build-kandev build-mock-agent e2e-plugin-package
(cd apps/web && pnpm e2e:run --host --no-build --project=chromium e2e/tests/task/creation-auto-focus.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project=mobile-chrome e2e/tests/task/mobile-creation-auto-focus.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Name boot coverage `TestAutoFocusNewTasks...` so its command executes the new
test. Confirm the Vitest filter includes every modified caller suite; extend
the recorded exact command if inspection identifies another owner. Do not run
overlapping browser suites. Compare rendered UI to UI-01/UI-02 and record any
unresolved mismatch.

## Files likely touched

- `apps/backend/internal/user/{models/models.go,dto/dto.go,service/service.go,store/sqlite.go}` and handlers plus adjacent tests.
- `apps/backend/internal/backendapp/boot_state_routes.go` and a focused boot test; `apps/backend/internal/settingscatalog/defaults.go`.
- `apps/web/lib/types/http-user-settings.ts`, `lib/ssr/user-settings.ts`, settings slice/defaults, and discovery catalog/generated snapshots.
- New `apps/web/components/settings/creation-auto-focus-settings.tsx` and `creation-auto-focus-settings.test.tsx`; `general-settings.tsx`; five locale catalogs.
- `apps/web/components/task-create-dialog-{submit,helpers,types}.tsx` or `.ts` as existing, sidebar, canvas, board, task-header and mobile consumers named in the design, with their tests.
- `apps/web/hooks/domains/kanban/use-kanban-actions.ts` and its tests.
- GitHub/GitLab/Jira/Linear/Azure DevOps launchers and Improve Kandev success wrappers, plus their existing tests.
- New E2E files named above; `docs/public/tasks-and-workflows.md`.

## Dependencies

None. One sequential vertical slice.

## Risks

See [plan risks](plan.md#risks). Inventory success consumers with `rg` before
implementation; a guard in only the submit helper is incomplete.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/creation-auto-focus.md)
- [System design](../../specs/tasks/system-design/creation-auto-focus.md)
- Existing `prevent-auto-start-agent-settings.tsx` and `sqlite_prevent_auto_start_test.go` patterns.
- Existing `e2e/tests/task/create-task.spec.ts` and mobile settings/switcher tests.

## Results

Implementation and verification complete on 2026-09-13. Backend user/settings catalog and boot projection tests passed. The broad frontend run passed 315 tests across 25 files; subsequent focused runs passed the final dialog (11 tests), Azure launcher, and user-settings fixture changes. TypeScript, changed-file ESLint, Prettier, localization checks and ratchet, settings catalog consistency, native builds, and the web build passed. Desktop browser verification passed, including saved false after reload, retained listing/task URLs, opener focus, manual opening, background agent execution, and re-enabled navigation. Phone browser verification also passed, including the native Create only control, creation from the task switcher, saved preference, 44px touch target, no horizontal overflow, opener focus, background agent execution, and re-enabled navigation. Public documentation validation passed (62 tests, 46 pages). All acceptance criteria are covered; there is no unresolved UI mismatch. Changes remain uncommitted.

Native builds replace the original full `make build-backend` command: the full target also cross-compiles remote helpers for Linux ARM and macOS, which these browser scenarios do not exercise. The full target was stopped after native agentctl built. Go checks use `GOCACHE=/tmp/kandev-auto-focus-go-cache` because the default cache is read-only in this environment.


### Review remediation

Codex review identified detached drawer-opener focus and extended UI readiness
assertions. A phone E2E regression reproduced focus remaining on the document.
The drawer now passes its surviving opener to the creation dialog when
background creation is selected. The shared browser helper arms a wait for the
observed repository local-status response before opening creation, then checks
submit eligibility with the default assertion timeout. Local status supplies
the preferred default branch; the probe was removed after identifying this
causal chain.

Local validation: 17 tests passed with
`pnpm --dir apps/web exec vitest run session-task-switcher-sheet.test.tsx task-create-dialog.test.tsx`;
changed-file ESLint, TypeScript, and `build:vite` passed. The focused phone
scenario passed with the added task-menu focus assertion; the desktop scenario
also passed. Remote CI/review verification remains pending after the fix push.


Claude's summary suggested verifying background plan-state isolation. Retained
session-keyed plan initialization as required by AC .3, and strengthened the
helper test to use real application/context stores. It verifies the background
plan document, mode, and context are prepared while the current task/session,
document, plan mode, and context files remain unchanged, with no navigation.
`pnpm --dir apps/web exec vitest run task-create-dialog-helpers.test.ts`
passed all 28 tests. This is regression coverage for the existing contract;
no production behavior changed for this suggestion.
