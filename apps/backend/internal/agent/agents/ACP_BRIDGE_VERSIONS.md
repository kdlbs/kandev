# Managed npm ACP runtimes

Kandev invokes these managed npm-provided ACP runtimes by unversioned package
name:

| Agent | Package | ACP arguments |
| --- | --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` | none |
| Codex | `@agentclientprotocol/codex-acp` | none |
| OpenCode | `opencode-ai` | `acp --print-logs --log-level ERROR` |
| Copilot | `@github/copilot` | `--acp` |
| Gemini | `@google/gemini-cli` | `--acp` |

Normal capability probes, sessions, container commands, and one-shot
inference use `npx --yes --prefer-offline` with the package name and arguments
above. OpenCode's error-only log flags are part of its managed command so
agentctl can observe terminal provider diagnostics without reading OpenCode's
private log files. This lets npm reuse its execution cache without making the cache a
durable installed-version guarantee. Kandev records the version reported by
the ACP initialize response instead of inferring it from source.

The **Update agent** action in Settings is the explicit freshness boundary for
the Kandev host. It resolves the upstream npm version, refreshes the
unversioned execution-cache entry with online preference, and then launches a
fresh ACP capability probe. Successful probes replace the advertised version,
models, modes, commands, and configuration options used for later launches.
Already-running sessions continue with their existing process.

ACP protocol negotiation and advertised capabilities are the compatibility
boundary. Kandev does not maintain an exact package-version allowlist or
silently roll back a runtime whose initialization fails. Package selection and
update commands come only from built-in agent metadata; callers cannot supply
package names, versions, registry URLs, or shell text.

Separately configured passthrough commands, native authentication helpers, and
native-only agents such as Cursor are outside this managed update path. Remote
executors and containers resolve their own unversioned runtime when they
launch; the host Settings action does not update those environments.
