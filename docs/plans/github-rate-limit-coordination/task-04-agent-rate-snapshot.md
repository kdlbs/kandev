---
id: 04-agent-rate-snapshot
title: Agent rate-state snapshot
status: pending
wave: 4
depends_on: [03-workflow-sync-backoff]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-004
system_design: ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 04: Agent Rate-State Snapshot

## Acceptance

- Kanban and Office tasks receive get_github_rate_limit_kandev.
- The task-authorized response includes known/fresh primary buckets, observed
  secondary retry source, and interactive/background admission decisions.
- The handler and service issue no provider request, including for cold state.

## Verification

- `cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -run 'Test.*(GitHubRate|ToolMetadata|Profile|MCP)' -count=1`
- `cd apps/backend && go test ./internal/github -run 'TestService.*RateLimitSnapshot' -count=1`

## Results

Pending.
