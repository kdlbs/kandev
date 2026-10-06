---
created: 2026-10-05
status: implemented
requirements:
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-001
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-002
system_design:
  - ../../specs/agents/system-design/command-autocomplete.md
legacy_specs: []
---

# Implementation Plan: Skill Command Autocomplete

## Overview

Show clean skill names, Mode and Active chips, and advertised argument hints while preserving raw command identity.
Deliver the adapter-to-composer slice with focused tests first, then prove desktop and phone behavior through existing E2E suites.
The requirement and system design statuses reflect the shipped contract and current intended implementation.

## Scope

### In scope

- Codex ACP skill classification and an additive optional command kind.
- Clean autocomplete and selected draft-chip labels, with provider names preserved for invocation.
- Matching by clean and raw names, identity collisions, serialization, and recall.
- Localized Skill badges in the shared autocomplete popup.
- Typed Codex `/plan` action metadata and Mode/Active badges from confirmed session configuration.
- Visible advertised argument hints, including `/goal`, without guessed categories.
- Desktop task chat, quick chat, and phone tests.
- A short composer section in `docs/public/tasks-and-workflows.md` during implementation.

### Out of scope

- Classification of untyped Claude or OpenCode entries from descriptions or filesystem scans.
- Changes to managed runtime versions, native Codex, passthrough, or other slash menus.
- New discovery services, execution behavior, or argument editors.

## Technical approach

Use the provider matrix in the [system design](../../specs/agents/system-design/command-autocomplete.md#provider-classification).
Codex ACP `$name` is the supported positive shape. Claude, OpenCode, and unknown provider shapes preserve names without guessed badges.

Task 01 owns the complete functional slice and its regression tests.
It adds optional skill classification and validated mode action data at `convertAvailableCommands`.
It keeps raw names through the lifecycle and WS store, then derives clean composer labels.
The TipTap node separates visible label from raw serialization across selection, copying, restoration, and recall.
It adds one optional badge slot to `PopupMenuItem` and complete translations.
The existing configuration handler keeps a separate confirmed option projection before saved preference overlays.
The menu uses that projection for Mode/Active display and shows provider argument hints as secondary text.

Task 02 owns browser proof and the public how-to note.
Use seeded mock command entries with the same fields as the captured providers.
Capture outbound user-message text through existing WS helpers to prove the exact raw invocation.
Do not invoke the real retro skill during tests.

## ASCII UI preview

### UI-01: Task or quick-chat slash autocomplete

Entry: type `/retro` with the agent command list available.
Before, verified by the user screenshot and source:

```text
Commands
  [icon] /$retro  Use when the user runs /retro...
```

After, desktop and phone share the same control order:

```text
Commands
  [icon] /retro [Skill]         Use when the user runs...
  [icon] /plan  [Mode] [Active] Turn plan mode off
  [icon] /goal                 Set a goal to keep pursuing
         Arguments: [<objective>|clear|pause|resume]
```

The list scrolls; the heading stays fixed. Category chips follow verified skill or action information.
The Active chip requires confirmed session configuration. Without confirmation, `/plan` shows Mode and a neutral description.
On phones, the description truncates before the badges. Argument hints occupy a secondary line inside the same option.
The row remains a single selectable target of at least 44 pixels.
The existing popup bounds and software-keyboard positioning apply.
Required structure: clean name, adjacent noninteractive badge, distinct raw command identity, and existing option semantics.
Spacing and icons are illustrative. Copy is localized.
Maps to AC-AGENTS-COMMAND-AUTOCOMPLETE-001.1, .2, .3, .6, .7, and AC-AGENTS-COMMAND-AUTOCOMPLETE-002.1 through .6.
Rich clipboard markup containing a command-shaped node is inserted as plain visible text, so untrusted HTML cannot create a command chip or change the sent command. This also maps to AC-AGENTS-COMMAND-AUTOCOMPLETE-001.8.

### UI-02: Selected skill draft

```text
Composer: [ /retro ] add context here |
                                   [Send]
```

The visible inline chip uses the clean name. Its serialized command is still `/$retro` for Codex ACP.
Selection keeps focus and creates no message. The separate Send action submits the edited draft.
This package adds the Skill badge to autocomplete rows; it does not require another badge inside the selected draft chip.
Maps to AC-AGENTS-COMMAND-AUTOCOMPLETE-001.4 and .5.

## Tests

| Criteria | Required targeted evidence |
| --- | --- |
| .1, .2, .7 | `conversion_test.go`: Codex `$retro`, bare `$`, builtin `review`, Claude `retro`, OpenCode `retro`, unknown-provider `$retro` |
| .1, .7 | Lifecycle cached-command tests retain kind; reconnect event includes kind without altering name |
| .1, .2, .3, .7 | `slash-command-types.test.ts` and `tiptap-suggestion.test.ts`: clean/raw matching, collisions, absent and unknown kind |
| .4, .8 | `tiptap-slash-command-extension.test.ts`, `tiptap-helpers.test.ts`, and editor history tests: raw serialization, copying, HTML round trip, forged command-shaped clipboard markup, recall, surrounding text, legacy fallback |
| .1, .5, .6 | Menu component tests: localized badge, option semantics, unchanged non-skill rows and focus behavior |

For REQ-AGENTS-COMMAND-AUTOCOMPLETE-002:

| Criteria | Required targeted evidence |
| --- | --- |
| .1, .5 | `conversion_test.go`: complete supported plan action, additive fields, malformed and unknown action shapes, unknown providers, and `$plan` skill precedence |
| .2, .3, .6 | `session-models.test.ts`, `session-models-startup.test.ts`, and `session-models-user-selection.test.ts`: confirmed projection before saved preference overlays, partial provider updates, delayed old-execution snapshots, invalidation, settled empty snapshot, and session isolation |
| .1 through .6 | Composer/menu tests: Mode and Active chips, active/default/unknown descriptions, live and partial confirmed updates, goal hints without a category, and selection without configuration mutation |

Test names must describe the behavior and identify applicable ACs where their ownership is unclear.

## E2E tests

- `tests/chat/slash-command-composer.spec.ts`, project `chromium`: clean menu name and Skill badge, Enter/Tab selection without send, raw explicit send, recall, quick chat, forged clipboard HTML, partial provider updates in an open menu, and startup snapshot ordering. Covers .1, .3, .4, .5, .8, and .2.3/.2.6.
- `tests/chat/mobile-slash-command-composer.spec.ts`, project `mobile-chrome`: clean skill row, touch selection without send, raw explicit send, visible badge, long descriptions, viewport containment, and measured 44-pixel row hit target. Covers .1, .4, .5, and .6.
- Include a mixed list with `retro` and `$retro` so identical display names retain separate selectable entries. Covers .2, .3, and .7.
- In both projects, seed the validated plan action and settled configuration. Prove Active only for confirmed `plan`, with default and unknown-state variants.
- Capture outbound `/plan` on explicit send. Assert selection alone sends no message and changes no configuration.
- Show `/goal` with its argument hint and no category chip. Cover phone hint wrapping, multiple badge widths, and menu containment.

These mode/state/hint scenarios cover AC-AGENTS-COMMAND-AUTOCOMPLETE-002.1 through .6.

## Work orders

- [x] [Task 01: Preserve skill identity and improve autocomplete](task-01-skill-presentation.md)
- [x] [Task 02: Prove desktop and phone flows](task-02-browser-verification.md)

Task 02 depends on Task 01. Work stays in the primary session unless the user later authorizes delegation.

## Verification results

Implementation and browser verification completed on 2026-10-05:

- ACP conversion, lifecycle cache, and reconnect transport Go tests passed with matching tests confirmed; `make -C apps/backend lint` reported 0 issues.
- Focused web regression suite passed: 12 files, 115 tests. Targeted ESLint and `pnpm run typecheck` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed; seven locale catalogs are complete and the new-code ratchet is clean.
- Managed desktop Chromium E2E passed 5 tests. Managed mobile Chromium E2E passed 2 tests against the built artifacts.
- Public docs tests passed (62 tests); the public docs validator checked 47 pages.
- `python3 scripts/list-docs.py validate` validated 351 decisions and 1367 specifications; specification-linter tests passed (36), and all specification files passed lint.
- `git diff --check` passed. Desktop and phone screenshots are retained under `/tmp/kandev-skill-command-autocomplete-evidence/`.

Review remediation verification on 2026-10-05:

- Fixed all review findings: authoritative partial provider updates merge into the confirmed projection; startup hides and fences delayed prior-execution snapshots; a typed-nil lifecycle manager preserves reconnect behavior; rich clipboard command markup becomes plain visible text; additive action metadata is accepted; and popup command mapping is memoized with shallow config selection.
- Focused Go tests passed for ACP metadata conversion and reconnect nil-manager handling; `make -C apps/backend lint` reported 0 issues.
- The focused frontend regression suite passed 37 tests across 3 files. Web typecheck and targeted ESLint passed without warnings.
- The latest managed desktop Chromium composer suite passed 8 tests and the phone suite passed 2 tests. Desktop tests cover partial provider updates in an open menu, delayed startup snapshots, and forged clipboard text preserving the explicit `/plan` send.
- Public docs tests passed (62 tests), the validator checked 47 pages, catalog validation covered 351 decisions and 1367 specifications, all specifications passed lint, and the specification-linter tests passed (36). `git diff --check` passed.
- CI follow-up added the missing `invalidateConfirmedConfigOptions` action to the `agent-session.test.ts` store fixture. The focused browser-locales handler/config suite passed 78 tests; web typecheck and targeted ESLint passed.

## Risks

- Stripping dollar markers from invocation can break provider handling. Preserve raw command names and test submitted text.
- Claude and OpenCode omit skill identity in the captured command entries. Their initial rows remain unclassified.
- Clean-name collisions can merge commands if IDs or maps use display names. Keep raw keys.
- A badge can hide long names on narrow screens. Reserve badge width and test rendered containment.
- Saved preferences can differ from provider state. Build Active from a separate settled projection and invalidate it during startup.
- A Mode label inferred from the name `plan` can misclassify a custom skill. Require the complete supported metadata shape.
- The configured OpenCode version was unavailable during probing. The comparison uses explicitly recorded installed versions.
