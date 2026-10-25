---
id: "03-responsive-clone-flow"
title: "Add desktop and phone clone controls"
status: done
wave: 3
depends_on:
  - 02-atomic-clone-api
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CLONE-003
acceptance_criteria:
  - AC-WORKSPACES-CLONE-003.1
  - AC-WORKSPACES-CLONE-003.2
  - AC-WORKSPACES-CLONE-003.3
  - AC-WORKSPACES-CLONE-003.4
  - AC-WORKSPACES-CLONE-003.5
system_design:
  - ../../specs/workspaces/system-design/clone-workspace.md
---

# Task 03: Add desktop and phone clone controls

## Summary

Add workspace identity actions in list cards and settings headers, with a shared clone form with desktop Dialog and
phone Drawer presentations. The name draft, submission logic, source ID,
response merge, and error handling remain shared.

## In scope

- Add typed `cloneWorkspaceAction` with encoded source path/name request and
  explicit errors. Preserve current ordinary creation behavior.
- Expose a visible accessible Clone card action for manageable supported
  sources; prevent the whole-card overlay from intercepting that action.
- Add localized copy-name suggestion, copy summary/exclusions, validation,
  pending state, cancel/focus return and error recovery without automatic POST retry.
- Guard synchronous double submission, merge using latest store state and the
  shared DTO mapper, deduplicate WS/HTTP arrivals, navigate to target overview,
  and preserve globally active workspace selection.
- Keep form draft above responsive branches; phone uses a short inset Drawer,
  single bounded scroll owner, safe areas and 44px controls. Apply `/mobile-parity`.
- Add keys to English and six complete real locale catalogs, generating the
  Traditional Chinese pair through `i18n:zh-hant`. Use `/tdd` for logic tests.

## Out of scope

Backend policy changes, active workspace switching, generic workspace settings
redesign, broad control sizing changes, or arbitrary configuration selection UI.

## Acceptance

1. Hook/action tests prove no request on invalid/unsupported source or blank
   name, one request on rapid clicks, retained draft on failure/resize, and an
   explicit uncertain-result check-list path.
2. Success merges once into current store state, retains existing projected
   scopes/visibility and concurrent workspace changes, navigates to the clone's
   overview, and leaves `activeId` unchanged.
3. Dialog/Drawer match UI-01/UI-02/UI-03 structure and localized copy, with real
   browser proof in Task 04 before this work package can be marked implemented.

## ASCII UI preview

Excerpt of [full preview](plan.md#ascii-ui-preview), covering 003.1-.5:

```text
UI-01 desktop | workspace card and clone dialog
| Team tools [Active] [···] | Resources | > |
+--------------------------------------------+
| Clone workspace                         X  |
| From: Team tools                           |
| Name [Team tools (copy)                  ] |
| Copy summary + exclusions                  |
|                        [Cancel] [Clone]    |
+--------------------------------------------+

UI-02 phone | card action opens inset bottom drawer
| Team tools [Active] [···]               > |
| Resources                                 |
    +-----------------------------------+
    | Clone workspace                   |
    | From: Team tools                  |
    | Name [Team tools (copy)        ]   |
    | Copy summary + exclusions         |
    | [Cancel]              [Clone]     |
    +-----------------------------------+

UI-03 shared form | failure retains draft
| Name [preserved draft                    ] |
| Could not clone. Try again.                |
|                        [Cancel] [Clone]    |
```

Labels are illustrative locale-key output. Pending keeps one form and disables
submission; uncertain network failure uses list-check wording. Phone geometry
follows `components/task/mobile/mobile-picker-sheet.tsx`; the footer clears the
safe area and only overflow content scrolls. Resize preserves shared draft.

## Verification

Fresh worktree prerequisite: `(cd apps && pnpm install --frozen-lockfile)` once
if dependencies are missing. Run all blocks from the repository root.

```bash
(cd apps/web && pnpm exec vitest run app/actions/workspaces.test.ts hooks/domains/workspace/use-workspace-clone.test.ts)
(cd apps/web && pnpm exec eslint app/actions/workspaces.ts app/settings/workspace/workspaces-page-client.tsx components/settings/workspaces/workspace-clone-dialog.tsx hooks/domains/workspace/use-workspace-clone.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Task 04 owns targeted managed production-build browser verification.

## Files likely touched

- `apps/web/app/actions/workspaces.ts` and `workspaces.test.ts`.
- `apps/web/app/settings/workspace/workspaces-page-client.tsx`.
- `apps/web/components/settings/workspaces/workspace-clone-dialog.tsx`.
- `apps/web/components/settings/workspaces/workspace-actions-menu.tsx`.
- `apps/web/components/settings/workspaces/workspace-settings-shell.tsx` and `.test.tsx`.
- `apps/web/hooks/domains/workspace/use-workspace-clone.ts` and `.test.ts` (new).
- `apps/web/src/settings-routes.coordinators.test.tsx`.
- `apps/web/lib/types/http.ts` if a named request type is useful.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/workspaces.json`.
- Generated pseudo-locale artifacts only through the existing i18n scripts.

## Dependencies

Task 02 public clone API and standard post-commit event projection.

## Risks

Workspace cards contain an absolute overlay link. Response handlers capturing
an old `items` array can lose concurrent list changes. Remounting responsive
branches can lose draft/focus, and untranslated copy-name suggestions can
escape JSX-only scanning.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/clone-workspace.md), 003.
- [Design](../../specs/workspaces/system-design/clone-workspace.md), responsive form and state.
- Existing workspace list, `mapWorkspaceItem`, workspace scopes/selectors,
  `settingsActionClassName`, Dialog/Drawer and mobile picker primitives.

## Results

24 targeted frontend tests passed. Scoped eslint and TypeScript checks passed. Traditional Chinese and pseudo catalogs regenerated; i18n:check and i18n:ratchet passed. Desktop/mobile browser evidence belongs to Task 04.

The final localized form summary explicitly excludes repository secrets and
keeps personal GitHub sign-in separate. All seven language catalogs and pseudo
were regenerated/checked; the phone browser test verifies this summary.

Prior visual refinement replaced the prominent labeled card button
with a ghost copy icon beside navigation. Use the shared square icon size
(28px desktop; at least 44px on phones/coarse pointers), a localized tooltip
and workspace-specific accessible name. Phone placement stays in the title
row, without a dedicated action row. Existing responsive clone flow tests
now assert desktop dimensions and phone square-target/header alignment.

The compact icon revision passed scoped ESLint and TypeScript checks. The first
TypeScript attempt exhausted its 2 GB heap; a sequential retry passed with a
3 GB heap under a 4 GB process cap. No application code changed for the retry.
Task 04 records the fresh desktop/phone browser evidence.

### Workspace actions placement refinement

The user requested an action attached to the workspace identity and another
entry inside the workspace page. Use one shared ellipsis menu beside the name
and Active badge in list cards and every workspace settings header. The labelled
Clone workspace item closes the menu before opening the existing form; cancel
returns focus to the persistent ellipsis trigger. The list's whole-card
navigation remains the main tap destination. Reuse the shared Radix inset phone
menu and clone Drawer, with 44px targets, bounded scroll and safe areas. The
agent-profile row actions are the nearest shipped secondary-action exemplar.
The hook and form move to shared workspace hook/component owners, without
changing clone persistence or authorization. Clone reads saved configuration.

```text
Desktop list: | Name [Active] [···] | Resources          | > |
Desktop page: | Folder | [Workspace v] [Active] [···]       |
Phone page:   | Folder | [Workspace v] [Active] [···] |
                         -> inset actions menu
                            [copy] Clone workspace
                         -> existing clone dialog/sheet
```

Verification adds wide-card action proximity, keyboard activation, menu-to-form
focus return, phone menu target sizes, header containment and real cloning from
the viewed workspace on a settings tab in both projects. Existing clone flow
and retry coverage remains in scope.

The final actions-menu revision passed 33 targeted unit tests (clone action,
shared hook and settings switcher), scoped ESLint, TypeScript, all locale gates,
and the six browser flows recorded in Task 04. New code reuses one menu and
closes it before opening the shared form; persistent triggers retain focus.

### PR CI route-fixture repair

Frontend CI on head `978267ccbdf521f5145ed72de7aadbfdce7961a0`
failed three coordinator-route tests because their narrow state-provider mock
did not expose the store API required by the new settings-header clone hook.
The local reproduction failed the same three tests. The routing fixture now
mocks the clone interaction hook while retaining the module's real eligibility
helper. Clone behavior remains covered by the hook and browser tests.

The expanded targeted run passed 82 tests across nine files: all settings-route
suites, the workspace settings shell and the clone hook. Scoped ESLint and
Prettier and TypeScript checks passed. This changes test isolation only; the product contract,
responsive UI and public documentation remain unchanged.

```bash
(cd apps/web && pnpm exec vitest run src/settings-routes.coordinators.test.tsx src/settings-routes.test.ts src/settings-routes.workspace-data.test.tsx src/settings-routes.workspace-revision.test.tsx src/settings-routes.bootstrap.test.ts src/settings-routes.feature-toggles.test.tsx src/settings-routes.plugin.test.ts components/settings/workspaces/workspace-settings-shell.test.tsx hooks/domains/workspace/use-workspace-clone.test.ts --maxWorkers=1)
```
