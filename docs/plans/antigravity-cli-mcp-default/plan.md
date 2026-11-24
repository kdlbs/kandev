---
created: 2026-11-24
status: in_progress
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-006
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
legacy_specs: []
---

# Implementation plan for Antigravity CLI MCP startup

## Overview

Make Kandev supply `--process-per-session` for every qualified local Antigravity CLI session. Apply this rule at process launch so existing profiles and all models receive the same MCP support.

## Evidence

Kandev build `ede4bcff91b` selected `agy acp` 1.5.15 for a worktree session. The ACP response omitted the HTTP MCP capability. The adapter filtered both transport options for Core and stopped before `session/new`.

With the worker flag, `initialize` advertised HTTP MCP. A loopback probe with the Kandev route shape received `tools/list` and returned one tool. The OAuth metadata requests received `404` on their own paths. [Google ACP flags](https://antigravity.google/docs/ide/extensions/acp#flags)

The CLI accepted duplicate enabling flags. A later disabling flag suppressed HTTP advertisement, and a later enabling flag restored it. A plain CLI conversation also resumed under worker mode with its original native ID.

## Scope

- Local and worktree task launches, fresh starts, and restores of CLI conversations.
- Command previews, capability and model probes, and utility inference.
- Version and flag checks for CLI selection, plus compatibility with existing profile flags.
- Existing CLI sign-in and native conversation routing.

The repair uses the existing ACP MCP transport, helper routes, and profile store. Standalone and remote runtime provisioning follow the [Antigravity runtime design](../../specs/agents/system-design/antigravity-acp-agent.md).

## Technical approach

1. Qualify the host CLI in `antigravityruntime.resolveQualifiedCLI`. Check its version and the worker flag in bounded help output. Return the complete worker command.
2. Keep worker mode enabled after profile arguments apply. Reuse the same Antigravity argument rule in lifecycle, preview, and utility launch composition. Apply any wrapper to the resulting command.
3. Construct the CLI sign-in command in `AntigravityACP.ResolveLoginCommand` as `agy acp --login`. Use the original runtime variant for saved conversations.
4. Keep the common adapter's capability check. The worker must advertise HTTP before Kandev sends Core through ACP.

| Launch context | Applicable rule | Failure handling |
| --- | --- | --- |
| New local or worktree CLI process | Worker mode, independent of profile and model | Select the standalone runtime for new work if CLI qualification fails |
| Saved CLI conversation | Worker mode with the recorded CLI variant and native ID | Report the saved-runtime failure if the CLI cannot qualify |
| Saved standalone conversation or remote executor | Existing standalone arguments and authentication | Use the existing standalone and executor paths |
| CLI sign-in | `agy acp --login` | Use the existing sign-in result and refresh flow |

## Verification

Extend the existing selection, launch, resume, preview, probe, and login tests. Add a launch-to-wire regression with an ACP fixture that advertises HTTP only when worker mode is active. Run the commands in the work order.

For end-to-end evidence, start a task in an isolated Kandev instance with the qualified CLI and a profile without manual worker flags. Verify that the ACP request includes Core and that the MCP endpoint receives tool discovery. Then stop and resume a saved CLI conversation and compare its native ID. Use two model selections to exercise the same launch rule. The existing UI and API expose these outcomes.

## Work orders

- [ ] [Task 01 Enable HTTP MCP by default for the Antigravity CLI](task-01-cli-worker-mode.md) (in progress)

## Verification results

The Linux `agy` 1.5.15 diagnostic probes reproduced the capability mismatch and verified worker mode, tool discovery, flag precedence, and native resume. Implementation qualification now requires the worker option, and all local launch composition points enforce it after profile arguments.

The exact package-filtered Go test command and public documentation checks passed. `make -C apps/backend build` passed, with the existing warning that macOS artifacts remain unsigned because no signing tool is installed. Resolver regressions verify value-aware flag placement and qualification from successful help declarations. The two-model ACP fixture verified HTTP advertisement and Core delivery through `session/new`. A lifecycle fixture verified that stopping one launch reaps only its child, a fresh manager resumes with the stored native ID using the saved CLI runtime, and its child is reaped on stop. The isolated task smoke reached ACP startup but could not create a native session because the isolated home had no Google sign-in. The plan remains in progress until the isolated task flow can create a real native session, discover tools, and resume it.

## Risks

- The minimum CLI version establishes ACP support. The worker flag check establishes eligibility for editor-supplied MCP.
- A saved profile can contain a disabling flag. The final CLI command must enforce worker mode before it reaches a wrapper.
- A worker is a child process. Verify that the existing process owner reaps it on stop and preserves other sessions.
