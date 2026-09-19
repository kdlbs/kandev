---
created: 2026-09-19
status: in-progress
requirements:
  - REQ-AGENTS-MUSE-ACP-001
  - REQ-AGENTS-MUSE-ACP-002
  - REQ-AGENTS-MUSE-ACP-003
system_design:
  - ../../specs/agents/system-design/muse-acp-agent.md
---

# Implementation Plan: Muse Code ACP Agent Support

## Overview

Add Meta's Muse Code CLI (released 2026-08-05, backed by Muse Spark) as a
built-in Kandev agent with structured chat, tool calls, MCP, one-shot
inference, CLI passthrough, and session resume.

Muse has **no native ACP server**. Its structured surface is the Muse Session
Protocol (MSP) served by `muse serve`. Per the runbook ("If the upstream CLI
has no ACP server, add or reuse a bridge that speaks ACP"), this reuses the
community `@bex-co/muse-code-acp` adapter (Apache-2.0, built on the official
`@muse-code/sdk`), which bridges MSP to ACP over stdio. The adapter is
npm-distributed, so the launch follows the Pi shape: a managed npm runtime for
structured sessions and inference, and the native `muse` binary for
detection, install, login, and passthrough.

## Integration contract

- **Structured launch:** `npx --yes --prefer-offline
  @bex-co/muse-code-acp@<effective-version>`, no arguments; pinned in
  `managed_npm_runtime_versions.json` and kept current by
  `update-agent-runtime-pins`. Requires Node.js 22+.
- **Passthrough:** `["muse"]` with `--model {model}`. No passthrough MCP
  strategy (as with Goose).
- **Detection:** `muse --version` on PATH. The adapter ships no Muse binary,
  so an adapter-only host is correctly reported as not installed.
- **MCP:** delivered through ACP `session/new`; the adapter writes stdio and
  HTTP servers into a temporary Muse config overlay. No `ProjectMCPStrategy`.
- **Session resume:** native, via the adapter's `session/load`/resume over
  `muse serve`. Session root `{home}/.local/share/muse`
  (`$XDG_DATA_HOME/muse/sessions`).
- **Auth:** `muse login` (browser) writes `~/.config/muse/auth.json`, copied
  by the `files` remote-auth method. `META_API_KEY` is the headless `env`
  method and takes priority over stored auth.
- **Install script:** downloads `https://dev.meta.ai/install.sh` to a
  temporary file and runs it with `MUSE_INSTALL_DIR` set to the first
  writable absolute PATH directory, then verifies `muse --version`.
- **Permissions:** empty settings; the adapter forwards Muse's approval
  requests (including per-stage shell approvals) to the ACP client, where
  agentctl's auto-approve policy applies.
- **Models and modes:** discovered by the host-utility probe from the
  adapter's `session/new` config options; no static list.

## Live verification

Verified on macOS arm64 with Muse Code 1.3.0 (1.3.0-R3401.1) and adapter
0.6.1, through `acpdbg` and a raw ACP client:

- `acpdbg probe muse-acp`: initialize, `session/new`, one model and five modes
  (`default`, `readOnly`, `plan`, `bypassApprovals`, `rejectApprovals`).
- `acpdbg prompt`: a turn that created a workspace file with a shell tool
  call and ended with `end_turn`.
- A stdio MCP server passed in `session/new` was started and its tool called
  (`mcp__everything__echo`). `acpdbg mcp-probe` reports the HTTP sentinel
  as unobserved because Muse connects MCP servers only when a turn starts.
- `acpdbg session-load`: history replayed and a follow-up turn recalled the
  earlier turn.
- The adapter uses `model` from `~/.config/muse/settings.json`, otherwise its
  built-in `muse-spark-1.2`; native `muse` requires `schema_version` in that
  file and otherwise uses the provider catalogue default.

## Known limitations

- The adapter is community-maintained and not affiliated with Meta. Its
  verified Muse host versions are 1.1.1 and 1.2.1; the Muse launcher
  self-updates, so a newer host can drift from the adapter's pinned MSP
  schema (the adapter reports this as an advisory `hostCompatibility` entry).
- Token usage and delegated workers are not reported by the adapter.
- Windows is not covered: Muse's installer is POSIX shell.

## Work orders

- [Task 01: Add the Muse Code ACP agent](task-01-add-muse-acp-agent.md)

## Verification

- Unit: shared `newACPAgentSpecs` matrix (IDs, every command surface,
  detection, logos, session dir), `TestManagedNPMRuntimeContracts`, and
  `muse_acp_test.go` (installer behaviour, remote auth, login, resume/MCP).
- Manual: Settings > Agents detection, profile creation, a structured task
  with tool calls and an MCP tool, resume after restart, model/mode refresh,
  and passthrough.
