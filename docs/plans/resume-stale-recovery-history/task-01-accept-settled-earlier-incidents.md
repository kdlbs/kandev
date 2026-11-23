---
id: "01-accept-settled-earlier-incidents"
title: "Accept settled earlier incidents during Resume"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 01: Accept settled earlier incidents during Resume

## Summary

`ResolveSessionRecoveryBlockWithNativeEvidence` must clear the current block
when the session also carries a pending restore attempt and a settled recovery
notice from an earlier, fully settled incident, even though the lifecycle proof
does not list that earlier submission. Also make the refusal observable.

## Regression test (write first, must fail)

New file `apps/backend/internal/task/repository/sqlite/native_resume_settled_earlier_incident_test.go`,
real temporary SQLite, mirroring session `cbf39cbf`:

- Generation 1, native session `N`, `executors_running` row for execution `E`,
  cursor `I:g1` with received = projected.
- Earlier submission `S0`: `cancelled`/`cancelled`, `fully_settled_at` set,
  terminal stream `I:g1`, terminal sequence > 0, matching terminal effect;
  resolved block `B0` (`unknown_prompt_outcome`, submission `S0`).
- Pending `silent_restart_restore` attempt whose checkpoint source block is `B0`
  (state `open` in the checkpoint) and source recovery names `S0`.
- Session metadata recovery notice: phase `settled`, submission `S0`.
- Current submission `S1`: `interrupted_unknown`, generation 1; open block `B1`
  (`unresolved_durable_work`, empty submission/stream).
- Proof lists only `S1`.

Cases:
- `TestResolveNativeEvidenceAcceptsSettledEarlierIncident`: returns `true`; `B1`
  resolved with action `resume`; the attempt is completed as superseded; `S1`
  stays `interrupted_unknown`.
- `...RejectsUnsettledEarlierIncident`: `S0` without `fully_settled_at` → false.
- `...RejectsEarlierIncidentMissingTerminalEffect` → false.
- `...RejectsMismatchedProofRow`: proof includes `S0` with a different hash → false.
- `...LogsRefusingStep`: an observed logger records one WARN with the
  `prepared_attempt` step for the unsettled case.

## Implementation

1. `native_resume_historical_terminal.go`: in `preparedHistoricalTerminalAttemptMatchesTx`
   and `historicalTerminalNoticeMatchesTx`, when `nativeResumeSubmissionEvidence`
   finds no row, accept the `loadHistoricalTerminalSettlementTx` result; when a
   row is present, keep `historicalTerminalEvidenceMatches`. Do not relax any
   other condition.
2. `native_resume_binding.go` `ResolveSessionRecoveryBlockWithNativeEvidence`:
   on each `false` return, log one WARN with session, block ID and step
   (`snapshot`, `incident_blocks`, `prepared_attempt`, `notice`, `block_update`).
3. Orchestrator `deliveryRecoveryRefusalReason`: when the error comes from
   `sessionRecoveryBlockResolutionGuidance` after a native-evidence attempt, use
   `sql_resolution_refused`. Add it to the closed reason set and the design list.

## Out of scope

- Lifecycle proof construction, journal, frontend.

## Acceptance

- New tests pass with `-race`; the first fails before the change with `false`.
- Existing `NativeResume*`, `HistoricalTerminal*` and silent-restore tests pass.

## Result

The SQL settlement remains authoritative when an earlier submission is absent
from the current proof. Present proof rows still require an exact match. Added
coverage for unsettled sources, missing terminal effects, mismatched proof rows,
and a single `prepared_attempt` refusal warning. The existing missing-evidence
case now removes the SQL terminal effect, matching the actual refusal boundary.

Passed:

- `go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite/ -run 'NativeResume|HistoricalTerminal|SettledEarlier|SilentRestore' -count=1`
- `go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'NativeResume|DeliveryRecovery|RefusalLog' -count=1`
- `go test -trimpath -tags fts5 ./internal/orchestrator/ -run 'TestExplicitResumeClassifiesNativeSQLResolutionRefusal' -count=1`

## Commands

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite/ -run 'NativeResume|HistoricalTerminal|SettledEarlier|SilentRestore' -count=1
go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'NativeResume|DeliveryRecovery|RefusalLog' -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```
