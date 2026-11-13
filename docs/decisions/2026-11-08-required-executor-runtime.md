# ADR-2026-11-08-required-executor-runtime: Required executor runtime admission

**Status:** accepted
**Date:** 2026-11-08
**Area:** backend, protocol

## Context

Issue [#5189](https://github.com/kdlbs/kandev/issues/5189) reports Local Docker work running inside the backend service container.
At main `9aba7b2e`, missing built-in backends can fall back to standalone under the production warning policy.
The image disables Docker registration by default. Requested metadata can therefore disagree with actual runtime identity.
Cache and promotion paths can return an existing worker without checking that boundary.

The user explicitly requires the selected runtime or refusal, including retained paths.
Existing compute ownership and workspace contracts do not grant permission to substitute execution environments.

## Decision

A nonempty effective executor selection is required after existing precedence resolution.
It must use its mapped permitted runtime or fail before work starts outside that boundary.
This also applies to concrete selections supplied by saved defaults and workflow bindings.
Only a genuinely unspecified selection uses the existing standalone default.
Internal allow/warn policies cannot override a required selection.

Admission compares actual retained runtime identity before new commands, prompts, credentials, or promotion.
Passive recovery can retain truthful observation and authorized Stop without admitting new work.
Mismatch does not authorize relabelling, automatic environment migration, or destructive cleanup.
A separately authorized environment transition retains its existing exact-predecessor retirement rules.
The predecessor cannot satisfy the new runtime and must not be confused with same-binding fallback evidence.

Lifecycle remains the only writer of actual execution inventory.
Session APIs expose that inventory separately from configured executor/profile identity.
Docker success requires a distinct daemon-verifiable worker container identity.
Containerized backend membership and filesystem markers never prove worker isolation.

## Consequences

Installations that depended on implicit host fallback will receive actionable failures.
They must deliberately choose a supported executor or configure the requested one through their normal operator process.
This repair does not mount sockets, edit deployment configuration, or add an isolation bypass.
Healthy explicit runtimes and Local/Worktree defaults retain their mappings.

Historical mismatches can block new work while preserving history, inventory, and Stop.
Implementers must guard reuse and prompt boundaries as well as initial provisioning.
Runtime diagnostics require an additive read projection, without a new SQL source of truth.
The package remains a design handoff. Accepted direction does not mean implementation is complete.

## Alternatives Considered

- **Change only production wiring to deny:** Corrects missing-backend creation but leaves caches, promotion, and late credential checks unguarded.
- **Special-case disabled Docker:** Leaves identical required-selection failures for other recognized missing built-in backends.
- **Keep warning fallback with more logging:** Preserves execution outside the user's selected boundary.
- **Add a per-request fallback permission:** Creates another authority and compatibility surface without a requested supported use case.
- **Relabel fallback workers as Docker:** Makes configuration agree cosmetically while concealing the actual process boundary.
- **Automatically replace retained workers:** Risks workspace, conversation, journal, and ownership continuity beyond this repair's authority.
- **Use hostname or `/.dockerenv`:** Cannot distinguish the backend service container from a separate runner.

## References

- [Runtime requirements](../specs/executors/requirements/runtime-selection.md).
- [Runtime design](../specs/executors/system-design/runtime-selection.md).
- [Private executor admission](2026-10-15-private-executor-admission.md).
- [Execution inventory ownership](0003-executors-running-as-execution-id-source-of-truth.md).
- [Authenticated durable sessions](2026-09-10-durable-harness-session-boundaries.md).
- [Plan](../plans/explicit-executor-runtime-isolation/plan.md).
