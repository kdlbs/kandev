# Cursor plugin MCP compatibility and ownership evidence

## Tested environment and binary

- **Cursor Agent version:** `2026.09.26-dd393fe`
- **Platform:** Darwin arm64 (macOS)
- **Protocol:** ACP v1 (`cursor-agent acp`) and CLI (`cursor-agent`)

## Observed behavior and evidence limits

Read-only inspection of the installed `2026.09.26-dd393fe` bundle on 2026-09-28 established:

- Native plugin identifiers use `plugin-<plugin-name>-<server-name>`; OAuth lookup indexes the credential object by that exact identifier.
- Native enabled-plugin discovery uses `GetEffectiveUserPlugins` and filters `isEnabled`. Cache presence alone is not enablement evidence. The repair removes the unverified `installed.json` / `plugins.json` registry fallback and selects only the exact immutable revision supplied by the native service.
- Credential storage uses the full normalized workspace slug. The truncated hashed directory is a worker socket path, not the credential location.
- MCP approval and disabled-server stores are separate from OAuth credentials. This repair leaves them untouched.

The reported task materialized bare `atlassian` while the shared credential key was `plugin-atlassian-atlassian`. It also imported cached Figma without enabled-state evidence. Isolated regression tests reproduce these mismatches without reading personal credentials.

Earlier claims that ACP registered all copied plugin tools lacked retained wire evidence and remain withdrawn. The following native smoke establishes local registration, credential lookup and tool calls in both modes; it does not establish Atlassian token validity or successful Jira access.

## Isolated native smoke (2026-09-28)

Cursor Agent `2026.09.26-dd393fe` ran with a disposable HOME and workspace at
`/private/tmp/kandev-cursor-native-smoke`. A local HTTP MCP fixture required a
synthetic bearer credential under the exact key `plugin-kandevfixture-fixture`.
The native project `mcp-auth.json` linked to a synthetic shared master, using
Cursor's full normalized workspace slug.

- `cursor-agent mcp enable plugin-kandevfixture-fixture` enabled only the fixture.
- `cursor-agent mcp list-tools plugin-kandevfixture-fixture` listed `fixture_ping`.
- `cursor-agent --trust acp` received ACP v1 initialize, session/new and
  session/prompt through a temporary JSON-RPC harness. It approved `allow_once`
  only for the exact fixture tool permission request.
- The local server observed authenticated `initialize`, `tools/list` and
  `tools/call`. ACP reported tool completion with `success: true`, and the
  assembled response returned `isolated-fixture-success`.
- Terminal `cursor-agent --trust -p --output-format json` also completed an
  authenticated `tools/call` and returned `isolated-fixture-success` with
  `is_error: false`. Its disposable `.cursor/cli.json` allowed only
  `Mcp(plugin-kandevfixture-fixture:fixture_ping)`.
- Before adding the fixture-only permission, the terminal prompt and stock
  `acpdbg prompt` rejected tool execution. Those exit statuses were not counted
  as success. No blanket tool or server approval was used.

Account initialization used the existing account access token in child memory
with `AGENT_CLI_CREDENTIAL_STORE=memory` and `CURSOR_AUTH_TOKEN`. No account token
was written to the fixture files; a final scan found zero persisted copies.
No personal MCP credentials, real Jira calls, live-task edits or blanket
approvals were used. Owned fixture servers and children stopped after the run.
Temporary harness and frame artifacts remain outside the repository; retained
repository evidence contains only commands, tool identifiers and outcomes.

## Lifecycle-backed recovery harness status (2026-09-28)

An opt-in Go harness now calls the actual lifecycle preparation path against a
local OAuth-capable HTTP MCP fixture, then uses Cursor TUI and ACP transports.
The recovery case starts with the fixture requiring authentication, records the
native `authentication_required` list-tools diagnostic, writes only a synthetic
fixture token into the isolated task auth file, and prepares again. It then stops
the first ACP child, starts a replacement child, loads the saved ACP session ID
without creating a new session, and calls `fixture_ping`.

The full opt-in harness passed on 2026-09-28 after correcting the fresh-auth
assertion. The terminal and ACP both called the local fixture. The recovery case
observed native authentication-required behavior, then restarted the ACP child,
loaded the same saved session ID without `session/new`, preserved its prior
conversation, and called the fixture successfully. See the canonical
[Agent MCP preparation compatibility evidence](../agent-mcp-preparation/compatibility-evidence.md)
for the exact command, test names, results, and the separation between native
smoke, backend tests, and mocked browser coverage.

The harness uses a temporary HOME and workspace, keeps the existing account
token in child memory only, injects synthetic fixture credentials, and scans the
temporary home before cleanup to reject account-token persistence. It performs
no real provider operation or personal MCP call.

## Supported manifest shapes

### 1. Standalone MCP files (`mcp.json` / `.mcp.json`)
```json
{
  "mcpServers": {
    "atlassian": {
      "type": "streamable-http",
      "url": "https://mcp.atlassian.com/v2/mcp"
    }
  }
}
```

### 2. Plugin descriptor manifests (`.cursor-plugin/plugin.json` / `plugin.json`)
```json
{
  "name": "harness",
  "mcpServers": {
    "figma": {
      "type": "http",
      "url": "https://mcp.figma.com/mcp"
    }
  }
}
```

### 3. Global user configuration (`~/.cursor/mcp.json`)
```json
{
  "mcpServers": {
    "custom-server": {
      "command": "node",
      "args": ["server.js"]
    }
  }
}
```

## Discovery, Policy, Precedence and Reconciliation Rules

1. **Eligible Discovery Roots & Selection**:
   - Native inventory: a bounded, read-only request to Cursor’s fixed account service returns enabled plugins. The observed macOS keychain reader is supported; unsupported credential stores omit native marketplace candidates.
   - Local plugins: `<cursorHome>/plugins/local/*/{mcp.json,.mcp.json,.cursor-plugin/plugin.json,plugin.json}`.
   - Cache: `<cursorHome>/plugins/cache/<marketplace>/<plugin>/<revision>` must match the enabled service entry exactly, including its pinned revision when present. Missing revisions, ambiguous identities and unsafe roots are skipped; no newest-cache fallback is used.
   - User global config: `<cursorHome>/mcp.json`.
   - Disable inheritance reads the native `projects/<full-workspace-slug>/mcp-disabled.json` for the task’s primary source repository and task workspace only. Unrelated credential-source workspaces cannot veto imports. Unavailable selected disable state omits automatic candidates.

2. **Descriptor & Manifest Support**:
   - Inline `mcpServers` object and relative file references (e.g. `"mcpServers": "config/servers.json"`).
   - Relative references are strictly contained within `pluginRoot` after symlink evaluation with 1MB bounded I/O.
   - Supported variables (`${CURSOR_PLUGIN_ROOT}`, `${PLUGIN_ROOT}`) are expanded in `command`, `args`, and `env` without shell evaluation.
   - Entries with unsupported execution context (such as required `cwd`) or unresolved variables `${...}` are skipped.

3. **Transport Validation**:
   - Validates mutually exclusive command (stdio) vs URL (network) shapes.
   - Normalizes network transports (`http`, `sse`, `streamable-http`).
   - Rejects conflicting or unknown transports without masking valid siblings.

4. **Executor Policy Enforcement**:
   - All imported candidate servers are resolved against the executor's `mcpconfig.Policy`.
   - Transport allow/deny, server allowlist/denylist, URL rewrites, and environment injection apply to imported servers.
   - Explicit profile server names (including any denied by policy) and `kandev` are strictly reserved, preventing plugins from resurrecting denied profile servers.

5. **Precedence Order**:
   `Internal kandev > Explicit Profile MCP > User-Owned Project Servers > Global User ~/.cursor/mcp.json > Eligible Plugin Manifests (deterministic candidate precedence)`

6. **Persistent Ownership & Reconciliation**:
   - Backend-private ownership state is tracked per workspace in `.cursor/.kandev-mcp-imports.json` recording imported server names and sha256 JSON fingerprints.
   - On each launch/preparation, previously imported servers whose fingerprints match (unmodified by user) are removed if they are no longer in the allowed import set (e.g. preference disabled or plugin deleted).
   - User-edited entries (fingerprint mismatch) and pre-existing user project servers are preserved as user-owned.
   - Writes to `.cursor/mcp.json` and `.cursor/.kandev-mcp-imports.json` are atomic with 0600 permissions. Symlinked or malformed project files are left untouched.
