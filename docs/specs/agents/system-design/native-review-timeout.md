---
title: Shared profile execution for native review
system: agents
status: current
created: 2026-10-15
owners: [backend]
requirements:
  - REQ-AGENTS-NATIVE-REVIEW-TIMEOUT-001
---

# Shared profile execution for native review

Move the existing session/host utility profile loops to a small utility-service
helper with the same concrete-execution callback. Resolve review profiles through
ProfileExecutionResolver and retain transient route metadata on the run-local
reviewer identity. The review package imports no agent runtime. Batches retain
the current route instead of selecting again. Successful results retain the
existing parse/publish/complete pipeline and one review run.

The shared helper owns only the bounded invocation loop, with at most eight
candidate dispatch attempts per prompt batch. ProfileExecutionResolver
and the existing dynamic engine own order, generation fences, failure policy and
retry/reset deadlines. A due pending decision is resumed through the existing
ResumePendingRoute method; a caller-context timer waits for that deadline without
a polling loop. Neither the helper nor review changes the configured policy.

A typed request-local deadline with a live parent does not establish that its
child terminated: the HTTP transport can time out while provider execution is
still active. Until the native child owner supplies exact terminal proof, that
error remains unclassified and cannot authorize automatic retry or fallback.
This is a conservative intermediate implementation; the original typed-child
deadline acceptance remains open. Provider prose is not promoted to typed proof.
Parent cancellation/deadline and user-visible partial output stop further
dispatch. Existing classified provider failures still use the saved routing
policy. The 600-second review-run budget, five-minute HTTP client budget and
ten-second detached failure-write budget remain unchanged.

Unconfirmed deadlines retain their sanitized original error and wrapped cause.
Bound utility/default profiles keep their native default-model behavior;
explicit reviewer profiles keep their prior model validation.

The lifecycle/host-utility callback still owns actual child creation, admission
and settlement. This change does not treat a timeout as proof of child exit or
refund capacity/provider claims. The separate child-owner/admission integration
must retain uncertain predecessors. No new ledger, review run or state owner is
introduced. Missing candidates or exhausted attempts fail the same existing run;
the existing failure writer is reused, with write errors propagated to its caller.

The runtime facade forwards the canonical inference identity/capacity contracts.
Utility dispatch attaches its server-created call ID and prepared logical profile.
Review dispatch first checks the stored task/session/run relationship. The common
loop changes only the concrete execution-profile field for each callback; logical
profile and parent call/run stay fixed. Typed capacity waits/refusals stop before
provider classification or session-to-host fallback, including errors whose cause
is a deadline. The child admission implementation remains a separate integration.
