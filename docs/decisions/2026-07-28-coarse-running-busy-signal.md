# ADR-2026-07-28-coarse-running-busy-signal: Restore Coarse Running Prompt Admission

**Status:** accepted
**Date:** 2026-07-28
**Area:** backend, frontend, protocol

## Context

ADR-0049 allowed a `RUNNING` session to accept input when Kandev inferred that
the foreground had yielded to recognized background work. In practice, ACP
providers do not report subagent and background-work lifecycles with enough
consistent identity, nesting, or terminal-state detail to make that inference a
reliable admission boundary. Misclassification can enable overlapping prompts,
show an incorrect background status, or leave a session transition waiting on
an event the provider never emits.

## Decision

Restore the conservative pre-ADR-0049 operator contract:

- Every `RUNNING` session rejects direct prompt admission and routes composer
  input through the existing queued-message path.
- Every `RUNNING` session is exposed as `foreground_activity=generating`.
  Settled sessions do not expose a background activity tier.
- `session.activity_changed`, `session.state_changed`, boot payloads, REST
  session DTOs, and task aggregates follow the same coarse activity policy.
- The in-memory background-work tracker and adapter attestations remain in place
  as dormant accounting infrastructure. They may continue to supply
  `active_subagent_count`, but they do not relax admission or select an
  operator-visible background activity tier.

Re-enabling fine-grained admission requires a new decision backed by provider
protocol guarantees and fixtures that cover launch, foreground yield,
correlation, and accountable terminal completion.

## Consequences

Prompt admission is safe and consistent across ACP providers, and the composer
cannot advertise direct input that the coarse lifecycle considers busy. An
operator may need to queue a follow-up while a held-open foreground turn waits
on background work. Detached work that outlives a completed foreground turn is
no longer shown as a distinct background-running tier. Keeping the tracker
avoids a risky large revert and preserves evidence for a future protocol-backed
implementation.

## Alternatives Considered

- **Revert ADR-0049 and all implementation commits.** Rejected because later
  lifecycle hardening and accounting work spans many subsystems; removing it
  immediately creates more release risk than disabling the policy boundary.
- **Disable only prompt admission.** Rejected because the UI would still
  advertise direct input or a background state that no longer controls
  admission.
- **Add a runtime feature toggle.** Rejected for this release because an
  operator-facing switch would expose behavior known to be unreliable rather
  than establishing one safe default.
