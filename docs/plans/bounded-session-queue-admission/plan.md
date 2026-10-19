---
status: done
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Bounded Session Queue Admission Delivery

## Outcome and authority

Port the existing bounded-acquisition queue repair from fork change
`0797bea018f8802210e913dc30008cefb34f521e` to upstream base
`15ed289c050f8bd2bd675c4542ffb796525bf991`. Preserve the original callback
lifetime, FIFO/cancellation, taken-message requeue, retryable transport result,
and durable attachment-cleanup recovery. This is one repair package, not a new
task or a changed recovery policy.

- [Requirements](../../specs/tasks/requirements/bounded-session-queue-admission.md)
- [System design](../../specs/tasks/system-design/bounded-session-queue-admission.md)
- [Work order](task-01-bound-acquisition-and-cleanup.md)

## Dependencies and exclusions

Main already contains #4479's classified private readiness-expiry recovery and
#4501's sanitization changes. Preserve those contracts while adding the
admission-timeout predicate. Do not classify unrelated provider deadlines as
readiness expiry or safe acquisition failure.

## Risk and verification

The acquisition budget must never be propagated to admitted callbacks, and a
callback deadline must not be relabeled as an acquisition timeout. Preserve
session references when cancelled waiters leave and preserve durable cleanup
after budget exhaustion. Retain deterministic FIFO, cancellation, SQLite,
compensation/shutdown, cleanup/restart, API, and desktop/phone regression tests.
Run contributor code only on hosted runners. Focused proof runs establish
regressions; full actual current-head backend, frontend, and E2E suites qualify
the final change. Exact commits, commands, attempts, and raw results are retained
in the PR review evidence, separate from this durable plan.

## Implementation verification

Permanent production-path regression coverage is retained in the work order.
Targeted hosted Go race/SQLite, cleanup lease-race, queue-client/UI, and
retry-disabled desktop/phone checks passed. Full published-head qualification
and the ordinary merge receipt belong to PR #4503's delivery evidence; the
focused results do not substitute for those full suites.
