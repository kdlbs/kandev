# ADR-2026-10-30-sandbox-evidence-and-operation-authority: Separate evidence claims and typed operation authority

**Status:** proposed for design review
**Date:** 2026-10-30
**Area:** protocol

## Context

The initial Nono adapter exposes less information than the pinned runtime supplies. Network and command audit records already exist. Nono 0.81.0 adds CONNECT approval handling, but proxy requests can have PID zero and a shared proxy session label. Its session registry has no current effective-policy digest. A completed audit signature proves a different fact from live confinement.

The current optional sandbox protocol rejects unknown fields. Its filesystem-shaped approvals cannot represent a command or HTTP endpoint without losing authority boundaries.

## Decision

Keep process liveness, launch validation, runtime observations, audit integrity, and signer trust independent. Preserve the existing strict active-confinement rule. Never synthesize fresh evidence from an old launch or finalized audit.

Introduce sandbox protocol major 2 over the existing optional six RPCs. Preserve major 1 decoding and semantics for captured older attempts. Use typed filesystem, TCP, command, and HTTP operations with separate static grants and approval envelopes. No automatic downgrade is permitted.

A child operation binds to verified process ownership. A runtime-service operation binds to its protected, attempt-owned channel and declared route. PID zero is an explicit absence, not an identity to replace with the agent PID.

Core retains policy authority, immutable records, supervisor/PTY ownership, and cancellation. The dedicated provider owns Nono formats and runtime-specific compilation. Basic command wrapping remains in core.

## Consequences

Operators receive useful partial evidence without an unsupported protection badge. Providers can expose service-originated approvals without weakening child checks. Versioning, policy migration, channel isolation, and end-to-end enforcement require independent tests. Older reports and signatures remain historically interpretable.

Nono's native approval proxy must prove exact-destination enforcement before it replaces the earlier upstream-relay path for new capability-selected attempts. A source declaration alone cannot enable support.

## Alternatives considered

Treating a running process as verified confinement hides missing policy evidence. Treating every fact as unknown discards useful observations. A synthetic policy digest in a provider reply supplies no new evidence.

Appending new payloads under protocol 1 contradicts strict unknown-field validation. Encoding commands or endpoints in a string target loses typed ceiling checks. Accepting PID zero through the child identity check destroys its meaning.

A shared mutable network allowlist would make one approval available to unrelated connections. Global host trust-store changes would exceed one attempt's authority. Neither is allowed.

## References

- [Original boundary](2026-10-22-optional-sandbox-provider-boundary.md).
- [Evidence design](../specs/agents/system-design/sandbox-evidence.md).
- [Operation design](../specs/agents/system-design/sandbox-operation-approvals.md).
- [Provider major 2](../specs/plugins/system-design/sandbox-provider-v2.md).
