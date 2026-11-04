# Copilot profile plugins compatibility evidence

Date: 2026-10-28 (initial checks 2026-10-27)
Owner: work order 00
Status: diagnostic investigation and supported process preparation complete

This record supports the implementation in
[plan.md](plan.md). It does not claim behavior that this machine could not
observe.

The [completion package](../copilot-profile-plugin-completion/plan.md) extends
this same ledger rather than creating another support matrix.
[Completion task 03](../copilot-profile-plugin-completion/task-03-native-qualification.md)
records eight actual native attempts and limitations below.
The [accepted native-authority decision](../../decisions/2026-10-28-copilot-native-policy-authority.md)
removes privileged policy certification as a production prerequisite. Copilot
owns native policy/consent; supported inventory/metadata versions, verified
roots, and valid launch contexts remain required.

The original Linux evidence is preserved separately from the macOS follow-up.
An early missing-selector pause did not prove native incompatibility.
The user later chose no administrator/system-policy changes. A tempdir is not
a managed macOS policy source. Before completion WO-04, production code retained
the empty policy gate. WO-04 removed that gate and delivered supported native
process preparation. Native policy and component activation remain Copilot's
responsibility.

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
not edit Copilot configuration or request a policy exception. These
restrictions do not themselves certify native enterprise enforcement.

Limitation: this machine had no authorized Copilot authentication. It could not
observe an authenticated `session/new` veto for a managed-prohibited plugin.
That Linux case remains unverified. The adapter set below is deliberately
narrow. A demonstrated native incompatibility requires an explicit support
decision; absence of a privileged test is not such a demonstration.

## Inventory-qualified subset

This table preserves the original Linux results, not the later macOS
observations or the revised target admission contract.

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

At the original baseline, mount permission was empty: `policyVerifiedVersions`
rejected nonempty starts with `policy_unverified` before inventory/start.
That AC-AGENTS-COPILOT-PLUGINS-007.2 gate is superseded by 007.6 and 005.8.
Completion WO-04 removed it under native authority. Current preparation admits
valid local/worktree selections for `1.0.88` and `1.0.104`, while retaining
root, identity, executor, wrapper, and argument-conflict validation. Unknown
versions remain unsupported. Preparation eligibility is not native policy
approval or enterprise certification.

## Not exercised

Original Linux exclusions (see the subsequent macOS observations separately):

- Authenticated plugin tool calls and native authentication reuse.
- Plugin-provided MCP servers, native MCP disables, and managed prohibitions
  under an authenticated session.
- Windows and macOS native paths.
- Remote/container execution. Nonempty selections are rejected there by design.

## Completion observations: macOS arm64

The sections above preserve the original Linux fixture results. The completion
execution used `darwin/arm64` (macOS 26.7.1) against baseline
`fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58` plus the uncommitted follow-up package.
During those native diagnostic attempts, the managed pin and production mount
gate were unchanged. The later WO-04 implementation removed only the artificial
gate; the pin remains unchanged.

The first three attempts per candidate used this native command, substituting
the exact candidate and a private capture directory:

```bash
(cd apps/backend && env -u KANDEV_COPILOT_NATIVE_POLICY_FIXTURE KANDEV_COPILOT_NATIVE_ARTIFACT_DIR=<private-session-artifact-root> KANDEV_COPILOT_NATIVE_VERSION=<1.0.88-or-1.0.104> go test -trimpath -tags copilot_native -count=1 -timeout 10m ./internal/agent/copilotplugins -run '^TestCopilotPluginNativeAcceptance$' -v)
```

All six invocations exited 1, each running one top-level acceptance test with
no subtests. These are incomplete harness attempts, not successful qualification
or demonstrated native veto failures:

| Candidate | Observed progress | Limitation |
| --- | --- | --- |
| 1.0.88 | Exact package/binary identity and two plugin roots, including the special-character path | Initial capability/setup errors; final `session/new` rejected an omitted SSE `headers` field |
| 1.0.104 | Exact identity and two roots after removing unsupported `--no-remote`; all four plugin/native/ACP sentinels initialized and listed tools | Prompts ended without a tool call |

Inventory visibility alone does not satisfy the repeated native-use case.
Primary inspection found that the harness's `mcp__server__tool` allowlist did
not match the `server-tool` identifier in earlier successful native Copilot
traffic. The failed synthetic prompts reported unavailable tools. These six
initial attempts do not establish native incompatibility, authentication
failure, or enforcement of a prohibition.

One corrective invocation per candidate then used the same command with the
fixed selector, version-specific flags, and SSE `headers: []`. Both exited 1
with one top-level test and no subtests:

| Candidate | Actual corrective observation | Remaining limitation |
| --- | --- | --- |
| 1.0.104 | Each plugin-target, sibling, native-config, and ACP-injected sentinel received one valid `tools/call` and sent one successful result | No ACP tool-update events; endpoint evidence establishes coexistence, not managed enforcement |
| 1.0.104 disable case | Target had no initialize/list/call traffic; allowed sibling completed one call/result | No authoritative denial captured; do not claim certification |
| 1.0.88 | All four sentinels initialized, listed, and received endpoint calls | Then-current ACP-update correlation failed; paired disable was not reached, and the later evidence-assertion correction was not natively rerun |

Final offline verification passed: 10 top-level tagged tests and 20 subtests
with zero skips, ordinary provider tests, three focused race tests, native-test
discovery, seven-file Go lint with zero issues, and whitespace. The tool-selector
regression had a real assertion Red followed by Green. Exact commands and
counts are in completion WO-03. All eight owned runtime roots were removed;
subprocesses/listeners closed; raw captures remain private.

The managed-policy fixture issue is now concrete, rather than an unset-selector
assumption. Official deployment documentation requires forced macOS MDM
preferences, the regular root-owned
`/Library/Application Support/GitHubCopilot/managed-settings.json`, or
account-bound server-managed settings. Candidate help provides no per-run
managed-policy path override. No isolated macOS policy context was available;
Linux container results would not establish macOS managed enforcement. Existing MDM,
system policy, enterprise settings, credentials, and activation were not changed.

Primary sources:

- [Managed settings deployment](https://docs.github.com/en/copilot/how-tos/administer-copilot/manage-for-enterprise/use-managed-settings/deploy-managed-settings).
- [Managed settings schema, including `deniedMcpServers`](https://docs.github.com/en/copilot/reference/enterprise-administrators/enterprise-managed-settings).
- [ACP session setup](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v1/session-setup.mdx).

Managed policy was not run. The user explicitly chose native enforcement
without administrator rights or system-policy changes. Diagnostic delivery is
complete, not a fully passing enterprise qualification. WO-04 implemented
supported process preparation under native authority; missing enterprise
evidence is not a launch gate. Raw frames/help remain private; only synthetic
identifiers and sanitized observations are recorded here.

## Completion verification exception: macOS SSH teardown

WO-04's extended race command fails in the existing
`TestSSHPassthroughResetReplacesChangedWorkspaceMCP` cleanup, not native startup.
The error is `operation not permitted` from `manager.StopForTeardown` at
`manager_ssh_passthrough_test.go:58`. The same failure was reproduced using a
disposable archive of pristine baseline `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`,
without any follow-up source or tests:

```bash
(cd apps/backend && go test -json -trimpath -race -count=3 ./internal/agent/runtime/lifecycle -run '^TestSSHPassthroughResetReplacesChangedWorkspaceMCP$')
```

All three top-level runs failed; subtests produced eight passes and four
failures. This establishes a pre-existing macOS teardown failure, not a new
Copilot regression or an initial process-start denial. Do not describe the
extended race command as passing. Unrelated process-group cleanup was not
changed. The disposable baseline archive was removed and raw results remain
private.
