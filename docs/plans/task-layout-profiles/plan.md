---
spec: docs/specs/ui/task-layout-profiles.md
created: 2026-07-19
status: complete
---

# Implementation Plan: Task Layout Profiles

## Overview

Extend the existing `saved_layouts` user setting instead of introducing new persistence. First harden profile-list validation and add pure reusable-layout operations, then build the responsive settings editor and connect the workbench default path, and finally prove persistence and task isolation in Playwright. The runtime remains compatible with the existing layout menu and environment-scoped layout restoration.

## Backend

### Saved layout validation

- `apps/backend/internal/user/service/service.go`: extend `applySavedLayouts` to reject empty IDs, duplicate IDs, and more than one `is_default` profile while retaining the existing count and name checks.
- `apps/backend/internal/user/service/service_test.go`: add table-driven validation coverage, including acceptance of zero or one custom default.

No schema, repository, DTO, or endpoint changes are required; `users.settings.saved_layouts` and the existing GET/PATCH contract remain authoritative.

## Frontend

### Profile domain

- Add `apps/web/lib/layout/layout-profiles.ts` and focused tests for built-in template descriptors, reusable-layout validation, effective-default resolution, immutable create/rename/duplicate/delete/default operations, and legacy-layout compatibility status.
- Reuse `LayoutState`, `PANEL_REGISTRY`, `getPresetLayout`, session-panel normalization, and the existing `SavedLayout` HTTP type. Editor-created layouts use canonical reusable panel IDs and never introduce task-specific IDs.
- Extract shared profile mutations from `apps/web/components/task/layout-preset-selector.tsx` so the task menu and settings page cannot diverge on default/delete behavior.

### Settings page and editor

- Add `apps/web/app/settings/general/layouts/page.tsx` with the existing user-settings hydration pattern.
- Add `apps/web/components/settings/layouts/layout-settings.tsx` for profile selection, create/duplicate/rename/delete/default/reset commands, dirty-state handling through the shared Settings floating save coordinator, error feedback, and narrow-viewport composition.
- Add focused editor components under `apps/web/components/settings/layouts/` that render lightweight panel placeholders, use the existing Dockview layout engine for pointer/touch split and tab manipulation, and expose contextual keyboard/touch commands beside the selected split. Every command has hover/focus help. Agent is permanent; reusable panels are single-instance.
- Represent direct built-in edits as reserved hidden overrides so each built-in remains one visible row with `Built-in` and optional `Customized` status. Reset removes the reserved override and restores the code-defined layout.
- The preview is isolated from `useDockviewStore` and runtime panel components so Browser, VS Code, and Terminal have no runtime side effects in Settings.
- Add `Layouts` to `apps/web/components/settings/general-nav.ts` and the settings tree; register `/settings/general/layouts` in `apps/web/src/settings-routes.tsx` and cover route resolution.

### Workbench integration

- Add a compatibility resolver used by `apps/web/components/task/dockview-desktop-layout.tsx` so only a valid reusable custom default reaches `setUserDefaultLayout`; invalid legacy defaults fall back to the built-in Default.
- Preserve `apps/web/lib/state/dockview-store.ts` precedence: environment layout first, effective user default for fresh/reset paths second, built-in fallback last.
- Verify `apps/web/hooks/domains/session/use-ensure-default-terminal-ordinary.ts` remains panel-driven, so omitting `terminal-default` prevents `createUserShell`.
- Keep `apps/web/components/task/layout-preset-selector.tsx` behavior compatible while reusing profile operations and making deletion of the selected custom default explicitly fall back to the built-in Default.

## Tests

- **Saved profile list validation:** `apps/backend/internal/user/service/service_test.go`; table-driven tests cover blank/duplicate IDs, multiple defaults, limits, names, and valid zero/one-default lists.
- **Profile operations and effective default:** `apps/web/lib/layout/layout-profiles.test.ts`; pure unit tests cover create, duplicate, rename, delete-default fallback, set-default uniqueness, valid layout requirements, and legacy compatibility.
- **Default layout resolution:** focused tests for the resolver used by `dockview-desktop-layout.tsx`; valid reusable layouts are normalized and invalid/unreadable defaults return `null`.
- **Terminal side-effect guard:** `apps/web/hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts`; no `terminal-default` panel means no `createUserShell`, while the existing panel path still migrates once.
- **Settings SPA route:** `apps/web/src/settings-routes.test.ts` and general navigation tests cover the Layouts route without direct component fetches.

## E2E Tests

- **Desktop profile workflow:** `apps/web/e2e/tests/settings/layout-profiles.spec.ts`; edit built-in Default directly, remove Terminal with contextual controls, verify action help, save through the shared floating control, and reset the built-in override.
- **Fresh task and reset behavior:** the same desktop spec creates and opens a fresh task, verifies no Terminal tab/default shell, proves an existing customized task is unchanged after a default update, then verifies Reset Layout applies the latest default.
- **Mobile settings parity:** `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`; navigate through the settings drawer, select and reorder a built-in tab with the contextual touch controls, save its hidden override, and verify there is no horizontal page scroll.
- **Existing layout regressions:** run `apps/web/e2e/tests/task/task-default-layout.spec.ts` and `apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts` with the new specs.

## Public Documentation

- Update `docs/public/sessions-and-review.md` with the Layouts settings path, effective-default behavior, and the distinction between reusable defaults and task-specific restored layouts.

## Implementation Waves

Wave 1 (parallel in the shared worktree):

- [x] [Task 01 - Saved layout validation](task-01-saved-layout-validation.md)
- [x] [Task 02 - Layout profile domain](task-02-layout-profile-domain.md)

Wave 2 (parallel):

- [x] [Task 03 - Layout settings editor](task-03-layout-settings-editor.md)
- [x] [Task 04 - Workbench default integration](task-04-workbench-default-integration.md)

Wave 3:

- [x] [Task 05 - E2E, docs, and verification](task-05-e2e-docs-verification.md)

The user authorized subagents after the approval checkpoint. Tasks 01 and 02 ran in parallel with separate file ownership; Tasks 03 and 04 do the same in Wave 2. The parent agent owns shared plan synchronization and integration review. Read-only editor and E2E research informed the implementation tasks.

## Risks

- Existing saved layouts may contain task-specific or old serialized Dockview payloads. Detection must not mutate or discard them silently.
- Dockview pointer behavior alone is insufficient on mobile and for keyboard users. Explicit move/split commands are part of acceptance, not a fallback enhancement.
- Settings previews must never mount real task panels or start terminal/editor/browser resources.
- Environment-scoped restoration must remain higher precedence than the new default, or changing Settings would unexpectedly rewrite active work.
- User-setting updates replace the full profile list, so each mutation must use the latest authoritative store value and adopt the response to avoid lost updates.

## Verification

```bash
make fmt
make typecheck
make test
make lint
make build-web
cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts tests/task/task-default-layout.spec.ts tests/layout/saved-layout-session-isolation.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/settings/mobile-layout-profiles.spec.ts --workers=1
```
