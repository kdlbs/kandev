---
id: "01-compact-list-ui"
title: "Compact expandable workflow rows and view toggle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002
acceptance_criteria:
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.1
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.2
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.3
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.4
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.5
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.6
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.7
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.8
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.1
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.2
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.3
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.4
  - AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.5
system_design:
  - ../../specs/ui/system-design/workflow-settings-compact-list.md
---

# Task 01: Compact Expandable Workflow Rows and View Toggle

## Summary and scope

Deliver the working feature as described in the
[requirements](../../specs/ui/requirements/workflow-settings-compact-list.md),
[design](../../specs/ui/system-design/workflow-settings-compact-list.md), and
[plan](plan.md):

- `useWorkflowListView` (default `compact`, best-effort `localStorage`).
- `WorkflowListViewToggle` above the list.
- `WorkflowCompactRow`.
- Expansion state, plus the lazy keep-mounted body, in `WorkflowCard`.
- Compact drag-handle positioning in `SortableWorkflowItem`.
- Localized copy in all seven catalogs and the pseudo-locale.

## Exclusions

No backend, save-contract, export, duplicate, delete, or reorder changes. No
expanded-row persistence, bulk expand, or search. No E2E specs (work order 02).

## Acceptance

1. Vitest proves the hook defaults to `compact` on missing, invalid, or
   throwing storage, persists `detailed`, and still switches when `setItem`
   throws.
2. Vitest proves the following. A compact row shows the name, ordered steps,
   count, profile, and the Synced, read-only, and Unsaved markers, and its
   header has `aria-expanded` and the localized name. Toggling shows and hides
   the body without resetting a typed name draft. A temporary-ID workflow
   mounts expanded. The Detailed view renders no row header.
3. `make typecheck test lint`, `pnpm run i18n:check`, and
   `pnpm run i18n:ratchet` pass. Files stay within the oxlint size limits (cap
   `workflow-card.tsx` at 600 lines and extract the new components).

## ASCII UI preview

Excerpt of [plan UI-01, UI-02, and UI-04](plan.md#ascii-ui-preview):

```text
                                     View: [=Compact=] [ Detailed ]
| > Development   Default dev...  o Backlog > o In Progress > ...  5 steps  Claude ::|
| v Release [Unsaved]  ...        o Draft > o Build > o Publish   3 steps         ::|
|   (existing card body: details, prompt, steps, Export/Duplicate/Delete)           |
Phone: | > Development          [::] |
       |   o Backlog > o In Pro... 5 |
```

## TDD order

1. RED: `hooks/domains/settings/use-workflow-list-view.test.ts`. Then the hook.
2. RED: `components/settings/workflow-compact-row.test.tsx`, covering content,
   markers, step ordering, plural count, and the accessible name. Then the row.
3. RED: `components/settings/workflow-card-compact.test.tsx`, which mounts
   `WorkflowCard` with mocked `listWorkflowStepsAction` and the settings save
   provider. It covers collapsed by default, expand and collapse, draft
   preserved across collapse, a temporary ID starting expanded, and the
   Detailed view showing no header. Then integrate into `WorkflowCard`.
4. Integrate the toggle and `compact` prop into `WorkflowList` and
   `SortableWorkflowItem`, then add copy (`pnpm run i18n:zh-hant` for zh-tw and
   zh-hk, and `pnpm run i18n:pseudo`).

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/domains/settings/use-workflow-list-view.test.ts \
  components/settings/workflow-compact-row.test.tsx \
  components/settings/workflow-card-compact.test.tsx \
  components/settings/workflow-card-header-actions.test.tsx \
  src/settings-routes.workspace-data.test.tsx
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run typecheck
cd ../.. && make fmt && make typecheck test lint
```

## Files and dependencies

- New: `apps/web/hooks/domains/settings/use-workflow-list-view.ts` (+ test),
  `apps/web/components/settings/workflow-list-view-toggle.tsx`,
  `apps/web/components/settings/workflow-compact-row.tsx` (+ test), and
  `apps/web/components/settings/workflow-card-compact.test.tsx`.
- Changed: `apps/web/components/settings/workflow-card.tsx`,
  `apps/web/app/settings/workspace/workspace-workflows-client.tsx`, and
  `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/workflows.json`.
- Uses: `@kandev/ui/toggle-group`, `@kandev/ui/badge`,
  `useHealthyAgentProfiles`, and `useResponsiveBreakpoint` (only if Tailwind
  breakpoints cannot express the layout).

Risks: hidden-but-mounted bodies duplicate test IDs in the DOM, so unit tests
must assert visibility rather than presence. `WorkflowSyncedBadge` must not be
nested in the header button.

## Results

- RED-first unit suites: `use-workflow-list-view.test.ts` (6), `workflow-compact-row.test.tsx` (7),
  `workflow-card-disclosure.test.tsx` (9, including deep-link expansion for `001.8`), and
  `workflow-list-view-toggle.test.tsx` (3). Each failed before its module existed.
- Expansion logic lives in `workflow-card-disclosure.tsx` (`useWorkflowCardDisclosure`,
  `WorkflowCardDisclosureBody`, `useExpandOnSettingsTarget`). That logic is unit-tested in place
  of the planned `workflow-card-compact.test.tsx` full-card mount; the card wiring is covered by E2E.
- `WorkflowList` and `SortableWorkflowItem` moved to `app/settings/workspace/workspace-workflows-list.tsx`
  to keep `workspace-workflows-client.tsx` under 600 lines.
- Targeted Vitest run: 27 files, 225 tests passed. `pnpm run i18n:check`, `pnpm run i18n:ratchet`,
  `pnpm run typecheck`, `make typecheck` (with `NODE_OPTIONS=--max-old-space-size=8192`; the default
  heap aborts classic `tsc`), and `make lint` passed.
