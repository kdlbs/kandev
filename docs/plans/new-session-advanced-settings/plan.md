---
created: 2026-10-26
status: complete
requirements:
  - REQ-UI-NEW-SESSION-ADVANCED-001
  - REQ-UI-NEW-SESSION-ADVANCED-002
system_design:
  - ../../specs/ui/system-design/new-session-advanced-settings.md
legacy_specs: []
---

# New session advanced settings

## Overview

One focused work order makes New Agent/handoff optional MCP selection match
New Task's Advanced settings. User confirmed the exemplar on 2026-10-26.
Inspection of current sibling plans found no explicit work order for this
creation-form layout. No new task/session or delegated worker is required.

## Scope

Replace the oversized inline creation picker with a collapsed Advanced settings
section and a compact overlay picker. Preserve current form and selection
semantics. Exclude backend changes, existing-session Customize, profile settings,
native package controls and changes to New Task itself.

## Technical approach

Follow the [design](../../specs/ui/system-design/new-session-advanced-settings.md).
Use the new-session form as state owner; move only presentation. Reference
`TaskCreateAdvancedSettings` and `TaskCreateMCPSettingRow` without importing their
task-specific context or help text. Inspect live main before implementation.

## UI preview

Structured preview replaces ASCII per the conversation's rich-output preference.
These are structural requirements; typography uses the existing New Task tokens.

| View | Order and grouping | Picker behavior |
| --- | --- | --- |
| UI-01 current desktop | Profile; Context; outlined Session MCP additions; expanded heading/help/search/cards; Prompt; actions | Inline list stretches form |
| UI-02 proposed collapsed | Profile; Context; Prompt/recovery; Advanced settings; Cancel/Start Agent | No visible MCP list or count |
| UI-03 proposed expanded | Same, with compact MCP servers / selected-count row under Advanced settings | Count opens bounded anchored popover; form height stays stable |
| UI-04 proposed touch | Same outer hierarchy with touch-size disclosure and row | Count opens sheet with title/close/search and one scrolling list; closing returns to row |

UI-02/03 cover AC-001.1 through AC-001.4. UI-04 covers AC-001.3 and AC-002.2/3.
Loading/empty/unavailable feedback stays inside the picker or count control as
appropriate; selection and prompt drafts survive close/collapse (AC-002.1/4).
Context remains outside Advanced settings. No additional optional fields move.

## Tests and E2E

Extend `components/task/new-session-dialog.test.tsx` or add focused
`components/task/new-session-advanced-settings.test.tsx` for disclosure, count,
draft retention and no-submit actions (AC-001.1/2/4, AC-002.1/4).

Add `e2e/tests/session/new-session-advanced-settings.spec.ts` and
`mobile-new-session-advanced-settings.spec.ts`, sharing a narrow helper if useful.
Cover actual creation with explicit additions, collapse/reopen, handoff,
keyboard Escape/focus, long-name and empty/loading/unavailable states, coarse
tablet and phone viewport containment (all ACs). Retain New Task selection and
existing-session picker checks as regression controls. Inspect desktop/phone
captures; record any unavailable state fixture rather than claiming coverage.

## Work orders

- [x] [Task 01: Compact launch configuration](task-01-compact-launch-configuration.md)

## Verification results

Implementation is complete. All 48 targeted unit tests, 20 desktop browser tests
and 13 mobile browser tests passed. Web typecheck, full lint and i18n checks
passed. Desktop, phone, 320px pseudo-locale and coarse-tablet captures were
inspected. See [Task 01](task-01-compact-launch-configuration.md#results) for
commands, regression coverage, documentation checks and runner setup.
PR review added IME confirmation and accessible-count regressions. Remote CI
and automated review verification remain in progress.

## Risks

- Popover portal/focus ownership inside a modal can close or trap the wrong layer.
- Conditional rendering can erase drafts if selection state moves into the shell.
- Shared selection content serves existing-session controls; keep its behavior intact.
- Current main and pending siblings can change shared picker code before delivery.

## Implementation baseline

The four handoff files were restored in this isolated workspace. After the user
requested implementation, the branch was fast-forwarded to main at
`6a1a2ae10003c6bccc754ab144c6035ac38158d1`. At this baseline the existing selector
also serves Task Edit. The implementation therefore adds a creation-only overlay
and reuses the shared selection body without changing its other consumers.
