# Copilot profile plugins compatibility evidence

Date: 2026-10-28 (initial checks 2026-10-27)
Owner: work order 00
Status: reproduced local evidence plus an explicit authentication limitation

This record supports the implementation in
[plan.md](plan.md). It does not claim behavior that this machine could not
observe.

## Environment

- Repository baseline: `a6144c3cb8ef586b7e56f904fae9acaf38569113`
- Host: Linux x64, disposable fixture directories under `/tmp/opencode`.
- Runtimes: `@github/copilot@1.0.88` (repository pin) and
  `@github/copilot@1.0.104` (supplied observation candidate).
- No developer home, existing authentication store, or login was used.
- Inherited proxy and CA variables were preserved.

## Reproduced results

### Package and executable identity

Commands:

```bash
npm install --prefix /tmp/opencode/copilot-fixture/runtime \
  --no-audit --no-fund --package-lock=false --save-exact @github/copilot@1.0.88
node -p 'require(process.argv[1]).version' \
  /tmp/opencode/copilot-fixture/runtime/node_modules/@github/copilot/package.json
HOME=/tmp/opencode/copilot-fixture/home \
  COPILOT_HOME=/tmp/opencode/copilot-fixture/config \
  /tmp/opencode/copilot-fixture/runtime/node_modules/.bin/copilot --version
```

Result: package metadata and the binary both reported `1.0.88`. The same
procedure reported `1.0.104` for the second fixture.

### Inventory shape

| Runtime | Fixture state | Observed `plugin list --json` |
| --- | --- | --- |
| 1.0.88 | Empty disposable home | Flat array `[]` |
| 1.0.88 | External `--plugin-dir` fixture | Flat array row with `source: "external"` |
| 1.0.88 | Local directory marketplace install | Flat array row with `source: "live"` and `installedFrom: "<marketplace root>"` |
| 1.0.88 | Remote marketplace install | Flat array row with `source: "installed"` |
| 1.0.104 | Empty disposable home | Flat array with a built-in `browser-use` row, `source: "builtin"`, `enabled: false` |
| 1.0.104 | Local directory marketplace install | Flat array row with `source: "live"` and `installedFrom` |

The 1.0.88 empty-home result and 1.0.104 built-in result show that a true empty
inventory and an error/unavailable result must remain distinct. The parser
accepts the flat array and the documented legacy `{plugins, errors}` envelope;
a non-empty legacy errors array fails the snapshot.

### Installation metadata

For 1.0.88, a remote marketplace install created
`<COPILOT_HOME>/config.json` with leading `//` comments and an
`installedPlugins` entry containing `name`, `marketplace`, `version`,
`installed_at`, `cache_path`, `source_sha`, and `enabled`. The adapter reads
only `name`, `marketplace`, `version`, and `cache_path`.

For a local directory marketplace, `settings.json` holds
`extraKnownMarketplaces` and `enabledPlugins`, while `installedFrom` in the
inventory points at the marketplace directory. The adapter resolves the plugin
root from that marketplace manifest and its relative `source` path.

For 1.0.104, a local directory marketplace install created no `config.json`;
the live root remains derivable from `installedFrom` and `marketplace.json`.
A first remote `copilot-plugins` install failed because that marketplace is no
longer a default in 1.0.104. After `copilot plugin marketplace add
github/copilot-plugins`, the same remote install succeeded and wrote the same
`installedPlugins` / `cache_path` shape, plus an unrelated `firstLaunchAt`
field that the adapter ignores.

```bash
env HOME=<fixture>/home COPILOT_HOME=<fixture>/config \
  <fixture>/runtime/node_modules/.bin/copilot plugin marketplace add github/copilot-plugins
env HOME=<fixture>/home COPILOT_HOME=<fixture>/config \
  <fixture>/runtime/node_modules/.bin/copilot plugin install spark@copilot-plugins
``` This is an upstream default-marketplace change,
not a Kandev defect.

### Manifest and directory checks

Fixture plugin manifests at `.github/plugin/plugin.json` were read successfully.
The adapter also accepts root `plugin.json`, `.plugin/plugin.json`, and
`.claude-plugin/plugin.json` in the documented precedence. Root directories,
missing roots, unreadable files, manifest-name mismatches, and version
mismatches are classified as unavailable before a process starts.

### Configuration directory precedence

`copilot plugin list --config-dir=<dir> --json` and the split
`--config-dir <dir>` form were both accepted by 1.0.88. The CLI flag overrode
`COPILOT_HOME`. Repeated conflicting values resolved last-wins at the native
level; the adapter rejects a repeated, conflicting profile value instead of
silently choosing one.

### Repeated native mounts

Commands (abridged; the wrapper script execs the fixture binary):

```bash
env HOME=... COPILOT_HOME=... apps/backend/bin/acpdbg probe --timeout 60s \
  --exec "<fixture>/run-copilot.sh --acp --plugin-dir <plugin-a> --plugin-dir <plugin-b>" \
  --workdir <fixture>/work --out <fixture>/frames --stderr
```

Results:

- 1.0.88: ACP `initialize` succeeded with `agentInfo.version = 1.0.88` and
  `session/new` returned `Authentication required`. The repeated mount flags did
  not prevent startup.
- 1.0.104: ACP `initialize` succeeded with `agentInfo.version = 1.0.104` and
  `session/new` returned `Authentication required`.
- `--plugin-dir` external rows appeared in the inventory, including a path with
  a space when the CLI received it as one argument. The ACP probe wrapper itself
  split arguments on shell-like whitespace, so the native ACP run with the
  spaced path was not a valid repeated-mount check.
- `copilot mcp list` did not expose plugin-provided MCP servers in this
  unauthenticated fixture, so plugin MCP loading could not be observed.

### Native policy boundary

Official references checked on 2026-10-27 state:

- A `--plugin-dir` mount is read-only and has no persisted activation to
  change.
- Managed `enabledPlugins` and `extraKnownMarketplaces` values win for their
  entries; a local user cannot re-enable, disable, or repoint them.
- Plugin MCP servers follow native loading precedence, and explicit CLI MCP
  configuration has the highest precedence.

The adapter adds only read-only `--plugin-dir` mounts. It does not run
`plugin enable`, `plugin disable`, `plugin install`, or `plugin update` and does
not edit Copilot configuration. It therefore cannot grant a policy exception.

Limitation: this machine had no authorized Copilot authentication. It could not
observe an authenticated `session/new` veto for a managed-prohibited plugin.
That remains unverified. The qualified set below is deliberately narrow, and a
future runtime that bypasses a native veto must be removed from it.

## Inventory-qualified subset

| Capability | 1.0.88 | 1.0.103 | 1.0.104 |
| --- | --- | --- | --- |
| Flat inventory | Reproduced | Supplied only | Reproduced |
| Built-in rows | Not observed | Unknown | Reproduced |
| External rows | Reproduced | Supplied only | Not observed |
| Live root via `installedFrom` | Reproduced | Supplied only | Reproduced |
| Installed root via `cache_path` | Reproduced | Supplied only | Reproduced |
| Repeated ACP mounts | Reproduced (startup) | Supplied only | Reproduced (startup) |
| Authenticated plugin/MCP use | Not observed | Supplied only | Not observed |
| Native policy veto | Not observed | Not observed | Not observed |

Inventory qualification: `1.0.88` and `1.0.104`. `1.0.103` remains unqualified
because only supplied observations exist. Other versions return
`runtime_unsupported`; Kandev does not fall back to a global CLI.

Mount permission: none. `policyVerifiedVersions` is intentionally empty. Safe
inventory discovery remains available, but a nonempty profile start fails with
`policy_unverified` before any native inventory command or process start. This
is the explicit AC-AGENTS-COPILOT-PLUGINS-007.2 gate; it must not be removed
without authenticated evidence for the selected version and platform.

## Not exercised

- Authenticated plugin tool calls and native authentication reuse.
- Plugin-provided MCP servers, native MCP disables, and managed prohibitions
  under an authenticated session.
- Windows and macOS native paths.
- Remote/container execution. Nonempty selections are rejected there by design.
