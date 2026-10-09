---
id: "03-edit-layouts"
title: "Build the desktop and phone settings editor"
status: done
wave: 3
depends_on:
  - "02-resolve-shortcuts"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.4
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 03: Build the desktop and phone settings editor

## Summary

Deliver Appearance > Sidebar with a draft preview, group editing, and a searchable picker. Use the shared save coordinator and focused phone settings flow.

## In scope

- SPA route and settings discovery; visibility, group CRUD, drag/drop, explicit moves, picker readiness and errors.
- Workspace-bound draft, saved revision, save/discard/reset/conflict handling, locale catalogs, and phone composition.

## Out of scope

- Applying saved layouts to production navigation; live automation bubbles.

## Acceptance

- Users can build the four-shortcut example, move entries, hide sections, and save/reload through both editor presentations.
- Save failures/conflicts preserve drafts; workspace changes never cross-apply edits.
- Phone flow has one scroll owner, 44px targets, accessible moves, and localized validation.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview). These views implement the
acceptance criteria listed above.

### UI-01: Desktop editor, draft with hidden entries

Entry: Appearance > Sidebar or Customize sidebar.

```text
Sidebar                         Workspace: Default
Changes apply to your navigation in this workspace.

LAYOUT                           DRAFT PREVIEW
:: Home                 [off]    > Shortcuts [GH][S][C][A]
:: New Task              [on]    Tasks
:: Shortcuts             [on]      ...
   Name [Shortcuts        ]
   :: [GH] GitHub          [x]
   :: [S]  Slack           [x]
   :: [C]  My canvas       [x]
   :: [A]  My automation   [x]
   [+ Add shortcut]
:: Automations          [off]
:: Canvases             [off]
:: Integrations         [off]
   Tasks                [fixed]
[+ Add shortcut section]  [Restore defaults]
                 [Discard] [Save changes]*
```

`::` is a drag handle with explicit move controls. `*` is the existing shared
save control, not a new local footer. The draft preview does not change live
navigation. Fixed Tasks and required inbox entries remain outside reorder scope.

### UI-02: Phone editor, focused section

Entry: Menu > Customize sidebar. Full-page settings navigation.

```text
< Sidebar              Default
Shortcuts
Name [Shortcuts              ]

[GH] GitHub               [...]
[S]  Slack                [...]
[C]  My canvas            [...]
[A]  My automation        [...]
[+ Add shortcut]

... menu:
  Move up
  Move down
  Move to section >
  Remove

        [Save changes]*
```

Back returns to the layout list with visibility switches. Add shortcut opens a
searchable picker; selection returns to this section draft. The page has one
scroll owner and safe-area clearance. Explicit move controls are always reachable.

### UI-03: Picker and save failures, shared content

```text
< Add shortcut
[Search shortcuts...       ]
Built-in / Plugins / Canvases / Automations
[GH] GitHub
[S]  Slack
[C]  My canvas
[A]  My automation

Empty: No matching shortcuts
Loading: Loading shortcuts...
Error: Could not load shortcuts [Retry]

Save conflict: Layout changed elsewhere.
Your draft is retained. [Load latest]
Save error: Could not save changes. [Retry]
```

Desktop uses the existing picker overlay; phone uses the focused picker step.
Loading, empty, and error are alternatives, not simultaneous messages.

## Verification

Run from the repository root. Install dependencies once with
`(cd apps && pnpm install --frozen-lockfile)` if this worktree has no install.
Use TDD for new logic and the named E2E scenarios. Managed E2E commands rebuild
assets; run desktop and phone commands sequentially without worker overrides.

```bash
(cd apps/web && pnpm exec vitest run components/settings/sidebar-layout-editor.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/sidebar-customization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-customization.spec.ts)
```

## Files likely touched

- `apps/web/app/settings/sidebar/page.tsx (new)`
- `apps/web/components/settings/sidebar-layout-editor.tsx and focused helpers (new)`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/settings-save-provider.tsx (reuse)`
- `apps/web/lib/settings-discovery/`
- `apps/web/src/spa-routes.tsx`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/*sidebar-customization.spec.ts (new)`

## Dependencies

02-resolve-shortcuts.

## Risks

The settings takeover hides the regular sidebar. Keep an explicit draft preview so editing remains understandable.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), IDs above.
- [System design](../../specs/ui/system-design/sidebar-customization.md), corresponding sections.
- [Plan](plan.md), test matrix and previews.
- Scoped `apps/web/AGENTS.md` and `apps/backend/AGENTS.md` where applicable.

The editor E2E assertions in this work order target saved settings and the draft
preview. Task 04 extends these same tests with live navigation assertions.

## Results

Implemented the `/settings/sidebar` editor with workspace-bound drafts,
visibility and section controls, shortcut picker, conflict/error retention,
restore defaults, shared save coordination, and a focused phone section editor.
Drag-and-drop group and shortcut reordering is supported with explicit 44px move
controls as the accessible and phone-friendly alternative.
The editor and supporting operation tests passed, together with typecheck and
the i18n checks. The dedicated desktop and phone customization E2E scenarios
also passed, covering deferred edits, four shortcut additions, visibility,
group reorder, save/reload, the focused phone flow, and touch sizing.
