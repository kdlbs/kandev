---
created: 2026-08-30
status: done
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
legacy_specs: []
---

# Implementation Plan: Sidebar Task Colors

## Overview

Add the portable rule contract first. Then add rule resolution and repository identity before repository discovery and the responsive editor.

This order keeps persistence, derivation, and user interaction independently testable. The final work order proves the complete desktop and mobile flow.

Tasks 05 and 06 change the original manual-color boundary. They add atomic backend storage first, then migrate browser values and switch each client.

## Scope

### In scope

- Compact Sort and Group by disclosures.
- Personal automatic colors shared by all saved sidebar views and workspaces.
- Ordered first-match rules for seven task facts.
- Safe workflow-step color reuse.
- Shared dimension options with explicit differences from sidebar filters.
- Search and refresh for workspace, local, remote, and plugin repositories.
- Desktop and mobile rule editing.
- Portable settings, failure recovery, localization, and E2E coverage.
- Personal manual colors that synchronize through backend settings.
- A safe import of legacy browser colors.

### Out of scope

- Shared workspace colors or a task-owned color field.
- Additional rule dimensions.
- Rule import or export.
- Real-time cross-browser delivery of manual color changes.
- Public documentation. Existing public docs do not describe sidebar view settings.

## Technical approach

### Portable settings contract

Add `sidebar_task_color_automation` to the typed Go user settings model. Carry it through DTOs, service validation, stored JSON, and events.

Store rule order, targets, stored labels, and output selections in the `users.settings` JSON value. Preserve disabled incomplete rules and enforce exact field limits.

Add the matching wire and store types in the web application. Normalize missing or invalid values through the common user-settings mapper.

Add one serialized optimistic mutation helper. It sends a complete replacement and respects settings revisions.

Store manual colors as per-user task decisions in `users.settings`. Keep colors and clear tombstones separate from automatic rules.

Add a narrow per-task patch to `PATCH /api/v1/user/settings`. Apply each patch to the latest settings value during every CAS attempt.

Use `if_missing` patches for legacy import. Normal edits overwrite supplied task IDs. Clear operations store tombstones.

Read `kandev.taskColors` only after backend settings load. Import valid values in bounded batches, then remove the legacy browser value.

Replace the localStorage subscription with a selector for backend settings. Keep automatic colors as presentation precedence over the stored manual value.

### Dimension options and color presentation

Extract shared workflow and step option builders from `use-filter-value-options.ts`. Keep filter operators and filter matching in their current registry and engine.

Use explicit automatic-rule semantics for executor profile, raw task state, repository identity, priority, and normalized origin. Label the executor selector Executor profile.

Add a ten-color automatic palette without changing the seven-color manual menu. Add a safe workflow-step parser with a gray fallback for unsupported values.

### Rule resolution

Add a pure rule normalizer and resolver under `apps/web/lib/sidebar/`. Keep persisted color keys separate from CSS classes.

Extend `TaskSwitcherItem` with workspace, priority, origin, executor profile, step color, and repository identities. Update desktop, mobile, and archived projections.

Add the pure repository identity matcher with the rule resolver. Join task repository links to full repository records at each projection boundary.

Update the task color menu to identify the winning automatic rule. Do not remove the manual choice.

### Repository catalog

Compose saved repositories, local discovery, built-in remote providers, and plugin providers behind one catalog hook.

Add an explicit refresh generation to remote repository discovery. Reject late results and retain successful provider groups after partial errors.

Build workspace, provider, and local catalog options with the identity primitives from Task 02. Match provider options with provider, host, scope, and repository ID.

### Responsive editor

Extract the Task row header into a shared disclosure primitive. Use it for Sort, Group by, Task row, and Automatic colors.

Add ordered rule cards and a focused repository picker. Keep the mobile picker inside the existing drawer.

Create new rules as disabled incomplete rules. If the list contains 50 rules, disable Add rule and show a localized message.

Add localized copy in all shipped catalogs. Generate the Traditional Chinese catalogs with the existing script.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.*` | Disclosure component tests and sidebar Playwright geometry |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.*` | Dimension-option tests, rule resolver tests, projection tests, task-row tests, and live recoloring Playwright flows |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.*` | Repository catalog tests and repository picker Playwright flows |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.1` through `.4` | Backend user-settings tests and frontend mapper or mutation tests |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.5` through `.11` | Desktop and mobile Playwright flows |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.1` through `.5` | Backend patch, import, tombstone, and CAS tests |
| `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.6` through `.10` | Frontend migration, rollback, desktop, and mobile tests |

## E2E tests

- `apps/web/e2e/tests/task/sidebar-automatic-colors.spec.ts` covers ordered state rules, live recoloring, reload persistence, and the stored API value.
- `apps/web/e2e/tests/task/mobile-sidebar-automatic-colors.spec.ts` covers the drawer flow, touch repository selection, the focused picker pane, and reload persistence.
- `apps/web/e2e/tests/task/sidebar-task-color-sync.spec.ts` covers desktop manual-menu selection, backend persistence, reload, and legacy-key absence.
- `apps/web/e2e/tests/task/mobile-sidebar-task-color-sync.spec.ts` covers a server-backed manual color in the existing phone drawer.

## Work orders

- [x] [Task 01: Persist automatic color settings](task-01-persist-automatic-color-settings.md)
- [x] [Task 02: Resolve effective task colors](task-02-resolve-effective-task-colors.md)
- [x] [Task 03: Build repository target catalog](task-03-build-repository-target-catalog.md)
- [x] [Task 04: Deliver responsive automatic-color editor](task-04-deliver-responsive-automatic-color-editor.md)
- [x] [Task 05: Persist personal manual colors](task-05-persist-personal-manual-colors.md)
- [x] [Task 06: Adopt server-backed manual colors](task-06-adopt-server-backed-manual-colors.md)

## Verification results

- `go test ./internal/user/... ./internal/task/dto ./internal/task/handlers ./internal/task/service ./internal/backendapp`: 3,282 tests passed.
- `make -C apps/backend lint` and `make -C apps/backend build`: passed.
- Focused frontend Vitest suite: 12 files and 80 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:zh-hant`, `pnpm run i18n:check`, and `pnpm run e2e:sleep-ratchet`: passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check`: passed.
- Desktop and mobile automatic-color Playwright specs: one test passed in each project.
- Task 05 backend and frontend settings verification: 1,158 Go tests across 7 packages and 88 focused Vitest tests passed.
- Task 06 focused frontend verification: 76 Vitest tests passed; typecheck, lint, Traditional Chinese generation, and i18n checks passed.
- Task 06 desktop and mobile sync Playwright specs: one test passed in each project.

## Risks

- User-settings writes can race with other open clients. Revision-aware mapping and serialized writes must prevent stale rollback.
- Plugin repository providers can return late or malformed data. Generation guards and identity validation must isolate those results.
- Workflow-step colors use more keys than the manual task palette. One safe registry must cover both sets.
- Global rules can contain targets from another workspace. Stored workspace identity and unavailable labels must prevent collisions.
- Desktop, mobile, and archived task projections expose different facts today. Each projection needs focused coverage.
- A global setting inside a saved-view editor can appear view-specific. Visible scope copy must remove that ambiguity.
- Whole-map replacement can erase a concurrent browser's edit. Manual colors need per-task patches that reapply after CAS conflicts.
- A later browser can contain an old local color. Clear tombstones must win against every import-missing request.
- Legacy browser data can exceed one request. The frontend must use bounded batches and remove the key only after all batches succeed.

## Prerequisite

If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from `apps/` before the first package command.
