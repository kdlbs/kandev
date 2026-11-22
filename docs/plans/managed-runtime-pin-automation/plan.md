---
created: 2026-11-20
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
| `@agentclientprotocol/claude-agent-acp` | `0.107.0` | `0.109.0` |
| `@agentclientprotocol/codex-acp` | `2.7.0` | `2.8.0` |
| `@github/copilot` | `1.0.111` | `1.0.114` |
| `@google/gemini-cli` | `0.68.0` | `0.70.0` |
| `@openai/codex` | `0.169.0` | `0.171.0` |
| `@opencode/cli` | `2.3.0` | `2.3.2` |
| `opencode-ai` | `1.19.5` | `1.19.6` |

## Validation

- `node --test scripts/update-agent-runtime-pins.test.mjs` passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers)` passed.

## Work orders

- [x] [Task 01: Refresh managed runtime pins](task-01-refresh-pins.md)
