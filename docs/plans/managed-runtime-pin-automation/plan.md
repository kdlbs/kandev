---
created: 2026-11-27
status: completed
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
legacy_specs: []
---

# Managed runtime pin refresh

## Overview

The scheduled managed runtime pin updater generated this delivery record after
the trusted catalogue and managed-runtime command validation passed. It records
one grouped review change and does not activate a runtime.

## Changed versions

| Package | Previous | Proposed |
| --- | --- | --- |
| `@agentclientprotocol/claude-agent-acp` | `0.109.0` | `0.113.0` |
| `@agentclientprotocol/codex-acp` | `2.8.0` | `2.12.1` |
| `@github/copilot` | `1.0.114` | `1.0.116` |
| `@google/gemini-cli` | `0.70.0` | `0.71.1` |
| `@openai/codex` | `0.171.0` | `0.172.0` |
| `@opencode/cli` | `2.3.2` | `2.4.2` |
| `pi-acp` | `0.0.34` | `0.0.35` |

## Validation

- `node --test scripts/update-agent-runtime-pins.test.mjs` passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers)` passed.

## Work orders

- [x] [Task 01: Refresh managed runtime pins](task-01-refresh-pins.md)
