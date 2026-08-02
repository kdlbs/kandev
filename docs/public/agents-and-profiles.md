---
title: "Agents and Profiles"
description: "Install agent CLIs and create profiles for models, modes, flags, secrets, permissions, passthrough, and MCP."
---

# Agents and Profiles

An **agent** is Kandev's integration with a coding-agent CLI. A **profile** is its reusable launch configuration. Create separate profiles when model, credentials, or permissions need different trust boundaries.

Agent authentication is separate from repository and integration credentials.

## Quick path

1. Rescan or install the agent on the host running Kandev.
2. Authenticate it as the same operating-system user.
3. Create a profile and verify model, mode, permissions, environment, and executor compatibility.
4. Use the advanced sections only when you need passthrough, MCP, or custom launch behavior.

## Install or detect an agent

Open **Settings > Agents** (`/settings/agents`). Kandev scans the host on which its backend runs, not the browser computer.

The production registry currently shows Auggie, Claude, Codex, Copilot, Gemini, OpenCode, Amp, Qwen, iFlow (beta), Droid, Kilocode, Pi, Cursor, Kimi, Kiro, Qoder, Trae, `omp`, Devin, Grok, and Hermes. An entry is usable only when its executable is supported on the current platform and available to the Kandev process. Development and E2E profiles can add mock agents that are not product integrations.

Hermes launches with `hermes acp`. Install the required `hermes` executable from its **Settings > Agents** card, which runs the official Hermes installer. Hermes currently supports task and workspace sessions. Office-assigned skill injection is not yet supported.

1. Select **Rescan** after installing or updating a CLI.
2. If the card offers an install action, review the command before running it. Installation runs on the Kandev host.
3. If the card reports that login is required, open its login terminal or authenticate the CLI as the same operating-system user that runs Kandev.
4. Open the seeded default profile and review it before selecting the agent in a workflow step or session. Discovery creates one default profile the first time it provisions an agent. If you deliberately delete every profile, later rescans do not recreate one; create a replacement manually.

The status shown on this page is authoritative for the current host. A CLI that works in your interactive shell can still be absent from Kandev when the service has a different `PATH`, home directory, or operating-system user.

### Update a managed agent runtime

The update icon is available on managed Claude, Codex, OpenCode, Copilot, and
Gemini agent cards. It updates the runtime on the Kandev host.

1. Select the update icon.
2. Review the current version, target version, and command.
3. Select **Approve update**. If the update fails, select **Retry update**.
4. Wait for the capability refresh to finish.

Kandev disables approval when the versions already match or the version check
is incomplete. A successful refresh updates the advertised models, modes,
configuration options, commands, and runtime version without a page reload.

- Active sessions keep running; later probes and launches use the refreshed runtime.
- Passthrough agents, authentication helpers, remote executors, and running containers are unchanged.
- If the update or refresh fails, Kandev keeps the previous capability catalogue and shows the error. Authenticate the host agent, then retry.

<details>
<summary>Add a custom terminal agent</summary>

### Add a custom terminal agent

Use **Settings > Agents > Add TUI Agent** for a CLI that Kandev does not register. Enter a display name, command, and optional model label. `{{model}}` in the command is replaced by the selected model value, then the entire command is split on whitespace with Go's `strings.Fields`.

That parser is not a shell and is not quote-aware: quotes and backslashes do not preserve a path or model containing spaces as one argument. Custom TUI agents always use terminal passthrough. They do not gain ACP features such as structured permission prompts, model discovery, modes, or session configuration merely by being added. Test the exact resulting argument split before assigning it to work.

</details>

## Create and configure a profile

Select an agent, create a profile, then open **Settings > Agents > _Agent_ > _Profile_**. The page shows the resolved command preview and only the settings supported by that agent.

| Setting | Runtime behavior |
|---|---|
| Name | Label shown in workflow, session, and automation selectors. |
| Model | Requested through ACP when the agent supports model selection. Leaving it unset uses the agent's default where the form allows that. |
| Mode | Requested with ACP `session/set_mode`. The choices come from the installed agent. |
| Configuration options | Dynamic ACP values requested with `session/set_config_option`. |
| CLI flags | Enabled entries are tokenized and appended to the ACP launch command. |
| Command prefix | Optional ACP-only launcher argv prepended to the command, for example `greywall --`. |
| Environment | Literal values or references to Kandev secrets, resolved when the process starts. |
| CLI passthrough | Uses the CLI's native terminal interface instead of a structured ACP conversation. |
| Auto-approve all permissions | Answers automatically: the first `allow_once`/`allow_always` option, otherwise the first option supplied by the agent; no options cancels. It is off by default. |
| MCP servers | Adds profile-specific external MCP servers when the agent supports MCP. |

Model, mode, command, and configuration choices are probed from the locally installed CLI and cached. The managed **Update agent** action refreshes them automatically; after other CLI changes, refresh the profile manually. Probe status can report **auth required**, **not installed**, **not configured**, or **failed**; a saved model name does not prove that the current provider account can use it.

### Monitor capability and subscription status

Use the profile refresh control after installing, authenticating, or upgrading an agent. A manual refresh updates both the advertised models, modes, and commands and the visible capability status, so an old failure banner does not remain authoritative after the local CLI recovers.

<details>
<summary>Office agent quota and provider usage</summary>

### Monitor Office agent quota

> [!EXPERIMENTAL]
> Office mode is feature-flagged and disabled in the production profile by
> default. Enable **Office mode** under **Settings > System > Feature Toggles**
> and restart Kandev to try it; its routes and agent surfaces are still in
> progress. For stable host-CLI installation and profile configuration, use
> **Settings > Agents**.

When [Office mode](feature-status.md) is enabled, open **Office > Agents** and
select an Office agent to see its **Subscription Quota** on the overview; the
Office dashboard also summarizes the highest utilization across subscription
agents. These cards appear only for supported subscription agents with provider
credentials.

For account-wide provider usage across supported providers, install the
[Provider Usage plugin](https://github.com/kdlbs/kandev-plugin-provider-usage).
It adds a provider pill to the session top bar and can add a compact display to
the global status bar. The optional status-bar display requires **App status
bar** to be enabled under **Settings > System > Feature Toggles**; enable it
and restart Kandev for the change to take effect. If it remains disabled, the
session top-bar pill remains available; enabling it also adds the global
status-bar and phone Status drawer display. Configure the plugin under
**Settings > Plugins > Provider Usage**. These usage surfaces are operational
signals, not a billing ledger or a guarantee that the next request will be
accepted; provider availability, account policy, and concurrent usage still
apply.

</details>

<details>
<summary>CLI flags and ACP command prefixes</summary>

### CLI flags

Each flag entry has a raw value, description, enabled state, and an agent-specific default where applicable. Only enabled entries reach the process. Kandev tokenizes each raw value as command arguments: `--add-dir /shared` becomes two arguments.

The field is not a shell script. Pipes, redirects, variable expansion, and command substitution do not run as shell syntax. Empty or malformed quoting is rejected. Keep separate profiles for materially different permission or workspace flags, and recheck customized flags after upgrading the CLI.

Some older profiles contain compatibility fields such as Auggie's `allow_indexing`; current launch behavior is represented by the active profile settings and flags.

### ACP command prefixes

**Command prefix** is available for ACP launches, not terminal-passthrough launches. Kandev parses the prefix into structured argv rather than running a shell: quote a path containing spaces, for example `"/opt/launchers/safe wrapper" --`. The resulting argv is prefixed to the agent command, so shell features such as pipes, redirects, variable expansion, and command substitution are not evaluated.

The prefix must contain a nonempty first argv element that is not flag-like. Malformed quotes, a trailing escape, an empty launcher, or a prefix beginning with `-` is rejected when you save or preview it. If an older persisted profile contains an invalid prefix, Kandev fails the launch rather than silently running the agent without its configured launcher. Check the command preview after changing a prefix.

</details>

## Environment variables and secrets

Create reusable secrets at **Settings > Secrets** (`/settings/general/secrets`), then select a secret reference in a profile environment entry. Secret names are 1–100 characters and values are 1–10,000 characters. Editing a secret with a blank value keeps the saved value.

Kandev encrypts secret values at rest with AES-256-GCM. The encryption key is `<KANDEV_HOME_DIR>/data/master.key` (by default `~/.kandev/data/master.key`) and is created with owner-only file permissions. `KANDEV_DATABASE_PATH` does not relocate this key. Protect and back it up with the Kandev database; losing it makes stored values unreadable. Anyone with access to the Secrets settings can reveal the plaintext.

Profile environment rules are:

- at most 100 entries;
- key length at most 256 characters and value length at most 8,192 characters;
- keys cannot contain `=` or a NUL character, and values cannot contain NUL;
- duplicate keys are rejected;
- `TASK_DESCRIPTION` and every `KANDEV_*` key are reserved;
- an entry must use either a literal value or a secret reference, never both.

Secret references are resolved at process launch. A deleted, missing, or unreadable secret causes that environment entry to be omitted; Kandev does not fall back to an old value. Empty resolved values are also omitted. Profile values fill missing environment keys but do not overwrite environment supplied by the executor or Kandev runtime.

Literal values remain in profile configuration. A secret reference avoids copying a token there, but the selected agent and its child processes still receive the plaintext at runtime. Use narrowly scoped credentials, and keep read-only review profiles separate from profiles allowed to publish, merge, deploy, or administer external systems.

## Permissions and unattended work

In a structured ACP session, the agent can present a permission request and its available responses. With **Auto-approve all permissions** disabled, a person chooses a response in the session. With it enabled, the runtime selects the first allow-once or allow-always response without waiting. If the agent supplies no allow response, Kandev selects its first response even when that response is not approval; with no responses, it cancels.

Auto approval can authorize shell commands, file changes, network calls, or any other capability exposed by that agent. Agent-specific flags that suppress permission prompts can be broader still. Use either only with a constrained executor, repository, environment, and credential set.

Workspace automation selectors do not offer passthrough agent profiles or Local executor profiles. **Run**-mode automations also cannot wait for a permission response: an unanswered request is rejected and the run fails. Use **Task** mode when a person must approve agent actions, or use a profile whose safe work does not prompt. See [Automation and MCP](automation-and-mcp.md).

## Structured ACP and terminal passthrough

ACP sessions can expose typed messages, tool updates, permission requests, models, modes, dynamic configuration, todos, usage, and resume metadata. Each capability depends on the agent's actual ACP implementation. ACP-only profile settings, including command prefixes and structured configuration, do not add those capabilities to a terminal-passthrough CLI.

Passthrough preserves the CLI's native PTY interface. It is useful when the native terminal has features that ACP does not expose, but Kandev cannot manufacture structured capabilities that are absent. Custom TUI profiles are locked to passthrough. Profile-specific MCP injection also varies by CLI; verify the command preview and the MCP section before depending on it.

> **MCP credential exposure:** MCP headers and environment values are stored in profile configuration. Codex may place them in process arguments, and Cursor or Pi may leave them in project files after teardown. Use short-lived, narrowly scoped credentials and review persisted files.

<details>
<summary>Configure external MCP servers</summary>

## Add external MCP servers to a profile

When an agent advertises MCP support, open its profile's **MCP** section. The editor accepts either a servers map or an object containing `mcpServers`.

Supported server types are `stdio`, `http`, `sse`, and `streamable_http`. If `type` is absent, a `command` implies `stdio` and a `url` implies `http`. Connection mode can be:

- `auto`: per-session for stdio and shared for a network transport;
- `per_session`: create a connection for each agent session;
- `shared`: reuse a network connection. Stdio cannot use shared mode.

The built-in task-aware server is injected separately. A profile server named `kandev` is ignored so it cannot replace the task server.

Executor policy can allow or deny transports or server names, rewrite URLs, and inject environment values. In the current launch wiring, profile MCP resolution starts from the standalone allow-all baseline for every executor and then overlays the selected executor profile's explicit MCP policy. A blank SSH, Sprites, or other remote policy therefore inherits allowed `stdio`, HTTP, SSE, and streamable HTTP transports; it does **not** receive the deny-all remote default defined elsewhere in the runtime. Set an explicit restrictive executor MCP policy before relying on remote isolation.

MCP JSON, including `headers` and server `env`, is stored as profile configuration. The raw editor has no secret-reference field for those values, so do not paste long-lived credentials into it unless access to the profile store is an acceptable boundary. Prefer a server that can inherit a narrowly scoped profile environment secret.

Passthrough injection adds CLI-specific exposure. Codex encodes MCP environment values and HTTP headers into `-c` process arguments, which another local user may read through process inspection. Cursor and Pi write project-local `.cursor/mcp.json` or `.pi/mcp.json`; when either file already exists, Kandev merges its entries and deliberately does not remove them at teardown because it does not own the user's file. Review and remove persisted entries or credentials yourself.

Kandev does not validate a server-name syntax centrally, so blank or unusual names can fail or be transformed differently by each CLI. A missing command/URL, unsupported or denied transport, or server-name policy denial skips that server with a warning. Configuring `shared` mode for a stdio server is different: it aborts profile MCP resolution with an error. Review launch/session logs when an expected tool is missing.

</details>

## Delete profiles and custom agents

Deleting a profile is irreversible. Kandev checks references before deletion:

- active sessions and watcher references are soft conflicts; force bypasses the active-session check and soft-deletes the profile, while disabling affected watchers is best effort rather than guaranteed cleanup;
- feature-flagged Office routing-tier references are hard conflicts and cannot be forced;
- Kandev attempts to clean every ephemeral task with a session using the profile, including matching Quick Chat, Configuration Chat, and Run-mode automation work. A cleanup failure is logged and does not prevent profile deletion, so audit leftover resources afterward.

Only custom TUI agents can be deleted from the agent list. Built-in definitions remain registered even when their CLI is not installed.

## Troubleshooting

- **Agent unavailable after install:** confirm the executable as Kandev's service user, select **Rescan**, and compare the service `PATH` with your shell.
- **Login required:** use the agent card's login terminal or sign in under Kandev's operating-system user; signing in as another user does not help the service.
- **Model, mode, or command probe fails:** authenticate first, refresh discovery, and choose a value advertised by the installed version.
- **Launch fails after editing flags:** inspect the command preview, remove stale arguments, and correct unmatched quotes or trailing escapes.
- **Environment value is absent:** confirm the secret still exists, the key is not reserved, and an executor/runtime variable is not already taking precedence.
- **MCP server is absent:** confirm agent MCP support, valid JSON, transport mode, executor policy, and the session warning logs.
- **MCP tools are missing from one agent session:** open the neutral plug button in that session's chat toolbar. **Delivered** means Kandev included the server in the agent launch but cannot yet observe a connection; **Connected** means the built-in server saw MCP initialize; **Active** means it served `tools/list`. A gray filtered/unavailable row explains an intentional omission, while red is reserved for an explicit sanitized error. Third-party profile servers normally remain Delivered because they connect directly to the agent and Kandev cannot inspect them. The report is scoped to that session and its current execution, so compare the affected agent's own toolbar rather than another agent on the task.
- **Automation cannot select a profile:** passthrough agent profiles and Local executor profiles are intentionally omitted from the automation selectors.

Related: [Executors](executors.md), [Automation and MCP](automation-and-mcp.md), and the contributor guide [Adding a new agent CLI](add-agent-cli.md).
