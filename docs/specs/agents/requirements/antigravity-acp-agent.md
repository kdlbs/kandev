---
status: draft
system: agents
created: 2026-09-03
owners:
  - kandev
---

# Google Antigravity ACP agent requirements

## Overview

Google publishes a standalone ACP server for Antigravity as a signed native
binary, separate from the `agy` CLI. Kandev needs it as a built-in agent so a
user can select it for a task, run and resume a session over ACP, and use MCP
servers and skills, without inventing a distribution, credential, or
session-storage mechanism Google already owns. The agent system owns the durable
decision: an agent identity plus its discovery, launch, config-root,
authentication, and capability contract; transport and process supervision
belong to the runtime. This first layer assumes the operator already installed
the server; automated download and remote or containerized provisioning are
named exclusions, not silence.

## Terminology

- **ACP server:** The executable in the ACP registry entry `antigravity-acp`
  (`agy_acp_server.par`, `.exe` on windows). Not the `agy` CLI.
- **Harness:** The `localharness_external` executable in the same archive, which
  the ACP server locates and drives over gRPC.
- **Config root:** `<home>/antigravity-acp`, where `<home>` is `$GEMINI_HOME`
  when set and non-empty and `~/.gemini` otherwise. Holds settings, credentials,
  trusted workspaces, and conversations. What Kandev declares about it is
  REQ-AGENTS-ANTIGRAVITY-ACP-004.
- **Advertised capabilities:** The `agentCapabilities` and `authMethods` in the
  ACP `initialize` result.

## Prior art

Receipts first, because a leg with no receipt did not run.

**Wiki leg: did not run, tool unavailable.** Vault
`/Users/henry/Documents/henry/wiki`, collection `wiki`, `@henry`-pinned.
`obsidian-wiki` and `qmd` are off PATH; the vault returns `Operation not
permitted` to `ls` and `grep`.

**Cross-product leg: did not run, tool unavailable.** The `saas-kb` MCP server
and its `search_fsm_docs` tool are not exposed in this session.

**In-repo prior art: read directly.** Kandev's built-in ACP
agents are either managed npm runtimes on a pinned `npx` specification or
binaries found on PATH and launched by name. Antigravity is the second kind, and
Kandev has no managed non-npm download mechanism, which is why provisioning is
excluded below rather than reinvented. It departs from that group three times —
discovery checks a second file, the install script is empty, and there is no CLI
passthrough — each argued in the design's
[prior-art departure](../system-design/antigravity-acp-agent.md#prior-art-departure).

## Evidence

Values are of two kinds. **Measured** ones come from a manual probing session
against build `agy_acp_server_20260818_01_RC01`, darwin-arm64, on 2026-09-03;
the artifact was removed afterwards, so this pair summarizes that session rather
than transcribing it, and re-checking one means re-downloading that build.
**Registry** ones are read from the published `antigravity-acp/agent.json` and
were never executed; each use says so.

## Requirements

### REQ-AGENTS-ANTIGRAVITY-ACP-001: Built-in Antigravity agent identity

**Intent:** Give Antigravity a stable catalog identity so a user can select it
for a task and profile like any other built-in agent.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-001.1:** The agent identifier shall be
  `antigravity-acp`, matching the `agentInfo.name` the ACP server reports.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.2:** The display name shall be `Antigravity`
  and the catalog name shall be `Antigravity ACP Agent`.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.3:** The agent shall be enabled by default and
  present in the default registry.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.4:** The display order shall be `22`, distinct
  from every other built-in agent's, so the display-order sort never reaches its
  stable-sort tiebreak here.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.5:** The agent shall return a non-empty light
  and dark logo, and the light one for any other variant.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.6:** The agent shall declare the ACP
  protocol and shall not be a virtual agent.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.7:** Because REQ-AGENTS-ANTIGRAVITY-ACP-007
  contributes no install script, the description shall carry the install
  guidance, stating all four of: the registry entry `antigravity-acp` as the
  source; that the archive's two entries extract together into one directory;
  that the directory be on PATH; and the platform executable name from
  AC-AGENTS-ANTIGRAVITY-ACP-002.1. It shall embed no archive URL or build stamp,
  both of which rotate.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.8:** The agent shall declare no agent-specific
  permission settings; its catalog shall still expose the universal
  auto-approve setting.
- **AC-AGENTS-ANTIGRAVITY-ACP-001.9:** The agent shall not offer a CLI
  passthrough mode.

### REQ-AGENTS-ANTIGRAVITY-ACP-002: Discovery that fails closed on a partial install

**Intent:** Report Antigravity as installed only when it can run, so a user
never starts a session that silently loses capability mid-prompt.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-002.1:** Discovery shall look up exactly one
  executable name on the PATH of the Kandev process performing discovery:
  `agy_acp_server.exe` on windows, `agy_acp_server.par` on every other platform.
  The name is therefore defined on every platform. Registry-sourced.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.2:** When that executable is not on PATH,
  discovery shall report unavailable with an empty matched path and no error.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.3:** When the executable is on PATH, discovery
  shall additionally require a harness in the directory containing it, trying
  `localharness_external` then `localharness`, first match wins, each
  `.exe`-suffixed on windows and unsuffixed elsewhere. A name matches a regular
  file, with an execute bit required on non-windows hosts. A directory, dangling
  symlink, or unstat-able name does not match; discovery continues.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.4:** When `ANTIGRAVITY_HARNESS_PATH` is set to a
  non-empty value in the environment discovery observes, the
  AC-AGENTS-ANTIGRAVITY-ACP-002.3 check shall be treated as satisfied without
  inspecting the directory.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.5:** When the executable is present but
  neither AC-AGENTS-ANTIGRAVITY-ACP-002.3 nor AC-AGENTS-ANTIGRAVITY-ACP-002.4
  holds, the AC-AGENTS-ANTIGRAVITY-ACP-002.2 outcome shall apply unchanged.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.6:** When discovery reports the agent
  available, the matched path shall be the path PATH lookup returned, and MCP
  support and session resume shall both be reported as supported.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.7:** The directory inspected by
  AC-AGENTS-ANTIGRAVITY-ACP-002.3 shall be the directory of the path PATH lookup
  returned, without resolving symbolic links.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.8:** Discovery shall create no files, write no
  configuration, and start no process, so repeated calls over an unchanged PATH
  and filesystem return equal results.
- **AC-AGENTS-ANTIGRAVITY-ACP-002.9:** When the caller's context is cancelled or
  expires during discovery, discovery shall return that error rather than
  reporting the agent as unavailable.

### REQ-AGENTS-ANTIGRAVITY-ACP-003: One launch command across every surface

**Intent:** Keep the session, runtime, and inference commands identical, so a
session and a capability probe cannot disagree about how the agent starts.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-003.1:** On windows the argument vector shall be
  exactly `agy_acp_server.exe` with no arguments; on darwin and on every platform
  other than linux and windows, exactly `agy_acp_server.par` with no arguments.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.2:** On linux, the argument vector shall be
  exactly `agy_acp_server.par --uid=`, matching the published registry entry for
  both Linux platforms. Registry-sourced, not measured.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.3:** The platform selecting among
  AC-AGENTS-ANTIGRAVITY-ACP-002.1, AC-AGENTS-ANTIGRAVITY-ACP-003.1 and
  AC-AGENTS-ANTIGRAVITY-ACP-003.2 shall be that of the Kandev process building
  the command — the host, not an executor's target; a remote executor on another
  platform is a named exclusion below. Those three shall be total: a defined
  lookup name and a non-empty vector on every platform. Availability is decided
  by discovery alone, so no platform needs a special case here: anywhere else
  AC-AGENTS-ANTIGRAVITY-ACP-002.2 governs the outcome.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.4:** Command construction shall be
  deterministic: repeated calls on one platform shall produce equal vectors,
  consulting neither PATH, the filesystem, nor the environment.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.5:** The session, runtime, and one-shot inference
  commands shall produce the same argument vector for the same platform.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.6:** The argument vector shall not vary with
  the selected model, permission policy, auto-approve setting, agent type, or
  resume target.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.7:** This agent's own command construction
  shall not add `--debug` or `--notices`.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.8:** The working directory shall be the task
  workspace.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.9:** The runtime shall declare an empty
  environment map and no stripped variables, so the agent inherits the execution
  environment unmodified.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.10:** Resource limits shall be 4096 MB memory,
  2.0 CPU cores, and a one-hour timeout, matching other built-in ACP agents.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.11:** The runtime shall not request an
  immediate process kill on shutdown.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.12:** One-shot inference shall be supported.
- **AC-AGENTS-ANTIGRAVITY-ACP-003.13:** AC-AGENTS-ANTIGRAVITY-ACP-003.1 through
  003.7 bind this agent's own command construction, not the final launched argv:
  the shared builder appends the profile's `cli_flags` and any sandbox command
  prefix to every agent afterwards, and this agent shall declare no filter for
  either.

### REQ-AGENTS-ANTIGRAVITY-ACP-004: Config root, sessions, and skills

**Intent:** Point Kandev at the directories the server uses, so a long task
survives a relaunch and Kandev never writes state it does not own.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-004.1:** The session directory template shall be
  `{home}/.gemini/antigravity-acp`.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.2:** Kandev shall not set, override, or read
  `GEMINI_HOME`. The declared template is the server's default config root, so
  it holds exactly when `GEMINI_HOME` is unset or empty in the execution
  environment.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.3:** The agent shall declare native session resume
  and that a session can be recovered.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.4:** Kandev shall introduce no per-launch
  variation in the config root: one static template, no `GEMINI_HOME` set or
  injected by Kandev, and no config-root flag. A relaunch therefore presents the
  server the root the creating launch saw whenever the execution environment is
  unchanged, which is the root the server scopes session lookup to.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.5:** The project skill directory shall be
  `.agents/skills`.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.6:** The user skill directory shall be
  `.gemini/config/skills`, the first entry of the server's global skill pair.
  The server searches both entries, so this is a determinism choice rather than
  a capability one, and it keeps Kandev out of the `agy` CLI's
  `antigravity-cli/` domain.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.7:** The template nests inside the Gemini
  agent's `{home}/.gemini`; the two shall remain distinct values, neither
  aliased, merged, nor special-cased.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.8:** During a session, Kandev shall not
  create, modify, or delete any file under the config root. Its only write there
  is the user-initiated remote-auth copy in AC-AGENTS-ANTIGRAVITY-ACP-005.7,
  which seeds a remote host before a session starts.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.9:** When `GEMINI_HOME` is set and non-empty,
  the server resolves a config root the declared template does not describe.
  Kandev shall not detect, read, or reconcile it; that host is a named exclusion
  below.
- **AC-AGENTS-ANTIGRAVITY-ACP-004.10:** The container session-directory target
  shall be empty, so no bind mount is created for the config root and the
  declared template has no runtime effect today, matching the other
  native-binary ACP agents.

### REQ-AGENTS-ANTIGRAVITY-ACP-005: Authentication owned by the ACP server

**Intent:** Surface the server's own authentication contract instead of
duplicating it, so an unauthenticated agent reads as "needs login", not "broken".

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-005.1:** Kandev shall present the authentication
  methods the server advertises in its `initialize` result rather than a
  hardcoded list.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.2:** Those methods shall be presented in the order
  the server returned them, never re-sorted or re-ranked.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.3:** This agent shall declare no auth
  classification of its own, relying on the shared ACP handling of a `-32000`
  from a capability probe or session start, which records an
  authentication-required state rather than a failed one. An unauthenticated
  probe therefore records that state, not a success carrying an empty model
  list.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.4:** This agent shall declare no stderr-derived
  failure rule: when the server answers with a well-formed JSON-RPC error on
  stdout, the traceback it also writes to stderr for that handled error is not an
  additional or fatal launch failure.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.5:** Kandev shall surface the message the server
  returns when an `authenticate` call fails because a required environment
  variable is absent from the launch environment.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.6:** Kandev shall not mint, refresh, or
  rewrite credentials; it only copies files the server already wrote per
  AC-AGENTS-ANTIGRAVITY-ACP-005.7 and passes user-supplied variables.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.7:** Remote authentication shall offer a
  file-copy method for darwin and linux carrying `settings.json`,
  `acp_token.json` and `acp_business_token.json`, each under
  `.gemini/antigravity-acp`, which is also the target directory.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.8:** The file-copy method shall not require every
  listed file to exist; which token file is present depends on the
  authentication method the user chose.
- **AC-AGENTS-ANTIGRAVITY-ACP-005.9:** Remote authentication shall also offer
  environment-variable methods for `GEMINI_API_KEY` and `GOOGLE_API_KEY`.

### REQ-AGENTS-ANTIGRAVITY-ACP-006: Capabilities read from the agent, not inferred

**Intent:** Derive behavior from what the server advertises, so an echoed
protocol version is never taken for a capability claim.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-006.1:** This agent shall add no
  `protocolVersion`-derived branch to the shared ACP capability path, which
  already reads the advertised `agentCapabilities` and `authMethods`; the echoed
  `protocolVersion` carries no capability information.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.2:** The agent shall not declare an assumed
  HTTP or SSE MCP override, because the server advertises both.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.3:** MCP servers shall be passed through ACP
  `session/new`, and the agent shall not declare a project-local MCP
  configuration strategy.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.4:** The agent shall not declare that it
  namespaces MCP tool names by server.
- **AC-AGENTS-ANTIGRAVITY-ACP-006.5:** Billing type shall be resolved by the
  shared default rule rather than pinned by this agent.

### REQ-AGENTS-ANTIGRAVITY-ACP-007: No automated provisioning

**Intent:** Contribute no install script, so a remote bootstrap never runs prose
as a shell command or pulls a third of a gigabyte per host. Kandev executes that
value verbatim as remote shell.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-007.1:** The install script shall be the empty
  string.
- **AC-AGENTS-ANTIGRAVITY-ACP-007.2:** The agent shall therefore contribute no
  text to remote bootstrap scripts on any executor, and requesting an install job
  shall fail with the existing empty-install-script error, running nothing.
- **AC-AGENTS-ANTIGRAVITY-ACP-007.3:** When an executor cannot find the agent on
  its PATH, Kandev shall surface whichever failure that executor already
  produces and add no new preflight: SSH's existing preflight names the agent
  and the missing binary, elsewhere the spawn failure names the binary alone.
  Neither shall fall back to another agent, command, or a download.

### REQ-AGENTS-ANTIGRAVITY-ACP-008: Concurrent sessions

**Intent:** State that concurrency in the config root belongs to the ACP server,
so two tasks run at once and no one adds a lock the server does not expect.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-ACP-008.1:** Two concurrent Kandev sessions using this
  agent shall each launch an independent server process.
- **AC-AGENTS-ANTIGRAVITY-ACP-008.2:** Kandev shall not serialize those processes
  and shall not lock, guard, or arbitrate access to the shared config root;
  conflicting writes there are the ACP server's responsibility.
- **AC-AGENTS-ANTIGRAVITY-ACP-008.3:** Terminating one session shall not terminate
  the other nor remove shared state under the config root.

## Out of scope

Each exclusion is a contract, not silence. The design's
[deferred work](../system-design/antigravity-acp-agent.md#deferred-work) records
what a follow-up would have to decide for the first three.

- **Automated download and installation of the ACP server, and therefore
  containerized, Kubernetes, SSH and Sprites provisioning.** Kandev has no
  managed non-npm download mechanism, no image ships this binary, and
  REQ-AGENTS-ANTIGRAVITY-ACP-007 contributes no install script, so those
  executors report it not found.
- **A remote executor whose platform differs from the Kandev host's.** Discovery
  reads the host's PATH and every command surface is static, so a darwin host
  driving a hand-provisioned linux remote ships the darwin vector and drops the
  linux `--uid=` of AC-AGENTS-ANTIGRAVITY-ACP-003.2. Command construction takes
  no per-executor platform input, so this layer neither detects nor corrects it.
  REQ-AGENTS-ANTIGRAVITY-ACP-005's remote-auth methods serve the same-platform
  remote, which is unaffected.
- **Any non-default `GEMINI_HOME`, whether ambient or set per task.** The server
  moves its whole config root while Kandev declares the default root and never
  reads the variable, so session-dir seeding, the
  AC-AGENTS-ANTIGRAVITY-ACP-005.7 copy and the AC-AGENTS-ANTIGRAVITY-ACP-004.6
  user skill directory all address directories the server is not using: resume,
  remote auth and user skills are undefined there, not merely degraded.
- **Application Default Credentials for `agent-platform` remote auth.** A
  single-variable remote-auth method cannot express `GOOGLE_CLOUD_PROJECT` plus
  `GOOGLE_CLOUD_LOCATION` with ADC, so AC-AGENTS-ANTIGRAVITY-ACP-005.9 offers
  only the two API-key methods and an ADC-only remote host has no auth path.
- **A passthrough or TUI agent for the `agy` CLI.** `agy` 1.1.25 has no ACP mode,
  is a different binary, and authenticates into `~/.gemini/antigravity-cli/`
  rather than the config root, so binding both to one agent would make the
  passthrough toggle silently switch credential domain and conversation store.
- **Model catalog curation.** Models come from the existing ACP probe, which
  needs authentication first. No static model list, no model flag.
- **Windows and Linux verification.** Only darwin-arm64 was executed, so the
  windows `.exe` name and the linux `--uid=` argument in
  AC-AGENTS-ANTIGRAVITY-ACP-002.1/003.1/003.2 are registry-sourced and
  unverified.
- **Adding a failure reason to the discovery result.** Distinguishing "installed
  but missing its harness" in user-facing text would change a discovery contract
  shared by every agent, so the partial install reports plain unavailable.
