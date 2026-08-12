# ADR-2026-08-12-built-in-utility-default-inheritance: Built-in Utility Actions Inherit the Global Profile by Default

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend

## Context

Profile-backed utility agents can represent an action without a concrete profile override by using
the global default utility profile. The current migration and dependency cleanup paths can instead
mark a built-in action `unconfigured`, so the Utility Agents page shows an unavailable profile even
when a valid default is selected. The system needs to distinguish an empty built-in override from a
stale concrete profile binding so default inheritance remains convenient without weakening the
fail-closed behavior for deleted or disabled profiles.

## Decision

Built-in utility actions with a persisted `inherit` state and no concrete profile ID resolve the
saved global default utility profile. Legacy built-in rows still in the migration state's `explicit`
form with empty, unmatched, or ambiguous agent/model values are normalized to `inherit`; custom rows
with those migration results remain `unconfigured`. Existing `unconfigured` rows are preserved and
fail closed, including when their ID is empty, because an older release could have erased the ID of a
deleted explicit binding and its original intent cannot be recovered safely.

When an explicit profile binding is deleted, the binding keeps its stale profile ID and becomes
`unconfigured`. When the global default profile is deleted, inherited built-in rows remain inherited.
The UI displays the selected default for inherited actions, while concrete stale bindings remain
visible as unavailable and execution continues to fail closed until repaired.

## Consequences

- Legacy built-in actions still in the migration state recover automatically from ambiguous data, and
  inherited actions recover automatically from replacement of the global default.
- Selecting a default profile is sufficient for the normal built-in action path; users do not need
  to edit every action row.
- Explicit stale bindings remain diagnosable and do not silently change permissions, models, or
  credentials.
- Ambiguous empty `unconfigured` rows from older releases remain unavailable until repaired instead
  of risking a silent permission or model change.
- Migration and dependency tests must cover both empty inherited rows and stale concrete IDs.

## Alternatives Considered

1. **Keep every legacy migration row unconfigured.** Rejected because built-in rows in the migration
   state have no recoverable concrete override when matching is ambiguous.
2. **Make every unconfigured row inherit the default.** Rejected because older releases erased
   deleted profile IDs, so an empty unconfigured row could be an explicit stale binding.
3. **Select the first eligible profile during migration.** Rejected because matching profiles can
   carry different models, permissions, flags, wrappers, or credentials.
