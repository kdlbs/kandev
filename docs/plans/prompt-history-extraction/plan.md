---
created: 2026-09-23
status: implemented
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001
system_design:
  - ../../specs/plugins/system-design/prompt-history-extraction.md
legacy_specs: []
---

# Implementation Plan: Prompt History Extraction

## Overview

Remove Kandev's built-in Prompt history panel and the core-only browser code
that feeds it, while keeping every generic Host contract the replacement plugin
uses. The durable contract is
[REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001](../../specs/plugins/requirements/prompt-history-extraction.md);
the technical path is
[Prompt History Extraction System Design](../../specs/plugins/system-design/prompt-history-extraction.md).

This is a deletion package. It ships no new panel, API, or backend data contract.
Delivery order: remove the panel surfaces, remove the core prompt projection,
then prove saved-layout compatibility and the plugin evidence path. Backend
repository deletion paths also remove session ordinal markers with their owning
task or session; this is persistence cleanup, not a change to prompt behavior.

The specification work is already complete in this design turn: the UI panel
requirement and design are deprecated and superseded, and the turn-duration and
alias contracts no longer reference the removed panel. Work orders change
production code, tests, and browser coverage only.

## Inputs and authority

- [Extraction requirements](../../specs/plugins/requirements/prompt-history-extraction.md)
  and [design](../../specs/plugins/system-design/prompt-history-extraction.md).
- [Host prerequisites](../../specs/plugins/requirements/prompt-history-extraction-host.md):
  authoritative for what the plugin may consume and what core must keep.
- [Deprecated UI panel requirements](../../specs/ui/requirements/prompt-history-panel.md)
  and [superseded design](../../specs/ui/system-design/prompt-history-panel.md):
  the behavior being removed.
- `plugin-registry/plugins.yaml`: the replacement plugin is
  `kandev-plugin-prompt-history` (`Fclem/kandev-plugin-prompt-history`).
- In-repo fixture plugin `apps/web/e2e/fixtures/plugins/prompt-history-plugin/`
  plus `apps/backend/cmd/plugin-fixture/fixture-package/`: the executable
  parity and API-surface evidence.

## Scope

### In scope

- Removing the built-in panel from the desktop and Office workbench "+" menu and
  from the native-mobile `Panels` picker, including its dockview component,
  layout identity, store action, and locale copy.
- Removing the core user-message prompt projection, its pagination hooks, its
  layout-profile placeholder, and its panel-only test coverage.
- Relocating the transcript turn-duration helpers out of the removed module.
- Removing internal per-session prompt-sequence markers transactionally with
  hard-deleted owning tasks and sessions, without changing live ordinal behavior.
- Proving saved layouts, saved profiles, and per-environment layouts that
  contain the retired panel restore cleanly, and keeping the plugin fixtures
  green on desktop and phone.

### Out of scope

- Any change to the browser conversation façade, task-panel registration and
  navigation, `host.ui.PromptMentionText`, favorite state, message or turn data
  contracts, live `prompt_index` behavior, or the `author_type=user` filter.
- Implementing, publishing, or versioning the external replacement plugin.
- Migrating stored layouts to the plugin's panel id, and carrying the panel's
  browser-local state into the plugin.
- New panels, layout redesign, or a replacement placement for the removed
  panel beyond what the plugin already provides.

## Technical approach

Task 01 deletes the panel surfaces and their registrations, ending with a
workbench that offers no built-in Prompt history panel and no layout or locale
identity for it. Task 02 then deletes the state, hooks, and derivation that only
the panel consumed, moves the turn-duration helpers to a neutral module, and
leaves the identity `prompt-history` nowhere in core web code. Task 03
makes every route that applies a stored payload tolerate the retired identity —
the on-ready restore, the environment-switch slow path, the preset dropdown, the
custom-default build, both maximize readers, and the retained hidden-right-pane
column — through two
shared helpers in a neutral module (a serialized-layout sanitizer and a
`LayoutState` component filter), and replaces the removed browser coverage with
the two behaviors that still need end-to-end evidence: the plugin path on
desktop and phone, and a saved layout that contains the retired panel.

No database migration or bulk browser-storage rewrite is added. Sanitizers run
when payloads are applied; maximize fallback restoration may save the filtered
layout to per-environment browser storage before clearing its maximize snapshot,
self-healing that local value. Six routes reach the renderer
unfiltered today (the environment-switch slow path, the preset dropdown, the
custom-default build, the two maximize readers, and the retained
hidden-right-pane column), so removing the identity
from `DESKTOP_VALID_COMPONENTS` and
`KNOWN_PANEL_IDS` is necessary but not sufficient; `filterEphemeral` is not the
answer for the live `LayoutState` path, because its set omits canvas, review,
editor, diff, and commit panels and it preserves empty groups by design.

## ASCII UI preview

Structural choices are requirements; spacing is illustrative.

```text
UI-01: Desktop task workbench "+" menu, open in a task group (before -> after)

BEFORE (current, source-verified)      AFTER (required)
+-------------------------------+      +-------------------------------+
| Sessions >                    |      | Sessions >                    |
| Terminals >                   |      | Terminals >                   |
| Browser                       |      | Browser                       |
| VS Code                       |      | VS Code                       |
| Plan                          |      | Plan                          |
| <plugin task panels>          |      | <plugin task panels>          |
| <task canvases>               |      | <task canvases>               |
| Todos                         |      | Todos                         |
| Prompt history        <-- out |      | Changes      (when closed)    |
| Changes      (when closed)    |      | Files        (when closed)    |
| Files        (when closed)    |      | <review panels>               |
| <review panels>               |      | <repository scripts>          |
| <repository scripts>          |      +-------------------------------+
+-------------------------------+      The replacement plugin contributes its
                                       own row through the same menu.
```

```text
UI-02: Phone "Panels" bottom-navigation picker (before -> after)

BEFORE (current, source-verified)      AFTER (required)
+---------------------------+          +---------------------------+
| Panels                    |          | Panels                    |
|---------------------------|          |---------------------------|
| <task canvases>           |          | <task canvases>           |
| Prompt history     <-- out|          | <plugin panels>           |
| <plugin panels>           |          +---------------------------+
+---------------------------+          44 px rows, one scrolling sheet.
```

```text
UI-03: Saved layout containing the retired panel (required behavior)

BEFORE (current behavior)              AFTER (required)
+-----------+-------------+            +-----------+-------------+
| Chat      | Prompt      |            | Chat      | Todos       |
|           | history     |            |           |             |
+-----------+-------------+            +-----------+-------------+
The stored entry keeps a tab that     The stored entry is dropped on restore;
the renderer still knows.             every other panel, size, and maximize
                                      state is preserved, with no notice.
```

Required: no Prompt history row or option anywhere; no empty or placeholder
panel appears for a restored retired entry; the remaining layout is untouched.
Illustrative: exact row order, row height, and icon choices come from the
existing menu and picker primitives.

## Tests

| Criteria | Target evidence | Owner |
| --- | --- | --- |
| 001.1, 001.9 | `dockview-add-panel-items.test.tsx`, `mobile/session-mobile-layout.test.tsx`, `mobile/session-mobile-bottom-nav.test.tsx`, `dockview-desktop-layout.test.ts`, `dockview-panel-content` tests | 01 |
| 001.3 | `pnpm run i18n:check` plus the catalog diff (task keys in 01, compact unit keys in 02) | 01, 02 |
| 001.2, 001.5 | `use-lazy-load-sentinel.test.ts`, session-slice tests, `lib/turn-duration.test.ts`, targeted backend prompt-index tests; task 02's zero-match search proves the naming half of 001.4 | 02 |
| 001.4 | The fixture parity suites in the 001.6 row, which exercise the facade, `host.ui.PromptMentionText`, task-panel registration and navigation, plugin-panel placement, and the favorite store | 03 |
| 001.8 | `components/task/chat/messages/message-actions.test.tsx`, alias-rendering tests | 02 |
| 001.6 | `tests/plugins/prompt-history-plugin.spec.ts`, `tests/plugins/mobile-prompt-history-plugin.spec.ts`, `tests/plugins/mobile-plugin-task-panel.spec.ts` | 03 |
| 001.7, 001.10 | `tests/settings/layout-profiles.spec.ts` (rewritten case, seeded-profile flow), `dockview-layout-restore.test.ts` (retired-panel and maximized-blob cases), `lib/layout/layout-profiles.test.ts` (retired component in a default profile), `lib/state/dockview-right-pane.test.ts` (retained hidden-right-pane column), `lib/state/dockview-env-switch-action.test.ts` (environment-switch slow path and the maximize route on a switch), `lib/state/dockview-preset-persistence.test.ts` (preset-dropdown apply, both branches) | 03 |

The deleted core panel suite (`prompt-history-panel.spec.ts`,
`prompt-history-auto-load.spec.ts`, `mobile-prompt-history-panel.spec.ts`, and
`helpers/prompt-history-long-seed.ts`) is replaced, not relocated: the plugin
fixture specs already own prompt pagination, ordinals, durations, aliases,
favorites, navigation, and mobile placement through public contracts.

## E2E tests

| File and project | Required outcome | Criteria |
| --- | --- | --- |
| `tests/task/prompt-history-removed.spec.ts`, chromium (new) | The "+" menu has no Prompt history row in a default task; the retired tab's absence is asserted in the seeded saved-profile case below | 001.1, 001.9 |
| `tests/settings/layout-profiles.spec.ts`, chromium (rewritten case) | A saved profile containing the retired panel opens the task without that tab and with the other panels present | 001.7, 001.10 |
| `tests/task/mobile-prompt-history-removed.spec.ts`, mobile-chrome (new) | The Panels picker offers no Prompt history option, and the task still exposes Chat, Plan, Changes, Files, and Terminal navigation | 001.1, 001.9 |
| `tests/settings/mobile-layout-profiles.spec.ts`, mobile-chrome (rewritten case) | The touch layout editor no longer lists Prompt History and still round-trips a surviving reusable panel | 001.1, 001.3 |
| `tests/plugins/mobile-plugin-task-panel.spec.ts`, mobile-chrome (rewritten assertions) | After its only plugin is disabled, the picker has no plugin option, the Panels entry is absent when no canvas or plugin panel can populate it, and Chat stays active | 001.9 |
| `tests/plugins/prompt-history-plugin.spec.ts`, chromium | Plugin pagination, ordinals, privacy, alias/favorite display, navigation, and terminal state still pass on the retained façade | 001.6 |
| `tests/plugins/mobile-prompt-history-plugin.spec.ts`, mobile-chrome | Touch panel entry, older-page loading, alias display, and Chat navigation still pass | 001.6 |

Run E2E with the managed runner only (`pnpm e2e:run`); do not pass all-worker
overrides, and run the two projects separately.

## Mobile parity

Removing a panel changes mobile composition, so the phone path is verified, not
assumed. The `Panels` picker keeps its grouped sheet, 44 px touch rows, single
vertical scroller, and plugin-panel entries; only the core Prompt history option
disappears. The `Panels` bottom-navigation entry is rendered from
`showPromptHistory || hasTaskCanvases || mobilePluginPanelsAvailable`; dropping
the first term means a task with no canvases and no enabled plugin panel stops
offering an entry whose sheet would be empty. That is the intended outcome, not
a regression to paper over in a test. `useEffectiveMobilePanel` and the plugin-panel lifecycle keep
deciding which surface is shown: a plugin panel that is loading or failed keeps
its own surface, where the plugin or the Host's unavailable placeholder renders,
and one that is removed, or whose registration is gone once its lifecycle is
ready, resolves to Chat instead of an empty surface. No desktop-only
composition is substituted for the phone sheet.

## Work orders

- [x] [Task 01: Remove the built-in panel surfaces](task-01-remove-panel-surfaces.md)
- [x] [Task 02: Remove the core prompt projection](task-02-remove-core-projection.md)
- [x] [Task 03: Harden and prove layout compatibility](task-03-layout-compat-and-e2e.md)

Execution order is 01 -> 02 -> 03. The three work orders touch the same
frontend compilation unit, so they are sequential.

## Risks

- Deleting the projection before the panel is unreferenced leaves a broken
  build. Task 01 removes every importer first; Task 02 verifies with typecheck
  and lint before finishing.
- Session-slice edits can silently disturb transcript merging. Task 02 requires
  the transcript and session-slice suites to pass unchanged.
- Moving the duration helpers can change the transcript hover row. Task 02 keeps
  the helper behavior identical and requires the message-action tests to pass
  without edits to their assertions.
- The layout compatibility proof can pass vacuously if the seeded profile is
  dropped for an unrelated reason. Task 03 seeds a profile whose other panels
  are canonical, and asserts those panels are present after restore.
- Browser-side maximized state is the least obvious of the unfiltered routes
  (`dockview-layout-restore.ts` and `dockview-store.ts`); without the shared
  helpers the retired identity reaches `api.fromJSON`, the component guard
  throws inside it, and the caught rollback deletes the blob or falls back to
  the default layout. Task 03 covers every route, the layout, and the maximized
  blob.
- Saved layout profiles validate against `REUSABLE_PANEL_IDS`, so removing the
  retired id without normalizing the profile makes a user's customized default
  layout invalid and silently reverts them to the built-in default. Task 03 owns
  the profile normalization and its unit case.
- Removing the core specs can hide a plugin regression. Task 03 runs the plugin
  fixture suites on both projects as part of its gate, not only the new spec.

## Completion and documentation

No public page documents the built-in panel; the browser conversation façade
pages (`docs/public/plugins-authoring.md`, `docs/public/websocket-api.md`) stay
accurate because that contract is unchanged. Verify that during Task 03 with a
repository-wide search, and update a public page only if it names the removed
panel.

Keep the requirements doc `active`. When all three work orders pass, promote the
extraction system design from `draft` to `current` and change this plan to
`implemented`, recording the commands and results in each work order.
