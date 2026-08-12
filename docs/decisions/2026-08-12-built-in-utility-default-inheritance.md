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

Built-in utility actions with no concrete profile ID use `inherit` and resolve the saved global
default utility profile. Legacy built-in rows with empty, unmatched, or ambiguous agent/model values
are normalized to `inherit`; custom rows with those migration results remain `unconfigured`.

When an explicit profile binding is deleted, the binding keeps its stale profile ID and becomes
`unconfigured`. When the global default profile is deleted, inherited built-in rows remain inherited.
The UI displays the selected default for inherited actions, while concrete stale bindings remain
visible as unavailable and execution continues to fail closed until repaired.

## Consequences

- Existing built-in actions recover automatically from ambiguous legacy data and from replacement of
  the global default.
- Selecting a default profile is sufficient for the normal built-in action path; users do not need
  to edit every action row.
- Explicit stale bindings remain diagnosable and do not silently change permissions, models, or
  credentials.
- Migration and dependency tests must cover both empty inherited rows and stale concrete IDs.

## Alternatives Considered

1. **Keep every ambiguous built-in row unconfigured.** Rejected because built-in actions have a
   documented global-default path, and this leaves valid installations showing unusable controls.
2. **Make every unconfigured row inherit the default.** Rejected because it would erase the
   fail-closed distinction for an explicitly deleted or disabled profile.
3. **Select the first eligible profile during migration.** Rejected because matching profiles can
   carry different models, permissions, flags, wrappers, or credentials.
