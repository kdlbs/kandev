# ADR-2026-07-28-coarse-running-busy-signal: Default to Coarse Running Prompt Admission

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

Restore the conservative pre-ADR-0049 operator contract as the shipped default:

- Every `RUNNING` session rejects direct prompt admission and routes composer
  input through the existing queued-message path.
- Every `RUNNING` session is exposed as `foreground_activity=generating`.
  Settled sessions do not expose a background activity tier.
- `session.activity_changed`, `session.state_changed`, boot payloads, REST
  session DTOs, and task aggregates follow the same coarse activity policy.
- The in-memory background-work tracker and adapter attestations remain in place
  as accounting infrastructure. They may continue to supply
  `active_subagent_count`, but they do not relax admission or select an
  operator-visible background activity tier by default.

Retain ADR-0049's complete Claude Code behavior behind the default-off
`features.claudeBackgroundPromptHandoff` runtime toggle. The exception is
effective only for `claude-acp` sessions (plus the mock provider in E2E):

- adapter-attested Claude subagents, background shells, and Monitor watches may
  expose the background activity tier;
- a generation-matched Claude foreground handoff may admit the next prompt
  while the durable session remains `RUNNING`; and
- every other provider remains coarse even when the toggle is enabled.

The toggle is experimental, high risk, restart-required, and off in every
embedded profile. Missing provider identity fails closed. Its purpose is to let
designated contributors gather protocol fixtures and refine the behavior
without changing the release default.

## Consequences

Prompt admission is safe and consistent across ACP providers by default, and
the composer cannot advertise direct input that the coarse lifecycle considers
busy. An operator may need to queue a follow-up while a held-open foreground
turn waits on background work. Detached work that outlives a completed
foreground turn is not shown as a distinct background-running tier unless the
Claude experiment is deliberately enabled.

The opt-in path preserves a focused real-world test bed without exposing known
ACP lifecycle uncertainty to every user. Enabling it accepts the original
risks: missing, delayed, duplicated, or ID-less Claude lifecycle frames can
misclassify activity or promptability. The environment variable or persisted
runtime override provides a kill switch, but it does not make those signals
authoritative.

## Alternatives Considered

- **Revert ADR-0049 and all implementation commits.** Rejected because later
  lifecycle hardening and accounting work spans many subsystems; removing it
  immediately creates more release risk than disabling the policy boundary.
- **Disable only prompt admission.** Rejected because the UI would still
  advertise direct input or a background state that no longer controls
  admission.
- **Remove the fine-grained path completely.** Rejected because a default-off,
  Claude-scoped experiment gives the original contributor a controlled way to
  reproduce and improve all supported background modes without changing the
  release default.
