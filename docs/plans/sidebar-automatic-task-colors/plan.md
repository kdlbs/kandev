---
created: 2026-08-30
status: implemented
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
legacy_specs: []
---

# Implementation Plan: Sidebar Automatic Task Colors

## Overview

Add the portable rule contract first. Then add rule resolution and repository identity before repository discovery and the responsive editor.

This order keeps persistence, derivation, and user interaction independently testable. The final work order proves the complete desktop and mobile flow.

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

### Out of scope

- Shared workspace rules or a persisted per-task color.
- Additional rule dimensions.
- Rule import or export.
- Public documentation. Existing public docs do not describe sidebar view settings.

## Technical approach

### Portable settings contract

Add `sidebar_task_color_automation` to the typed Go user settings model. Carry it through DTOs, service validation, stored JSON, and events.

Store rule order, targets, stored labels, and output selections in the `users.settings` JSON value. Preserve disabled incomplete rules and enforce exact field limits.

Add the matching wire and store types in the web application. Normalize missing or invalid values through the common user-settings mapper.

Add one serialized optimistic mutation helper. It sends a complete replacement and respects settings revisions.

Keep the existing manual task-color store device-local. Do not copy manual colors into backend settings.

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

## E2E tests

- `apps/web/e2e/tests/task/sidebar-automatic-colors.spec.ts` covers ordered state rules, live recoloring, reload persistence, and the stored API value.
- `apps/web/e2e/tests/task/mobile-sidebar-automatic-colors.spec.ts` covers the drawer flow, touch repository selection, the focused picker pane, and reload persistence.

## Work orders

- [x] [Task 01: Persist automatic color settings](task-01-persist-automatic-color-settings.md)
- [x] [Task 02: Resolve effective task colors](task-02-resolve-effective-task-colors.md)
- [x] [Task 03: Build repository target catalog](task-03-build-repository-target-catalog.md)
- [x] [Task 04: Deliver responsive automatic-color editor](task-04-deliver-responsive-automatic-color-editor.md)

## Verification results

- `go test ./internal/user/... ./internal/task/dto ./internal/task/handlers ./internal/task/service ./internal/backendapp`: 3,282 tests passed.
- `make -C apps/backend lint` and `make -C apps/backend build`: passed.
- Focused frontend Vitest suite: 12 files and 80 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:zh-hant`, `pnpm run i18n:check`, and `pnpm run e2e:sleep-ratchet`: passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check`: passed.
- Desktop and mobile automatic-color Playwright specs: one test passed in each project.

## Risks

- User-settings writes can race with other open clients. Revision-aware mapping and serialized writes must prevent stale rollback.
- Plugin repository providers can return late or malformed data. Generation guards and identity validation must isolate those results.
- Workflow-step colors use more keys than the manual task palette. One safe registry must cover both sets.
- Global rules can contain targets from another workspace. Stored workspace identity and unavailable labels must prevent collisions.
- Desktop, mobile, and archived task projections expose different facts today. Each projection needs focused coverage.
- A global setting inside a saved-view editor can appear view-specific. Visible scope copy must remove that ambiguity.

## Prerequisite

If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from `apps/` before the first package command.
