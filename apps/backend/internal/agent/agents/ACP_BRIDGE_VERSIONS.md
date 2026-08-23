# Managed npm ACP runtimes

Kandev invokes these managed npm-provided ACP runtimes with the exact effective
version. The default values below are the reviewed pins in
`managed_npm_runtime_versions.json` at this commit. An operator selection
replaces the default for that installation until **Use Kandev default** clears
it.

| Agent | Package | Default version | ACP arguments |
| --- | --- | --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` | `0.70.0` | none |
| Codex | `@agentclientprotocol/codex-acp` | `1.6.0` | none |
| OpenCode | `opencode-ai` | `1.18.18` | `acp --print-logs --log-level ERROR` |
| Copilot | `@github/copilot` | `1.0.75` | `--acp` |
| Gemini | `@google/gemini-cli` | `0.52.0` | `--acp` |

Normal capability probes, sessions, container commands, and one-shot inference
use `npx --yes --prefer-offline package@effective-version` with the ACP
arguments above. For example, an unmodified Claude installation currently
launches `npx --yes --prefer-offline @agentclientprotocol/claude-agent-acp@0.70.0`.
OpenCode's error-only log flags
are part of its managed command so agentctl can observe terminal provider
diagnostics without reading OpenCode's private log files. The exact top-level
package is pinned, but npm transitive ranges, its cache, and the registry still
affect reproducibility. Kandev records the version reported by the ACP
initialize response instead of inferring it from source.

The **Update agent** action in Settings is the explicit freshness boundary for
the Kandev host. Its candidate preparation resolves the requested trusted
`package@effective-version` with online preference, then launches a fresh ACP
capability probe. Successful probes replace the advertised version, models,
modes, commands, and configuration options used for later launches.
Already-running sessions continue with their existing process. The normal
launch path remains offline-preferred; the update path is online-preferred so
it can refresh stale npm metadata.

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
