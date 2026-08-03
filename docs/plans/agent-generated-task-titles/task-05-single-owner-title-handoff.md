---
id: "05-single-owner-title-handoff"
title: "Single-owner title handoff"
status: done
wave: 5
depends_on: ["02-mcp-title-tool"]
plan: "plan.md"
spec: "../../specs/tasks/agent-generated-titles.md"
---

# Task 05: Single-owner title handoff

## Acceptance

- A prompt-first task starts pending and unowned. The first eligible structured or passthrough
  task-mode session whose initial turn begins atomically persists its session ID as owner. Session
  preparation, Config, Office, and External modes never claim.
- Concurrent launch attempts produce exactly one owner. The owner receives the title instruction and
  `set_task_title_kandev` schema; every other session receives neither, even after owner launch or title
  failure. A retry/resume of the same owner remains eligible while pending.
- Claim persistence happens before initial-prompt recording/composition, title-capable MCP
  configuration, and agent-process startup. A claim error fails the launch closed without sending or
  storing an ambiguously gated first turn or starting its agent process. Workspace/agentctl-only
  preparation remains ordinary task mode and does not claim.
- The title action requires the server-injected session ID to match the persisted owner. Owner success
  and human rename remove pending and owner keys atomically; non-owner calls are rejected without
  mutation; stale task updates cannot restore either resolved key.
- New ownership metadata mutations use the normal task-update publication path. SQLite and PostgreSQL
  implementations have equivalent compare-and-set semantics.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers -run 'Test.*(AgentTitle|TaskTitle|TitleOwner)'
cd apps/backend && go test ./internal/mcp/server ./internal/sysprompt -run 'Test.*(AgentTitle|TaskTitle|Sysprompt)'
```

Run the repository package with `KANDEV_TEST_POSTGRES_URL` when a PostgreSQL test database is available;
CI must exercise the environment-gated parity case.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- focused SQLite/PostgreSQL repository tests
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- focused service, handler, executor, and orchestrator tests

## Dependencies

The shipped pending-title lifecycle and MCP mode/tool from Tasks 01–02.

## Parallelism

Sequential in the primary conversation. The repository claim contract must exist before launch and MCP
paths can consume it.

## Inputs

- Spec sections: **Data model**, **Task MCP**, **Permissions**, **Failure modes**, and concurrency
  scenarios.
- ADR-2026-08-02 single-owner decision.
- Existing pending-title compare-and-set and first-turn canonicalization paths.

## Risks

- Initial prompts are pre-wrapped and persisted in more than one entry path; missing one can disagree
  with executor MCP mode even when the repository claim is correct.
- Stale pending snapshots can resurrect the owner after resolution unless both internal keys are
  excluded from merge payloads.
- Rows-affected behavior and JSON extraction/set/remove expressions must stay equivalent across SQLite
  and PostgreSQL.

## Result

Implemented the atomic SQLite/PostgreSQL owner claim, owner-bound MCP mutation, stale-snapshot
protection, and shared claim wiring across direct, prepared, message-start, workflow, structured,
passthrough, resume, and executor paths. Config, Office, External, and workspace-only preparation do
not claim ownership; an owner retry is idempotent and a failed owner is not reassigned.

Verification passed with focused package tests, `go test -race` on the affected packages, and the full
`go test -tags fts5 -race ./...` backend suite. `golangci-lint run ./...` also passed. PostgreSQL parity
is environment-gated and was not run locally because no test DSN was available.
