---
id: "01-settle-unaccepted-start"
title: "Settle unaccepted start capture"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.1
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-007.1
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01: Settle unaccepted start capture

## Summary

Reproduce the terminal retry loop with an enabled policy row whose start
was never accepted. Persist explicit unavailability and release the lifecycle
capture fence without manufacturing endpoints.

## In scope

Repository precheck and SQL predicate; real-repository coordinator regression;
lifecycle successor-fence verification. Reuse existing requirement/design pair.

## Out of scope

Live database repair, start-capture performance tuning, generic retry redesign,
UI, schema migrations, and unrelated capture qualification work.

## Acceptance

1. An enabled, unaccepted-start row settles unavailable and incomplete; repeated
   terminal handling succeeds without capturing new endpoints.
2. Ready finalization without accepted starts, stale revisions, and overwrites
   of completed rows remain rejected; accepted-start behavior remains intact.
3. Successful unavailable settlement releases the generation fence, allowing
   successor admission through the existing lifecycle path.

## Verification

From the repository root, run the new regression first and record its expected
failure before production edits. Then run the package checks below. If the full
SQLite suite exceeds its global timeout without assertion failures, retain that
result and use the scoped race command for local capture coverage; CI owns the
complete repository suite.

```bash
(cd apps/backend && go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle)
(cd apps/backend && go test -trimpath -race ./internal/task/changes ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'TurnChange|TurnCapture|TerminalCapture|PromptAdmissionCapture|CoordinatorSettlesUnacceptedStart')
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the actual changed Go paths. If
`KANDEV_TEST_POSTGRES_DSN` is configured, include the same repository regression
against PostgreSQL under `-race`; otherwise record that coverage as unavailable.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/turn_changes.go`
- `apps/backend/internal/task/repository/sqlite/turn_change_start_failure_test.go` (new)
- `apps/backend/internal/task/changes/coordinator_start_failure_test.go` (new)
- `apps/backend/internal/task/changes/coordinator_test.go` (test-double parity if needed)
- Existing lifecycle tests in `manager_turn_changes_test.go` and
  `session_turn_capture_wait_test.go` supply fence evidence without new test changes.

## Dependencies

None.

## Risks

Keep Go and SQL eligibility predicates aligned. Do not mutate the stored
capture policy to make an incomplete row eligible. The production reproduction
must use real repository behavior; fake-only tests missed the invariant.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/turn-changed-files.md), ordered lifecycle and bounded operation.
- [System design](../../specs/tasks/system-design/turn-changed-files.md), admission/terminal fences and bounds.
- [Immutable interval decision](../../decisions/2026-10-07-immutable-turn-change-intervals.md).
- Root-cause evidence and existing repository/coordinator/lifecycle tests.

## Results

- RED: the new repository test rejected unavailable finalization; the real
  coordinator test returned `unavailable turn change finalization lost its
  ownership revision` after injected start-persistence timeout.
- GREEN: both focused regressions passed after the repository predicate fix.
- Full package command: coordinator passed (0.345s), lifecycle passed (177.162s).
  SQLite reached its ten-minute global timeout during setup of
  `TestWalkthroughRepo_CreateGet`, with no assertion failure reported.
- Scoped race command: coordinator passed (1.380s), SQLite passed (9.322s),
  lifecycle passed (3.522s). This includes the new regressions and existing
  terminal retry, immutable endpoint, disabled capture, and successor fences.
- SQL guard passed. Persistence store conformance passed under race (57.624s).
- Changed-package lint passed with zero issues for all three changed Go files.
- Review follow-up adds explicit rejection of `failed` availability without an
  accepted start. `go test -trimpath -race ./internal/task/repository/sqlite
  -run TurnChangeUnavailableWithoutAcceptedStart -count=1` passed (1.413s).
- Documentation catalog, specification lint, PR documentation coverage
  preflight, and diff whitespace checks passed.
- PostgreSQL regression is included but skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is not configured. CI retains full-suite coverage.

The implementation reuses existing lifecycle fence tests instead of adding
another test of the same success path. Existing requirement/design lifecycle
statuses remain unchanged because the parent feature's qualification work is
outside this repair. Public docs already describe unavailable capture; no
operator workflow, public API, or rendered UI changes.
