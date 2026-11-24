---
id: "01-refresh-pins"
title: "Refresh managed runtime pins"
status: completed
wave: 1
depends_on: []
created: 2026-11-20
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.5
  - AC-AGENTS-RUNTIME-UPDATES-001.7
  - AC-AGENTS-RUNTIME-UPDATES-001.9
  - AC-AGENTS-RUNTIME-UPDATES-001.10
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
---

# Task 01: Refresh managed runtime pins

## Summary

Refresh the reviewed managed npm runtime defaults from stable `latest`
metadata. The scheduled updater validates the catalogue and managed runtime
commands before writing this completed record.

## Changed packages

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

## Results

The managed runtime pin updater and the six-package Go validation completed
before this record was generated. The workflow does not install, activate, or
merge a runtime. Package, argument, Copilot plugin, and documentation coverage
contracts remain in their owning tests.
