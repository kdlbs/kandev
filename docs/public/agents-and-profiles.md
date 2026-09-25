---
title: "Agents and Profiles"
description: "Install agent CLIs and create profiles for models, modes, flags, secrets, permissions, passthrough, and MCP."
---

# Agents and Profiles

An **agent** is Kandev's integration with a coding-agent CLI. A **profile** is its reusable launch configuration. Create separate profiles when model, credentials, or permissions need different trust boundaries.

The composer model picker shows the session's agent CLI logo beside the selected model and beside **Model** in the open popover. This identifies the CLI running the session, even when it supports models from several vendors.

Agent authentication is separate from repository and integration credentials.

## Quick path

1. Rescan or install the agent on the host running Kandev.
2. Authenticate it as the same operating-system user.
3. Create a profile and verify model, mode, permissions, environment, and executor compatibility.
4. Use the advanced sections only when you need passthrough, MCP, or custom launch behavior.

## Install or detect an agent

Open **Settings > Agents** (`/settings/agents`). Kandev scans the host on which its backend runs, not the browser computer.

![Settings > Agents showing detected agent CLIs, profiles, configured status, unavailable status, update indicators, and New profile controls.](../screenshots/settings-agents.png)

The production registry currently shows Auggie, Claude, Codex, Copilot, Gemini, OpenCode, Amp, Qwen, iFlow (beta), Droid, Kilocode, Pi, Cursor, Kimi, Kiro, Qoder, Trae, `omp`, Devin, Grok, Hermes, Goose, Muse, and Antigravity. An entry is usable only when its executable is supported on the current platform and available to the Kandev process. Development and E2E profiles can add mock agents that are not product integrations.

Hermes launches with `hermes acp`. Install the required `hermes` executable from its **Settings > Agents** card, which runs the official Hermes installer. Hermes currently supports task and workspace sessions. Office-assigned skill injection is not yet supported.

Goose launches with `goose acp`. Its **Settings > Agents** card runs only the official `download_cli.sh` installer. Homebrew (`block-goose-cli`) and pip (`pip install goose-ai`) are manual alternatives. Configure your model provider with `goose configure`. Goose currently supports task and workspace sessions.

Antigravity has no automated install: Google distributes `agy_acp_server.par` (`agy_acp_server.exe` on Windows) and its `localharness_external` or `localharness` sibling (`localharness_external.exe` or `localharness.exe` on Windows) as a signed archive through the [ACP registry](https://github.com/agentclientprotocol/registry/tree/main/antigravity-acp) rather than npm, so extract both files into one directory and add it to PATH yourself. Kandev fails discovery closed when the harness sibling is missing or not executable, so a partial extraction reports as not installed rather than as a broken session.

### Muse command surfaces

Muse Code has no native ACP server, so Kandev runs it through the community
[`@bex-co/muse-code-acp`](https://github.com/bex-co/muse-code-acp) adapter,
which is not affiliated with Meta:

- Structured ACP sessions and one-shot inference use the managed
  `@bex-co/muse-code-acp` package at the selected effective version. This
  needs Node.js 22 or later.
- The adapter drives the native `muse` executable through `muse serve`; CLI
  Passthrough starts `muse` directly.
- On POSIX hosts, the Muse install action runs the official
  `https://dev.meta.ai/install.sh` installer into the first writable directory
  on the backend's `PATH`. Windows has no automated Muse install action because
  the official installer is a POSIX shell script.

Kandev detects Muse when `muse` is on the `PATH` of the backend process and
responds to `--version`. Sign in with `muse login`, or set `META_API_KEY` for
headless and remote executors. Muse keeps sessions under
`~/.local/share/muse`, which Kandev uses for session resume.
Kandev removes `XDG_CONFIG_HOME` and `XDG_DATA_HOME` overrides from Muse
processes so credentials and sessions use these managed paths. Executors use an
isolated home directory for both `~/.config/muse` and `~/.local/share/muse`; the
real host home directory is not mounted.

The adapter starts sessions on the `model` in Muse's
`~/.config/muse/settings.json` and falls back to its own default otherwise,
while the interactive `muse` CLI falls back to Meta's catalogue default. To
pin one model for both, set it there, for example
`{"schema_version": 1, "model": "muse-spark-1.3"}`.

### Pi command surfaces

Pi uses separate executables for its two Kandev modes:

- Structured ACP sessions and one-shot inference use the managed `pi-acp`
  package at the selected effective version.
- CLI Passthrough starts the globally installed `pi` executable.
- The Pi install action runs `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`.

After installation, select **Rescan**. Kandev detects Pi when `pi` is on the
`PATH` of the backend process and responds to the non-interactive `--version`
check. The `pi-acp` adapter is not the interactive terminal CLI, and managed
ACP runtime selection does not replace the passthrough command.

1. Select **Rescan** after installing or updating a CLI.
2. If the card offers an install action, review the command before running it. Installation runs on the Kandev host.
3. If the card reports that login is required, open its login terminal or authenticate the CLI as the same operating-system user that runs Kandev.
4. Open the seeded default profile and review it before selecting the agent in a workflow step or session. Discovery creates one default profile the first time it provisions an agent. If you deliberately delete every profile, later rescans do not recreate one; create a replacement manually.

The status shown on this page is authoritative for the current host. A CLI that works in your interactive shell can still be absent from Kandev when the service has a different `PATH`, home directory, or operating-system user.

### Update a managed agent runtime

The update icon is available on managed Claude, Codex, OpenCode, Copilot,
Gemini, Pi, and Muse agent cards. It updates the runtime on the Kandev host.

Each managed runtime has a reviewed Kandev default. If you have not selected a
version, Kandev uses that exact default for probes, sessions, standalone
inference, containers, and SSH commands. A successful version update stores
your exact selection for this Kandev installation. The selection takes
precedence for the current default generation. **Use Kandev default** clears
it, and a later shipped package or reviewed default resets it during startup.
Kandev does not store the default as a user selection.

When a Kandev upgrade changes the managed package or its reviewed default,
Kandev removes the older selection during startup before the service becomes
ready. New probes and launches then use the reviewed default for that release.
On the first startup with this generation tracking, Kandev treats an existing
selection without a generation marker as legacy and resets it once.
When the package and default stay the same, Kandev preserves your selection
across restarts and unrelated upgrades. A process that is already running is
not replaced, so the new default applies when Kandev starts a future process.

After startup, open the update control to select any validated stable version,
including an older version. This lets you roll back the new default when a
provider or environment requires it. The selection remains active until you
change it or a later Kandev release changes that agent's package or default.

When the cached npm check finds a newer stable release, the update control has
a blue dot and its accessible label includes the effective and latest
versions. The dot is only a hint. Opening the control performs a fresh,
authoritative preview before any update starts. If the check is unavailable,
the control has no dot but remains usable.

1. Select the update icon.
2. Review the current version, active version, available stable versions, and command.
3. Keep the latest version selected for a normal update, or select an older stable version to roll back.
4. Select **Update runtime**, **Roll back runtime**, or **Repair runtime**.
5. Wait for the exact version to prepare and pass its ACP capability probe.

OpenCode has one additional host-runtime rule. When the `opencode` executable
is on the Kandev host `PATH`, the update installs the selected `opencode-ai`
package version globally and the follow-up capability probe runs that same
executable. Containers, SSH executors, and hosts without that executable keep
using the managed `npx` runtime. This keeps the update result aligned with the
runtime that Kandev will use for host utility calls.

Kandev enables the action only after the backend validates the selected version
against the trusted package catalogue. It does not accept package names, npm
tags, prereleases, registry URLs, or command text. When the active version,
observed version, and target version match, the action is disabled as **Up to
date**. A successful activation updates the advertised models, modes,
configuration options, commands, and runtime version without a page reload.

- The active exact version survives Kandev and browser restarts.
- Later host-local probes, utility calls, and standalone sessions use the active exact version.
- Active sessions keep running. They are not restarted or hot-swapped.
- Passthrough agents and authentication helpers remain unchanged. The Settings
  update job prepares the host runtime only; remote executor and new-container
  commands still use the effective exact version and must resolve that package
  in their own environment. Running sessions and containers are unchanged.
- If preparation, ACP validation, authentication, or persistence fails, Kandev keeps the previous active version and capability catalogue. Select another stable version or retry the same target.
- Kandev may prepare the exact version again if npm removes its cache entry. Kandev does not own an offline package inventory, and global npm cache cleanup is not required.

#### Recover a stale npm runtime lookup

Host-local managed runtimes normally start with npm's offline-preferred lookup.
If npm has stale package metadata and cannot resolve the selected
`package@version`, the first ACP startup can fail even though the configured
registry contains that exact version. Kandev recognizes this specific npm
resolution error, removes only the deterministic `_npx` execution tree for the
selected package and version, then retries the same command once with an
online-preferred metadata lookup.

Kandev also performs this recovery while it builds the host capability
catalogue used by agent profiles. A successful retry publishes the recovered
models and keeps the saved model, fallback model, mode, runtime version, and
enabled state unchanged. The profile remains selectable and does not show a
capability warning. Kandev reports a failed capability status if it cannot
prepare the retry or repair the cache, or if the one online retry fails.

The same recovery applies to managed runtime startup on a local PC, in a local
Docker executor, or in a remote SSH executor. Kandev sends the repair request
to the agentctl process that owns the failed execution. That process resolves
npm's cache with the agent environment and removes only the selected execution
tree. It does not repair the Kandev host cache, delete sibling execution trees,
change the registry, or clear the global npm cache.

The retry keeps the selected package, exact version, command prefix, model,
permissions, and session identity. It does not change the npm registry or
silently select another version. When the retry succeeds, no recovery card is
shown. When it fails again, Kandev and Office show one **Retry runtime** action
with collapsed technical details.

Managed `npx` runtimes use a Kandev-owned npm project directory, so an `.npmrc`
in the task repository does not change the managed runtime's package lookup.
The task repository remains the command's working directory. User and global npm
settings still apply, including `min-release-age` and `before` policies. If an
age policy blocks the selected version, Kandev does not repair the npm cache or
repeat the lookup online. Wait until the version meets the policy, or select an
older trusted version, then choose **Retry runtime**.

Do not use `npm cache clean --force` as the normal recovery step. It removes
unrelated npm data and does not target the stale execution tree. If the
specialized retry cannot resolve the runtime, check that the Kandev service
uses the expected npm installation and configured registry. Run `npm config get registry` as the Kandev service user to inspect the registry used by that process. Then use the runtime update controls to select and prepare another trusted stable version.

<details>
<summary>Add a custom terminal agent</summary>

### Add a custom terminal agent

Use **Settings > Agents > Add custom agent** for a CLI that Kandev does not register. Enter a display name, a protocol, and a command. The entire command is split on whitespace with Go's `strings.Fields`.

That parser is not a shell and is not quote-aware: quotes and backslashes do not preserve a path or model containing spaces as one argument. Test the exact resulting argument split before assigning it to work.

A **Terminal** agent runs as passthrough. It also takes an optional model label, and `{{model}}` in the command is replaced by that value. It gains no ACP features: no structured permission prompts, no model discovery, no modes, no session configuration.

An **ACP** agent is driven over the Agent Client Protocol instead, so it does get structured chat, tool calls, permission prompts, and probed models and modes. Enter the command that starts the CLI's ACP server, usually behind a flag such as `--acp`. Its model comes from the capability probe rather than the command, and its MCP servers travel in `session/new`, so neither the model label nor the MCP strategy is offered.

</details>

## Create and configure a profile

Select an agent, create a profile, then open **Settings > Agents > _Agent_ > _Profile_**. The page shows the resolved command preview and only the settings supported by that agent.

| Setting                      | Runtime behavior                                                                                                                                                 |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Name                         | Label shown in workflow, session, and automation selectors.                                                                                                      |
| Model                        | Requested through ACP when the agent supports model selection. Leaving it unset uses the agent's default where the form allows that.                             |
| Require exact model          | Per-profile opt-in. When enabled, Kandev stops before inference unless the executor advertises and accepts the saved model. It disables fallback controls without erasing their saved values. |
| Fallback settings            | Compatible profiles can use an advertised explicit fallback or automatic provider-default continuation. If the saved model is absent and exactly one bracketed variation is advertised, Kandev can use that variation with a warning. |
| Mode                         | Requested with ACP `session/set_mode`. The choices come from the installed agent.                                                                                |
| Configuration options        | Dynamic ACP values requested with `session/set_config_option`.                                                                                                   |
| CLI flags                    | Enabled entries are tokenized and appended to the ACP launch command.                                                                                            |
| Command prefix               | Optional ACP-only launcher argv prepended to the command, for example `greywall --`.                                                                             |
| Environment                  | Literal values or references to Kandev secrets, resolved when the process starts.                                                                                |
| Provider                     | Native uses the agent default. OpenAI-compatible sends ACP requests to the configured HTTP(S) router and can use a Kandev global API-key secret.                 |
| CLI passthrough              | Uses the CLI's native terminal interface instead of a structured ACP conversation.                                                                               |
| Enabled                      | Keeps the profile available to existing sessions and settings while hiding it from new task, session, handoff, and Quick Chat selectors.                         |
| Auto-approve all permissions | Answers automatically: the first `allow_once`/`allow_always` option, otherwise the first option supplied by the agent; no options cancels. It is off by default. |
| MCP servers                  | Adds profile-specific external MCP servers when the agent supports MCP.                                                                                          |
| Share local Cursor MCP credentials | Enabled by default for Cursor ACP and Cursor-strategy terminal profiles. It applies only to local executions that share the backend home directory. |

Agents can inspect and update declared profile settings through the compact
`search_settings_kandev`, `describe_setting_kandev`, `get_settings_kandev`,
and `update_settings_kandev` tools. Use the separate
`agent_profile_mcp` target for the profile MCP document. The settings tools
preserve profile validation and save replacement lists atomically. They never
return environment values or MCP credentials; use references or the existing
interactive credential flow when a secret is required.

Model, mode, command, and configuration choices are probed from the locally installed CLI and cached. The managed **Update agent** action refreshes them automatically; after other CLI changes, refresh the profile manually. Probe status can report **auth required**, **not installed**, **not configured**, or **failed**; a saved model name does not prove that the current provider account can use it.

### Use an OpenAI-compatible provider

Open the **Provider** section in a profile that supports this feature. Select
**OpenAI-compatible provider**, enter the absolute router base URL, and select
an optional global API-key secret. Enter the model ID that the router accepts.
The model field is free text because the router owns the model catalogue.

Kandev sends the provider URL and optional bearer key through the ACP gateway
authentication request when a new process starts. A running process keeps its
existing provider settings until it restarts. CLI passthrough cannot use this
provider because that path does not run the ACP authentication handshake.

Configuration options are resolved for the model selected in the profile. An
agent can therefore show a different option set for each model. If a model
change makes a saved option value unsupported, Kandev removes that value after
a successful resolution; a failed resolution keeps the draft unchanged so you
can retry it.

### Use a dynamic profile

> [!EXPERIMENTAL]
> Dynamic agent routing is disabled in production by default. Enable
> `features.dynamicAgentRouting` under **Settings > System > Feature Toggles**
> and restart Kandev before creating or using a dynamic profile.

Choose the **Dynamic** agent family when you create a profile. Add concrete
profiles in the order that Kandev must try them. The logical dynamic profile
stays selected in the task and utility binding, while each launch uses one
concrete candidate behind the scenes. Candidates must be enabled, launchable
profiles. Dynamic profiles and rich Office profiles cannot be candidates.

Use the **Dynamic agents** card at the top of **Settings > Agents** to create
and list dynamic profiles. The card is hidden, and dynamic profiles cannot be
selected for new work, when dynamic routing is disabled. A new profile starts
with its name and candidates; enable or disable it later from the profile
editor.

Each candidate has a policy for two shared provider-error classes:

- **Transient errors** include capacity, overload, network, timeout, rate, and
  provider-availability failures.
- **Hard errors** include authentication, subscription, quota, credit, model,
  and provider-configuration failures.

For each class, configure exponential same-candidate retries, the maximum
retry count, the initial interval, reset-date waiting with a maximum wait, and
the final **Skip candidate** or **Stop for manual recovery** outcome. Kandev
uses a trusted future reset date at most once for a candidate and class when it
fits the configured maximum. It then applies the retry schedule and outcome.
Unclassified, task, repository, permission, tool, and ambiguous mid-turn
failures stop for manual recovery so Kandev does not repeat work. The error
catalogue is versioned and can grow as provider signals become known; an
ambiguous new signal fails closed. A future classifier may improve catalogue
coverage, but no model is called to classify errors today.

After a provider switch, the failed provider is paused for the route health
backoff, or until its trusted reset time when one is available. Kandev runs an
exclusive health probe before it becomes eligible again. This shared error
classification is used by task/Kanban and Office routing, while the per-
candidate policies are configured on dynamic profiles.

When a task launch waits for session capacity, Kandev keeps the selected
destination and retries it automatically. Inspecting another session does not
change the workflow's selected step or primary session. You can open another
session and use its normal controls. Kandev resumes it when the session can run;
the automatic session ceiling still applies unless you explicitly start,
resume, or message the session.

Provider errors that occur before a result can use the configured action, such
as retrying the current candidate or trying the next candidate. A started turn
with an ambiguous result does not switch providers automatically. If no
candidate is eligible, the session waits for a recovery action. After the
current turn settles, use **Retry current agent** or **Try next agent** in the
session recovery surface. These actions use the current route generation, so a
stale browser action does not replace a newer route decision.

### Host probes and executor model catalogs

The model list shown while editing a profile comes from a host probe. It is an
editing hint, not a launch gate. A profile remains selectable when its saved
model is missing from that host list. Profile selectors show an amber warning
icon for this difference. On a desktop pointer, hover or focus the icon to
read its tooltip. On a touch device, select the icon to open its warning
drawer. Inspect the model list in profile settings for discovery details.
Authentication, installation, and probe-failure indicators remain visible on
profile selectors.

At task launch, the selected executor's ACP catalog is authoritative. Kandev
sends the requested model only when the executor advertises it. Profiles are
compatible by default, so an unavailable saved model can use an advertised
explicit fallback, one unique bracketed variation, or the executor's current
or default model. Kandev records one warning with the requested and effective
models when it continues with a different model. Automatic fallback allows
provider-default continuation and ignores the saved explicit fallback.

Enable **Require exact model** when the profile must keep the saved model. The
executor must advertise and accept that model before the first prompt. An empty
or unsupported catalog, an unavailable model, or a failed apply stops the
session before inference. Kandev never sends an unadvertised model and never
rewrites the saved profile model.

The host model list is only an editing hint. A missing host-probe model keeps a
profile selectable and shows an advisory warning; the executor catalog decides
the launch result. Upgrades and omitted API fields keep existing profiles
compatible, with strictness off until a user enables it. Optional portable
configuration can copy selected allowlisted files to a remote executor, but it
cannot guarantee equal host and executor model catalogs.

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
the global status surface. That surface is off by default and follows the
portable **Show status bar** preference under **Settings > Preferences >
Appearance > Status Bar**; saving applies without a restart. If it is off, the
session top-bar pill remains available. When it is on, the plugin can also
appear in the desktop/tablet bottom bar or phone Status drawer. Configure the plugin under
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

Claude CLI passthrough uses standard output by default. Add an enabled `--verbose` entry to the profile CLI flags when you need diagnostic output.

Some older profiles contain compatibility fields such as Auggie's `allow_indexing`; current launch behavior is represented by the active profile settings and flags.

### ACP command prefixes

**Command prefix** is available for ACP launches, not terminal-passthrough launches. Kandev parses the prefix into structured argv rather than running a shell: quote a path containing spaces, for example `"/opt/launchers/safe wrapper" --`. The resulting argv is prefixed to the agent command, so shell features such as pipes, redirects, variable expansion, and command substitution are not evaluated.

The prefix must contain a nonempty first argv element that is not flag-like. Malformed quotes, a trailing escape, an empty launcher, or a prefix beginning with `-` is rejected when you save or preview it. If an older persisted profile contains an invalid prefix, Kandev fails the launch rather than silently running the agent without its configured launcher. Check the command preview after changing a prefix.

</details>

## Environment variables and secrets

Create reusable secrets at **Settings > Secrets** (`/settings/general/secrets`), then select a secret reference in a profile environment entry. Secret names are 1–100 characters and values are 1–10,000 characters. Editing a secret with a blank value keeps the saved value.

Kandev has two secret scopes:

- **Global** secrets are available to the current user across their workspaces. When authentication is disabled, Global is install-global.
- **Workspace** secrets belong to one workspace and can be selected by that workspace's repositories. They are not available to shared agent or executor profiles.

The General page manages Global secrets. Manage Workspace secrets from **Settings > Workspaces > _workspace_ > Secrets**. Agent and executor profile selectors intentionally show Global secrets only; a Workspace reference saved through an older or direct API path is rejected when the profile is saved or launched.

Copy or move a secret between scopes from the **Copy/Move** action on any secret row. The dialog picks Copy or Move, chooses a destination (Global or another workspace), and lets you edit the target name; it is pre-filled as `<name> (from Global)` or `<name> (from <workspace name>)` so copied secrets keep their origin visible. Moving removes the original after the copy is safely in place, and the value is transferred server-side between encrypted rows. It is never shown or copied through the browser. A target name that already exists in the destination blocks the action until you rename it.

Kandev encrypts secret values at rest with AES-256-GCM. The encryption key is `<KANDEV_HOME_DIR>/data/master.key` (by default `~/.kandev/data/master.key`) and is created with owner-only file permissions. `KANDEV_DATABASE_PATH` does not relocate this key. Protect and back it up with the Kandev database; losing it makes stored values unreadable. Anyone with access to the Secrets settings can reveal the plaintext.

Profile environment rules are:

- at most 100 entries;
- key length at most 256 characters and value length at most 8,192 characters;
- keys cannot contain `=` or a NUL character, and values cannot contain NUL;
- duplicate keys are rejected;
- `TASK_DESCRIPTION` and every `KANDEV_*` key are reserved;
- an entry must use either a literal value or a secret reference, never both.

Kandev resolves secret references at process launch and cold resume. A deleted, missing, or unreadable secret blocks launch before the agent starts. The error identifies the environment key and its source.

Secret deletion is blocked while an agent profile, executor profile, or repository environment references the secret. When you select Delete, Kandev checks first. If the secret is in use, a dialog lists the affected resources and does not offer a delete action. Remove or replace those references before deleting the secret.

If a reference is already broken, open the named profile and select the replacement secret for the affected key, save, and retry. For a visible repository reference, open that repository's environment settings and replace the binding. A redacted `repository` reference means the repository is in a workspace that you cannot access; a workspace user with edit permission must locate and replace the binding in that workspace's repository settings. If no such user exists, ask a workspace owner or administrator to grant access or repair the binding. Creating a secret with the same name does not repair the reference because each secret has a separate ID.

API clients can explicitly force deletion. This leaves broken references and blocks future launches until those references are repaired. See the [secret deletion API contract](websocket-api.md#settings-secrets-and-automations).

Repositories can bind an environment key to a Global secret or to a Workspace secret from the same workspace under **Settings > Workspaces > _workspace_ > Repositories**. A task inherits bindings from every attached repository. Repository bindings are secret references, never values, and a repository binding to a deleted or unreadable secret blocks that task's launch. If two sources provide the same key, Kandev deduplicates an identical secret reference and rejects every other collision; repository order never chooses a winner.

Literal values remain in profile configuration. A secret reference avoids copying a token there, but the selected agent and its child processes still receive the plaintext at runtime. Use narrowly scoped credentials, and keep read-only review profiles separate from profiles allowed to publish, merge, deploy, or administer external systems.

## Permissions and unattended work

In a structured ACP session, the agent can present a permission request and its available responses. With **Auto-approve all permissions** disabled, a person chooses a response in the session. With it enabled, the runtime selects the first allow-once or allow-always response without waiting. If the agent supplies no allow response, Kandev selects its first response even when that response is not approval; with no responses, it cancels.

Claude ACP also has a narrower built-in rule. When the agent uses the host-injected `kandev` MCP server, Kandev automatically selects an offered allow-once response, or allow-always when allow-once is not offered, even when **Auto-approve all permissions** is disabled. This covers every tool on that server, including task creation, task deletion, and plan changes. It is not a saved approval decision and is recreated for each task or Quick Chat session. Kandev still enforces MCP authentication, task and session authorization, user-question barriers, and workflow gates. Shell commands, file operations, third-party tools, and ambiguous requests keep the normal permission flow.

An external MCP client can also list live pending requests for an authorized task and submit one
exact option originally offered by the agent. The request-generation ID prevents an old approval
from targeting a replacement prompt that reused a provider pending ID. Kandev records who selected
which option before delivery and rejects concurrent or replayed answers. It does not expose hidden
environment values, headers, raw MCP arguments, or option metadata. See
[Resolve a live agent permission request](automation-and-mcp.md#resolve-a-live-agent-permission-request).

Auto approval can authorize shell commands, file changes, network calls, or any other capability exposed by that agent. Agent-specific flags that suppress permission prompts can be broader still. Use either only with a constrained executor, repository, environment, and credential set.

Workspace automation selectors do not offer passthrough agent profiles. Local executor profiles are available for the repository-free target; Worktree requires a repository. Hidden automation sessions receive a fixed workspace-scoped coordinator MCP surface. Visible normal-task automations use the ordinary task profile and MCP surface. The trusted automation principal is resolved before hidden-run dispatch, and a hidden automation task and its sessions cannot be used as mutation, messaging, stopping, spawning, or blocker targets. Cross-task spawning uses the target task's normal profile. Native provider continuation and compaction remain authoritative for a healthy reusable session; Kandev's fallback resume prompt uses only the newest 50 non-empty user or assistant messages and excludes tool events. See [Automation and MCP](automation-and-mcp.md).

## Structured ACP and terminal passthrough

ACP sessions can expose typed messages, tool updates, permission requests, models, modes, dynamic configuration, todos, usage, and resume metadata. Each capability depends on the agent's actual ACP implementation. ACP-only profile settings, including command prefixes and structured configuration, do not add those capabilities to a terminal-passthrough CLI.

Terminal custom profiles use passthrough and preserve the CLI's native PTY interface. This is useful when the native terminal has features that ACP does not expose, but Kandev cannot manufacture structured capabilities that are absent. Custom ACP profiles use structured ACP sessions. Profile-specific MCP injection also varies by CLI; verify the command preview and the MCP section before depending on it.

> **MCP credential exposure:** MCP headers and environment values are stored in profile configuration. Codex may place them in process arguments, and Cursor or Pi may leave them in project files after teardown. Use short-lived, narrowly scoped credentials and review persisted files.

### Share local Cursor MCP credentials

The **Share local Cursor MCP credentials** profile option is enabled by default for Cursor ACP and custom terminal profiles that use Cursor's MCP strategy. It applies only to local and worktree executions that use the same home directory as the Kandev backend. Remote and container executors, and profiles that set a different `HOME`, do not share the backend user's credentials.

On launch, Kandev combines valid `mcp-auth.json` files from other local Cursor projects into `~/.cursor/kandev-mcp-auth-unified.json`. For duplicate server names, the newest source file wins. Equal timestamps use project path order. Kandev links the current project's auth path to this private shared file. It preserves an existing regular `mcp-auth.json` file. While sharing is enabled, it replaces an existing symlink at that path when it points elsewhere. Disabling sharing does not restore the replaced target. On a disabled launch, Kandev removes only its own project auth link and leaves unrelated symlinks unchanged.

The shared auth file is keyed by MCP server name. Kandev does not compare server URLs or OAuth issuers between projects. A project with a matching server name can therefore use a copied credential even when its MCP configuration points to a different endpoint. Enable sharing only for trusted local project configurations. If a credential may have reached an unintended endpoint, revoke it through that provider.

To stop sharing, clear the option and launch that profile again. That launch removes only the Kandev-created link for its project. Running processes keep credentials they already loaded. Cursor may refresh credentials through the link, but Kandev does not copy refreshed credentials back to source projects. A later launch rebuilds the shared file from those source projects.

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
- enabled automations bound to the profile are disabled **before** it is deleted, and a failure to disable them aborts the deletion. Unlike watchers, an automation has no preflight that would notice a profile had vanished, so it would otherwise keep firing at a profile that no longer exists;
- feature-flagged Office routing-tier references are hard conflicts and cannot be forced;
- Kandev attempts to clean every ephemeral task with a session using the profile, including matching Quick Chat and Configuration Chat. A cleanup failure is logged and does not prevent profile deletion, so audit leftover resources afterward.

Only custom TUI agents can be deleted from the agent list. Built-in definitions remain registered even when their CLI is not installed.

## Troubleshooting

- **Agent unavailable after install:** confirm the executable as Kandev's service user, select **Rescan**, and compare the service `PATH` with your shell.
- **Login required:** use the agent card's login terminal or sign in under Kandev's operating-system user; signing in as another user does not help the service.
- **Model, mode, or command probe fails:** authenticate first, refresh discovery, and choose a value advertised by the installed version.
- **Launch fails after editing flags:** inspect the command preview, remove stale arguments, and correct unmatched quotes or trailing escapes.
- **Managed npm runtime cannot resolve its selected version:** Kandev checks the configured registry and retries the same version once after refreshing its exact `_npx` execution tree. If the retry fails, verify the service user's npm configuration and registry, then use **Settings > Agents** to prepare another trusted stable version. Do not start with `npm cache clean --force`.
- **Environment value is absent:** confirm the secret still exists, the key is not reserved, and an executor/runtime variable is not already taking precedence.
- **MCP server is absent:** confirm agent MCP support, valid JSON, transport mode, executor policy, and the session warning logs.
- **MCP tools are missing from one agent session:** open **MCP servers** in that
  session's chat toolbar. Select `kandev`, then find the tool in the scrollable
  list. Select the tool to inspect its description, token estimate, and
  arguments. Use **Back to tools**, then **Back to servers** on touch devices.
  **Connected** means the built-in server completed MCP initialize. **Active**
  means it served `tools/list`. A gray row explains an intentional omission.
  Red identifies an explicit sanitized error. **Delivered, connection
  unverified** applies to a profile server that connects directly to the agent.
  Kandev cannot inspect that server's tools. Inspect the affected session
  because each report belongs to one session and execution.
- **Automation cannot select a profile:** passthrough agent profiles and Local executor profiles are intentionally omitted from the automation selectors.

Related: [Executors](executors.md), [Automation and MCP](automation-and-mcp.md), and the contributor guide [Adding a new agent CLI](add-agent-cli.md).
