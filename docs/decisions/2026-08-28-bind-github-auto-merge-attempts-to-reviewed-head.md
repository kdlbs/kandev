# ADR-2026-08-28-bind-github-auto-merge-attempts-to-reviewed-head: Bind GitHub auto-merge attempts to the reviewed head

**Status:** accepted
**Date:** 2026-08-28
**Area:** backend, frontend, protocol, security, GitHub

## Context

Kandev refreshes a linked pull request before it starts an automatic merge.
The GitHub request does not include the head SHA from that refresh.
Thus, a force-push can change the merge target between the refresh and the request.

Kandev records an automatic merge signature only after GitHub accepts the request.
An unchanged provider error therefore starts another request during each later evaluation.
This behavior conflicts with the durable per-PR deduplication contract.

The automation error row has one Retry control.
For a stored automation error, this control reloads state but does not retry the failed operation.
An active queue observation can also prove that GitHub accepted a request while the old error remains visible.

## Decision

Each automatic GitHub merge request must include the non-empty head SHA from the fresh readiness snapshot.
GitHub must reject the request if the current head differs from that SHA.

Kandev must reserve an automatic merge attempt in durable per-PR state before it calls GitHub.
The reservation includes the readiness signature, head SHA, time, and attempt result.
The readiness signature includes all merge gates that can re-arm automatic work.

An unchanged signature cannot start another automatic request after an in-flight, failed, queued, or merged result.
A changed signature can start one new automatic request after all readiness gates pass.
If Kandev cannot persist the reservation, it does not call GitHub.

An explicit retry is a scoped command, not an options update or a state refresh.
The command names one linked pull request and authorizes one new evaluation of a failed signature.
It does not bypass readiness checks, head binding, repository scope, or GitHub policy.

Automation errors carry a typed operation source.
An authoritative active queue or merged observation reconciles the attempt as accepted.
That observation clears only an obsolete auto-merge error.

Pending GitHub merge operations use bounded status polling.
The clients wait at least one second between status requests and stop when the operation context ends.

## Consequences

- A reviewed readiness snapshot cannot merge a later force-pushed head.
- Restarts and repeated PR polls do not replay one automatic merge side effect.
- A user can retry a failed merge without toggling automation or changing the pull request.
- Queue adoption removes a stale merge-submission error without erasing an unrelated automation error.
- The per-PR automation state gains typed attempt results and error sources.
- Existing recognized merge errors need a compatibility backfill for their error source.
- Client tests need a controllable poll delay so focused tests remain fast.

## Alternatives Considered

### Trust the current GitHub head at request time

Rejected. The request could merge code that did not produce the readiness snapshot.

### Retry every provider error during later polls

Rejected. Repeated automatic side effects can consume rate limits and hide persistent authorization or policy errors.

### Use exponential automatic retry for transient errors

Rejected. GitHub can accept an asynchronous request before Kandev receives its final status.
An explicit retry or a changed readiness signature gives a safer authorization boundary.

### Reset the attempt by toggling auto-merge

Rejected. An options control must not serve as an implicit side-effect retry command.

### Overload the options PATCH with a retry field

Rejected. A retry is an action with per-request authorization and result semantics, not stored configuration.

## Related Decisions

- [Bind Automation Mutations to Event Targets](2026-08-09-bind-automation-mutations-to-event-targets.md)
- [Separate Current Contribution and Local Checkout Histories](2026-08-10-remote-contribution-head-drift.md)
