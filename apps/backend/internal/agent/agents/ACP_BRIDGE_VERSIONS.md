# ACP bridge versions

Kandev tests these npm-provided ACP runtimes with the following package pins:

| Agent | Package | Tested version | ACP runtime selection |
| --- | --- | --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` | `0.61.0` | Exact package spec through `npx` |
| Codex | `@agentclientprotocol/codex-acp` | `1.1.5` | Exact package spec through `npx` |
| OpenCode | `opencode-ai` | `1.18.4` | Installed `opencode` binary |

Claude and Codex use their exact package specs for normal sessions, container
commands, one-shot inference, and remote installation. The resolved spec is
part of `AgentCommand`, which existing lifecycle diagnostics log as
`agent_command`.

OpenCode intentionally uses the direct `opencode acp` command for discovery,
normal sessions, container commands, and one-shot inference. This keeps startup
offline-compatible and ensures discovery validates the executable actually
launched. Its installer remains pinned to `opencode-ai@1.18.4`, and discovery
runs that executable with `--version` before reporting it as supported. Missing,
malformed, failing, or mismatched version output reports the agent as unavailable
with the pinned install command as remediation. The ACP initialize response
separately records the runtime-reported agent name and version.

To update a bridge, change one version constant at a time, confirm the exact
version exists in the configured npm registry, capture only sanitized ACP wire
fixtures, and run the agent command-surface and ACP dialect tests before
changing the documented tested version. For OpenCode, also install the candidate
globally and confirm `opencode --version` before capturing fixtures. Do not add
prompts, file contents, credentials, or other user data to protocol fixtures.
