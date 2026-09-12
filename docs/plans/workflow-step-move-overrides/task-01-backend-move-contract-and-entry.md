---
id: "01-backend-move-contract-and-entry"
title: "Backend move contract and entry"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow-step-move-overrides/spec.md"
---

# Task 01: Backend move contract and entry

## Scope

Implement the typed EntryOptions contract across HTTP, WebSocket, MCP, task.moved, PendingMove, private move entries, and orchestrator step entry. Preserve existing move authorization and target validation while replacing the separately queued MCP prompt with one transition-scoped value. Reconcile this with upstream WIP-capacity queueing and lifecycle recovery so immediate, active-turn deferred, WIP-queued, promoted, and restarted moves all retain and consume EntryOptions exactly once.

## Likely files

- apps/backend/internal/workflow/move/ and its tests.
- apps/backend/internal/task/service/service_workflow.go and service_events.go.
- apps/backend/internal/task/handlers/task_http_handlers.go and task_ws_handlers.go.
- apps/backend/internal/orchestrator/watcher/watcher.go.
- apps/backend/internal/orchestrator/messagequeue/types.go, repository_sqlite.go, repository_memory.go, and focused repository tests.
- apps/backend/internal/orchestrator/event_handlers_workflow.go and focused pending-move/workflow tests.
- apps/backend/internal/mcp/handlers/config_task_handlers.go, mcp server registrations, and configuration-handler tests.
- apps/backend/config/prompts/config-context.md for the agent-facing contract.

## Acceptance

- HTTP, WebSocket, and move_task_kandev accept the normalized nested entry_options object, while the legacy MCP prompt remains a compatible alias and conflicting prompt values fail validation.
- Direct and deferred moves carry the same typed fields. Direct task.moved events expose only move_id; private entries and pending moves persist and restore all fields across SQLite reload and session transfer.
- Explicit profile, reset, and prompt precedence is applied at target entry in the specified order for existing sessions, switched or reused sessions, new auto-start sessions, passthrough, and no-auto-start queued prompts.
- Existing authorization, reachable-target, workspace, archive, WIP, active-session, and step-history behavior remains intact. Agent-facing overrides with no target session and no target auto-start are rejected before the task changes.
- A move hand-off prompt is delivered once and cannot be misdelivered to the source session after transition failure or profile switching.
- WIP-full moves retain reset, instructions, and profile overrides through queued source-exit, promotion, and restart recovery.
- Unsupported Office or passthrough combinations reject before the task changes.
- Tests cover normalization, event propagation, pending persistence/restart, invalid inputs, all entry paths, and duplicate-signal idempotence.

## Verification

    cd apps/backend && go test ./internal/workflow/models ./internal/task/service ./internal/task/handlers ./internal/mcp/handlers ./internal/mcp/server ./internal/orchestrator/messagequeue ./internal/orchestrator/watcher ./internal/orchestrator

    git diff --check
    git diff --name-only --cached | grep '\.go$' | xargs -r gofmt -l

## Handoff

Implementation complete, including WIP admission, promotion, restart recovery, and unsupported Office/passthrough rejection. Focused move-contract, MCP, queue, orchestrator, and task-service tests are present; the broader service package has unrelated filesystem-test failures in this environment. A fresh verification run is outside this documentation update.
