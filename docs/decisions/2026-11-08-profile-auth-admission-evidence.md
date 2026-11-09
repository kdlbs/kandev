# ADR-2026-11-08-profile-auth-admission-evidence: Use scoped runtime bindings for authentication admission

**Status:** proposed
**Date:** 2026-11-08
**Area:** backend

## Context

Before this change, workflow compatibility ignored agent-profile authentication and accepted nonempty executor secret IDs without resolving them.
Issue [#5180](https://github.com/kdlbs/kandev/issues/5180) demonstrated a working SSH API-key route rejected by that gate.
Admission must reflect authorized configuration without creating new secret-sharing authority.

## Decision

The proposed admission rule uses the selected profiles' source-aware environment and existing user-visible Global secret resolution.
Task and executor authorization precede secret reads.
All required bindings must validate before any configured authentication route can satisfy admission.
Transient decrypted values establish usability but never leave the validation call.
Preview performs no provider login, provisioning, or admission write.

API-key and OAuth methods retain separate catalog identities and provisioning.
The caller's existing authority is the limit. Shared profiles do not grant access to their owners' private keys.
This proposal does not depend on unmerged owner-grant or prepared-actor work in PR #5184.

The decision remains proposed pending review.
The admission checker and Claude API-key catalog method are implemented.
The previous validator and OAuth-only Claude catalog are historical.
Browser and SSH end-to-end checks remain pending.

## Consequences

Authorized profile API keys can pass without duplicate executor settings.
Bogus, inaccessible, or deleted references that previously passed configuration checks will fail.
Validation adds bounded local secret-store reads and decryption.
Provider-side credential revocation remains an execution-time concern.
Launch must repeat resolution because preview creates no credential lease or authority.

## Alternatives considered

- **Add only an API-key catalog method:** Insufficient because the checker still ignores profile environment bindings.
- **Accept any nonempty secret ID:** Cannot establish existence, scope, caller access, decryption, or a nonempty value.
- **Use metadata checks only:** Establishes scope and visibility but cannot detect an unreadable or empty stored value.
- **Borrow a profile owner's identity:** Widens access and duplicates a distinct, unmerged sharing proposal.
- **Authenticate against the provider during preview:** Adds network effects, cost, and account behavior to a read-only workflow operation.
- **Use host login state:** Does not establish which credentials the authorized remote child receives.

## References

- [Requirements](../specs/agents/requirements/remote-auth-admission.md)
- [Design](../specs/agents/system-design/remote-auth-admission.md)
- [Plan and evidence](../plans/claude-profile-admission/plan.md)
- [Existing private-compute decision](2026-10-15-private-executor-admission.md)
