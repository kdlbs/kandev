---
id: "01-typed-startup-evidence"
title: "Persist typed startup selection evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.21
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.22
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.23
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.24
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.25
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.33
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Task 01: Persist typed startup selection evidence

## Summary

Retain selection evidence from the actual model/mode decision through both
bootstrap error projections, persisted session history, and DTO/WS/boot reload.
Deliver an additive bounded contract; keep provider policy and failure ownership.

## In scope

- Read root/backend guidance and `/tdd`; prove model/mode cause loss with failing
  tests before production edits. Verify #4065 remains an ancestor/equivalent.
- Produce the design's five cause codes and closed reasons at model and mode
  checks. Add `start` operation, requested/effective selectors, the actual safe
  model passed to a failed application call, and no-prompt evidence. Preserve
  timeout/auth/cancellation behavior.
- Extend safe projection and task normalization. Enforce bounds and reject
  unsafe selector values before persistence, including existing typed detail.
- Extend orchestrator and executor projections, watcher/history payloads,
  LastAgentError/task-summary/DTO boundaries. Reuse JSON metadata and the atomic
  session failure commit; audit every explicit cause-field copy.
- Preserve execution/attempt fences, history repair, stale rejection, cleanup,
  and saved native conversation/settings identities.
- Add real SQLite persist/reopen tests and safe transport serialization tests,
  including legacy/malformed optional fields and sensitive sentinels.

## Out of scope

Frontend rendering, new SQL schema, workspace behavior, provider-restored
eligibility, silent fallback, unrelated selectors/routing, feature flags.

## Acceptance

1. Lifecycle model tables and mode tests prove source distinctions and verified
   no-prompt evidence; current cancellation/deadline/auth semantics still pass.
2. A specific normalized cause survives actual persistence/reload and history,
   DTO and summary JSON with no sensitive sentinel or raw provider error string.
3. Existing successor/cancellation/attempt guards still reject stale failures;
   #4065 strict and restored-policy regression assertions remain intact.

## Verification

Run from repository root. These are implementation commands, not planning results.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/models ./internal/task/repository/sqlite ./internal/task/dto)
(cd apps/backend && golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... ./internal/task/models/... ./internal/task/repository/sqlite/... ./internal/task/dto/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use existing `TestApplyStartModelPolicyExecutorAuthority`,
`TestBuildBootstrapLastAgentErrorUsesSafeCorrelatedProjection`,
`TestBootstrapFailureSuccessorFence`, and
`TestBootstrapFailureSuccessorFenceAfterFinalOwnershipRead` as patterns.
Add the named tests from the plan's acceptance matrix. Do not substitute a
serialization-only test for real durable reload. Record optional PostgreSQL
fixture skips separately; SQLite reload evidence is required.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go`
- `apps/backend/internal/agent/runtime/lifecycle/start_model.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/start_model_executor_authority_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_mode_source_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/executor/launch_failure.go`
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
- `apps/backend/internal/task/models/launch_errors.go`, `launch_errors_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/status_summary.go`, `status_summary_test.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary.go`
- `apps/backend/internal/task/repository/sqlite/session_bootstrap_failure_evidence_test.go` (new)

The source symbol inventory in the design bounds this list. Extend converter
coverage only when a current explicit copy drops the new fields.

## Dependencies

None. This additive persistence boundary precedes frontend contract adoption.

## Risks

Blank fresh-start operation and unchanged code allowlists can silently drop
causes. Unsafe values must be omitted before JSON persistence. New typed wrappers
must preserve `errors.Is` and current strict/fallback policy branches.

## Parallelism

`sequential`

## Inputs

- Requirement 006 draft criteria .21-.26 and .33.
- Design: Typed selection evidence; Safe persistence and transport.
- Existing explicit-resume-settings design and ADR; current lifecycle model/mode
  tests and executor bootstrap CAS/history repair tests.

## Results

Completed with the following evidence:

- TDD reproduced typed-cause loss for strict model selection and explicit mode
  application before changing production code. The initial tests failed because
  the formatted errors were not `BootstrapFailure` values.
- Lifecycle tests cover absent/empty catalogs, unavailable requested models,
  unsupported selection, failed application, missing effective modes,
  unconfirmed/mismatched modes, unavailable agentctl, and verified no-prompt
  evidence. Cancellation remains untyped; deadline remains timeout-classified.
- `go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/models ./internal/task/repository/sqlite ./internal/task/dto` passed all six packages. Reported durations: lifecycle 76.851s, executor 4.683s, orchestrator 138.184s, task/models 1.503s, SQLite 219.204s, task/dto 1.213s.
- After the final normalization-helper refactor, focused evidence, durability,
  DTO/summary, safe-projection, and successor-fence tests passed across
  `./internal/task/models ./internal/task/statussummary ./internal/task/dto ./internal/task/repository/sqlite ./internal/orchestrator/executor`.
- `golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... ./internal/task/models/... ./internal/task/repository/sqlite/... ./internal/task/dto/...`: `0 issues`.
- `python3 scripts/list-docs.py validate`: 333 decisions and 1261
  specifications validated. `python3 scripts/lint-spec-files.py --all`: all
  specification files passed. `git diff --check`: clean.
- `TestBootstrapSelectionEvidenceSurvivesReload` passed against SQLite after
  closing and reopening the repository. No PostgreSQL-specific fixture was run;
  the required durable SQLite reload check passed.

Task 02 consumes this typed contract; Task 03 preserves its turn ownership, and
Task 04 closes the correlated recovery-success history. See
[Task 04 results](task-04-recovery-success-history.md#results).

Review-finding follow-up: application failures now retain the safe model ID
actually passed to `SetModel` as `attempted_model`, while preserving the
configured `requested_model`. The advertised-fallback and unique-variation
regressions assert the actual call and durable evidence. If the attempted value
is unsafe and omitted, the UI uses cause-only copy instead of naming the
unattempted configured model.

Review-fix verification: `go test -race` passed for lifecycle, orchestrator,
executor, task models, SQLite repository, and task DTO packages after tightening
the exact-stamp settlement contract. The lifecycle regression verifies rejected
advertised fallback and unique variation values against the actual `SetModel`
argument and stored typed cause. `golangci-lint` reported zero issues across
the affected Go packages.
