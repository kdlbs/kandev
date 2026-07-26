# Core agent versions

Kandev tests these core npm-provided ACP runtimes with the following exact
package pins:

| Agent | Package | Tested version | ACP runtime selection |
| --- | --- | --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` | `0.62.0` | Exact package spec through `npx` |
| Codex | `@agentclientprotocol/codex-acp` | `1.1.7` | Exact package spec through `npx` |
| OpenCode | `opencode-ai` | `1.18.5` | Installed `opencode` binary |
| Copilot | `@github/copilot` | `1.0.75` | Exact package spec through `npx` |
| Gemini | `@google/gemini-cli` | `0.52.0` | Exact package spec through `npx` |

Claude and Codex use their exact package specs for normal sessions, container
commands, one-shot inference, and remote installation. The resolved spec is
part of `AgentCommand`, which existing lifecycle diagnostics log as
`agent_command`.

OpenCode intentionally uses the direct `opencode acp` command for discovery,
normal sessions, container commands, and one-shot inference. This keeps startup
offline-compatible and ensures discovery validates the executable actually
launched. Its installer remains pinned to `opencode-ai@1.18.5`, but discovery
accepts any `opencode` executable found on `PATH`; the ACP probe surfaces auth
or protocol-compatibility failures. The ACP initialize response separately
records the runtime-reported agent name and version.

Copilot always uses its exact package spec for ACP launch, even when a global
`copilot` binary is present on `PATH`, so sessions remain reproducible
regardless of auto-updates. Its runtime, inference, passthrough, and install
commands all use the same pinned spec. Gemini always uses its exact package
spec.

Cursor is intentionally not listed as pinned. Its supported installer selects
the current Cursor Agent build, and the CLI auto-updates by default without a
supported immutable version selector or CLI auto-update opt-out. Kandev keeps
using the official installer rather than claiming a version guarantee it
cannot enforce.

Update each agent independently inside the grouped version-update pull request:
confirm that the exact version exists in the configured npm registry, capture
only sanitized ACP wire fixtures when protocol evidence must change, and run
the agent command-surface and ACP dialect tests before changing the documented
tested version. For OpenCode, also install the candidate globally and confirm
`opencode --version` before capturing fixtures. Do not add prompts, file
contents, credentials, or other user data to protocol fixtures.
