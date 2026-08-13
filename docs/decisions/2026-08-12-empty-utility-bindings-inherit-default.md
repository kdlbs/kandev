# ADR-2026-08-12-empty-utility-bindings-inherit-default: Empty Built-in Utility Bindings Inherit the Default

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend

## Context

Older profile migration code persisted built-in utility actions with an empty profile ID and an
`unconfigured` binding. The previous default-inheritance decision preserved that state because it
did not distinguish a deleted explicit binding from a failed legacy migration. In practice, this
leaves every built-in action unavailable when the user has selected a valid global default. The
settings repair flow can also send an invalid explicit binding when the user chooses Default.

## Decision

A built-in utility action with an empty profile ID always means that no concrete override is
recoverable. The backend normalizes an empty `unconfigured` built-in binding to `inherit`, and the
action resolves through the user's saved global default profile. This normalization is idempotent
and persists the inherited state so the database does not continue to expose the action as
unavailable.

The Utility Agents picker treats its fallback callback value, the empty string, as an inherited
selection. Saving Default sends an empty `agent_profile_id` with `profile_binding_state: "inherit"`.
It never sends an empty explicit binding.

A non-empty stale profile ID still represents a recoverable explicit choice. Such a binding remains
`unconfigured` and fails closed until the user selects Default or another eligible profile. Custom
utility agents still require a concrete eligible profile.

## Consequences

- Existing installations with empty unavailable built-in actions recover on the next binding
  migration and use the selected global default.
- Selecting Default repairs the action through the normal Settings save flow.
- The system no longer preserves hypothetical explicit intent when no profile identity remains.
- Concrete stale overrides remain diagnosable and do not silently change model, permissions,
  credentials, or launch policy.

## Alternatives Considered

1. **Keep every empty `unconfigured` row fail-closed.** Rejected because no concrete identity can be
   repaired or audited, while the product contract already gives every built-in action a default.
2. **Copy the current default profile ID into every repaired row.** Rejected because built-in
   actions then stop following later changes to the global default.
3. **Convert every stale binding to `inherit`.** Rejected because a non-empty stale ID preserves a
   real explicit user choice and its security-relevant launch configuration.
