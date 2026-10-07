# Managed npm ACP runtimes

Kandev invokes these managed npm-provided ACP runtimes with the exact effective
version. The current default values are in the
[managed runtime catalogue](managed_npm_runtime_versions.json). An operator
selection takes precedence for the current default generation. It remains
effective until **Use Kandev default** clears it or a later shipped
package/default generation resets it during startup.

| Agent | Package | ACP arguments |
| --- | --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` | none |
| Codex | `@agentclientprotocol/codex-acp` | none |
| OpenCode v1 | `opencode-ai` | `acp --print-logs` |
| OpenCode v2 | `@opencode/cli` | `acp --print-logs` |
| Copilot | `@github/copilot` | `--acp` |
| Gemini | `@google/gemini-cli` | `--acp` |
| Pi | `pi-acp` | none |
| Muse | `@bex-co/muse-code-acp` | none |

Normal capability probes, sessions, container commands, and one-shot inference
use `npx --yes --prefer-offline --prefix ~/.kandev/managed-npm-runtime
package@<effective-version>` with the ACP arguments above. The prefix directory
is a canonical marker that the execution host replaces with a private,
user-scoped directory under its system temporary root before npm starts. It
stays outside the task workspace and mounted agent home. The agent process
still runs in the task workspace, but its project `.npmrc` does not control
managed runtime package resolution.
The `<effective-version>` placeholder resolves at launch to the exact Kandev
default or the exact operator selection. OpenCode's `--print-logs` flag lets
agentctl observe terminal provider diagnostics without reading OpenCode's private
log files. Its optional log-level argument is omitted because CLI versions accept
different value casing. The exact top-level
package is pinned, but npm transitive ranges, its cache, and the registry still
affect reproducibility. Kandev records the version reported by the ACP
initialize response instead of inferring it from source.

OpenCode stores the selected family and exact version in one install-wide
record. Fresh installations select managed v2. Existing v1 installations
remain on v1 until an operator selects the migration action in Settings. The
v2 command uses `@opencode/cli` even when a standalone `opencode` executable is
on the Kandev host `PATH`.

The lifecycle keeps the saved native session ID when it restores an OpenCode
session. It does not create a replacement session after an OpenCode restore
error. A v2 process can access the existing OpenCode session database, so the
runtime selector does not promise that a later v1 selection reverses changes
made by v2.

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
`package@<effective-version>` with online preference, then launches a fresh ACP
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
