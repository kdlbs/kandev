---
status: current
system: agents
created: 2026-10-27
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
---

# Copilot profile plugins system design

## Purpose and boundaries

Agents owns concrete profile selection and native process preparation.
This design includes storage, settings contracts, discovery, runtime loading, and responsive settings UI.
Discovery, profile state, settings, and supported process loading are implemented.
The artificial `policy_unverified` gate was removed under the user's
[native-authority decision](../../../decisions/2026-10-28-copilot-native-policy-authority.md).
Enterprise-policy evidence is diagnostic, not launch admission. See the
[compatibility evidence](../../../plans/copilot-profile-plugins/compatibility-evidence.md).
The [requirements](../requirements/copilot-profile-plugins.md) distinguish confirmed intent from recommendations.

Cursor supplies the selection/state reference, not extraction or credential bridging.
ACP `session/new` MCP injection remains intact.

Related contracts:

- [Profile capability discovery](profile-capability-discovery.md): authorized saved/draft contexts and cache fencing.
- [Host CLI discovery](host-cli-model-discovery.md): discovery does not replace the selected session runtime.
- [Settings parity](../../platform/requirements/agent-settings-parity.md): owned fields and mutation authority.
- [Portable configuration](../requirements/portable-agent-configuration.md): no implied plugin or credential portability.
- [Context reset decision](../../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md): retained sessions and replacement processes differ.
- [Native MCP preparation decision](../../../decisions/2026-09-28-profile-authorized-mcp-preparation.md): native policy remains authoritative, without adopting Cursor preparation here.
- [Native authority](copilot-profile-plugins-native-qualification.md): version-bounded preparation, diagnostic evidence, and failure recovery.

The linked native-authority ADR records the policy-ownership trade-off.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-COPILOT-PLUGINS-001 | Discovery API; Execution and cache safety |
| REQ-AGENTS-COPILOT-PLUGINS-002 | Profile state and persistence; Editor state |
| REQ-AGENTS-COPILOT-PLUGINS-003 | Identity and installation adapter |
| REQ-AGENTS-COPILOT-PLUGINS-004 | Process preparation; Process coverage; Command preview |
| REQ-AGENTS-COPILOT-PLUGINS-005 | Effective launch context; Executor boundary; Compatibility qualification |
| REQ-AGENTS-COPILOT-PLUGINS-006 | Editor state; Desktop and phone composition |
| REQ-AGENTS-COPILOT-PLUGINS-007 | Native authority; Privacy and diagnostics |

## Evidence and current checkout

The planning baseline is clean commit `a6144c3cb8ef586b7e56f904fae9acaf38569113`.
No Copilot process or real authentication store was accessed during planning.
[Work order 00's evidence](../../../plans/copilot-profile-plugins/compatibility-evidence.md)
later reproduced package identity, inventory, installation metadata, and repeated
ACP startup with disposable fixtures, without authentication.

Confirmed in this checkout:

- `CopilotACP.ID()` is `copilot-acp`. `BuildCommand` uses the selected managed npm package.
- `managed_npm_runtime_versions.json` pins `@github/copilot` to `1.0.88`.
- `ManagedNPMRuntimeSpec.InteractiveCommand(version)` provides the same package without `--acp`.
- `resolveProfileProbeContext` resolves authorized profile settings and secrets.
- `requireProfileDiscoveryScope` requires `org.config.manage`.
- `CursorMcpProfileSelection` provides inline selection, Refresh, and unavailable saved rows.
- `useAgentMcpDiscovery` guards profile/request generations, but calls agent-wide discovery. It is not a sufficient Copilot context resolver.
- Shared profile DTOs, the SQLite table rebuild, browser normalization, and draft reconciliation enumerate fields explicitly.
- `settings/catalog.ProfileDescriptor` derives descriptors from `dto.ProfileContractFields`. It replaces the shared defaults entry during generation.
- Fresh starts use `buildAgentCommandWithContext` and `configureAndStartAgent`.
  Replacements also use `buildFreshAgentCommandWithProfile` in `manager_interaction.go`.
- Terminal launches and `hostutility.ExecuteProfilePrompt` have separate command paths.
  Copilot passthrough currently has an unversioned npm base. The selected-runtime change below applies only to nonempty typed selections.

Supplied observations: Copilot `1.0.103/1.0.104` loaded `harness` through an explicit ACP plugin-directory mount.
Its read-only Atlassian call succeeded with existing native authentication.
Both ordinary CLI and ACP logs fetched managed policy. The automatic activation
difference remains unexplained. WO-00 reproduced inventory and ACP startup for
`1.0.88` and `1.0.104`; authenticated calls and managed-veto observation remain
unverified.

Official references, checked 2026-10-27:

- The [plugin reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference) documents flat inventory and the earlier envelope.
  Rows include names, source, native status, and optional marketplace, version, and installation provenance.
  Local directory-source marketplace plugins can use live roots. Manifests have version-dependent locations and precedence.
- The [configuration reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference) identifies `config.json` as internal state.
  It documents `COPILOT_HOME`, the legacy config-directory option, native MCP disables, and managed plugin policy.
- The [ACP reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/acp-server) describes `--acp`, stdio, and process-owned session options.
  ACP remains a changing upstream interface.

These references do not establish the private installation schema or explicit-mount policy enforcement for Kandev's `1.0.88` pin.

## Design choices and alternatives

| Choice | Decision | Reason and alternative |
| --- | --- | --- |
| Default | Empty set | Existing profiles gain no additional mounts. Automatic inheritance changes current behavior. |
| Meaning | Additive explicit mounts | Native runtime loading remains authoritative. Exclusive filtering is unproven. |
| Missing plugin | Keep preference, block a new start | Silent omission changes task capabilities. Saving repairs remains possible. |
| Sources | Qualified installed metadata only | Supports cached/direct/live roots when evidenced. Directory guessing cannot establish identity. |
| Executors | Backend host local/worktree only | Other executors lack host filesystem and credential facilities. |
| Paths | ACP plus profile-owned terminal/utility/probe paths | A profile keeps one meaning across supported process entry points. |
| Running sessions | No reload or termination | Native mounts are process-start state. |
| Pin | Preserve selected managed runtime | A newer global CLI is not evidence about the selected process. |

## Components and responsibilities

Introduce `internal/agent/copilotplugins` as a small backend provider boundary.
Implemented types are `InventoryRow`, `PluginID`, `LaunchContext`, `Snapshot`, and a typed error with a closed reason code.
It owns the command adapter, inventory parser, narrow installation adapter, root validation, and deterministic argv additions.
It does not import lifecycle, settings controllers, Cursor MCP strategies, or browser DTOs.

The controller owns authorization, saved/draft lookup, applicability, and safe DTOs.
Lifecycle owns executor admission and final environment composition.
Host utility owns runtime selection, admission, and probe caching. Both use the same provider boundary.

## Profile state and persistence

Add `copilot_selected_plugins` as a JSON array of stable IDs.
Use `CopilotSelectedPlugins []string` in domain/runtime structs and `copilotSelectedPlugins` in browser types.
Keep it separate from `mcp_selection_mode` and `mcp_selected_servers`.

- Create omission produces `[]`.
- Update omission preserves the committed field. `[]` replaces it with an empty selection.
- JSON `null` is invalid for this field. Preserve absent/null distinction during strict request decoding.
- Limit selections to 256 distinct IDs, each at most 512 UTF-8 bytes, and 128 KiB in aggregate.
- Validate canonical identity shape. Treat the preference as a set and sort canonical IDs for storage and comparison.
- Require concrete `copilot-acp` applicability for nonempty selections. Dynamic routing profiles do not own mounts.
  Concrete Copilot execution profiles used by Office retain the field normally.
- Allow an explicit empty list on other profiles for shared-client normalization. Reject a nonempty list there.
- Validate syntax at save time without requiring successful discovery or an installed plugin.
  Existing missing selections must remain clearable and unrelated edits must remain saveable.

Add a `TEXT NOT NULL DEFAULT '[]'` column to `agent_profiles`.
Include an idempotent additive migration, fresh schema, legacy replacement table, and copy expressions.
Extend shared insert/update/select/scan code and duplicate/conversion paths.
Malformed stored JSON fails visibly instead of becoming `[]`.

For ordinary updates, preserve omitted selections in the SQL write, not only in a controller read/modify/write snapshot.
Extend the existing supplied-field update seam or add a narrow selection-intent argument to its atomic update.
The committed response/event must contain the value from that write, including a concurrent earlier selection update.
Explicit full snapshots retain their existing replacement meaning.

Propagate the field through shared DTOs, controller conversions, HTTP/MCP settings handlers, boot/event projections, and browser mappings.
Add the independent `ProfileContractFields` entry, replacement semantics, applicability, and next-process timing.
Update `settingscatalog/defaults.go` where its fallback profile descriptor remains present.
Regenerate both `contract.generated.json` and `profile-contract.generated.json` with `cmd/settings-catalog`.
Keep generic settings mutation and legacy profile tools routed through the same DTO/domain contract.

## Identity and installation adapter

Identity format is `<source>:<marketplace>/<name>` with canonical percent-encoded segments.
An absent marketplace uses an empty segment, for example `installed:/my-plugin`.
The handoff example becomes `installed:gst-agentic-platform/harness`.
Version and root never enter the identity.
Preserve case and source values. Do not merge equal names across marketplaces.

Decode segments once for validation. Reject noncanonical encodings, invalid UTF-8, controls, separators, traversal segments, and empty names.
Only qualified `installed` and `live` identities can become new saved selections.
Built-in and unfamiliar source rows can have display identities but remain unselectable.
Duplicate inventory identities yield one unavailable row with `ambiguous_identity`, never an arbitrary winner.

Parse only these inventory containers:

1. A flat array with required typed `name`, `enabled`, and `source` fields.
2. A qualified legacy object with typed `plugins` and `errors` arrays. A nonempty errors array fails the snapshot.

Optional fields must have their documented types. Unknown additive row fields can be ignored.
Unknown container shapes, invalid required fields, duplicate object keys, trailing values, truncation, or nonzero exits fail the snapshot.
Do not strip banners or search stdout for an embedded array.

Read the effective `config.json` through a bounded JSONC compatibility adapter.
Decode only `installedPlugins` entries and the necessary identity, version, and root fields.
Discard unrelated fields. They never enter DTOs or logs.
Use a maintained JSONC parser. The repository has no current Go JSONC reader.
The implementation uses [HuJSON](https://github.com/tailscale/hujson), which supports comments and trailing commas.
Pin its revision during implementation and review the dependency diff.
Do not implement comment removal with regular expressions.

Work order 00 qualifies the candidate `cache_path` mapping against the selected package. It is not a public schema guarantee.
Match inventory source/name/marketplace and any jointly present version exactly.
Multiple matches, stale versions, missing roots, or conflicting fields are unavailable.
Never synthesize `installed-plugins/<marketplace>/<name>` or treat a URL as a filesystem directory.

| Source | Initial eligibility |
| --- | --- |
| Marketplace cached install | Exact qualified metadata root and matching manifest |
| Direct install without marketplace | Same adapter, with the empty marketplace identity |
| Directory-source marketplace/live root | Explicit qualified root, including legitimate external directories |
| `installedFrom` only | Provenance only until a versioned mapping establishes root semantics |
| Built-in | Visible, `builtin_not_mountable` |
| Unknown or ambiguous metadata | Visible, `unsupported_source` or `metadata_unavailable` |

Require an absolute, clean, existing readable directory. Reject root-level directories and special files.
Reject lexical traversal, control characters, device namespaces, and unsupported network roots.
Canonicalize an explicitly recorded symlink target once. Accept external targets only as explicit metadata evidence.
Never discover another directory through a symlink or a search.
Bound manifests to 256 KiB and native state to 2 MiB, including file-growth checks.
Use the exact manifest locations and precedence qualified for the runtime, then match the manifest name and compatible version.
Reject ambiguous manifests and symlinked manifest files. Do not parse executable components to reproduce them.
Recheck canonical identity immediately before process configuration. A changed root fails that start.

## Discovery API

Add `POST /api/v1/agents/:id/plugin-discovery` for the concrete `copilot-acp` provider.
The body contains `profile_id`, optional `launch_settings`, and `refresh`.
Use `ProfileLaunchSettingsRequest` for its existing complete environment/CLI/prefix snapshot.

Reuse `requireProfileDiscoveryScope` and `resolveProfileProbeContext` semantics.
Saved profiles authorize their identity. New drafts require a complete snapshot and profile-creation authority.
Reject cross-agent, foreign, dynamic, incomplete, or unknown request fields before command execution.
Resolve secret references on the server under existing global-secret scope rules.
Requests cannot supply roots, executables, packages, working directories, or native documents.

Response:

```json
{
  "agent_id": "copilot-agent-id",
  "profile_id": "profile-id",
  "provider_id": "copilot-acp",
  "status": "ready",
  "context_revision": "opaque-context-revision",
  "plugins": [
    {
      "id": "installed:gst-agentic-platform/harness",
      "name": "harness",
      "marketplace": "gst-agentic-platform",
      "version": "1.2.0",
      "native_enabled": false,
      "availability": "available"
    }
  ]
}
```

Top-level status is `ready`, `unavailable`, or `unsupported`.
Entry availability is `available` or `unavailable`, with optional `reason_code`.
Reasons include `cli_unavailable`, `timeout`, `cancelled`, `output_limit`, `invalid_inventory`, `legacy_errors`,
`runtime_unsupported`, `metadata_unavailable`, `ambiguous_identity`, `missing_root`, `manifest_mismatch`,
`unsafe_root`, `builtin_not_mountable`, `external_not_mountable`, `unsupported_source`, `manual_mount_conflict`,
`unsupported_launch_context`, and `policy_unverified`.
Use constant messages and metadata lengths capped at 512 bytes. Never return raw stderr or provenance paths.

Authorization/validation retain 401/403/404/400 behavior. Expected command failures return a typed unavailable result, never an empty-success snapshot.

## Effective launch context

Use `ManagedNPMRuntimeSpec.InteractiveCommand(selectedVersion)` plus structured `plugin`, `list`, `--json` arguments for discovery.
Resolve the version through the existing managed selection reader.
Extract and forward only qualified config-directory selector flags from the profile.
Never append arbitrary profile flags such as ACP mode, prompts, or plugin mounts to the inventory subcommand.
Retain the complete profile context for identity checks, without executing those unrelated flags during discovery.
Do not substitute the globally installed CLI. The global version can differ from this package.
Do not change the reviewed pin, invoke an update job, or add a fallback to another home.
Existing runtime machinery owns npm cache preparation.

The adapter receives the final environment, trusted working directory, selected command, runtime identity, and validated launcher prefix.
Configuration precedence is the qualified legacy `--config-dir` option, then `COPILOT_HOME`, then the effective user home plus `.copilot`.
Accept both `--config-dir=value` and split arguments when supported by that runtime.
Ambiguous repeated values or unsupported legacy behavior fail visibly.
Do not invent an XDG fallback. Older migration behavior requires explicit runtime evidence.
On Windows, derive the home through the existing platform process environment contract, including `USERPROFILE` where applicable.

For task starts, resolve after `resolveStrictEnvironment` and the final per-run overlay in `configureAndStartAgent`.
Compose inherited local child values with explicit overrides at this boundary, not during profile lookup or `BuildCommand`.
Pass the resulting home/config/path values to both resolution and the configured child, so they cannot disagree.
If the local agentctl base differs from the backend, use its trusted prepared environment rather than ambient backend values.
An unavailable base context is `unsupported_launch_context`, never an ambient-home fallback.

Host utility resolves the equivalent sanitized environment before its own process starts.
Existing environment composition owns precedence. Required cross-tier pure helpers belong in `internal/common`, with caller and child tests.
A wrapper that changes user, home, mount namespace, or filesystem is unsupported until it exposes a trusted equivalent context.
Ordinary wrappers retain their existing validated structured wrapper behavior.

## Execution and cache safety

Invoke executables directly with structured argv and context cancellation. Never pass plugin metadata through a shell.
Use the native runner's process-tree and bounded-wait patterns from `mcpconfig/cursor_native_mcp.go` as a reference.
Do not couple the new adapter to Cursor diagnostic types or MCP approval commands.
Extract a neutral runner only if both callers can retain their existing behavior and tests.

Use a 20-second command deadline, 2 MiB stdout, 32 KiB stderr, 1024 inventory rows, and bounded descendant cleanup.
Include runtime startup in the deadline. Do not return partial buffers as valid JSON.
Propagate request cancellation and close pipes on every exit path.
Settings inventory uses a disposable working directory and sends no model prompt.
New-process resolution uses the trusted final child working directory, including
host utility calls, so discovery and launch agree on relative inputs.

Use a separate bounded inventory cache: at most 256 entries, 30-second TTL for successful snapshots only.
Cache safe DTOs, not roots, raw metadata, or secret values.
Refresh bypasses the cache. New-process resolution rereads inventory and roots rather than trusting browser or cached paths.
Use the profile discovery HMAC/generation pattern for scope, selected runtime, final environment, CLI config inputs, prefix, and draft identity.
Secret values enter only the ephemeral HMAC input. They are never cache keys or logs.
Runtime activation and explicit refresh fence older results before return and publication.
Admission remains bounded. Request cancellation must not leave a detached single-flight process running without an owner.

## Process preparation

Store stable selection IDs in the authorized process-preparation snapshot, not resolved roots in profile state.
At each new process boundary:

1. Gate provider and executor. Return immediately for an empty selection.
2. Resolve the authoritative final environment and selected runtime.
3. Require a supported inventory/metadata adapter and obtain the current inventory.
4. Resolve every selected ID. Any missing or unsupported plugin fails the whole preparation.
5. Inspect enabled manual `--plugin-dir PATH` and `--plugin-dir=PATH` flags without changing their order.
6. Deduplicate equal canonical roots. Fail if the same plugin identity resolves to conflicting manual and selected roots.
7. Append typed `--plugin-dir`, root pairs in canonical ID order, before any end-of-options delimiter.
8. Configure the child with the same environment and argv, then let Copilot load the complete plugins.

Do not persist typed additions into the user CLI-flags list.
Rebuild from the base argv on retries so mounts do not accumulate.
Manual-only profiles with an empty typed selection keep existing behavior and do not acquire new validation.
Different selected plugins with colliding native components retain native precedence and warnings.
Kandev does not rewrite MCP names or override native load order beyond its deterministic argument order.

## Process coverage

| Path | Required preparation point | State rule |
| --- | --- | --- |
| Fresh ACP | `buildAgentCommandWithContext` snapshot, final `configureAndStartAgent` preparation | Resolve for each new child |
| Workspace-only promotion | Existing promotion in `manager_launch.go`, then common start preparation | Resolve when promoted, not during workspace allocation |
| Process replacement/context reset fallback | `buildFreshAgentCommandWithProfile`, common configure/start | Rebuild additions without duplication |
| New-process resume/recovery | Reconstructed profile snapshot and normal configure/start | Never reuse stale absolute roots |
| Managed startup retry | Same snapshot and final preparation | Keep selected runtime and plugin intent |
| Retained process adoption or in-process ACP reset | No new preparation | Preserve the running child's mount set |
| CLI passthrough fresh/resume/restart | `manager_passthrough.go` after terminal environment composition | Nonempty typed selection uses the selected managed interactive command |
| Profile utility prompt | `hostutility.ExecuteProfilePrompt` before `InferenceConfigDTO` dispatch | One profile snapshot per call |
| Profile capability/model probe | `ProbeProfileCapabilities` and profile model-config path | Include selected IDs in context/cache identity |
| Agent-wide startup/install/version probes | No profile plugin preparation | No implicit selection |
| Command preview | Symbolic additional-mount notice | No native command, root read, or executable placeholder |

Extend the profile probe launch snapshot with optional `copilot_selected_plugins`.
Omission uses the saved profile value, or `[]` for a new draft. An explicit array replaces it for that request.
Other agents retain their existing three-field snapshot behavior.
Plugin inventory discovery itself never mounts the selection. A missing saved selection must not prevent discovering its repair.

For nonempty typed selections, Copilot passthrough receives `InteractiveCommand(selectedVersion)` through its existing `BaseCommand` option.
This replaces its unversioned base for these profiles so qualification and execution use one runtime.
Empty selections and other agents retain current passthrough command selection.

Profile saves affect the next newly started process. A retained process keeps its original set even after save.
Use the normal authorized profile snapshot for a later fresh start, including explicit resume settings where existing policy supplies them.
Do not mix one profile revision's IDs with another revision's launch environment within a single preparation.
Capture the profile before composing its command and environment. A concurrent
edit cannot replace only the selected IDs inside that admitted snapshot.
The next preparation must capture the updated profile as a whole.
Passthrough command selection and mounts use that same captured profile,
including an explicit empty selection. Plugin admission checks the replacement
command's wrapper. Stored ACP base arguments exclude generated plugin mounts
so a later retry resolves current roots without retaining an earlier mount.

Preflight Copilot plugin admission, roots, and arguments before stopping a live
passthrough process for replacement. Reject isolated Copilot selections before
the SSH stop path. Preserve the existing remote MCP stop-before-file-retirement
ordering for providers that use it; this is not a generic reset reorder.
An error before handoff preserves process identity and releases only preparation
resources owned by the failed attempt.

## Executor boundary

Admit only backend-owned standalone local/worktree execution, including the legacy `local_pc` representation.
Check trusted executor/runtime classification, not merely a runtime string that an isolated provider can also return.
Docker, SSH, Sprites, Kubernetes, remote Docker, plugin executors, and unknown classifications reject nonempty selections.
Do this before reading host metadata or configuring an isolated child.
No directory mount, portable-bundle expansion, credential copy, or remote native command is added.
Empty selections bypass all new discovery, metadata, and executor checks.

## Editor state

Add a Copilot-only `CopilotPluginProfileSelection`, a small row derivation helper, safe types, and a domain discovery hook.
Use the existing action/API client boundary. Components never fetch directly.
Do not generalize `CursorMcpProfileSelection` or add Cursor inheritance radios to this control.

Wire `copilotSelectedPlugins` into normalization, profile create/update payloads, agent-level save helpers,
`useProfileEditorState`, `isProfileDirty`, editable reconciliation fields, and snapshot comparison.
Unchanged saves omit the field. Reordering the set is not a dirty change.
Duplication uses independent arrays. Save acknowledgements preserve newer local edits.
External updates use the existing conflict banner and explicit recovery actions.

Show the section on the concrete profile page and the concrete Copilot profile creation/edit flow that uses `ProfileFormFields`.
New drafts use a local generation rather than pretending their temporary ID is a saved profile.
Hide the selector for Cursor, custom strategy profiles, and dynamic routing profiles.

The hook keys responses by authorized profile/draft, launch-settings generation, runtime generation, and request generation.
Opening a saved profile performs one discovery. Launch-setting edits mark inventory stale and require Refresh.
They do not spawn a command on every keystroke.
Switching profiles or unmounting cancels the request and rejects late completions.
A failed refresh keeps the last same-context rows as stale, disables additions, and always allows removing selected IDs.
No cached rows from another home or draft can remain selectable.

Missing saved IDs become selected unavailable rows. Unchecking removes them from the draft only.
Do not disable their removal checkbox. Unavailable unselected rows cannot be checked.
Saving unavailable selections is allowed, with next-start consequences visible.
Discovery failure alone does not block unrelated profile saves.

## Desktop and phone composition

Place Plugins inline in Profile Settings, alongside the existing Cursor-reference region.
Show a title, Refresh, a short effect description, then name/marketplace/version rows with native status as secondary metadata.
The primary checkbox means selection in this Kandev profile, not the CLI's enabled field.
Explain that a plugin can execute MCP servers, skills, agents, commands, hooks, and other components.
Also show additive loading, local/worktree support, and next-process timing without exposing implementation paths.

Use `mobile-cursor-plugin-mcp.spec.ts` and `CursorMcpProfileSelection` for wrapping labels and unavailable rows.
Use the curated direct-navigation phone pattern.
On phones, enter through Settings > Agents > Copilot > Profile and keep one focused page.
Use the existing `settings-scroll-container`, with no nested plugin scroller, modal, or new drawer.
The current `SettingsFloatingSave` supplies Save and Reset, bottom clearance, and safe-area positioning.
Preserve its navigation discard dialog and conflict recovery.

Below 768px, stack row metadata below the name and let the header wrap.
Use `useResponsiveBreakpoint` for phone composition independently of pointer precision.
Phone and coarse-pointer Refresh and checkbox-label hit areas must measure at least 44px.
Desktop buttons retain the 28px standard. Do not set unconditional touch sizes.
Support Space/keyboard checkbox activation, focus visibility, and non-hover reason text.
Refresh retains a stable accessible name and a translated status region announces its outcome.
Long names, IDs, version strings, and translated explanations must wrap without horizontal page overflow.

ASCII views and state variants live in the [plan](../../../plans/copilot-profile-plugins/plan.md#ascii-ui-preview).
Their hierarchy and state behavior are structural. Exact English wording and spacing are illustrative.
All final text uses `t()` or `Trans`, seven shipped catalogs, generated Traditional Chinese, and generated pseudo.

## Command preview

Retain the existing base command preview and Copy behavior for executable base arguments.
Show a separate translated note listing selected plugin names/IDs and stating that directories resolve at process start.
Label the base command as incomplete when deferred mounts exist. Copy must not imply it includes those mounts.
Do not return raw roots, fake copyable directory placeholders, or secret values to the browser.
Preview never invokes inventory or policy discovery.

## Native authority

The [native qualification design](copilot-profile-plugins-native-qualification.md#native-authority)
owns explicit-mount policy and authentication boundaries. Selection grants no
policy exception, credential operation, or global activation write.
Native vetoes remain authoritative. Do not reject a supported launch merely
because enterprise-policy evidence or exact-platform certification is absent.
The legacy `policy_unverified` response remains understood by existing clients.

## Privacy and diagnostics

Return constant reason codes and bounded row metadata only.
Native stdout/stderr and native state stay inside the bounded adapter and are discarded after parsing.
Sanitized logs can contain operation, provider, status, reason, selected count, duration, and runtime version.
Never log environment values, raw flags/prefixes, roots, configuration documents, token fingerprints, hooks, or MCP definitions.
Use bounded log correlation, never plugin-ID metric labels.

Launch errors identify the affected stable plugin ID and a repair action.
Actions cover inventory refresh, installation repair, selection removal, or a supported executor/runtime.
They do not expose filesystem paths or raw provider errors.

## Compatibility qualification

The [admission contract](copilot-profile-plugins-native-qualification.md#qualification-and-admission)
keeps inventory/metadata versions bounded while delegating policy to Copilot.
Remove the separate policy-only allowlist; do not replace it with an
OS/architecture certification gate. Diagnostic evidence records its actual
platform without claiming unobserved enterprise enforcement.

## Implementation plans

- [Copilot profile plugins](../../../plans/copilot-profile-plugins/plan.md).
- [Copilot profile plugin completion](../../../plans/copilot-profile-plugin-completion/plan.md).
