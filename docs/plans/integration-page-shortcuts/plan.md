---
created: 2026-10-06
status: done
requirements:
  - REQ-UI-INTEGRATION-PAGE-SHORTCUTS-001
system_design:
  - ../../specs/ui/system-design/integration-page-shortcuts.md
legacy_specs: []
---

# Implementation Plan: Integration Page Shortcuts

## Overview

Add personal integration navigation shortcuts to the existing Keybindings
settings area. One sequential work order delivers the catalog, editor,
app-root dispatch, and focused desktop/phone evidence as a complete slice.

The [requirement](../../specs/ui/requirements/integration-page-shortcuts.md)
and [system design](../../specs/ui/system-design/integration-page-shortcuts.md)
are accepted. Implementation was authorized by the user's subsequent
“go go go” request and completed in the primary session.

## Scope

### In scope

- Six built-in integration destinations and active plugin Integrations links.
- Unbound defaults, recording, reset, shared Save, portable persistence, and conflicts.
- Global navigation that respects existing keyboard actions and workspace context.
- Stacked phone controls, localized copy, and targeted tests.

### Out of scope

- New dashboards, assigned default chords, provider state, plugin action settings
  relocation, command-palette expansion, and general shortcut refactoring.
- Backend schema/API changes, release flags, commits, publication, or delegation.

## Technical approach

1. Create `apps/web/lib/keyboard/integration-shortcuts.ts` from
   `WORKSPACE_INTEGRATIONS`, `APP_DESTINATIONS`, `pluginDestinations`, and
   `pluginDestinationId`. Implement the identities and Sentry fallback specified
   in the design. Keep core shortcut IDs closed and add an integration source to
   `ShortcutEntry` for shared conflict resolution.
2. Add `useIntegrationShortcuts` beside the existing app/plugin hooks and mount
   it between them in `GlobalCommands`. Reuse core/reserved conflict suppression,
   router navigation, current context resolution, and editable-target checks.
   Add a recorder marker that suppresses integration dispatch during recording.
3. Render an Integrations group within `KeyboardShortcutsCard` using the existing
   `ShortcutRecorder`. Feed the same host navigation catalog into global and
   plugin-detail conflict calculation without displaying plugin action rows in
   global settings. Share the existing plugin draft hook as `useShortcutDraft` for the keyboard
   settings contributor and keep chat-submit-key in a separate contributor;
   preserve complete-map rebasing and acknowledged revision handling using the
   existing plugin-editor patterns. Reset removes a key rather than storing the
   empty-key sentinel rejected by the backend.
4. Add all locale keys, targeted unit tests, desktop/phone E2E, and a short how-to
   addition in `docs/public/sessions-and-review.md`. Update the screenshot catalog
   description if its existing keyboard-settings caption needs wider wording;
   do not claim a screenshot has been refreshed without capturing it.

### Compatibility matrix

| Destination | Source/identity | Behavior | Evidence / fallback |
| --- | --- | --- | --- |
| Azure DevOps, GitHub, GitLab, Jira, Linear | Built-in catalog / `integration:<slug>` | Existing dashboard; connection-independent editor | Parameterized desktop navigation; existing route owns disconnected state |
| Sentry | Built-in catalog / `integration:sentry` | Existing connection settings | Desktop destination assertion; no new dashboard |
| Active plugin integration link | Registered nav / owner-qualified encoded destination | Existing registered route | Catalog identity unit tests plus packaged-fixture E2E |
| Disabled/uninstalled plugin | No live nav registration | Retained override is inert | Hook tests and fixture disable/re-enable E2E |
| Plugin action declaration | Existing `plugin:<plugin>:<keybinding>` | Remains on plugin detail page; conflict comparison only | Existing plugin card tests plus new navigation conflict case |
| Malformed/unknown saved entry | Override without eligible binding/target | No navigation or event consumption | Dispatcher unit tests |

## ASCII UI preview

### UI-01: Desktop integration keybindings

Entry: Settings > Preferences > Keyboard Shortcuts. Core rows remain above
the new group. The page content scrolls; the existing floating Save surface
appears only when the route is dirty.

```text
Keyboard Shortcuts
  ... existing Kandev shortcuts ...

  Integrations
  Open Azure DevOps          [Unbound]
  Open GitHub                [Ctrl+Alt+G] [Reset]
  Open GitLab                [Unbound]
  Open Jira                  [Press a key combo...]
  Open Linear                [Unbound]
  Open Sentry                [Unbound]
  Open Example: Reviews      [Unbound]

                    [Reset] [Save changes]
```

### UI-02: Phone integration keybindings

Entry: Settings index > Keyboard Shortcuts. Labels precede wrapping controls;
the route owns one vertical scroller. The keyboard chord is recorded using an
attached keyboard. Save stays above the safe area with last-row clearance.

```text
Settings > Keyboard Shortcuts
... existing settings ...

Integrations
Open GitHub
[Ctrl+Alt+G] [Reset]

Open Jira
[Press a key combo...]

Open Linear
[Unbound]
... scroll to further integrations ...

    [Reset] [Save changes]
```

### UI-03: Conflict and failed-save states (both viewports)

```text
Open GitHub  [!] Same shortcut as: Open Jira
[Ctrl+Alt+G] [Reset]

Save failed. Draft remains editable.
[Reset] [Save changes]
```

Group placement, desktop row/phone stacked order, shared draft/save semantics,
visible reset, and contained phone controls are structural requirements. Spacing,
the example chord, plugin name, and prose above are illustrative, localized by
the implementation. UI-01/02/03 map to AC .1-.3 and .6-.8; targeted rendered
evidence comes from the settings specs named below.

## Tests

All criteria are under `AC-UI-INTEGRATION-PAGE-SHORTCUTS-001`.
Cases below identify the targeted implementation coverage.

| AC | Test file and planned case |
| --- | --- |
| .1, .2, .4 | `lib/keyboard/integration-shortcuts.test.ts`: "covers every built-in integration and resolves its destination"; "isolates same nav IDs across plugin owners" |
| .3 | `components/settings/general-settings.test.tsx`: "saves integration overrides without losing concurrent unrelated keys"; "retains pending edits and rejects an older save response"; "failed save retains navigation draft" |
| .3 | `components/settings/keyboard-shortcuts-card.test.tsx`: recording/reset preserves unrelated entries; `general-settings.test.tsx`: clearing the final binding acknowledges an omitted empty map |
| .4, .5 | `hooks/use-integration-shortcuts.test.ts`: "opens saved destinations on every application route"; "uses current workspace after switching"; "ignores editable, recording, prevented, repeated, unbound and malformed events"; "removed registration is inert" |
| .6 | `keyboard-shortcuts-card.test.tsx`: finds core, navigation and plugin action collisions; dispatcher cases: core wins, navigation wins over plugin action, one navigation wins by catalog order |
| .1, .2, .3, .6, .8 | `components/settings/keyboard-shortcuts-card.test.tsx`: "renders integration rows with localized labels and unbound defaults"; "includes navigation conflict labels without rendering plugin actions"; "reset removes the override" |
| .6 | `components/settings/plugins/plugin-shortcuts-card.test.tsx`: "names conflicting host navigation actions"; `hooks/use-plugin-shortcuts.test.ts`: existing core precedence regressions plus integration dispatch interaction |

## E2E tests

Use the existing desktop `e2e/tests/settings/keyboard-shortcuts.spec.ts` for
new integration flows. Add one corresponding phone file
`e2e/tests/settings/mobile-integration-shortcuts.spec.ts` under `mobile-chrome`.
Use the packaged plugin fixture and existing upload/disable helpers. Its source
UI fixture now registers an integration navigation link. Ordinary phone touch
navigation is covered by `e2e/tests/integrations/mobile-integrations-nav.spec.ts`,
run alongside the new phone settings flow.

| Flow | AC | Project |
| --- | --- | --- |
| All six rows start unbound; record, Save, reload, invoke each destination, reset and save; existing core shortcut remains intact | .1, .3, .4 | chromium |
| Record over an already saved navigation chord without leaving settings (editable targets and repeat guards have dispatcher unit coverage) | .5 | chromium |
| Conflict warning names both actions; one navigation happens; plugin action rows stay in plugin detail | .2, .6 | chromium |
| Packaged plugin Integrations link receives a navigation binding, opens its registered page, becomes inert on disable, and resumes on re-enable | .2, .4, .5 | chromium |
| Failed settings PATCH leaves dirty controls and permits a successful retry | .3 | chromium |
| Settings-index entry by touch; stacked recorder/reset geometry; attached-keyboard record; Save, reload, invoke; ordinary integration link works by touch | .3, .4, .7, .8 | mobile-chrome |

Mobile assertions cover 44px touch dimensions, no document horizontal overflow,
one scroll owner, bottom-row Save clearance, and retained draft during desktop
to phone to desktop resizing. Use the configured phone device, plus 767/768px
boundaries for row composition. Capture a focused phone screenshot in this run.
Restore user settings and plugin lifecycle mutations in test cleanup.

## Work orders

- [x] [Task 01: Implement integration navigation hotkeys](task-01-integration-navigation-hotkeys.md)

## Verification results

Task 01 is complete. The work order records the exact commands and results.
The implemented UI matches UI-01/02/03: compact desktop rows, stacked phone
controls, advisory conflict labels, and retryable Save under the existing
coordinator. No unresolved structural differences remain.

Final implementation verification on 2026-10-06:

- Targeted Vitest: 11 files / 114 tests passed, followed by 18 dispatcher tests
  with the additional current-workspace case (115 distinct passing tests).
- Typecheck, targeted production ESLint, formatting, localization checks and
  the new-code ratchet passed. Seven real locales and pseudo are in sync.
- Desktop managed E2E: 9 tests passed; the added 767/768/1280px resize test
  passed separately. Mobile managed E2E: 2 tests passed, including ordinary
  integration touch navigation.
- Desktop and phone screenshots were captured and visually inspected. Geometry
  proves 44px phone controls, 28px desktop controls, contained rows, no document
  horizontal overflow, and last-row clearance above floating Save. Rotation and
  responsive boundary changes retain the unsaved hotkey.
- Public-doc validation tests (62), all public pages (47), documentation-package
  coverage, specification catalog and lint, and whitespace checks passed.

Browser verification exposed the server's omission of an empty shortcut map.
A regression was confirmed RED, then passed with the shared draft hook fix:
clearing the final saved binding now updates acknowledged runtime settings
without requiring a reload. Older response protection remains covered.

The implementation handoff left changes uncommitted. The user subsequently
requested a PR, authorizing commit and publication. No delegated work was performed.

Design validation on 2026-10-06:

- `python3 scripts/list-docs.py validate`: passed, 357 decisions and 1410 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog lookup discovers the new UI requirement/design pair.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed using the four new
  artifacts and the planned production module as its coverage-trigger input.
  This validates the package references; it is not implementation evidence.
- `git diff --check -- docs/specs docs/plans/integration-page-shortcuts`: passed;
  direct whitespace checks also passed for all four untracked new files.
- `git status --short -- docs/plans/integration-page-shortcuts`: confirmed the
  new plan/work order directory is untracked; artifacts remain unstaged.

Node was absent from the shell PATH; the documentation coverage preflight used
the installed Node 24 binary under the local mise toolchain. No product unit or
E2E tests ran during planning.

## Risks

- Sentry has a connection page only; its label must not imply a dashboard.
- Global capture listeners can consume recording events unless suppression is
  tested with an already saved navigation chord.
- Shared override maps must preserve unrelated and concurrently updated keys.
- Browser/OS-owned key combinations can remain unavailable to the application;
  use an application-received chord in E2E and retain existing recorder behavior.
- Plugin registration timing and disable cleanup require real fixture coverage.


## PR review remediation

The user requested remediation of PR #4274. Seven findings receive code or
coverage changes; the optional runtime display-name suggestion is documented:
settings supply installed display names, while dispatch consumes stable IDs and
hrefs without an additional plugin fetch.

- AC .2: repeated plugin integration identities retain the first eligible
  registration and matching label/path.
- AC .5/.6: legacy Tab and Shift+Tab bindings preserve focus; integration
  recording ends without capturing them. Ctrl/Cmd+Shift+P remains reserved for
  the command panel in both integration and plugin dispatch.
- AC .8: recorder descriptions expose the current localized binding and a
  polite, atomic live status announces changes.
- Unbound values are rejected by validation, with direct validator tests and
  distinct unbound/malformed dispatcher cases. Save tests arm the causal
  shortcut PATCH before clicking and use the default UI assertion timeout.

Behavioral RED reproduced the missing keyboard guards, duplicate identities
and recorder accessibility before the production fixes. Focused Vitest passes 132 tests across 12 files;
typecheck, targeted ESLint, formatting and localization checks pass. Public-doc
validation (62 tests and 47 pages), delivery-package coverage, specification
catalog/lint and whitespace checks pass. Managed desktop E2E passes all 11 tests
(log `/tmp/kandev-run.e2e.rj5NVUrf.log`); mobile E2E passes both tests
(log `/tmp/kandev-run.e2e.aPm9QrAk.log`). The desktop run rebuilt production
assets; the sequential mobile run reused those unchanged assets with
`--no-build`. Existing rotation, touch sizing, Save clearance and responsive
boundary assertions remain covered. Exact-head remote CI/review and current-base
merge-result verification are externally pending until the remediation push.
