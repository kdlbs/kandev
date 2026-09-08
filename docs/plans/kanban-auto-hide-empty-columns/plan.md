---
spec: docs/specs/ui/requirements/kanban-auto-hide-empty-columns.md
created: 2026-08-18
status: complete
---

# Implementation Plan: Auto-hide empty workflow steps

## Overview

Extend the manual column-visibility feature with a separate, opt-in per-workflow preference. Persist
only the preference. Derive empty steps from the rendered task projection, keep manual hiding
authoritative, and temporarily restore auto-hidden steps as drop targets while a move is active.

The implementation is sequential because the frontend behavior depends on the persisted settings
contract and drag behavior touches shared Kanban and Pipeline composition.

## Existing seams to reuse

- User display settings round-trip in `apps/backend/internal/user/**`, boot state, REST hydration,
  WebSocket echo, and `apps/web/hooks/use-user-display-settings.ts`.
- Per-workflow Columns UI in `apps/web/components/kanban/columns-menu.tsx`.
- Workflow-scoped visibility handling in `use-kanban-display-settings.ts` and
  `swimlane-container.tsx`.
- Kanban drag state in `swimlane-kanban-content.tsx`, Pipeline drag state in
  `swimlane-graph-content.tsx`, and phone drop targets in `mobile-drop-targets.tsx`.
- Existing manual-hidden move-target filtering in `kanban-board.tsx`.

## Data model and API

- Add `WorkflowIDsWithAutoHideEmptySteps []string` to the existing Go user-settings model and DTO.
- Add optional update field `workflow_ids_with_auto_hide_empty_steps` and propagate it through controller,
  service, storage JSON, boot payload, and user-settings events.
- Add `workflowIdsWithAutoHideEmptySteps: string[]` to frontend settings state and hydrate/persist it through
  existing settings helpers.
- Normalize as a sorted, deduplicated list. Default legacy state to `[]`.
- No new endpoint, SQL column, or migration.

## Frontend behavior

### Columns control

- Add a translated display-behavior section at the top of `ColumnsMenu`, separated from the manual
  column list, with no secondary description under the switch.
- Add `autoHideEmpty`, `onToggleAutoHide`, and touch-target-safe rendering to the shared component.
- Reuse the same component in the desktop/tablet swimlane header and phone drawer.

### Occupancy projection

- Extract a pure helper that returns occupied step ids from tasks after workflow, repository, and
  plugin filters but before free-text search.
- Derive auto-hidden live step ids only for workflows whose preference is enabled.
- Keep the manual hidden set separate and authoritative.
- Preserve the lane/header when all real steps are auto-hidden and render translated contextual
  guidance.

### Move presentation

- Pass all non-manually-hidden live steps as move targets.
- In Kanban DnD, reveal auto-hidden steps as droppable targets while drag state is active, without
  duplicating droppable ids. Keep Pipeline compact while its existing move controls retain the full
  non-manually-hidden step list.
- Keep Bulk Move behavior unchanged except that auto-hidden steps remain available.
- Reuse the existing phone drop-target surface and preserve mobile navigation.

## Internationalization

Add English source keys and complete `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw` catalogs for:

- auto-hide toggle label;
- conditional Pipeline destination tooltip copy;
- all-columns-empty contextual state;
- optional drag-target explanation if the existing accessible step title is insufficient.

No Unicode em dash is permitted in locale values.

## Public documentation

Update `docs/public/tasks-and-workflows.md` to distinguish:

- manually hidden columns, which stay hidden and are not move targets;
- auto-hidden empty columns, which return as destinations during a move.

Update the feature-spec index with the new spec.

## Tests

- Backend model/DTO/controller/service/store/boot tests for persistence and defaults.
- Frontend settings normalization, equality, payload, hydration, and toggle-hook tests.
- Pure occupancy and effective-step helper tests, including search stability and workflow isolation.
- Columns menu tests for default-off, toggling, touch geometry, and manual-state independence.
- Kanban/Pipeline component tests for ordinary hiding and drag-time targets.
- Chromium E2E for enable, persistence, collapse, drag reveal, cancel, successful drop, and manual-hidden
  precedence.
- Mobile Chrome E2E for the focused workflow, touch target, move destination, and no overflow.

## Implementation tasks

- [x] [Task 01 - Persist the per-workflow preference](task-01-persist-preference.md)
- [x] [Task 02 - Derive and render empty-column visibility](task-02-derived-visibility.md)
- [x] [Task 03 - Restore auto-hidden move targets](task-03-drag-targets.md)
- [x] [Task 04 - Prove end-to-end behavior and document it](task-04-e2e-docs.md)

All tasks are sequential. Tasks 02-04 depend on the contracts established by the preceding task and
touch overlapping Kanban composition or acceptance tests.

## Risks

- Search-driven layout churn if occupancy is derived at the wrong filtering seam.
- Duplicate DnD droppable ids if ghost targets and real columns mount together.
- A lane becoming unreachable when every column is auto-hidden.
- Manual and automatic state accidentally sharing persistence or move-target semantics.
- Incomplete locale updates failing the repo-wide i18n gates.

## Planned verification

```bash
make fmt
make typecheck
make test
make lint
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/kanban/auto-hide-empty-columns.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/kanban/mobile-auto-hide-empty-columns.spec.ts)
```

## Verification results

- Targeted Prettier, ESLint, and the E2E sleep ratchet passed for the new coverage and selectors.
- Chromium passed 1/1 test, covering default-off compatibility, persisted enablement, search-stable
  occupancy, manual-hidden precedence, drag reveal, cancellation, and successful drop.
- Mobile Chrome passed 1/1 test, covering focused-workflow persistence, the 44 CSS px toggle,
  drag-time destination recovery, cancellation cleanup, and document overflow.
- The E2E backend required a dedicated `KANDEV_AGENT_STANDALONE_PORT` locally because the live Kandev
  runtime already owned the default port; both successful runs used isolated temporary databases.
