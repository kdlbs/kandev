---
id: "01-pin-core-agent-runtimes"
title: "Pin core agent runtimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
decision: "../../decisions/2026-07-25-scheduled-core-agent-version-pins.md"
---

# Task 01: Pin core agent runtimes

## Acceptance

- Claude ACP, Codex ACP, OpenCode, Copilot, and Gemini use exact stable package
  versions on every npm-backed launch and install surface.
- Command-surface tests prove the exact specs, including native-binary fallback
  behavior for Copilot.
- Current-version documentation lists the five pins and explains why Cursor is
  intentionally not pinned.

## Verification

- `cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle`

## Files likely touched

- `apps/backend/internal/agent/agents/{claude_acp,codex_acp,opencode_acp,copilot_acp,gemini}.go`
- Agent tests beside those files
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- `README.md`
- `docs/decisions/0034-agentclientprotocol-codex-acp.md`

## Dependencies

None.

## Inputs

- ADR Decision and Consequences sections
- Plan sections "Core runtime pins" and "Current-version documentation"

## Output contract

Report the selected stable versions, files changed, targeted test result, risk
tags, and any upstream package ambiguity. Set this task to `done` only after
the verification command passes.
