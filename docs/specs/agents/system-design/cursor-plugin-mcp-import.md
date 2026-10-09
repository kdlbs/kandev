---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-002
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
---

# Cursor plugin MCP import system design

## Ownership and evidence

Agents owns discovery, profile preferences, and launch materialization. Executor locality is reused from the existing Cursor auth boundary. This extends [ADR 0014](../../../decisions/0014-passthrough-mcp-injection-strategies.md) without changing other CLI strategies or writing global configuration.

Review baseline: `adc5d67f55d19dd76161eaec5ae725e20a345acb`, plus the untracked design package, on 2026-09-28.
`CursorStrategy` is pure; `materializePassthroughFile` owns filesystem I/O. The existing writer merges generated entries over project entries and does not remove entries merged into user files. Neither blindly appending discoveries nor reusing that writer unchanged satisfies this package.

[Cursor's plugin reference](https://prod.cursor.com/docs/reference/plugins), checked 2026-09-28, documents inline or file-path `mcpServers` and plugin-root expansion. [Plugin management](https://prod.cursor.com/docs/plugins) distinguishes installation scope from marketplace availability. These docs do not establish an installed/enabled disk registry format or the behavior of the user's ACP binary.

Before production changes, Task 00 must record sanitized layout fixtures, installed/enabled/scope selection, a versioned ACP reproduction, and transport compatibility. Do not invent a registry path or treat all cached marketplace content as installed. If evidence is unavailable, leave that task blocked with the exact missing evidence.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-CURSOR-PLUGIN-MCP-001 | Discovery and policy; compatibility |
| REQ-AGENTS-CURSOR-PLUGIN-MCP-002 | Profile contract; settings surface |
| REQ-AGENTS-CURSOR-PLUGIN-MCP-003 | Launch routing; ownership and reconciliation |

## Discovery and policy

Use `internal/agent/mcpconfig/cursor_plugins.go` for filesystem discovery with an injected Cursor home and launch scope. Return candidates with source kind, canonical plugin root, source path, modification time, and parsed server values; flatten to `agentctltypes.McpServer` only after precedence and compatibility checks. Source metadata remains backend-only. A bare `[]McpServer` loses information needed for deterministic precedence and reconciliation.

Selection is evidence-driven: enumerate eligible installed roots first, then read supported manifests under local roots and exact inventory-selected cache roots. Do not recursively activate every `mcp.json`. Descriptor paths must remain within the selected plugin root, including after symlink resolution. Use bounded reads and traversal; skip missing, malformed, unreadable, unsupported, or oversized candidates without dropping valid siblings. Task 00 records concrete bounds and fixture layouts.

Accept root MCP wrappers and inline descriptor maps, plus contained relative descriptor references. Distinguish absent `type` with command (stdio) from absent `type` with URL (network); reject conflicting shapes and invalid field types. Normalize internal `streamable_http` versus external `streamable-http` deliberately. Expand supported plugin-root variables without shell execution or general environment expansion. Unsupported `cwd` or required variables must not be silently discarded: the current shared `McpServer` has no `cwd`; such entries remain unsupported unless a separately reviewed contract change adds support.

Resolve collisions before emission: reserved `kandev` is runtime-only; otherwise explicit profile > user-owned project > global user > plugin. Plugin ties use descending modification time then ascending canonical source path. Validate entries before choosing a winner. An explicit profile name remains reserved even when policy filters that server, so imports cannot reactivate it.

Materialization keeps the logical manifest name for policy and precedence, then emits the native `plugin-<plugin-name>-<server-name>` identifier for plugin candidates. Global and explicit definitions retain their names. Both logical and native names respect explicit profile reservations; project entries suppress an import under either name. Owned unchanged bare imports from older launches are removed through normal fingerprint reconciliation. The OAuth bridge continues to aggregate raw credential objects from eligible existing workspaces; no credential aliasing or approval-store mutation is needed for identity matching.

Cursor Agent `2026.09.26-dd393fe` obtains its native enabled-plugin inventory from `getEffectiveUserPlugins`, rather than a verified local installed registry. Kandev queries that inventory through the bounded client described below. Unavailable inventory skips cached marketplace roots while preserving independently eligible local/global definitions and credential sharing.

Apply the existing `mcpconfig.Resolve` transport/executor policy to imported candidates, including allow/deny rules, URL rewrites, and environment injection. Do not append imports after policy enforcement. Retain existing meaning of profile MCP config `Enabled`; the new preference independently controls imports, while executor policy still constrains them. Keep discovery failures best-effort; emit at most stable reason/count diagnostics without raw JSON, URLs, headers, environment values, or upstream error text.

## Compatibility

| Shape | Behavior | Evidence required |
| --- | --- | --- |
| Cursor ACP, local/worktree/local_pc | Import through ProjectMCPStrategy | Versioned reproduction and lifecycle test |
| Cursor-strategy terminal, same local classes | Same import and reconciliation | Fresh/resume/restart lifecycle tests |
| Other strategy, remote/container/unknown, different HOME | No host discovery or copying | Negative tests with inaccessible home |
| Supported stdio / HTTP / SSE / streamable HTTP | Preserve effective transport and supported fields | Parser and emitted-file fixtures plus real CLI smoke |
| Required cwd, unresolved variables, unknown transport | Skip entry, keep valid siblings | Negative fixtures |
| Unknown installation registry or scope | Do not import unproven roots | Task 00 evidence gate |

The current Cursor serializer omits network `type`. Task 00 establishes native accepted shapes; Task 03 must preserve effective transport rather than assume a URL-only entry preserves SSE. Do not promise every plugin authenticates: the independent [OAuth bridge](cursor-mcp-oauth-bridge.md) handles credential sharing.

## Profile contract

Persist `cursor_plugins_mcp_enabled INTEGER NOT NULL DEFAULT 1`; Go uses `CursorPluginsMCPEnabled bool`, wire DTOs use snake_case, browser state uses `cursorPluginsMcpEnabled`. Create/update DTOs use `*bool` to distinguish omission from false. Omitted create is true; omitted patch preserves the stored value.

Mirror the existing Cursor auth preference through schema initialization, migration and table rebuild copies, SELECT/INSERT/UPDATE/scan, duplicate, controller conversion, defaults, runtime `AgentProfileInfo` and `StoreProfileResolver`. Check all field-enumerating import/export, boot, action and event mappings. Resolve the execution profile rather than an Office stable identity. Missing profile resolution must not silently enable imports.

Update settings catalog and generated contracts via `go run ./cmd/settings-catalog`, then `--check`. Browser normalization, draft initialization, dirty detection, patch omission, conflict reconciliation, agent-level saves and creation defaults all preserve false. Unrelated edits omit an unchanged preference.

## Launch routing

Centralize eligibility and composition in a Cursor-only lifecycle helper used by `materializeRuntimeProjectMCP` and `applyPassthroughMCP`. Gate before filesystem access using value or pointer CursorStrategy, explicit local/worktree/legacy local_pc, and `sameCursorMCPAuthHome` semantics. Resolve profile state once per preparation and discover once; do not add discovery inside the generic `passthroughMCPServers` path for all providers.

Fresh ACP, ACP resume/restart, terminal initial/fresh/resume paths must all reach the helper before process launch. The no-agentctl-port ACP path keeps its current behavior and must not bypass stale-import reconciliation. The credential-sharing and import booleans are independent; test all four combinations.

## Ownership and reconciliation

Imported entries require ownership distinct from whole-file cleanup metadata. Task 00 must finalize the narrow Cursor ownership mechanism before Task 03. Its required contract is:

- Persist backend-private ownership across execution replacement/restart, keyed by canonical workspace and import generation, recording imported names and fingerprints. Do not put credentials or raw entries in browser-visible execution metadata.
- Since imports never overwrite user-owned project names, removal needs no backup of user secrets. Delete an entry only when it still equals the previous imported value. If edited, relinquish ownership and preserve it as project configuration.
- Before applying current candidates, reconcile previous unchanged imports, then choose current winners. Removed plugins and disabled preferences remove stale owned imports. Existing unrelated fields and entries survive.
- Serialize read/reconcile/write/ownership publication for the same workspace and fence cleanup with generation identity. Retain state long enough to recover partial writes and backend restart; document crash ordering in Task 00.
- Preserve leaf and parent symlink protections; use restrictive permissions and atomic replacement. Malformed or unsafe destinations stay untouched. If known stale imports cannot be safely removed, return a sanitized preparation error instead of launching with them enabled.
- Generic merge and whole-file cleanup must not bypass the Cursor ownership boundary or let an old execution delete a successor's file. Test both cleanup-before-successor and delayed-cleanup-after-successor.

This section is an implementation constraint, not a claim that existing cleanup already supports it. Task 00 must resolve storage, crash recovery, and shared-workspace concurrency with code evidence; production implementation remains gated until it does.

## Settings surface

Add a companion checkbox near `cursor-mcp-auth-preference.tsx`, shown for Cursor ACP and custom Cursor-strategy profiles using the existing capability predicate. Copy says it imports plugin and user MCP definitions on the next launch, applies only to matching local homes, and does not disable native Cursor global discovery or credential sharing.

Reuse the shipped profile settings page and its single scroll owner, shared draft/save/discard/conflict state, and inline preference row. An inline choice needs no new drawer. Phone labels wrap with a measured 44px active target and no horizontal overflow; desktop retains existing density. Existing navigation and safe-area handling remain the surface authority. Loading, failed save, and discard retain the existing form behavior.

Use `t()` and six language catalogs; generate Traditional Chinese through the existing generator and pseudo through the existing pseudo-locale process, not hand-authored pseudo strings. Task 04 owns public guide changes and rendered desktop/phone evidence.

## Implementation plans

- [Cursor plugin MCP import](../../../plans/cursor-plugin-mcp-import/plan.md)

## Native inventory and inherited disables repair

The [repair package](../../../plans/cursor-mcp-discovery-auth-repair/plan.md)
owns implementation of this extension. The existing import preference gates it;
OAuth credential sharing remains independent. Agents owns both boundaries.

### Observed service contract

Cursor Agent `2026.09.26-dd393fe` calls
`https://api2.cursor.sh/aiserver.v1.DashboardService/GetEffectiveUserPlugins`.
A read-only diagnostic confirmed a Connect JSON POST with
`{"excludeConfiguredVariables":true}`, `Content-Type: application/json`,
`Connect-Protocol-Version: 1`, and the existing Cursor account bearer token.
The response contains `plugins` and `marketplaces`; each effective entry has
`plugin`, `isEnabled`, and optional `pinnedGitRef`. Plugin metadata supplies
`name`, `gitRef`, and marketplace identity. This is a versioned private
compatibility contract, not a documented stable public API.

Use a small injectable native-inventory client, separate from the pure manifest
parser. Never scrape the interactive `agent mcp list` output or extract runtime
modules from minified bundles in production. Never infer enablement from OAuth
presence. Remove the fabricated `installed.json`/`plugins.json` registry
fallback for native marketplace eligibility.

Resolve the effective marketplace from embedded plugin metadata or marketplace
ID lookup. Require `isEnabled == true`, safe exact names, and an immutable
40-hex commit from `pinnedGitRef` before `plugin.gitRef`. Match the exact
`plugins/cache/<marketplace>/<plugin>/<revision>` regular contained root.
Do not choose newest mtime or another cached revision. Branch refs, release
archives, inline plugins, variables requiring secret resolution, missing roots
and ambiguous identities are unsupported until evidence extends the contract.
Do not download or install anything. Local explicit plugin directories and
global configuration retain their separate supported paths.

### Account authentication and failure boundary

The observed macOS keychain entry is service `cursor-access-token`, account
`cursor-user`. Read it through a platform credential-reader boundary, with
bounded execution and no shell interpolation. Only the token enters the
in-memory HTTP authorization header. No token, keychain output, provider error
body or raw inventory is logged or persisted. Do not use MCP provider tokens
for this service.

Initial service-backed support is the observed macOS keychain mode. Other
credential stores/platforms return a stable unsupported-source result until
their native credential contract is verified; do not borrow a token from a
different store, custom HOME or provider. Existing filesystem/global imports
and OAuth sharing retain their existing platform support.

Use a five-second total deadline covering credential read and HTTP request,
a two-MiB response bound, no redirect following, no retry/login/refresh,
strict required response-shape validation, and no persistent last-known
inventory fallback. The production origin is fixed; test clients inject a
transport, never a user-configurable credential destination. A successful empty
inventory is distinct from authentication failure, timeout or malformed input.
Emit sanitized reason/count diagnostics only. Do not perform service I/O while
holding the global project-file mutation lock. Fence per-workspace preparation
so an older completed inventory request cannot replace a newer preparation.

### Workspace disable inheritance

Resolve the primary source repository from trusted launch context
(`LaunchRequest.RepoSpecs()[0].RepositoryPath`, including the single-repository
compatibility field, and `MetadataKeyRepositoryPath` on resumed executions), never from agent messages, arbitrary cwd ancestors or whichever
workspace supplied credentials. Canonicalize that path and use
`DeriveCursorProjectSlug` to locate its Cursor `mcp-disabled.json`.
Also read the task workspace's own store. For multiple attached repositories,
the primary launch repository supplies inheritance; other repositories do not.
A repository-free task inherits no source-repository list. Launch preparation
writes `MetadataKeyRepositoryConfigured` from `RepoSpecs()` and overwrites any
task-supplied source-context keys. Ordinary attached folders do not establish
repository ownership. A configured repository with a missing primary path is
unavailable context, rather than a repository-free launch. Workspace promotion
projects a supplied primary repository before MCP preparation; when no repository
specification is supplied, it preserves the execution’s existing trusted source.
Terminal resume reads that persisted source context.

Read only regular non-symlink bounded JSON arrays of exact native identifiers.
Union the source-repository and task-local identifiers and reject matching
automatic plugin/global imports before composition and ownership publication.
Source/destination stores are read-only. An unrelated workspace's disable must
not affect the task, even if that workspace supplied the selected OAuth object.
Do not conflate a Git common directory with the explicitly selected repository
path; different source worktrees can have different Cursor preferences.

A missing disable file means no local veto. An existing unreadable, malformed,
symlinked or oversized selected store makes disable state unavailable: omit
automatic imports for that preparation and log a stable reason. Preserve
explicit profile/project configuration and internal Kandev tools. Missing
trusted source-repository metadata on a repository-backed resume is unavailable,
not repository-free; resolve it from the persisted launch context or skip
automatic imports. Canonicalization failure has the same behavior.
Re-enabling an import requires removing the source and task veto, when present.

### Credential and launch composition

Prepare the corrected whole-object OAuth snapshot and independently obtain
inventory/disable state before final plugin materialization. Preserve native
`plugin-<plugin>-<server>` names and logical-name precedence/policy behavior.
After filtering, reconcile stale owned imports; never remove user-edited
entries. Both ACP and terminal launch paths use the same preparation service.
No CLI blanket-approval flag, approval-file transplantation or direct disable-file
mutation is introduced. The [agent preparation design](agent-mcp-preparation.md)
extends this path with native, server-specific approval of final eligible imports,
authorized by the profile import preference and fenced by current disables.

Fixture success does not prove native tool registration. Before marking this
repair done, run the versioned CLI in a disposable workspace/home with a local
MCP fixture that requires a synthetic bearer token. Verify native server
registration and a harmless fixture tool call in both modes, plus Figma absence.
Separate native server approval is handled by the [agent preparation package](../../../plans/agent-mcp-preparation/plan.md).
It approves only final owned imports and preserves tool-call permissions.
