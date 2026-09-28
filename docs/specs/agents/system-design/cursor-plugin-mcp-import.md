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

Selection is evidence-driven: enumerate eligible installed roots first, then read supported manifests under cache/local/marketplaces. Do not recursively activate every `mcp.json`. Descriptor paths must remain within the selected plugin root, including after symlink resolution. Use bounded reads and traversal; skip missing, malformed, unreadable, unsupported, or oversized candidates without dropping valid siblings. Task 00 records concrete bounds and fixture layouts.

Accept root MCP wrappers and inline descriptor maps, plus contained relative descriptor references. Distinguish absent `type` with command (stdio) from absent `type` with URL (network); reject conflicting shapes and invalid field types. Normalize internal `streamable_http` versus external `streamable-http` deliberately. Expand supported plugin-root variables without shell execution or general environment expansion. Unsupported `cwd` or required variables must not be silently discarded: the current shared `McpServer` has no `cwd`; such entries remain unsupported unless a separately reviewed contract change adds support.

Resolve collisions before emission: reserved `kandev` is runtime-only; otherwise explicit profile > user-owned project > global user > plugin. Plugin ties use descending modification time then ascending canonical source path. Validate entries before choosing a winner. An explicit profile name remains reserved even when policy filters that server, so imports cannot reactivate it.

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
