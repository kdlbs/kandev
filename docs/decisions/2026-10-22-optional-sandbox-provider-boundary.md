# ADR-2026-10-22-optional-sandbox-provider-boundary: Keep wrapping in core and add an optional sandbox capability

**Status:** proposed for design review
**Date:** 2026-10-22
**Area:** protocol

## Context

Generic prefixes already span discovery, ACP, recovery, and native terminal paths in PR #4538.
Issue #3812 adds policy selection, approvals, and learning, which need a stronger authority and lifecycle contract.
Production plugins belong in dedicated repositories.

## Decision

Keep command composition, admission, policy authority, process ownership, and cancellation in core.
Add an optional versioned sandbox provider capability over the existing plugin transport.
The provider compiles policy and normalizes provider evidence. It cannot approve access or replace the admitted child command.
Place the production Nono provider in a dedicated repository.
A required provider failure blocks launch. It never removes confinement.

The initial control channel uses Nono's verified file-approval webhook protocol.
Core must prove isolation of its per-attempt callback capability before enabling approvals.
`inspect --json` supplies reports only.

## Consequences

Basic wrapping remains available without plugin installation.
The host owns persistent security decisions and cleanup when a provider fails.
The extension needs new protocol, manifest, SDK, and compatibility tests.
Policy-required mode needs authenticated operator authority and cannot rely on synthetic single-user administration.
Nono support remains specific to tested runtime versions, hosts, and transports.

## Alternatives considered

A mandatory wrapper plugin would make existing prefix execution depend on installation health without improving core admission.
Nono-specific core branches would couple process control to one provider's policy and release cadence.
Reusing remote executor providers would confuse environment provisioning with local confinement.
Terminal prompt scraping would mix agent input with operator decisions and lacks a stable request identity.
An in-process Nono SDK would not preserve the verified CLI profile and supervisor contract.
Unsigned loopback without a protected capability would accept forged requests, so it is insufficient.
Signed platform enrollment remains a future option if callback isolation cannot satisfy the initial local boundary.

## References

- [Agent design](../specs/agents/system-design/sandbox-policies.md).
- [Plugin design](../specs/plugins/system-design/sandbox-provider-capability.md).
- [Evidence](../plans/nono-sandbox-integration/investigation.md).
