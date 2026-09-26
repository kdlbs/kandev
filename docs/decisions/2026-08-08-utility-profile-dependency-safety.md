# ADR-2026-08-08-utility-profile-dependency-safety: Warn Before Breaking Utility Profile Bindings

**Status:** accepted (amended 2026-08-14)
**Date:** 2026-08-08
**Area:** backend, frontend, protocol

## Context

Utility agents are unattended callers. A profile disable or delete can therefore stop future
utility calls without a user being present to repair the binding. Existing profile deletion already
uses an in-use confirmation flow for other dependents. Legacy utility settings also need a safe
representation when an old agent/model pair matches no profile or several profiles.

## Decision

### Amendment (2026-08-14)

Utility bindings may name concrete or dynamic profiles. Dynamic candidate
references also participate in the same dependency dialog. A confirmed removal
keeps the stored reference: a missing logical dynamic profile makes the utility
binding unconfigured, while a missing concrete candidate makes only that route
ineligible. The dynamic conductor can select another configured candidate; this
is routing authorized by the selected profile, not an implicit utility-profile
replacement.

Profile dependency checks include utility-agent bindings.

- Disabling a referenced profile shows a dependency warning before the setting is saved. The user can
  cancel or confirm. Confirmation keeps the utility bindings unchanged; new calls fail closed until
  the user repairs them.
- Deleting a referenced profile uses the existing profile-in-use dialog. The dialog lists affected
  utility agents. Deletion requires explicit confirmation. A confirmed delete does not rewrite the
  utility rows or select a replacement profile.
- A dependency lookup error blocks both disable and delete. The backend fails closed because an
  unknown reference must not be treated as no reference.
- Profile changes do not cancel an in-flight utility call. That call uses its start-time launch
  snapshot.

Utility binding persistence has an explicit state separate from the profile ID:

- `inherit`: a built-in action uses the user default.
- `explicit`: the row names an eligible concrete or dynamic profile.
- `unconfigured`: migration or a prior profile change left no executable profile.

The migration keeps legacy agent/model values as read-only inputs. It writes `explicit` only when
exactly one eligible profile matches. Zero or multiple matches write `unconfigured`; they never become
implicit default inheritance.

## Consequences

- Users see the impact before they disable or delete a profile.
- Confirmed destructive changes remain reversible at the configuration level: utility rows keep
  their IDs and can be repaired after a replacement profile is selected.
- Utility execution remains fail-closed and never changes permissions by selecting a fallback.
- Profile CRUD must query utility references and return typed conflict details. The frontend must
  render those details in the existing warning/confirmation patterns.
- Utility persistence needs one additional binding-state field and migration tests for ambiguous
  legacy data.

## Alternatives Considered

1. **Silently allow the profile change.** Rejected because unattended calls would fail later with no
   warning at the point of change.
2. **Automatically reassign each utility agent to the default or first profile.** Rejected because
   this can change model, credentials, and permissions without an explicit user choice.
3. **Block every disable or delete until all utility rows are manually changed.** Rejected because
   disabling is a valid emergency action and the existing delete confirmation flow supports an
   explicit, informed override.
4. **Represent every empty profile ID as default inheritance.** Rejected because an ambiguous legacy
   binding would then run with a different profile by accident.
