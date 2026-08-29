---
id: 04-agent-rate-snapshot
title: Agent rate-state snapshot
status: done
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

Implemented `get_github_rate_limit_kandev` for Kanban and Office task
surfaces. The server binds the request to its current task, the backend derives
the workspace from that authorized task, and the GitHub service reads only
persisted connection metadata plus principal-coordinator memory. The response
reports non-secret principal identity, known/fresh core and GraphQL buckets,
Kandev-observed secondary state with `retry_source`, and current interactive
and background admission decisions. Active secondary states use the latest
resource retry boundary that Kandev is enforcing; an accepted response can
still clear that local upper bound early.

Validation passed:

- `go test ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -run 'Test.*(GitHubRate|ToolMetadata|Profile|MCP)' -count=1`
- `go test ./internal/github -run 'TestService.*RateLimitSnapshot' -count=1`
- `go test -race ./internal/github -run 'Test(ServiceGetWorkspaceRateLimitSnapshot|ObservedSecondarySnapshot)' -count=1`
- `go test ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -count=1`
