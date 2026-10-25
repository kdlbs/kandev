# ADR-2026-10-20-silent-prompt-acceptance: Silence does not prove a failed start

**Status:** accepted
**Date:** 2026-10-20
**Area:** backend, protocol

## Context

On 19 October, an accepted Codex prompt produced no output for five minutes.
Kandev classified it as never started and forced its execution to stop.
The interrupted dispatch then required durable delivery recovery.
The original reason for Codex's silence remains unknown.

Isolated codex-acp 2.2.2 / Codex 0.160.1 experiments reproduced a first stream
timeout notice at 300.152 seconds, just beyond Kandev's cutoff. Native WebSocket
retries also remained invisible to ACP until HTTP fallback. Acceptance and
absence of visible progress can therefore coexist.

## Decision

Prompt acceptance, visible progress, and terminal outcome are separate facts.
Acceptance by the current agentctl owner prevents an inactivity-only startup
failure. Missing acceptance evidence is unknown, not proof that dispatch failed.
Explicit startup errors and correlated terminal failures retain their existing
failure paths.

The five-minute watchdog remains advisory for silent, unsettled prompts.
It offers the existing Cancel turn action. Cancellation uses the existing
bounded protocol and owner checks. If cancellation cannot establish an outcome,
durable delivery remains blocked; neither silence nor a local cancellation
timeout permits replay.

Acceptance must not set output/effect evidence used by prompt retry policy.
Transport metadata must not extend the inactivity clock. Native Resume may
wait boundedly for projection, but must rebuild its complete authenticated
proof before resolving any block.

This supersedes the classification in
[the August decision](2026-08-18-never-started-agent-stall-terminal.md).
It also supersedes inactivity-only terminal teardown in
[the September decision](2026-09-02-terminal-stall-owns-process-teardown.md).
That decision's progress clock and execution-registry stop authority remain.

## Consequences

A silent accepted prompt can remain running until it completes, fails with
trusted evidence, or the user cancels it. The notice makes that state visible.
Kandev cannot promise that a silent provider will recover.

Restore must not invent acceptance from a session-info frame or an old native
session ID. Diagnostics report what Kandev observed and identify unavailable
provider information explicitly. This decision requires no vendor log reader,
new timer setting, database migration, or provider-specific watchdog threshold.

## Alternatives considered

- Increase the watchdog timeout: shifts the race without establishing failure.
- Update Codex alone: Codex 0.162.1 reproduced the hidden retry interval.
- Cancel every silent turn automatically: discards work without terminal proof.
- Treat any metadata as progress: a hung adapter could postpone detection.
- Remove projection validation: could release admission before effects settle.

## Implementation record

The [work package](../plans/silent-prompt-recovery/plan.md) tracks delivery.
This accepted decision describes intended behavior, not a shipped fix.
