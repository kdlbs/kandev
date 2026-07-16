---
id: "01-backend-contract-and-baseline"
title: "Persist the ACP configuration baseline"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 01: Persist the ACP Configuration Baseline

## Acceptance

- ACP option and option-value descriptions survive typed and metadata conversion into live events and discovery DTOs.
- The first settled effective config option values are persisted once in task-session metadata and an existing baseline is never overwritten.
- `session.models_updated` publishes the persisted baseline after restart as well as during the initial run.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp/... ./internal/task/... ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/meta_convert.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- Related backend tests and DTO conversion files

## Dependencies

None.

## Inputs

- Spec sections `What`, `Data Model`, and `Persistence Guarantees`.
- Existing `SessionRuntimeConfig` metadata helpers and `session_models` event pipeline.

## Output contract

Report the metadata shape, exact baseline-capture point, concurrency/write-once behavior, files changed, tests run, and residual risks.
