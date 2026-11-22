---
id: "04-replay-notices"
title: "Record detached episodes in the conversation"
status: done
wave: 3
depends_on:
  - "02-offline-budget"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-MCP-WAIT-005
acceptance_criteria:
  - AC-PLATFORM-DETACHED-MCP-WAIT-005.1
  - AC-PLATFORM-DETACHED-MCP-WAIT-005.2
  - AC-PLATFORM-DETACHED-MCP-WAIT-005.3
system_design:
  - ../../specs/platform/system-design/detached-mcp-call-wait.md
---

# Task 04: Record detached episodes in the conversation

## Summary

Stamp `detached_at_commit`, `committed_at`, `cancel_cause`, and `cancel_decided_at` at the journal terminal-commit point, then write one idempotent status record per terminal when the task service projects it, and render it localized.

## In scope

- Terminal stamp in `agentctl/server/process`; `stop_reason` stays `cancelled` so existing settlement classifiers are unchanged.
- Task service projection hook: `MessageTypeStatus` with `metadata.kind`, idempotency key `<delivery_stream_id>:<sequence>`, inserted in the projection transaction.
- Web renderer with copy in all seven catalogs; phone and desktop.
- Playwright coverage, desktop and mobile, driven by the mock agent.

## Out of scope

A banner, Reconnect action, or Disconnected state.

## Verification

- `go test -trimpath ./internal/task/service/ -run 'DetachedEpisode' -count=1`
- `cd apps/web && pnpm exec vitest run --reporter=dot detached`
- `cd apps/web && pnpm e2e:run -- detached-episode`
