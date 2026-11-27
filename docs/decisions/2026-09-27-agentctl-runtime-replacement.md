# ADR-2026-09-27-agentctl-runtime-replacement: Recover the child runtime without restarting the backend

**Status:** accepted
**Date:** 2026-09-27
**Area:** backend, protocol, operations

## Context

Issue #3962 identifies an unexpected standalone agentctl exit while the backend remains healthy.
Its Git staging explanation is unproven and is not the basis for this decision.
Existing clients retain startup credentials, so a launcher-only restart creates inconsistent owners.
The user requested an in-process recovery package for PR #3598.

## Decision

A backend-owned coordinator replaces the local agent runtime through generation-fenced immutable bindings.
It performs bounded automatic recovery and exposes an authorized manual retry after exhaustion.
The backend, browser connection, and unrelated remote executions remain live.
Infrastructure replacement never authorizes automatic prompt resend or replay of external side effects.
Per-session uncertainty persists independently of global runtime availability.

This replaces only the monotonic-availability and mandatory-full-restart decisions in
[the earlier crash-containment record](2026-08-08-agentctl-crash-containment.md).
Its immutable payload, sanitized diagnostics, and persistent visibility decisions remain applicable.
Implementation is pending. The [requirements](../specs/platform/requirements/agent-runtime-availability.md),
[design](../specs/platform/system-design/agent-runtime-availability.md), and
[work package](../plans/agentctl-runtime-replacement/plan.md) define the complete change.

## Consequences

All local-runtime consumers must use a shared connection owner and reject retired-generation results.
Safe recovery requires process identity, credential rotation, consumer rebinding, and durable reconciliation.
A healthy runtime may coexist with sessions that need explicit recovery.
The PR cannot ship an intermediate state where only some clients can follow a replacement.

## Alternatives considered

- Full backend restart only: preserves current wiring but unnecessarily interrupts healthy application services.
- Relaunch inside monitorExit: cannot coordinate consumers, ownership renewal, and session guards.
- Transparent HTTP mutation retries: can repeat prompts, Git operations, or external tool effects.
- Intercept harness Git: does not match the actual execution path or establish the reported root cause.
