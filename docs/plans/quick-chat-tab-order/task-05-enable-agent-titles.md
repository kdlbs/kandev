---
id: "05-enable-agent-titles"
title: "Enable Quick Chat agent titles"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-AGENT-TITLES-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.1
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.2
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.3
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.4
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.5
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.6
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.7
system_design:
  - ../../specs/tasks/system-design/quick-chat-agent-titles.md
---

# Task 05: Enable Quick Chat Agent Titles

## Summary

Extend the existing single-owner title lifecycle to ordinary Quick Chat tasks. Preserve eager agent
startup while the first pending-owner prompt receives the normal title context.

## In scope

- Send the user preference through the Quick Chat create request.
- Persist pending intent and claim the eager Quick Chat owner before process launch.
- Inject structured or passthrough title context while the owner remains pending.
- Reuse the existing title mutation, manual-rename precedence, and Quick Chat task-event update.
- Update the public Quick Chat and MCP title documentation.

## Out of scope

- Configuration Chat and Quick Terminal.
- A second title-generation agent or endpoint.
- Changes to title validation, ownership reassignment, or branch rules.

## Acceptance

- Enabled Quick Chats start with a provisional title and expose the owner-only tool before the first
  user request.
- Pending-owner prompts contain the correct server context for structured and passthrough agents.
- Disabled, manually renamed, failed, config, and non-owner paths preserve existing guarantees. The
  public guide documents the enabled behavior and exclusions.

## Verification

```bash
(cd apps/backend && go test ./internal/task/handlers ./internal/orchestrator ./internal/orchestrator/executor ./internal/mcp/server ./internal/sysprompt -count=1)
(cd apps/web && pnpm vitest run components/quick-chat/use-quick-chat-modal.test.ts lib/ws/handlers/quick-chat.test.ts)
(cd apps/web && pnpm run typecheck)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/components/quick-chat/use-quick-chat-modal.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`
- `apps/web/lib/api/domains/workspace-api.ts`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- A focused Quick Chat title-context handler test
- Orchestrator and executor tests for the existing eager owner/profile seam
- `docs/public/developer-tools.md`
- `docs/public/automation-and-mcp.md`

## Dependencies

None.

## Risks

- Eager launch and first-prompt composition occur in different paths. Tests must prove that both use
  the same persisted owner.
- The context repeats until pending state clears. Manual and agent title updates must stop it.

## Parallelism

`parallel-safe` with Task 01.

## Inputs

- The task-system title design and three related ADRs.
- `startQuickChatForAgent`, `httpStartQuickChat`, `ClaimTaskTitleSession`, and the MCP profile resolver.

## Results

Enabled ordinary Quick Chat agent-generated titles behind the existing user setting. Eager sessions
carry a durable provisional title and one claimed owner; structured and passthrough first prompts
receive the owner-only title capability. Config, Office, disabled, manual-rename, and non-owner
paths retain their existing restrictions. The focused backend/frontend tests, public-doc validation,
and full Quick Chat E2E coverage pass.
