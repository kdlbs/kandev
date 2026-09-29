# ADR-2026-09-16-consolidate-office-enqueue-paths: Consolidate Office Run Enqueue Onto One Authoritative API

**Status:** accepted
**Date:** 2026-09-16
**Area:** backend

## Context

Before this work, four separate code paths could insert an Office run row: the dashboard
reactivity adapter, the approvals decider, the workflow engine's `queue_run` step action,
and `runtime.Actions.SpawnAgentRun`. Each declared its own copy of the deduplication window,
and each resolved (or omitted) actor, causing-run, and routine attribution independently.
A launch-safety gate — a depth ceiling, a self-trigger allowance, a concurrency ceiling —
attached to any one of them was silently absent from the other three. This is exactly how
the reason-scoping gap this PR closes
([self-trigger suppression](../specs/office/requirements/self-trigger-suppression.md))
survived undetected across four prior review rounds: `SpawnAgentRunInput.Reason` was
agent-supplied free text with no registry check, and no single seam existed where that check
could have caught every caller at once.
[Office Enqueue Consolidation](../specs/office/requirements/enqueue-consolidation.md) states
this as a delivery prerequisite of unattended launch safety rather than a clause of any one
limit, because it is a boundary change with its own blast radius, not new logic at one call
site.

## Decision

`runs/service.Service.QueueRun` / `QueueRunAndReturn` is the single authoritative enqueue
API. Every run row in the system is inserted there and nowhere else. The dashboard adapter,
the approvals decider, the workflow engine's `queue_run` action, and
`runtime.Actions.SpawnAgentRun` are delegating callers: each resolves its own arguments —
the typed actor (`AC-OFFICE-RUN-CAUSATION-001.15`, a required field, never defaulted), the
causing run, the routine attribution — and calls through the authoritative API rather than
holding an insert of its own. A delegating caller whose call to the authoritative API fails
surfaces that error to its own caller; it does not fall back to a local insert
(`AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6`), because a fallback insert is precisely the
ungated path this decision removes.

Consolidation does not change what is inserted for any existing wake reason — same agent
profile, reason, payload, idempotency key, and coalescing outcome before and after
(`AC-OFFICE-ENQUEUE-CONSOLIDATION-001.5`) — only where the insert happens.

The dedup/idempotency window, previously declared three times across the four paths, is now
declared exactly once at the authoritative API (`AC-OFFICE-ENQUEUE-CONSOLIDATION-001.4`).

A structural test (`internal/office/shared/enqueue_consolidation_test.go`) guards against a
second insert path being added silently, asserting against the run-row insert mechanism
itself rather than enumerating known callers by name
(`AC-OFFICE-ENQUEUE-CONSOLIDATION-001.3`) — a list of callers is what fell behind the first
time. The guard is currently file-level (it allowlists the files permitted to reach the
insert) rather than call-site-level; that gap is tracked, accepted, and not closed by this
decision.

## Consequences

- A new wake reason or new runtime action that needs to queue a run must delegate through
  `QueueRun`/`QueueRunAndReturn`; it cannot add its own insert without the structural guard
  failing the build.
- Every launch-safety gate — depth ceiling, self-trigger allowances (both the per-reason and
  the reason-independent total added by this PR), workspace/causing-run resolution — now
  applies uniformly to all four call sites, including retroactively to the three that
  previously bypassed a gate attached only to one of them.
- The dashboard adapter, approvals decider, and `SpawnAgentRun` all now thread a typed actor
  and causing-run identity through to the seam instead of smuggling attribution in a payload
  document or omitting it.
- `queueRunInline` and the scheduler's legacy fallback insert path are now dead code, three
  times confirmed unreachable from any production boot path; their removal
  (`AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6`'s "no fallback insert" made literal) is tracked as
  follow-up work on a separate card, not part of this decision.
- The consolidation guard's current file-level (not call-site-level) precision is a known,
  accepted gap: it stops a new file from adding a second insert path but would not catch a
  second insert added inside an already-allowlisted file. Tightening it to call-site
  precision is tracked as non-blocking follow-up.

## Alternatives Considered

1. **Gate each of the four paths independently**, duplicating the checks at each insert
   site. Rejected: four independent copies of every gate is exactly the failure mode this
   decision exists to remove, and is how the reason-scoping gap went undetected across four
   review rounds.
2. **Keep the four paths, add a lint rule enumerating known call sites.** Rejected explicitly
   by `AC-OFFICE-ENQUEUE-CONSOLIDATION-001.3`'s own reasoning: a list of callers is what fell
   behind in the first place, and a new caller added without updating the list defeats the
   rule silently.
3. **Route the four paths through a shared library function with no structural test.**
   Rejected: without a test that fails on a new insert path, nothing stops a future call site
   from adding its own insert instead of calling the shared function, which reproduces the
   original problem one refactor later.
4. **Consolidate the dedup-window declarations first, as a separate, smaller change.**
   Rejected: the single declaration falls out of having one seam: a phased approach would
   need a third, temporary bridging mechanism to keep three declarations in sync until the
   seam work lands, which is more code than doing both together.

## Related decisions

- [Instance-wide launch-claim lock scope and budget fail-mode asymmetry](2026-09-16-instance-wide-claim-lock-and-budget-fail-mode-asymmetry.md),
  the sibling ADR from the same design, which assumes this document's single enqueue seam
  when describing the enqueue-time advisory lock's scope.
