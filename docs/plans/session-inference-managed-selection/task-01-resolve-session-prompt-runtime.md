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
- Add `TestSessionInferenceUsesSelectedManagedRuntimeVersion` for a Copilot
  version selection.
- Add `TestSessionInferenceUsesSelectedOpenCodeFamily` for a managed OpenCode
  v2 selection.

## Out of scope

- Host utility and sessionless prompt paths, which already resolve the
  selection.
- Changes to how a selection is stored or validated.

## Verification

- `go test -tags fts5 -run TestSessionInference ./internal/agent/runtime/lifecycle/`
  fails before the change and passes after it.
- `go test -tags fts5 ./internal/agent/runtime/lifecycle/` reports the same
  failures before and after the change.
- `gofmt -l` and `go vet -tags fts5` report nothing for the lifecycle package.
