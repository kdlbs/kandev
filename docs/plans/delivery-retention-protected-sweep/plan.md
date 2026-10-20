---
created: 2026-10-20
status: complete
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md
legacy_specs: []
---

# Delivery retention protected sweep repair

## Overview

Stop automatic cleanup from repeatedly scanning a fixed protected inventory
without waiting. Platform owns this repair because it owns the durable-delivery
retention and scheduling contract. Existing AC-PLATFORM-DELIVERY-RETENTION-005.5
and .005.6 already cover fair discovery and idle cadence. No requirement change
or new ADR is needed: this corrects the implementation of the existing invariant.

## Evidence and root cause

The supplied report attributes 108–111% CPU and 13 million candidate visits in
about eight minutes to repeated cleanup passes with no removals. Those live
measurements are reported evidence, not independently recaptured in this task.

Source confirms that `agentDeliveryDiscoveryAdvanced` ORs increases relative to
the immediately previous compaction, receipt and prompt positions. These
positions wrap independently. Two sources alternating positions 1 and 2 out of
phase can report forward progress on every batch while revisiting the same four
protected records. `committedRetentionWork` includes this discovery hint;
`runPass` requeues retention and sets `ContinuationPending` at its budget;
`loop` then immediately starts another pass. Increasing Interval cannot affect
this path.

A temporary focused test, `TestReproProtectedDiscoveryStaggeredWrap`, exercised
the production progress helper with disjoint compaction and receipt identities
and twenty staggered protected-only revisits. It failed because every visit
reported progress. Remove the throwaway test after diagnosis; permanent coverage
must also drive real repository selection and worker scheduling.

The existing historical-sweep regression requires at least one non-advancing
batch in twenty calls; it does not cover two independently advancing sources.
The worker fake reports exhaustion explicitly and cannot expose this mismatch.

## Scope

### In scope

- Committed before/after discovery vectors, chain-scoped maxima and idle rechecks.
- Real small SQLite/PostgreSQL protected-only and mixed eligible/protected fixtures.
- Multi-pass fake-clock scheduling, rollback, restart and later eligibility checks.

### Out of scope

- Live-instance changes, restarts, data repair or CPU profiling.
- Eligibility, replay, settlement, locking, schema or configuration changes.
- UI changes, new metrics and fixed sleeps during productive cleanup.

## Technical approach

Follow the throughput design's Discovery across immediate passes section.
Expose typed progress vectors in `internal/task/models/agent_delivery.go` only
after the existing repository transaction commits. Derive progress once with the
same helper used by `writeAgentDeliveryRetentionProgress`; preserve exact tuple
ordering and wrapped/empty resets. Keep the existing batch-local hint for callers
that inspect it, but do not use it alone to authorize worker continuation.

In `internal/task/deliveryretention`, hold component-wise maxima through the
entire immediate chain, across five-second pass boundaries. Use novel discovery
or actual cleanup to repeat phases. Reset at an actual wait/restart boundary.
Keep Snapshot aggregate-only and all maintenance/cancellation/error precedence.

Both SQLite and PostgreSQL use the shared repository path. All agent providers
reach this worker through the same backend journal; no provider-specific protocol
change is needed. Unknown event types retain their existing protected fallback.
Offline maintenance retains its existing bounded-operation contract.

## Tests

Task 01 names exact tests and commands. Cover AC-005.1, .005.5 and .005.6 with
staggered wraps, productive cleanup and more than one pass. Cover AC-006.3 with
aggregate snapshots and AC-006.6 with committed restart and idle revisit cases.
Use the full AC-PLATFORM-DELIVERY-RETENTION prefix for test annotations.
A mixed inventory must retain every protected row and reach at least one later
eligible row; a universal all-protected case must reach idle without removals.
No rendered behavior changes; repository-to-worker tests are the relevant
end-to-end boundary, with no artificial browser suite.

## Work orders

- [x] [Task 01: Bound protected discovery continuation](task-01-bound-discovery.md)

## Companion records

This supersedes the batch-local discovery exhaustion assumption in
[database maintenance throughput Task 02](../database-maintenance-throughput/task-02-continuous-retention.md).
That completed package remains historical delivery evidence. The broader
[retention package](../backend-agent-delivery-retention/plan.md) and
[stream storage package](../delivery-stream-storage/plan.md) retain their existing
scope and status. Their eligibility and retirement evidence is not reopened.

## Verification results

Implementation complete. The temporary reproduction failed as expected on the
current code; its file was removed. Passed: `python3 scripts/list-docs.py
validate` (392 decisions, 1668 specifications), `python3
scripts/lint-spec-files.test.py` (36 tests), `python3
scripts/lint-spec-files.py --all`, and `git diff --check`.
PR-documentation preflight passed through `.github/scripts/pr-docs.cjs`
`validateCoverage`, using this plan, work order, requirement and design with
planned production paths (status `covered`, no errors). Node was located at
`/home/jcfs/.nvm/versions/node/v24.18.0/bin/node` because this shell's PATH
omitted it. Permanent regressions and the existing worker race suite passed; the targeted
repository race command passed on SQLite and disposable PostgreSQL 17 (10.552s).
Changed-package lint passed with zero issues. See Task 01 for the real worker
RED/GREEN evidence, later idle rechecks and exact commands. PR CI/reviews and
merge remain delivery-session work, not inferred from local validation.

## Risks

- Resetting maxima at a pass boundary recreates continuous CPU use.
- Comparing whole vectors lexicographically lets one source hide another.
- Empty or wrapped positions must match the actually committed state.
- Repeated low keys need later idle rechecks; maxima must not survive those waits.
- PostgreSQL execution requires KANDEV_TEST_POSTGRES_DSN and must not be claimed
  passed when environment-gated subtests skip.
