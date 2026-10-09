---
id: "01-add-muse-acp-agent"
title: "Add the Muse Code ACP agent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MUSE-ACP-001
  - REQ-AGENTS-MUSE-ACP-002
  - REQ-AGENTS-MUSE-ACP-003
acceptance_criteria:
  - AC-AGENTS-MUSE-ACP-001.1
  - AC-AGENTS-MUSE-ACP-001.2
  - AC-AGENTS-MUSE-ACP-002.1
  - AC-AGENTS-MUSE-ACP-002.2
  - AC-AGENTS-MUSE-ACP-002.3
  - AC-AGENTS-MUSE-ACP-003.1
  - AC-AGENTS-MUSE-ACP-003.2
  - AC-AGENTS-MUSE-ACP-003.3
system_design:
  - ../../specs/agents/system-design/muse-acp-agent.md
---

# Task 01: Add the Muse Code ACP Agent

## Summary

Add `agents.MuseACP`, register it, pin the adapter in the managed npm runtime
catalogue, and document the command surfaces.

## In scope

- `muse_acp.go`, logos, registry entry, catalogue pin, bridge-version table.
- Shared ACP agent matrix, managed-runtime contract, and Muse-specific tests.
- Public agent docs.

## Out of scope

- Implementing MSP in Kandev, Windows installation, and passthrough MCP.

## Acceptance

`go test ./internal/agent/agents ./internal/agent/registry` and
`make lint-backend` pass; a live Muse host is detected, runs a structured task
with a tool call, and resumes it.
