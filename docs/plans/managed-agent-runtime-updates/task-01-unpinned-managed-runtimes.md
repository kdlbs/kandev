---
id: "01-unpinned-managed-runtimes"
title: "Define unpinned managed runtimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
role: implementer
model_tier: default
---

# Task 01: Define unpinned managed runtimes

## Acceptance

- Claude, Codex, OpenCode, Copilot, and Gemini expose their hard-coded managed
  npm package and use an unversioned `--prefer-offline` npx command for every
  ACP build/runtime/inference surface.
- The built-in update command uses direct argv with `--prefer-online`; no
  version, `latest`, registry, package, or shell text comes from a caller.
- Command-surface tests cover all five agents, and Copilot's managed ACP path no
  longer changes to a native binary.

## Verification

- `cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle`

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`
- `apps/backend/internal/agent/agents/{claude_acp,codex_acp,opencode_acp,copilot_acp,gemini}.go`
- Tests beside those five agent files
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`

## Dependencies

None.

## Inputs

- Spec sections `What`, `Scenarios`, and `Out of scope`
- ADR `Decision` and `Consequences`
- Plan sections `Managed npm runtime contract` and `Tests`
- Existing `agents.Command`, `InferenceAgent`, and optional capability patterns

## Output contract

Report intent/acceptance, base/head SHA, changed entry points, spec/ADR sections,
risk tags (`runtime-resolution`, `npm-cache`, `agent-launch`), the exact test
result, and uncertainties. Update only this task file to `done`; do not edit
`plan.md`.
