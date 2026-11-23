---
id: "01-resolve-session-prompt-runtime"
title: "Resolve session-bound prompt runtimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.7
  - AC-AGENTS-OPENCODE-V2-001.4
system_design:
  - ../../specs/agents/system-design/runtime-updates-02.md
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 01. Resolve session-bound prompt runtimes

## Summary

Build session-bound utility prompt commands from the execution's resolved
managed runtime. A selected version or an adopted OpenCode family applies to
these prompts as it does to task launches.

## Scope

- Add `resolveSessionInferenceCommand` to the lifecycle manager. For a managed
  agent, resolve the command options for the execution's runtime, set the
  native-binary preference for that runtime, and build the command. Keep the
  static inference command for other agents.
- Use the resolved command in `ExecuteInferencePrompt` and
  `ExecuteInferenceProfilePrompt`.
- Hold OpenCode migration admission for the actual utility agent until the
  agentctl request returns, even when the session execution uses another agent.
- Validate the built command before sending the request to agentctl.
- Add `TestSessionInferenceUsesSelectedManagedRuntimeVersion` for a Copilot
  version selection.
- Add `TestSessionInferenceUsesSelectedOpenCodeFamily` for a managed OpenCode
  v2 selection.
- Cover profile selection, standalone native and remote managed OpenCode
  commands, command resolution failures, empty commands, and migration
  admission while agentctl handles an OpenCode utility request.

## Out of scope

- Host utility and sessionless prompt paths, which already resolve the
  selection.
- Changes to how a selection is stored or validated.

## Verification

Run these commands from `apps/backend`.

- `go test -trimpath -tags fts5 -run '^TestSessionInference' ./internal/agent/runtime/lifecycle/` passed.
- `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/` passed.
- `go test -race -trimpath -tags fts5 -run '^TestSessionInference' ./internal/agent/runtime/lifecycle/` passed.
- `gofmt -l` on the three changed Go files returned no files.
- `go vet -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/` passed.
- `bash scripts/lint-go-changed` on the three changed Go files reported `0 issues`.
- `git diff --check` passed.
