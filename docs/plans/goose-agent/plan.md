---
created: 2026-09-03
status: in-progress
referenced_runbook: docs/public/add-agent-cli.md
---

# Implementation Plan: Goose (AAIF) ACP Agent Support

## Overview

Add the Goose open-source coding agent (Agentic AI Foundation / AAIF, formerly
Block; repo `aaif-goose/goose`) as a first-class Kandev agent. Users can select
Goose as their coding agent; Kandev detects it, launches it over ACP, wires
permissions and MCP through `session/new`, supports one-shot inference, CLI
passthrough, and session resume.

Goose speaks **ACP over stdio** via `goose acp`, the exact protocol Kandev's
agentctl adapter factory accepts. It is distributed as a **native CLI binary**
(install script `download_cli.sh`, Homebrew `block-goose-cli`, or `pip install
goose-ai`), so it is discovered purely on PATH and uses **no** managed
npm runtime. Config lives under `~/.config/goose`; session/history data under
`~/.local/share/goose`. Credentials come from `goose configure`.

Because Goose is a native-binary ACP agent, this is the runbook's "Ship a
structured ACP integration" path (`docs/public/add-agent-cli.md`), modeled on
`agents/gemini.go` (example) and `agents/agent.go` (contract); Hermes, Qwen,
and Devin are the closest siblings.

## Verified integration contract

Evidence is drawn from the Goose source, an ACP client reference
(`catatafishen/agentbridge#1009`, merged), terminal-transcript detection
(`herdrdev/herdr#3279`), and Paseo's per-agent catalog for positioning.

- **Launch:** `["goose", "acp"]`; passthrough `["goose"]`.
- **Protocol:** `agent.ProtocolACP` (only protocol the adapter factory accepts).
- **MCP:** delivered natively via ACP `session/new` (`mcpServers`); **no**
  `ProjectMCPStrategy` needed. Goose supports Stdio and HTTP (StreamableHttp)
  MCP but **rejects SSE**; keep `AssumeMcpSse=false`.
- **Developer built-in:** `goose acp` auto-enables Goose's `developer` built-in
  extension by default; this is expected and documented.
- **Global extensions:** Goose loads globally-enabled extensions even when
  `session/new` MCP servers are passed; there is no skip knob on `goose acp`.
  Accepted behavior; a slow unrelated global extension may delay `initialize`.
- **Session resume:** ACP `session/load`; native resume enabled with session
  root `{home}/.local/share/goose`.
- **Auth:** declarative config (`~/.config/goose`); primary remote path is a
  `files` `RemoteAuth` copy of the config dir; optional `goose configure` PTY
  login.
- **Permissions:** Goose presents real interactive approval prompts; map
  `PermissionKeyAutoApprove` via `PermissionApplyMethodAgentctlAutoApprove`.
- **Environment:** no required environment variables; Goose reads provider
  credentials from its config files.
- **Install script:** Kandev downloads the official script to a temporary file,
  runs it with `CONFIGURE=false` in the first writable absolute directory on
  the current `PATH`, and verifies the installed `goose acp --help` command.

> **Deviation from earlier plan draft:** `NativeBinaryAgent` is **not**
> implemented. The interface (`agent.go`) is specifically for npm-distributed
> packages that also ship a standalone CLI (e.g. Copilot) to prefer over
> `npx`. Goose is a pure native binary with no npm launch path, so the
> interface does not apply; implementing it would be a meaningless flag.

## Files

### Create

- `apps/backend/internal/agent/agents/goose_acp.go`
- `apps/backend/internal/agent/agents/goose_acp_test.go`
- `apps/backend/internal/agent/agents/logos/goose_light.svg`
- `apps/backend/internal/agent/agents/logos/goose_dark.svg`
- `docs/plans/goose-agent/plan.md` (this document; shipped in the PR as a
  reference for reviewers and future contributors)

### Modify

- `apps/backend/internal/agent/registry/registry.go` — add
  `agents.NewGooseACP()` to `LoadDefaults()` after `NewHermesACP()`.
- `apps/backend/internal/agent/agents/new_acp_agents_test.go` — add a `GooseACP`
  entry to `newACPAgentSpecs` so the shared contract matrix, detection, logos,
  and display-order tests cover it.

### Docs

- `docs/public/agents-and-profiles.md` — document Goose as a supported agent
  (install, launch via `goose acp`, credentials via `goose configure`) at parity
  with peers.

## Steps

1. **Contract pinning (TDD)** — write `goose_acp_test.go` first, pinning:
   identity (`goose-acp`, "Goose ACP Agent", "Goose", DisplayOrder 22);
   `["goose","acp"]` across BuildCommand/Runtime.Cmd/InferenceConfig.Command;
   passthrough `["goose"]`; `Runtime().Protocol == agent.ProtocolACP`;
   `SessionDirTemplate == "{home}/.local/share/goose"`; InstallScript; RemoteAuth
   files for darwin/linux; no provider environment requirements; logos non-empty;
   bounded ACP-help detection.
2. **Implement `goose_acp.go`** — `type GooseACP struct { StandardPassthrough }`;
   marker interfaces `Agent`, `PassthroughAgent`, `InferenceAgent`,
   `LoginAgent`; `goose` binary via `Cmd`/`Command` builders (no shell strings);
   `IsInstalled` via a bounded `goose acp --help` identity check; `Runtime()` with
   Protocol ACP, native resume, pinned session dir, `AssumeMcpSse=false`,
   no `StripEnv`; `RemoteAuth` file copy; `InstallScript`; `PermissionSettings`;
   `InferenceConfig`.
3. **Register** in `registry.go` `LoadDefaults()`.
4. **Test wiring** — add Goose to `new_acp_agents_test.go` matrix; add SVGs.
5. **Validate** — `make -C apps/backend` package tests (agents, registry,
   lifecycle, adapter), `make lint-backend`, `make test-backend`, and the
   real-adapter `make -C apps/backend test-e2e` (gated, may need a real goose
   install / login).
6. **Manual smoke** — `make dev`, confirm Goose appears in the agent/profile
   picker, start/prompt/cancel/resume, model/mode refresh, permissions,
   `session/new` MCP registration via `acpdbg mcp-probe goose`.
7. **Docs + i18n** — update public docs; any user-facing copy via `t()` / i18n
   ratchets (agent name/description surface via the backend DTO, English).
8. **Commit** — single `feat(agents): add Goose ACP agent support` with this plan
   doc included.

## Risks

- **ACP wire compatibility:** `goose acp` is documented ACP-compatible and its
  stdio entry is stable (low impact). A bridge shim is the fallback if a
  mismatch surfaces; verify with a real spawn early.
- **Tool-title humanization (from #1009):** Goose humanizes MCP `tool_call`
  titles (`"extension: read file · /arg"`); confirm Kandev's stream layer
  handles or normalizes these, else add a small parser.
- **Global extensions auto-load:** accepted; may delay `initialize` if a slow
  unrelated extension is configured.
- **Detection:** `goose acp` is a blocking server; use its bounded `--help`
  command, not the server itself or a provider-opening probe.
- **Windows support** is unverified upstream; treated as a caveat.
