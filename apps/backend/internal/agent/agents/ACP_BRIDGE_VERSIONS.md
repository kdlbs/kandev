# Managed npm ACP runtimes

Kandev invokes these managed npm-provided ACP runtimes with the exact effective
version. The default values below are the reviewed pins in
`managed_npm_runtime_versions.json` at this commit. An operator selection takes
precedence for the current default generation. It remains effective until
**Use Kandev default** clears it or a later shipped package/default generation
resets it during startup.

| Agent | Package | Default version | ACP arguments |
| --- | --- | --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` | `0.75.1` | none |
| Codex | `@agentclientprotocol/codex-acp` | `1.10.0` | none |
| OpenCode | `opencode-ai` | `1.18.29` | `acp --print-logs --log-level ERROR` |
| Copilot | `@github/copilot` | `1.0.83` | `--acp` |
| Gemini | `@google/gemini-cli` | `0.58.0` | `--acp` |
| Pi | `pi-acp` | `0.0.33` | none |

Normal capability probes, sessions, container commands, and one-shot inference
use `npx --yes --prefer-offline package@effective-version` with the ACP
arguments above. For example, an unmodified Claude installation currently
launches `npx --yes --prefer-offline @agentclientprotocol/claude-agent-acp@0.75.1`.
OpenCode's error-only log flags
are part of its managed command so agentctl can observe terminal provider
diagnostics without reading OpenCode's private log files. The exact top-level
package is pinned, but npm transitive ranges, its cache, and the registry still
affect reproducibility. Kandev records the version reported by the ACP
initialize response instead of inferring it from source.

If a managed startup or host capability probe reports the strict npm `ETARGET`
error for the selected exact package and version, Kandev makes one recovery
attempt. The colocated agentctl process resolves its own npm cache, removes only
that package's deterministic `_npx` execution tree, and retries the same command
with online metadata preference. Capability recovery publishes the successful
catalogue without changing persisted profile selections. Host repair uses the
failed probe's runtime environment and waits for concurrent host utility
processes before replacing the tree. Runtime recovery applies to standalone,
local Docker, and remote SSH executors. Sibling trees, the global npm cache,
the registry, and the selected version remain unchanged.

The **Update agent** action in Settings is the explicit freshness boundary for
the Kandev host. Its candidate preparation resolves the requested trusted
`package@effective-version` with online preference, then launches a fresh ACP
capability probe. Successful probes replace the advertised version, models,
modes, commands, and configuration options used for later launches.
Already-running sessions continue with their existing process. The normal
launch path remains offline-preferred; the update path is online-preferred so
it can refresh stale npm metadata.

At startup, Kandev records the package and default version as the agent's
default generation. If either value changes, it removes the prior operator
selection before runtime consumers start. An unchanged generation preserves
the selection. The reset affects future probes and launches only; an active
agent process continues to run with its current runtime.

ACP protocol negotiation and advertised capabilities are the compatibility
boundary. Kandev does not maintain an exact package-version allowlist or
silently roll back a runtime whose initialization fails. Package selection and
update commands come only from built-in agent metadata; callers cannot supply
package names, versions, registry URLs, or shell text.

Separately configured passthrough commands, native authentication helpers, and
native-only agents such as Cursor are outside this managed update path. The
install-wide effective version is included in commands built for remote
executors and new containers, but the Settings action does not prepare their
package cache. Each remote environment must resolve the exact package when it
launches.
