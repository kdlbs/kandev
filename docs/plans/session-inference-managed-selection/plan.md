---
created: 2026-11-23
status: done
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-OPENCODE-V2-001
system_design:
  - ../../specs/agents/system-design/runtime-updates-02.md
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Fix plan for session-bound utility prompt runtimes

## Overview

Session-bound utility prompts launch the shipped default package instead of
the runtime the operator selected. An installation that pinned a managed
version keeps running the default for these prompts. An installation that
adopted OpenCode v2 keeps running the v1 package.

`AC-AGENTS-RUNTIME-UPDATES-001.7` requires the effective exact version for
utility calls. `AC-AGENTS-OPENCODE-V2-001.4` requires the v2 package for
future launches after adoption. The OpenCode design lists the consumers that
must use the resolved runtime.
[Runtime update requirements](../../specs/agents/requirements/runtime-updates.md#req-agents-runtime-updates-001-managed-agent-runtime-versions-and-updates),
[OpenCode adoption requirements](../../specs/agents/requirements/opencode-v2-adoption.md#req-agents-opencode-v2-001-default-and-explicit-adoption),
[OpenCode command resolution](../../specs/agents/system-design/opencode-v2-adoption.md#runtime-definitions-and-command-resolution)

## Root cause and evidence

`ExecuteInferencePrompt` and `ExecuteInferenceProfilePrompt` send
`InferenceConfig().Command` to agentctl. Managed agents build that command
with `CachedACPCommand()`, which names the shipped default version. OpenCode
builds it from the v1 runtime. Task launches resolve the selection through
`resolveManagedRuntimeCommandOptions`, but these two paths never call it.

On `4ebc1e2fd`, a Copilot selection of `1.0.104` produced
`@github/copilot@1.0.114` for a session-bound prompt. An OpenCode v2 managed
selection produced `opencode-ai@1.19.6`.

## Technical approach

Resolve the session-bound command the way a task launch on the same execution
does. For a managed agent, call `resolveManagedRuntimeCommandOptions` with the
execution's runtime, apply the native-binary preference for that runtime, and
build the command with `BuildCommand`. Agents without a managed runtime keep
their static inference command.

At the shipped defaults, `InferenceConfig().Command` and `BuildCommand` return
the same arguments for managed agents other than OpenCode. Standalone OpenCode
can select an installed native binary, so its command can change from the
static `npx` command even when the selected package family stays at its default.
Remote OpenCode runtimes use the managed package command.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-AGENTS-RUNTIME-UPDATES-001.7` | A lifecycle test selects Copilot `1.0.104` and checks the session-bound prompt command version. |
| `AC-AGENTS-OPENCODE-V2-001.4` | A lifecycle test supplies a managed v2 OpenCode selection and checks that the prompt command names the v2 package. |
| Profile command selection | A profile utility prompt uses the selected Copilot version. |
| OpenCode runtime policy | Standalone native and remote managed utility prompts use the command selected for their execution runtime. |
| Failure and admission safety | Selection errors and empty built commands stop before agentctl; an OpenCode utility on a non-OpenCode session holds migration admission until agentctl returns. |

## Work orders

- [x] [Task 01. Resolve session-bound prompt runtimes](task-01-resolve-session-prompt-runtime.md)

## Verification results

Task 01 is complete. The session inference tests cover managed version and
family selection, profile command selection, native standalone and managed
remote OpenCode commands, selection errors, empty command validation, and
migration admission through the agentctl response.

The focused session inference tests, the full lifecycle package, and the
focused race-detector tests passed. `go vet` passed, `gofmt -l` returned no
files, `git diff --check` passed, and changed-file Go lint reported zero
issues.

## Risks

- A managed agent whose inference command diverges from its ACP command would
  change behavior. No built-in agent diverges at the shipped defaults.
