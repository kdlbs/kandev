# ADR-2026-10-02-bounded-managed-npm-startup-retry: Retry managed npm setup without deleting shared runtime trees

**Status:** accepted
**Date:** 2026-10-02
**Area:** backend, agentctl, workflow

## Context

Issue 4152 reports burst launches ending FAILED before ACP initialization.
The original process-exit cause remains unconfirmed. Existing recovery handles only strict exact-package npm ETARGET.
The user prioritizes automatic setup recovery, especially npm failures, over secondary failed-session inspection protection.

Managed task launches and host capability probes can share the same npm execution tree.
Deleting that tree during automatic recovery can interfere with another process that still uses its files.
This interference is a design risk, not a proven cause of the reported incident.

## Decision

Permit one automatic replacement for classified temporary npm errors or a confirmed unexpected ordinary exit before ACP initialization completes.
Use typed phase and process-generation evidence. Closed-pipe text alone is insufficient.
Known permanent failures, signal termination, cancellation, unknown evidence, and post-initialize failures remain ineligible.

Keep the original exact package, version, session, executor, and environment.
Confirm old-process cleanup before replacement. Use a cancellable two-second delay plus up to one second of jitter.
Bound recovery to 90 seconds or the caller's earlier deadline. Reuse the existing one-replacement generation fence.

Automatic task and host capability-probe recovery shall not delete shared npm execution trees.
Known npm recovery uses online-preferred metadata. Unexplained early exit retries the unchanged command.
Probe eligibility remains strict ETARGET; this decision does not add broad probe retries.
Explicit Settings maintenance retains its existing scoped cache repair contract.

This qualifies the automatic deletion portions of the
[executor-local repair decision](2026-08-24-agentctl-local-managed-runtime-cache-repair.md)
and [host probe recovery decision](2026-09-07-host-utility-managed-runtime-recovery.md).
The executor-local ownership, exact-version selection, and configured-registry rules remain authoritative.

## Consequences

A transient setup failure can recover without user action or a new session.
Randomized delay spreads retries from a burst without serializing healthy launches.
Permanent or repeated failures remain visible. No automatic recovery can promise success during an outage.

Non-destructive recovery may leave a genuinely corrupted execution tree unresolved.
That case remains an explicit maintenance action instead of risking active sibling processes.
Older agentctl versions retain strict ETARGET recovery; missing structured evidence never grants the new early-exit fallback.

## Alternatives considered

- A global launch mutex: delays healthy sessions and does not establish the reported cause.
- Delete the shared execution tree for every setup failure: can disrupt siblings and misdiagnoses unexplained exits as cache corruption.
- Retry arbitrary ACP errors: can replay conversation or prompt work after initialization.
- Unlimited retries: hides permanent failures and can consume resources indefinitely.
- Wait for proof of the original npm race before adding recovery: unnecessary for the independently confirmed retry-coverage gap.

## Related records

- [Requirements](../specs/agents/requirements/managed-npm-runtime-recovery.md)
- [Design](../specs/agents/system-design/managed-npm-runtime-recovery.md)
- [Primary fix package](../plans/managed-npm-startup-resilience/plan.md)
