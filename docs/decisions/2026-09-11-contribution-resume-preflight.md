# ADR-2026-09-11-contribution-resume-preflight: Separate resume admission from push history readiness

**Status:** proposed
**Date:** 2026-09-11
**Area:** backend, protocol, security

## Context

A contribution branch can change while a task is stopped. The startup dry-run
push then rejects local HEAD as non-fast-forward and prevents the agent from
resuming. A history rejection does not establish missing write permission.

The existing contribution binding decision requires a preflight before startup.
The local-first contribution decision preserves task work after remote drift.
Resume needs an explicit rule that reconciles these contracts.

## Decision

For existing contribution sessions, a typed history-only preflight rejection
does not prevent resume. Initial creation retains its existing gates. Unknown,
authentication, permission, destination, and transport failures remain blocking.
The probe remains non-mutating and does not report rejected pushes as successful.

This qualifies the preflight rule in
[the contribution binding ADR](2026-08-04-remote-contribution-bindings.md).
It preserves [user-controlled version replacement](2026-08-12-local-first-contribution-replacement.md).
Resume never selects a merge strategy or replaces either history automatically.

## Consequences

The agent can inspect local work after remote updates. A later real push can
still fail, including for permission reasons not established by the dry run.
All repositories must pass admission independently. Tests must cover cold
resume, workspace promotion, mixed failures, and malformed probe output.

## Alternatives considered

- Keep every rejection fatal: prevents the agent from inspecting the state
  that needs reconciliation.
- Skip every preflight on resume: hides authentication and destination faults.
- Pull or force-push during resume: chooses a history policy without user intent.

## Delivery

See the [fix package](../plans/contribution-resume-recovery/plan.md).
This decision is proposed; production behavior has not changed.
