---
id: "05-routine-log-volume"
title: "Reduce routine diagnostic repetition"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.1
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.2
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.3
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.4
system_design:
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Task 05: Reduce routine diagnostic repetition

## Summary

Remove repeated routine entries at three known sites. Keep every safety decision and genuine warning/error diagnostic effective.

## In scope

- Required-store state-change logging keyed by state and affected stores.
- Removal of routine per-session idle-reclaim refusal debug records.
- Unsupported MCP alternative severity after the surviving transport set is known.
- Synthetic before/after entry and byte measurements in work-order results.

## Out of scope

- Global sampling, production threshold changes, event-bus log refactors, new metrics, and reclaim or transport-selection policy changes.

## Acceptance

- Repeated identical health results emit no repeated info transition. First state, changed affected stores, failure, and recovery remain observable.
- Routine reclaim refusals emit no per-session debug record. Decision fixtures and genuine failed-probe/successful-reclaim diagnostics remain correct.
- A rejected MCP alternative uses debug only when its same-name supported transport survives. Total refusal remains a warning and selection order remains unchanged.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/requiredstores -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'Test.*(Reclaim|IdleSession)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Mcp|MCP|Filter)' -count=1)
```

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/health.go`
- `apps/backend/internal/persistence/requiredstores/health_transition_log_test.go (new)`
- `apps/backend/internal/orchestrator/reconcile_liveness.go`
- `apps/backend/internal/orchestrator/reclaim_lifecycle_logging_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- Shared health state needs mutex protection without deadlocking probe callbacks.
- Do not create a suppression map keyed by session IDs or change what qualifies for reclaim.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/platform/system-design/runtime-diagnostic-signal.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Required-store health now emits the first aggregate state and later entries only when the aggregate state or sorted unhealthy store IDs change. Probe tracking, readiness, maintenance admission, and independent per-sweep failure warnings are unchanged.

Routine idle-reclaim refusal debug entries are removed for both active language-server leases and normal fail-closed dispositions. Classification, guards, row state, successful reclaim info, and liveness-probe warnings remain unchanged. MCP filtering still selects the first surviving entry and preserves every decision; a rejected alternative logs at debug when a same-name supported server survives, while total refusal remains a warning.

The health observer test covers repeated healthy and failed results, a changed failing-store set, partial recovery, full recovery, and sorted store IDs. Reclaim tests cover ordinary and active-lease refusals, preserved liveness warnings, successful reclaim info, and the existing refusal decision matrix. MCP observer tests cover supported alternatives and total refusal.

Passed:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/requiredstores -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'Test.*(Reclaim|IdleSession)' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Mcp|MCP|Filter)' -count=1)`

Synthetic byte comparison used fixed representative Zap JSON records and 100 repeated calls. At the info threshold, 100 unchanged health transitions measured 17,900 bytes before and 179 after (17,721 fewer); 100 unsupported MCP alternatives measured 18,700 warning bytes before and zero debug bytes after (18,700 fewer). At the debug threshold, 100 routine refusal records measured 31,600 bytes before and zero after. At that threshold MCP still emits 100 records, with the debug encoding measuring 18,800 bytes versus 18,700 warning bytes before. These controlled estimates exclude all installation log data.
