---
created: 2026-10-26
status: implemented
requirements:
  - REQ-PLATFORM-DETACHED-MCP-WAIT-001
  - REQ-PLATFORM-DETACHED-MCP-WAIT-002
  - REQ-PLATFORM-DETACHED-MCP-WAIT-003
  - REQ-PLATFORM-DETACHED-MCP-WAIT-004
  - REQ-PLATFORM-DETACHED-MCP-WAIT-005
  - REQ-PLATFORM-DETACHED-MCP-WAIT-006
system_design:
  - ../../specs/platform/system-design/detached-mcp-call-wait.md
legacy_specs: []
---

# Implementation Plan: Detached MCP call wait

## Overview

Agent to Kandev calls wait through a backend stream outage instead of failing
after 5 seconds. A bounded offline budget cancels the turn at a clean boundary.
This reimplements the narrowed call-waiting half of kdlbs/kandev#4255 on the
journal and reattachment work that landed in #3598 and #4380. It links both.

## Work orders

- [x] [Task 01: Wait for attach and report unknown outcomes](task-01-wait-and-outcome.md)
- [x] [Task 02: Offline budget tracker and configuration](task-02-offline-budget.md) (after 01)
- [x] [Task 03: Keepalive, harness floor, and agent guidance](task-03-keepalive-floor-guidance.md)
- [x] [Task 04: Record detached episodes in the conversation](task-04-replay-notices.md) (after 02)

## Dependency order

01 and 02 ship in one PR (01 first); 04 follows 02; 03 is independent and can run beside 01. Wave 1: 01, 03. Wave 2: 02. Wave 3: 04.

## Changed-path coverage

`apps/backend/internal/mcp/server`, `internal/agentctl/server/{api,process}`,
`cmd/agentctl`, `internal/orchestrator/executor`,
`internal/agent/runtime/{agentctl,lifecycle}`, `internal/agent/agents`,
`internal/sysprompt`, `config/prompts/kandev-context.md`, `internal/task/service`,
and `apps/web` message rendering with seven locale catalogs.

## Risks

- A budget cancel races a late attach (004.9); the tracker mutex decides it.
- The unowned reaper (default 10 min) can end an instance before the 15-minute
  default budget; 004.8 leaves reapers unchanged and 004.12 warns at creation.
- Upgrade impact: a profile with `MCP_TOOL_TIMEOUT` under 16 minutes now fails
  launch with a typed error. Public docs and release notes must say so.
- Codex has no measured tool-timeout floor; follow-up outside this PR.
- The notice's detach-interval proof depends on the projected cursor; if it is
  unprovable, no notice is written (silence over a wrong notice).

## Results

All four work orders shipped in kdlbs/kandev#4809. Validation: `go test -trimpath`
over `internal/mcp/server`, `internal/agentctl/server/{instance,process}`,
`internal/sysprompt`, `internal/agent/runtime/lifecycle`; `scripts/lint-go-changed`
clean; web typecheck, i18n checks and Playwright `detached-episode` (desktop and
mobile) green. Known gaps carried as follow-ups: the offline budget does not
cancel a turn on adapters whose `Prompt` returns before the turn ends (Codex
app-server), and a failed notice write is not retried.
