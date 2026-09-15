---
id: "02-record-suppressed-terminations"
title: "Record suppressed session terminations"
status: done
wave: 2
depends_on:
  - "01-guard-retained-capacity"
plan: "plan.md"
requirements:
  - REQ-OFFICE-SESSION-TERM-004
acceptance_criteria:
  - AC-OFFICE-SESSION-TERM-004.1
  - AC-OFFICE-SESSION-TERM-004.2
  - AC-OFFICE-SESSION-TERM-004.3
  - AC-OFFICE-SESSION-TERM-004.4
system_design:
  - ../../specs/office/system-design/task-session-termination-01.md
---

# Task 02: Record Suppressed Session Terminations

## Summary

Count suppressed terminations under the office namespace, labelled only by the
guarded reason and the retained capacity, and document the new counter beside
the office metrics families the repository already lists. A suppression that is
only logged is not queryable, which leaves a mistaken retained-capacity answer
indistinguishable from a quiet system.

## In scope

- An expvar map in the office dashboard package counting suppressions,
  following the `"k1=v1;k2=v2"` label idiom of
  `internal/orchestrator/office_stall_metrics.go`.
- Labels restricted to the guarded reason (three values) and the retained
  capacity (two values), plus a distinct label value for a suppression caused
  by a read failure.
- A test asserting the counter increments on each suppression path and that no
  task, agent, step, or session identifier reaches a label.
- The observability entry in `CLAUDE.md`, listing the new counter beside
  `office_stall_*` and `routing_*`.

## Out of scope

- The determination and the guarded call sites, which Task 01 owns.
- Any new log record. Task 01 lands both records; this work order only counts.
- A Prometheus exporter or any change to the `/debug/vars` wiring.
- A gauge or snapshot of currently-suppressed sessions. These are event
  counters, matching the office stall detectors' stated model.

## Acceptance

- Each suppression increments the counter exactly once, with the reason and
  capacity that produced it.
- A read-failure suppression is countable separately from a
  retained-capacity suppression.
- Every label value comes from a fixed set; a test fails if an identifier is
  introduced as a label.
- `CLAUDE.md`'s Observability section names the counter and its label
  dimensions.

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/office/dashboard/... -race -count=1
make -C apps/backend lint
cd apps/backend && go build ./...
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/dashboard/session_termination_metrics.go`
- `apps/backend/internal/office/dashboard/session_termination_metrics_test.go`
- `apps/backend/internal/office/dashboard/service_tasks.go`
- `CLAUDE.md`

## Dependencies

- Task 01 creates the suppression outcome this work order counts and the call
  sites the increment is issued from.

## Risks

- An expvar map key built from an unbounded value grows without limit. Both
  dimensions are closed sets; assert that in the test rather than trusting the
  call sites.
- `expvar.NewMap` panics on a duplicate name. Keep the counter's registration
  in one file with a name not already taken by `office_stall_*` or
  `routing_*`.
- Documenting the counter in `CLAUDE.md` without the label dimensions makes it
  unusable from a query. Name both.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/task-session-termination.md`,
  REQ-OFFICE-SESSION-TERM-004.
- `docs/specs/office/system-design/task-session-termination-01.md`, section
  "Observability".
- `apps/backend/internal/orchestrator/office_stall_metrics.go` for the label
  idiom and the counters-only rationale.
- `CLAUDE.md`, Observability section.

## Results

- Added `internal/office/dashboard/session_termination_metrics.go` with
  `office_session_term_suppressed_total`, following the
  `"k1=v1;k2=v2"` expvar label idiom used by the office scheduler and the
  stall detectors.
- Labels are `reason` (three values) and `outcome` (`runner`, `seat`,
  `read_failed`), so a read fault is countable apart from a genuinely retained
  capacity. Both increments are issued from the single guarded termination
  helper, so no call site can forget one.
- `TestSuppressedTermination_IsCounted` asserts one increment per cause using
  before/after deltas, since expvar maps are process-global and shared across
  the package's tests. `TestSuppressionLabels_AreBounded` scans every label the
  package produced, rejecting any that carries an identifier, has other than
  two dimensions, or names a value outside the closed sets. It fails rather
  than passing vacuously when no label was recorded.
- Documented the counter and both label dimensions in `CLAUDE.md`'s
  Observability section, beside `office_stall_*` and `routing_*`.
- `go build ./...` passes, `internal/office/dashboard` passes with `-race`, and
  `golangci-lint` reports 0 issues for the package.
