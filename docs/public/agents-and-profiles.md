---
title: "Agents and Profiles"
description: "Configure agent CLIs, models, modes, flags, environment variables, passthrough, MCP, and permission behavior."
---

# Agents and Profiles

An **agent** identifies a CLI integration. An **agent profile** is the reusable launch policy for that integration. Profiles let one installed agent serve different repositories or trust levels without repeatedly entering models, flags, and environment configuration.

## Built-in and custom agents

Kandev discovers supported ACP-native CLIs and adapter commands, including Claude Code, Codex, GitHub Copilot, Gemini CLI, Amp, Auggie, OpenCode, Cursor, Devin, Qwen, Factory Droid, iFlow, Kilocode, Pi, Kimi, Kiro, Qoder, Trae, Oh My Pi, and Grok.

Availability depends on the current platform, executable/install command, and the agent's own authentication. The list in **Settings > Agents** is authoritative for the running Kandev version.

Custom TUI agents can run an arbitrary CLI command in a PTY even when they do not speak ACP. Configure the executable, arguments, display name, optional model/description, and whether Kandev waits for the terminal. TUI mode does not manufacture ACP capabilities that the CLI lacks.

## Profile fields

Agent profiles can configure:

| Field | Effect |
|---|---|
| Name | Human-readable profile selection label. |
| Model | ACP model identifier applied at session start when the agent advertises model selection. |
| Mode | Optional ACP session mode applied with `session/set_mode`. |
| Config options | Dynamic ACP session options applied with `session/set_config_option`. |
| CLI flags | Tokenized command arguments; only enabled entries reach the subprocess. |
| Environment variables | Plain values or references to named Kandev secrets injected into the agent process. |
| CLI passthrough | Opens the native TUI execution style instead of relying only on structured ACP chat. |
| Auto approve | Lets Kandev agentctl approve ACP permission requests automatically. |
| MCP policy | Controls which Kandev/external MCP servers are made available in the session. |

Some fields appear only when the agent advertises the related capability. A profile copied from another machine can reference a model or mode that the locally installed agent does not provide; inspect discovery/status before using it.

## CLI flags

Each flag entry stores a raw string, description, and enabled state. At launch, Kandev shell-tokenizes the raw string, so one entry such as `--add-dir /shared` becomes two arguments.

- Use separate profiles for materially different permission or workspace flags.
- Do not paste shell pipelines into a flag field; it is an argument list, not a shell startup script.
- Agent updates can add, rename, or remove flags. Recheck user-modified profiles after upgrading a CLI.
- `allow_indexing` remains a compatibility field for older Auggie profiles; current launch behavior is represented by CLI flags.

## Environment and secrets

Environment variables are visible to the agent subprocess and anything it launches. Prefer a named secret reference for tokens. Secret values are still available at runtime to that process; the reference avoids duplicating plaintext in profile configuration, not exposure to the selected agent.

Use the narrowest credential that completes the job. Separate read-only review profiles from write-capable implementation/release profiles where practical.

## Permission requests and auto approval

ACP agents can ask the client to approve tool use. With **Auto approve** disabled, Kandev displays permission requests in the session so a user can choose an offered response. With it enabled, agentctl automatically selects an allowed response.

Auto approval is high impact. It can permit shell commands, file changes, network calls, or other tools exposed by the agent. Combine it only with an executor, credentials, and repository scope that are safe for unattended work.

Agent-specific flags that bypass permissions can be broader than ACP auto approval and may prevent Kandev from showing a request at all. Treat both as explicit security policy.

## ACP versus passthrough

Structured ACP sessions provide capabilities such as model/mode selection, typed messages, permission requests, tool updates, todos, usage, and resume metadata when the agent supports them.

Passthrough preserves the agent's terminal UX. It is useful for CLIs whose native interface has features not exposed through ACP. Kandev can inject task MCP configuration for supported passthrough integrations, but the agent controls how it loads and uses that configuration.

## Usage and billing hints

Kandev can show subscription utilization for supported CLIs when local credential files expose that data. Profiles using API keys are represented separately. This is operational visibility, not a universal billing ledger; provider reports, reset windows, and availability vary.

## Troubleshooting

- **Agent unavailable:** verify the command under the same user and PATH as Kandev, then refresh discovery.
- **Login required:** use the profile's login/terminal flow or authenticate the CLI outside Kandev under the service user.
- **Model rejected:** choose an advertised model and verify the provider account has access.
- **Flag fails at startup:** run the resolved command manually and remove stale/custom arguments.
- **Environment missing remotely:** configure the agent variable on the agent profile and executor prerequisites on the executor profile; local shell exports do not automatically cross SSH/container boundaries.
- **MCP tool absent:** inspect agent MCP support, profile MCP policy, executor policy, and task mode.

Contributor guide: [Adding a new agent CLI](add-agent-cli.md).
