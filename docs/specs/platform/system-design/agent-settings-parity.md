---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-003
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-004
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-007
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-008
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-009
---

# Agent-accessible Kandev settings system design

## Purpose and boundaries

Platform owns the shared discovery contract across settings domains.
Each domain remains the source of truth for its values and operations.
Profiles establish the first implementation slice. They are not the delivery boundary.
The [domain adoption design](agent-settings-domains.md) defines the required core domains and explicit exceptions for this delivery.

The frontend navigation catalog remains a presentation contract.
This design connects existing destinations to domain setting IDs without replacing navigation, ranking, translations, or focus behavior.

The [ownership decision](../../../decisions/2026-09-08-domain-owned-settings-catalog.md) defines the extension boundary.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-001` | Catalog, Discovery tools, Coverage |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-002` | Shared requests, Mutation adapters, Dynamic choices |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-003` | Authority, Sensitive values |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-004` | Editor reconciliation |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-005` | Coverage |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-007` | Discovery tools, Target contract |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-008` | Compact update validation, Mutation adapters |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-009` | Lifecycle and compatibility tools, Sensitive values |

## Current implementation

- `internal/agent/settings/handlers/handlers.go` declares private create/update bodies and maps them to controller requests.
- `internal/agent/settings/controller/profile_crud.go` owns defaults, normalization, dependencies, dynamic-profile versions, and persistence.
- `internal/mcp/server/config_handlers.go` declares a smaller schema and manually forwards selected fields.
- `internal/mcp/handlers/config_agent_handlers.go` decodes another smaller request shape.
- The UI saves through `components/settings/agent-profile-page-state.ts` and the existing agent settings actions.
- `internal/mcp/handlers/workflow_step_parity_test.go` compares REST and MCP notifications over a shared store.
- `internal/common/config/catalog.go` describes startup settings. It does not own profile values.
- `lib/settings-discovery/` describes navigation destinations. It does not own mutation schemas.

All backend paths in this document are relative to `apps/backend/`.
All frontend paths are relative to `apps/web/`.

## Catalog

A new `internal/settingscatalog` package owns metadata types and bounded metadata search.
It contains no database access, credentials, global setters, or provider execution.
The agent domain contributes entries from `internal/agent/settings/catalog`.
Other domains contribute adjacent catalog adapters, as specified in the domain adoption design.
Backend composition supplies registered adapters to MCP handlers. The shared package does not import domain services.

Each setting descriptor contains:

| Field | Meaning |
| --- | --- |
| `key` | Stable identity, such as `agent_profile.fallback_model` |
| `domain` | Owning settings domain, such as `agent_profiles` or `user_preferences` |
| `resource_type` | Registered adapter identity, such as `agent_profile`, `workflow_step`, or `user_settings` |
| `scope` | Caller-owned, workspace-owned, or install-owned, with explicit selector rules |
| `description`, `aliases` | Curated English metadata for agents, without saved values |
| `label` | Short human-readable setting name for search results |
| `schema` | Field shape derived from the shared domain request |
| `default_behavior` | Explicit default, provider inheritance, or no default |
| `support` | Status, reason, and optional recovery destination |
| `classification` | Writable field, computed field, action, or reasoned exception category |
| `operations` | Compact update binding, lifecycle operations, field paths, and replacement rules |
| `dependencies` | Related setting keys, resource references, and required companion fields |
| `required_scope` | Existing domain authority, including `org.config.manage` for writes |
| `applies_when` | Metadata update, future launch, future selection, or provider-dependent behavior |
| `sensitive_paths` | Paths that require omission or redaction in new value reads |
| `settings_href` | Canonical settings route after resource resolution, or explicit absence |

Descriptors are immutable process metadata. They do not store current values or authority decisions.
Descriptions stay adjacent to operation bindings. No second hand-maintained type or field allowlist exists in MCP.
Dynamic maps declare typed values rather than enumerating provider-specific keys.

Search returns at most 50 entries, with a default limit of 20 and an offset cursor.
Queries have a maximum length of 256 characters. Matching is case-insensitive token matching over metadata only.
Search tokenization splits whitespace, dots, underscores, and hyphens.
Exact keys and labels rank first, followed by prefixes and aliases, then descriptions. Stable keys break ties.
Domain context can narrow results but cannot make an unrelated field match by itself.
No fuzzy match changes a key or selects an update target.
An empty query lists the domain's descriptors in stable order.
This delivery does not introduce embeddings, external search, or translated backend search.

A result includes `key`, `label`, `description`, `resource_type`, `field_path`, `support`, `match_reason`, and `settings_href` when resolvable.
It names `describe_setting_kandev` as the next step. It does not embed full schemas, current values, or resource lists.
Descriptions explain behavior and important dependencies. For example, fallback-model metadata states that automatic fallback takes precedence.
An ambiguous query returns separately labeled candidates. Resource selection remains explicit.

Registry construction rejects duplicate keys, missing owners, unresolved operation bindings, and unsupported schema shapes.
A writable descriptor requires a registered validator, mutation adapter, and authority rule.
Every exclusion requires a reason. Documentation-only descriptors cannot become writable by changing a status string.

## Shared requests

Move the HTTP create/update body definitions into exported agent-domain contracts under `internal/agent/settings/dto/`.
Both transports decode these types and use one conversion path into controller requests.
Transport identifiers stay outside the domain patch. Compact reads and updates use the shared `target` object.
Existing lifecycle and compatibility tools retain their profile or agent selectors.
Dependency confirmation remains an operation argument, not a saved setting.

Generate domain validation schemas from the shared typed contracts and enrich fields with catalog descriptions.
Use the existing JSON Schema dependencies. Compile every generated schema through `internal/mcp/toolschema.Compile`.
Generated schemas must preserve the existing Draft 7 validation boundary.
Compatibility adapters remain explicit and covered by tests.
The compact tools advertise only stable envelope schemas. Full domain schemas are returned by description or used internally for validation.
No enum of setting keys and no `oneOf` of all resource schemas appears in those envelopes.

The following table is the initial field inventory. It is design evidence, not another runtime allowlist.

| Fields | Operations and semantics |
| --- | --- |
| `name` | Create/update. Required nonblank name on creation and when supplied on update. |
| `model`, `mode` | Create/update. Empty model retains the existing provider-default behavior. |
| `fallback_model`, `auto_fallback` | Create/update with the existing fallback precedence. |
| `config_options` | Create/update. Replace the supplied map, then apply current domain sanitization. |
| `auto_approve`, `allow_indexing` | Create/update. Preserve legacy compatibility. Mark `allow_indexing` as deprecated metadata. |
| `cli_passthrough`, `cli_flags` | Create/update with current capability rules. A supplied flags list replaces the full list. |
| `env_vars`, `command_prefix` | Create/update through existing validators and secret-reference checks. |
| `enabled` | Update. Creation retains the domain's existing enabled default. |
| `dynamic` | Create/update through current candidate validation, feature availability, and version rules. |
| `mcp.enabled`, `mcp.servers` | Compact read/update with `resource_type: agent_profile_mcp`. Separate resource operation using the profile ID. |

The MCP document also carries opaque `meta`. Classify this field as preserved metadata, not an editable UI control.
The shared document adapter preserves it when the caller omits it.
The HTTP `mcpServers` spelling remains a compatibility alias for `servers`.
The compact document adapter accepts canonical `servers`. Existing dedicated MCP tools remain external compatibility adapters.

`id`, `agent_id`, `kind`, timestamps, billing metadata, and other computed response fields are not generic writable settings.
Each noneditable response field has an explicit coverage classification.
Agent-level fields such as `supports_mcp` remain outside the profile mutation contract.

On update, omission leaves a value unchanged. False, zero, empty strings, empty maps, and empty lists survive transport conversion.
Empty collections mean replacement with an empty collection where the domain supports clearing.
There is no universal reset operation or new meaning for null.
Existing HTTP null behavior remains compatible. Newly exposed MCP fields reject null unless their published schema explicitly permits it.

## Discovery tools

Register a `configuration-settings` group through the existing typed MCP profile registry.
The configuration and external surfaces receive these tools. Ordinary task, Office, and automation surfaces do not receive this group.
The configuration surface replaces adopted read/update tools with the compact group after equivalent coverage passes.
The external surface retains existing tool names and successful argument forms for compatibility.
Legacy tools remain visible until their adapters pass their work-order checks.
Lifecycle and interaction tools remain explicit. External compatibility tools retain their existing schemas.

| Proposed tool | Arguments | Result |
| --- | --- | --- |
| `search_settings_kandev` | `query`, optional `domain`, `limit`, `cursor` | Descriptor summaries, covered domains, and next cursor |
| `describe_setting_kandev` | `key`, optional `target`, `context`, `operation`, `include_resource_schema` | Detailed descriptor, dependencies, optional operation schema, and current capability status |
| `get_settings_kandev` | `target`, `keys` | Saved-value projection for up to 50 keys, with explicit redaction and source |
| `update_settings_kandev` | `target`, `changes`, optional `options` | Accepted field names and revision metadata, or a structured tool error |
| `list_settings_resources_kandev` | `resource_type`, optional `workspace_id`, `query`, `limit`, `cursor` | Authorized target summaries, explicit empty state, and next cursor |

`describe_setting_kandev` works without a target for static schema information.
The setting key determines the descriptor's resource type. A supplied target must match that type.
For creation, the domain's validated `context` names required parents, such as `agent_id` for a profile.
`operation` defaults to `update` and also accepts `create` for profile lifecycle schema discovery.
`include_resource_schema` defaults to false. When true, the result includes the selected operation's complete domain schema and required fields.
This supports related settings and dynamic-profile creation without loading every schema upfront.
The agent adapter's context contains optional `agent_id`, `model`, `mode`, and `config_options` for discovery before saving.
Other adapters define their own closed context schemas. Unknown context fields fail before domain lookup.
Context supplies no identity, authority, executable binding, or persistence operation.

`get_settings_kandev` requires one registered resource type and target. It does not infer a profile from the calling agent.
`keys` contains canonical descriptor keys belonging to that resource type.
Unknown domains and keys return typed errors. A domain outside coverage is not an empty successful value response.
New reads include `source: profile`, available revision metadata, and the descriptor's change timing.
Profile fields carry `updated_at`. The separate MCP document does not borrow that timestamp or claim an equivalent revision.
An inherited model has an explicit inheritance marker. A saved profile read never claims to show an active session's effective model.
The tool does not accept arbitrary paths or resolve credential contents.

The configuration prompt at `config/prompts/config-context.md` introduces discovery before settings changes.
It teaches search, resource lookup, describe, read, and update without repeating every optional field.
The prompt states supported domains and preserves the existing destructive-action confirmation instruction.

## Target contract

The shared target is `{resource_type, resource_id?, workspace_id?}`. No resource-type enum appears in its advertised schema.
The registry defines each type's required selectors. Unknown types and extra selectors fail closed.

- `user_settings` accepts neither resource ID nor workspace ID. The existing user service resolves the trusted caller.
- Workspace singleton adapters require `workspace_id` and reject a resource ID.
- Entity adapters require `resource_id`. Their domain resolves and authorizes the containing workspace.
- A supplied workspace ID must match the authorized entity's workspace. It never overrides ownership.
- Install singleton adapters accept neither ID. They enforce their existing organization scope.
- `runtime_flag` uses its registered flag key as `resource_id`, allowing one atomic override operation per request.

Resource lookup is a fifth bounded discovery tool, not another settings mutation tool.
It uses existing authorized domain list services and returns only IDs, labels, parent context, and complete targets.
The default limit is 20, the maximum is 50, and the query limit is 256 characters.
Query matching is limited to resource labels and public identifiers, never prompt bodies, command strings, credentials, or arbitrary saved values.
Filtering and authorization precede pagination. Cursors bind the caller, resource type, query, and workspace scope.
Foreign resources and missing resources produce the same not-visible result. Singleton lookup returns only the caller's permitted singleton.
Where existing lists are unbounded, adapters add bounded service queries rather than loading the entire table for every page.
Existing list tools remain compatibility operations. No new per-domain list tool is required.

## Compact update validation

The outer `update_settings_kandev` schema requires a target and nonempty object of changes.
It allows at most 50 top-level changed fields and a 256 KiB serialized changes object.
The root envelope is closed. The nested changes object receives domain validation at invocation.
These limits do not replace existing stricter domain limits.

The backend performs this sequence before mutation:

1. Resolve the exact registered resource adapter and trusted caller context.
2. Authorize the operation and load the selected resource through its domain service.
3. Select the current update schema from that adapter and resource context.
4. Validate the patch shape, then validate the resulting resource through the domain's update operation.
5. Decode the shared typed patch and invoke the existing domain mutation once.
6. Return accepted field names and available revision metadata without echoing sensitive values.

Closed typed objects reject unknown keys. Only domain-declared maps permit arbitrary keys with the domain's value type.
An update cannot mix profile fields with the separate MCP document, create an undeclared resource type, or write an arbitrary database column.
The request never contains executable bindings or a caller-supplied schema.
The optional `options` object receives a closed schema from the selected adapter.
Profile adapters accept `force`, false by default. Other adapters reject this option unless their domain explicitly supports it.
Storage acknowledgements retain their exact confirmation tokens. No generic force flag bypasses unrelated domain checks.
Descriptions return option schemas on demand. Adding domain options does not enlarge the advertised outer envelope.

Domain validation checks the patch together with unchanged saved values and authorized references.
Adapters never perform an unlocked read, merge, and full replacement when that can erase unrelated concurrent changes.
Partial updates use atomic domain patches. Required read-modify-write operations use the domain's transaction or version mechanism across REST, WS, and MCP.
An adapter-only mutex cannot protect other writers or multiple backend processes.
No single request spans independent resources. A domain operation can update dependent rows atomically when its invariant requires that operation.
Validation failures produce no writes. Failed post-save probes report a saved result with degraded capability, not a false rollback claim.

Validation errors contain `code`, canonical setting key, JSON field path, a safe expected-type description, and the suggested description call.
Errors never echo submitted values. Unknown-field suggestions are advisory and are never applied automatically.
The server validates against current rules even when the caller used an older description response.
Capability failures retain their typed state and do not cause guesses or retries with relaxed validation.
Static registry schema revisions are content hashes for cache invalidation, not authority tokens or resource concurrency versions.

## Lifecycle and compatibility tools

Creation remains explicit because dynamic profiles require related values in one valid create request.
`create_agent_profile_kandev` keeps `agent_id`, `name`, legacy optional `model` and `auto_approve`, and adds an optional `settings` object.
New fields go inside `settings`, without expanding the advertised outer schema.
Model becomes optional to support the existing domain default behavior.
The backend validates the combined create request against the shared create contract.
A field supplied both at the top level and inside `settings` is rejected, including equal duplicate values.
`settings` cannot override `agent_id` or another transport identifier.
Discovery supplies the complete create schema on request, including dynamic candidates and their required policy fields.

Existing external `update_agent_profile_kandev` and MCP-document tools retain their current argument shapes and meanings.
Their adapters use the same validators and services. New fields are accessed through the compact tools rather than added to legacy schemas.
The configuration surface omits the adopted legacy update tools to avoid duplicate ways to change the same settings.
Profile/resource listing and explicit create/delete operations remain available.
Profile replacement sends the existing `tools/list_changed` notification so clients refresh their advertised tools.

## Dynamic choices

The agent domain adapter uses `Controller.ResolveAgentModelConfig` and existing capability discovery.
Profile context comes from the saved profile plus explicit model-context overrides.
Create-time discovery accepts an existing agent ID before a profile exists.
The resolver retains its current cache, timeout, and provider-neutral response.

The descriptor returns the resolver's status and full applicable choice snapshot.
`probing`, `auth_required`, `not_installed`, and failure states remain explicit.
The assistant cannot treat a stale or unavailable snapshot as an empty valid choice set.
Static setting discovery remains available when the provider probe fails.
Provider-dependent values remain the domain's validation responsibility. This work does not add a second provider policy engine.

## Mutation adapters

Existing REST paths, compact MCP operations, and compatibility tools call the shared domain conversion and validation path.
Successful writes use the existing controller and stores. There is no new schema migration.
The profile MCP document retains its separate service.
HTTP requires `enabled` and normalizes an absent servers map to an empty map.
Existing MCP calls treat omitted fields as unchanged. Preserve these transport defaults through explicit adapters to the same document operation.
The catalog describes that distinction. Equivalent full inputs must produce equivalent stored documents.
Both adapters call the controller so profile existence and MCP-support checks remain consistent.
Profile creation followed by an MCP document update remains two operations, without an atomicity claim.

The REST and MCP adapters translate typed domain failures into their native error envelopes.
MCP preserves error code and structured dependency details across both backend clients.
`DispatcherBackendClient` currently discards those details. Extend its error representation and the streamed backend client consistently.
Existing human-readable error text remains available to older callers.
An unknown argument fails at the existing tool-schema boundary before dispatch.

Successful profile creation and updates produce the existing domain notification exactly once.
Keep either the existing REST broadcast or MCP event publication for its operation.
Do not add another publication to a shared helper while retaining both callers' publication.
Do not introduce synthetic agent responses as a source of UI truth.

The current MCP-document writers publish no update notification.
Add `agent.profile.mcp_config.updated` as an invalidation event with `profile_id` and the domain's routing scope.
Each accepted REST or MCP document write publishes this event once after persistence.
The event carries no document values. An authorized open editor refetches through its existing read action.
Refetches use a request generation and profile-ID guard. An invalidation during a pending read triggers a fresh read.

## Authority

REST profile mutations currently use `authz.ScopeOrgConfigManage` and the interim browser interlock.
MCP must enforce the same organization scope from trusted request context before domain mutation.
Use `authn.IdentityFromContext` and the existing `authz` subject/scope resolver.
The transport owns whether authentication is enforced. Missing identity cannot become unrestricted access under enforced authentication.
The established auth-disabled path preserves single-user operation.

In-session requests retain the owner identity from `internal/mcp/scope`.
External requests retain the authenticated PAT identity.
The browser interlock remains a browser transport guard. MCP does not fetch or reuse its token.
Existing automation restrictions and raw-WebSocket rejection remain effective.
The catalog cannot manufacture a scope, user identity, profile ownership, or MCP surface from request arguments.

`options.force` remains false by default for dependency-sensitive profile updates.
A conflict returns affected utility and dynamic-profile references without a write.
The configuration assistant uses its existing question tool before a confirmed retry.
The same operation remains unavailable when the caller lacks organization authority.

## Sensitive values

Search and descriptions contain schema metadata only.
New saved-value projections redact literal environment values and credential-bearing MCP subdocuments.
Secret IDs remain references. The adapter never calls secret resolution to satisfy discovery.
Redacted values include an explicit marker and cannot be round-tripped as replacement values.
The MCP-document descriptor names the compact resource binding and the external compatibility tools' current exposure limits.

Existing legacy tools can expose raw profile MCP documents under their current contract.
This delivery does not claim a product-wide secret-redaction migration.
New discovery logs contain setting keys, resource IDs, and outcome codes, without request values or provider error payloads.

## Editor reconciliation

The existing `agent.profile.updated` handler updates `settingsAgents` and profile choices.
`useAgentProfileSettings` derives the selected profile from that store.
Extend `useProfileEditorState` to reconcile authoritative profile updates with its local draft and saved baseline.
Apply the same contract to `dynamic-agent-profile-editor-state.ts` and `dynamic-agent-profile-editor-draft.ts`.
Dynamic candidate edits retain the existing server version and dependency rules.
The separate `use-profile-mcp-config.ts` hook consumes the new document invalidation and maintains its own baseline and draft.
An MCP document conflict blocks only its contributor. The save coordinator continues to own route-level feedback.

- A clean editor adopts a newer profile and remains clean.
- A dirty editor preserves its draft and records an external-change conflict.
- A conflicted contributor blocks Save through the existing save coordinator and shows localized recovery copy.
- The existing discard/reset flow adopts the latest authoritative profile and clears the conflict.
- The editor correlates its submitted snapshot with its own response and notification. Its acknowledgement is not an external conflict.
- A late save response cannot replace a newer authoritative baseline or remove edits made after submission.

Use the existing `updatedAt` ordering and profile-ID guards for delivered snapshots.
This is client reconciliation, not a new server compare-and-swap guarantee.
Simultaneous concrete-profile writes retain the current server behavior. Dynamic profiles retain their existing version conflict contract.

Desktop and phone use the current profile route, advanced sections, and shared Save surface.
The nearest phone exemplar is `e2e/tests/settings/mobile-agent-profile-config-selector.spec.ts` and its profile page.
Recovery copy stays inline beside the affected editor. It needs no new modal, drawer, or navigation layer.
The existing content scroll owner and safe-area-aware save surface remain in use.
Any new touch action uses a 44px hit area only for coarse pointers. Fine-pointer controls retain current density.
All new visible copy uses the five locale catalogs. Traditional Chinese values use the existing generation command.

## Coverage

Coverage has three independent sources for every adopted domain:

1. Shared typed create/update contracts and response fields.
2. UI payload construction, typed editable fields, and settings controls.
3. Catalog descriptors, generated domain schemas, and advertised envelope schemas.

A generated metadata snapshot supplies stable field keys and schemas to frontend tests.
The proposed `cmd/settings-catalog` exporter supports `--check` without writes.
Generation is deterministic. Tests fail when the checked-in snapshot differs from the domain catalog.
The frontend mapping ties each domain patch key and settings control to a setting ID or explicit coverage classification.
Dynamic provider keys belong to the `config_options` descriptor, not a frozen list.

Go contract tests compare complete field sets, requiredness, nested shapes, and explicit exclusions.
Mutation tests call real registered MCP tools and real REST handlers over the same domain fixture.
They compare persisted values, defaulting, invalid inputs, dependency details, and emitted notifications.
At least one negative fixture adds a field outside the catalog. Another removes a descriptor.
Both fixtures must cause the coverage assertion to fail.

The existing backend and frontend test workflows run the contract checks on relevant changes.
Add the exporter freshness command to backend CI with path coverage for its frontend snapshot.
Tests assert that removing a required check or narrowing its path coverage causes the CI wiring check to fail.
No branch-protection or repository-setting changes are part of this package.

The search suite owns a curated query set independent of descriptor aliases.
Cases include exact keys, `backup model`, `reasoning effort`, `command prefix`, `keyboard shortcut`, `archive confirmation`, and `workflow agent`.
Additional cases cover repository scripts, executor defaults, notification events, integration watches, runtime locks, ambiguous `permissions`, and an unrelated query.
Cases assert the expected first result or candidate set, correct resource scope, match reason, and no saved-value search.
The alias and ranking implementation cannot rewrite expected results automatically.

An added-setting fixture verifies that compact tool names and advertised schemas remain byte-identical.
The description result and backend validator must recognize that same new setting.
The named create tool's envelope receives the same stability check.
Compatibility tests cover legacy external calls without requiring legacy schemas to grow.

Focused browser tests apply a real MCP mutation while the profile page is open.
They verify the clean update, reload persistence, dirty-draft recovery, and an acknowledgement during an in-flight save.
Fixtures cover concrete fields, dynamic candidates, and the separate MCP document.
Desktop and mobile scenarios use disposable profiles and causal event waits.
The browser never dispatches raw `mcp.*` WebSocket actions as a shortcut.

## Persistence and rollout

No new settings store, startup source, or runtime flag identity is required.
Domain adapters reuse current persistence and feature gates. Necessary atomic patch queries do not introduce a new persistence owner.
External compatibility tool names and successful argument shapes remain stable.
Compact updates and discovery ship together after their work-order checks pass.
The configuration profile switches each adopted operation to the compact group with the existing list-changed notification.
Public docs list all delivered domains and their specific limitations.
The package is incomplete while an eligible required domain remains not-yet-supported.
Domain adapters do not copy startup or runtime-flag registries.

## Related decisions

- [Domain-owned settings catalog](../../../decisions/2026-09-08-domain-owned-settings-catalog.md)
- [MCP argument validation](../../../decisions/2026-08-01-validate-mcp-tool-arguments.md)
- [MCP tool profiles](../../../decisions/2026-08-08-mcp-tool-profiles.md)
- [Utility dependency safety](../../../decisions/2026-08-08-utility-profile-dependency-safety.md)
- [Settings save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
- [Navigation boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)
