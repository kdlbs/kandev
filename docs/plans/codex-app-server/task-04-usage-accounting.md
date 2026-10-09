---
id: "04-usage-accounting"
title: "Attributed usage and cost accounting"
status: done
wave: 4
depends_on:
  - "03-background-lifecycle"
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-004
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-001.1
  - AC-COSTS-CONVERSATION-USAGE-001.2
  - AC-COSTS-CONVERSATION-USAGE-001.3
  - AC-COSTS-CONVERSATION-USAGE-001.4
  - AC-COSTS-CONVERSATION-USAGE-001.5
  - AC-COSTS-CONVERSATION-USAGE-002.1
  - AC-COSTS-CONVERSATION-USAGE-002.2
  - AC-COSTS-CONVERSATION-USAGE-002.3
  - AC-COSTS-CONVERSATION-USAGE-002.4
  - AC-COSTS-CONVERSATION-USAGE-004.1
  - AC-COSTS-CONVERSATION-USAGE-004.2
  - AC-COSTS-CONVERSATION-USAGE-004.3
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 04: Attributed usage and cost accounting

## Summary and scope

Translate native response and cumulative token reports into versioned observations and persist them through the existing ledger.
Add turn/response projections, optional provider thread estimates, committed-write invalidation, and compatible Office consumption.
Preserve existing ledger arithmetic and plugin-facing fields.

## Exclusions

No second ledger writer, invoice claims, undocumented currency conversion, or historical usage reconstruction.

## Likely files and ownership

- Native adapter usage normalization and fixture tests.
- `internal/agentctl/types/streams/agent.go`, lifecycle event DTOs, orchestrator usage publisher.
- `internal/task/usage/`, usage models/repository/migrations, `task_usage_handlers.go`.
- `internal/common/costs/pricing.go` and native normalization arithmetic tests.
- `internal/office/service/prompt_usage*` consumers and mirrored DTOs.
- Existing API contracts and event registration for usage invalidation.
- Legacy `docs/specs/task-cost-ledger/spec.md` scope note linking the observation extension. Do not grow an oversized legacy file.

## Acceptance

1. Response, turn, and session totals remain correct under duplicate replay, fork baselines, interrupted turns, late children, and optional-event absence.
2. Native subset normalization and exact observation IDs preserve both task and Office costs; existing rows and consumers keep their meaning.
3. Authorized reads expose provenance and completeness, while unknown currency, unknown tiers, and unavailable provider estimates remain explicit.

## TDD and verification

Implement the plan's usage tests first, including 1,000 input / 600 cached / 200 output / 80 reasoning = 1,200 total.
Cover two equal-sized distinct responses, missing response frames, late frames after fallback selection, and repeated cumulative snapshots.
Add SQLite/PostgreSQL migration and aggregation tests using current repository fixtures.
Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/codexappserver/... ./internal/common/costs ./internal/task/usage ./internal/task/repository/sqlite ./internal/task/handlers ./internal/orchestrator/... ./internal/office/service/...)
(cd apps/backend && go test -race ./internal/task/usage)
(test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestPostgres.*(Usage|Native)' -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Set `KANDEV_TEST_POSTGRES_DSN` for an isolated test database before the database command.
Record whether PostgreSQL fixtures actually ran. A skipped suite leaves this work order pending.

## Risks

Reasoning and cached tokens are subsets for native Codex but not interchangeable with old ledger semantics.
A response usage event has no guaranteed model field; unresolved model attribution must stay unpriced.
Do not publish native terminal usage in addition to response observations.

## Results

SQLite ledger, native normalization, usage DTO, API handler, task aggregation, Office consumer, and orchestrator publication tests passed. PostgreSQL 16 coverage passed for 14 usage and migration tests, including unique observations, rollups, cost/token bounds, and migration compatibility. No PostgreSQL-specific conversation-fork test currently exists.
