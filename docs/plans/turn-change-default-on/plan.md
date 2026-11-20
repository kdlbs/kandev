---
created: 2026-11-20
status: implemented
requirements:
  - REQ-TASKS-TURN-CHANGES-001
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Enable turn changed files by default

## Overview

New accounts start with **Show changed files after each turn** on. Settings
without a saved value use the same default. Existing accounts keep their saved
choice. Tasks owns the preference because it controls capture at turn admission
and card visibility in the transcript.

## Scope

### In scope

- Default-on account seeds, backend reads, and frontend state.
- Save, reload, and hydration coverage for an explicit off value.
- The preference default and current location in the user guide.

### Out of scope

- Existing preference values, capture timing and limits, and historical data.

## Technical approach

The account store seeds both the synthetic default user and new accounts from
`newSidebarPresentationJSON`. Set that key, `defaultUserSettings`, and
`createDefaultUserSettings` to true. The pointer and nullish checks already
preserve saved false values. The existing migration also preserves them.
A settings read error still disables capture for that turn.

The [earlier default-off work](../turn-change-default-off/plan.md) identifies the
same three defaults. This plan changes them together and uses the existing
settings save path. Follow the
[settings authority design](../../specs/tasks/system-design/turn-changed-files.md#settings-and-authority).

## ASCII UI preview

### UI-01 Conversation preference for a fresh account

```text
Settings > Preferences > Task Behavior > Conversation

Before
Show changed files after each turn                 [OFF]

After
Show changed files after each turn                  [ON]
```

Desktop and phone use the same switch row in the existing settings scroll
area. Phone users can tap the switch and save a choice to turn it off.
`AC-TASKS-TURN-CHANGES-001.1` governs the initial value. `.2` governs the saved
off choice.

## Tests

- Update the backend account-seed and settings-decoder expectations. The new
  expectation fails against today's stored default and missing-key fallback.
- Update `turn-changed-files-user-settings.test.ts` to expect true before
  hydration. Hydrate false explicitly, then omit the field in a partial update.
- Keep existing migration and service tests for a saved false value.

## E2E tests

Update the existing desktop and phone Conversation settings specs. Verify a
fresh account starts on, save off, and reopen the settings page. The phone
case also covers Reset before saving. Both flows cover `.1`–`.2`.

## Work orders

- [x] [Task 01. Default changed files to on](task-01-default-on.md)

## Verification results

- Account seeds, settings reads, and browser hydration use the on default.
  SQLite and PostgreSQL 16 tests confirmed that saved off choices persist.
- Five focused frontend tests and the desktop and phone browser cases passed.
- Typecheck, lint, public docs validation, and specification checks passed.

## Risks

- Previously seeded off values use the same stored boolean as a saved opt-out.
  Preserve both values when reading an existing account.
