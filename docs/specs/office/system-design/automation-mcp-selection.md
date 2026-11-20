---
status: current
system: office
created: 2026-11-04
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-001
  - REQ-OFFICE-AUTOMATION-MCP-002
  - REQ-OFFICE-AUTOMATION-MCP-003
  - REQ-OFFICE-AUTOMATION-MCP-004
---

# Automation MCP selection

## Ownership and current evidence

Office owns desired automation choices. Agents owns package catalogs, resolution, provider delivery, and accepted execution evidence.
Use the [decision](../../../decisions/2026-11-04-automation-mcp-selection.md),
[native package design](../../agents/system-design/builtin-mcp-management.md), and
[plugin-tool design](automation-plugin-tools.md).

Source baseline: `74abd2c92887`. Paths below are relative to `apps/backend` unless stated otherwise.
`Executor.PrepareTaskSessionMCPProfile` resolves Automation packages from a durable automation/run relationship.
Explicit selections use a v2 manifest. Legacy null selections keep the fixed Automation catalog.
The existing package builders serve selected optional tools. The dispatcher validates the same accepted authority.
The workspace resolver adds automation references to repository, profile, task, and session selections.
The editor owns selection drafts through the shared Save/Discard surface.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-OFFICE-AUTOMATION-MCP-001 | Saved selection, Native save, Workspace servers, Target modes |
| REQ-OFFICE-AUTOMATION-MCP-002 | Package authority, Configuration guard |
| REQ-OFFICE-AUTOMATION-MCP-003 | Execution binding, Workspace servers, Compatibility |
| REQ-OFFICE-AUTOMATION-MCP-004 | Editor, Run evidence, Validation |

## Saved selection

Store `mcp_selection` as JSON text with a `null` default and `mcp_selection_revision` as BIGINT, initially zero, to `automations` on SQLite and PostgreSQL.
Use a typed value with `builtin_packages` and `definition_ids` arrays.
A stored null means legacy/default behavior, not a corrupt explicit selection.
An explicit private selection contains `core` and zero or more optional package IDs.
Do not serialize endpoint configuration, credentials, installation grants, or observed health here.

Create/update/get projections and web types carry the value and revision.
An omitted update preserves the value. Request null is invalid.
`definition_ids: []` clears direct additions. `builtin_packages: ["core"]` removes optional private packages for future starts.
Reject unknown IDs, duplicates, missing Core, disabled/foreign definitions, malformed arrays, and managed-conversation selection writes.
Validate explicit normal-task packages against existing regular-task workspace policy.
Native profile MCP server writes are excluded from private automation Configuration because legacy profile fallback would bypass native grants.
Private choices do not inherit the setting labelled Allow Configuration for regular tasks.

Selection updates require `expected_mcp_selection_revision`. Compare and increment inside the automation write transaction.
Legacy creation uses revision zero. An explicit creation starts at revision one. Unrelated updates omit the selection and do not change its revision.
Validate current catalog references before the write. Compare the revision and validated task mode in the transactional UPDATE on both databases.
Every update uses the revision read by the service in its transactional compare-and-swap.
Selection updates and mode changes that clear a selection require the editor revision. Unrelated updates can conflict if another write changes the selection before their transaction commits.
Delivery resolves current definitions again and reports deleted or disabled references as unavailable.
MCP options reject more than 100 repository IDs before lookup.
Catalog usage counts scan automation references once per workspace response.
Never hold database locks over provider or MCP calls.
Catalog validation is optimistic across stores. Confirmed deletion can race a save and leave an unavailable reference.
Unconfirmed deletion never removes a definition, even when no reference is observed. Runtime resolution rechecks availability.
Return the exact committed selection. Conflicts return a typed conflict and preserve the editor draft.

Retain plugin selections independently. Target-mode changes clear incompatible explicit selections in the same save.
The UI explains this before Save. Switching between private and normal mode stages the destination's defaults for review.
Portable export omits both new fields. Configuration MCP creation and plugin schedules cannot set them.
All store projections, migration paths, retry snapshots, and export serializers need explicit regression coverage.

## Native save

Extend the native-intent pattern in `internal/automation/plugin_tools.go` without treating its context flag as wire data.
Only authenticated native create/update handlers can admit new selection values.
Agent MCP context and plugin Host schedule calls cannot establish intent.
Omitted fields can pass through existing unrelated edits.

Add a native workspace-authorized `automation.mcp_options` action and matching client helper.
Return shared package descriptors, workspace definition summaries, and inherited repository/profile origins for the draft's selected context.
The request includes only draft repository/profile identifiers. Validate their workspace and access before reading options.
Return no secret values. This action never starts a process or probes a server.
Do not hardcode package names or membership in React.

Use a narrow reader interface wired in `internal/backendapp` for automation selection at launch.
The reader validates the durable run/task relationship, workspace, owner, and target mode.
An arbitrary metadata `automation_id` or agent argument alone does not prove that relationship.
Read the desired selection once for a fresh private start and pass that immutable value through preparation.

## Package authority

Extend package eligibility and descriptors for Automation using existing package IDs and routes.
Core on Automation calls `registerAutomationTools`, not the ordinary task Core builder.
The other selected package builders remain shared. Remove surface-only rejection inside those builders where the trusted manifest supplies authority.
Audit tool registration, handlers, payload defaults, scope checks, and guidance for every selected package.

Extend `BuiltinManifest` with an automation-specific versioned binding containing automation ID and selection revision.
Keep existing v1 task bindings readable. Use v2 only for the new Automation binding.
Validate workspace, task, automation ownership, surface, catalog version, required Core, and package origins.
Use origin `automation` for optional private packages. Required Core keeps origin `required`.
Never accept this manifest from agent tool arguments.

Extend the existing action-to-package catalog and `guardedMCPDispatcher` authorization.
The permitted native action set is Automation Core plus the selected package actions.
The automation guard still validates resource references, attribution, and coordination self-target restrictions.
Top-level payload keys use canonical lowercase ASCII names so case-insensitive handler decoding, including Unicode case-fold aliases, cannot bypass these checks. Unicode values and nested user content remain valid.
Profile targets must belong to this workspace or remain accessible under existing shared-profile rules.
Do not globally bypass `authorizeAutomationRequest` when Configuration is present.
Existing plugin list/invoke actions continue through their dedicated selection checks.

Rich output and Walkthroughs bind authoring to the caller's hidden task and session through existing handlers.
Diagnostics uses its existing authorized context. Configuration uses its existing operation-specific handlers.
Each package must pass discovery plus successful invocation tests, not only tool-count assertions.
Package routes must reject cross-package calls and raw actions outside the accepted manifest.

## Configuration guard

Keep `Principal.Surface` as Automation. Carry the separate trusted package grant through existing built-in authority context.
Allow configuration operations only when their package is bound and the existing resource authorizer accepts the owner and target.
Workspace-owned resources stay in the automation workspace. Shared profiles, prompts, and executors retain existing shared-resource authorization.
Do not infer permission from a tool name, a read-only annotation, or a successful native save alone.

Audit configuration workflow, agent/profile, MCP, prompt, executor, settings, and automation handlers.
Reject package-policy keys, task-package preferences, automation selections, and plugin approvals through every generic and dedicated writer.
This includes nested payloads, create/copy/import paths, and writes targeting other automations.
Selection may not transfer through task creation, session spawning, or continuation metadata.
Task-local questions, automatic title ownership, and self-coordination do not become available through optional packages.

## Execution binding

Extend `executor_builtin_mcp.go` to resolve the trusted private selection rather than skipping Automation.
Use `PrepareMCPLaunch` and `PrepareMCPContinuation` as the existing preparation boundary.
The same accepted manifest drives routes, injection, dispatch, and `internal/sysprompt` guidance.
Bind before first start of a prepared helper, including pre-created workspaces.

Saving does not mutate a retained process. Reused turns keep its accepted manifest.
Fresh retry/replacement starts read current selection instead of an old retry snapshot.
A save racing with launch leaves the launched revision accepted and the new choice pending.
Validate ownership during backend adoption and retained helper recovery.
Preserve existing CAS protection against late execution metadata writes.

Declare helper capability `automation-mcp-selection-v1`, in addition to native package support.
Explicit automation package selection requires that capability before fresh start.
The capability covers automation-specific registration, manifest validation, and dispatch preservation.
An old helper must fail with upgrade guidance rather than launch a broader or incomplete catalog.
Legacy null selections retain the existing fixed path until an explicit selection is saved.

## Workspace servers

Reuse `mcpconfig.Resolver`, transport policy, secret resolution, and accepted setup reporting.
Do not introduce an automation MCP client or copy definition credentials into automation rows.

Add a backend-only typed additional-selection input to `ResolutionContext`.
It contains automation identity, selection revision, and definition IDs captured during launch preparation.
Add their `automation` provenance to the normal union of repository, profile, task, and session selections.
Extend both the transactional `ReadMCPResolutionInputs` path and the fallback `loadOrigins` path.
Resolve referenced catalog definitions in the existing catalog read snapshot.
Selection IDs remain stable references. Current definition revision and secret values resolve through the existing delivery contract.

Capture the direct automation contribution in safe accepted execution metadata.
Same-process preparation or third-party reconfiguration reuses that contribution rather than rereading changed automation choices.
Inherited selections and definition changes continue to follow their existing runtime rules.
At a fresh process start, capture the current direct contribution again.
Extend accepted setup comparison and provenance DTOs without introducing a separate health model.

Catalog usage and guarded deletion must count automation references through a narrow automation usage provider.
Do not duplicate direct IDs into task selection rows for private runs.
A normal remove is blocked while referenced. An explicitly confirmed force-remove deletes the definition. Automation references remain visible as unavailable until an explicit editor save.
The automation and catalog stores have separate ownership. Preserving references avoids a partial cleanup being reported as a successful configuration edit.
A definition that becomes unavailable still yields the existing safe resolution decision, never a name-based replacement.

## Target modes

Private runs resolve the current automation selection at each fresh process start, including repository-free scratch runs.
The current plugin live-admission policy is separate and unchanged.

Normal tasks apply the template once during creation, before any agent start.
Seed existing task MCP selections and built-in task preferences through their validated services.
Do not copy runtime manifests or native-intent flags.
A partial template application must prevent agent start and remain recoverable through existing dispatch/retry handling.
Retries must not create duplicate tasks or overwrite task choices after successful creation.
Reused normal tasks retain their task-owned choices. New tasks receive the latest template. Unavailable definitions are omitted from new task grants and recorded as creation-time resolution decisions in task metadata.
These diagnostics never authorize a server. Current resolution decisions take precedence when that definition is selected later.

Managed-conversation targets omit these controls and keep destination-owned policy.
They reject explicit selection payloads rather than silently ignoring them.

## Editor and run evidence

Extend `apps/web/components/automations/automation-editor.tsx` through small domain components and hooks.
MCP access appears after Then and before Settings, using the existing plugin section as the local exemplar.
Order: Built-in packages, Workspace MCP servers, Plugin tools.
Show Core as Required. Reuse the shared bordered package rows, status badges, detail header, and tool information from task/workspace pickers.
Each optional row has an explicit On/Off switch beside a separate button for package details.
The shared control also remains available in detail. Core shows Required without a switch.
`AutomationPackageChoice` serves both list and detail and maps checked state to membership in `builtin_packages`.
It uses the shared Switch primitive, without task inheritance state or reset actions.
A blocked selected package must remain removable, but a blocked unselected package cannot be enabled.
Preserve pending/read-only handling and target-mode defaults. Do not rewrite untouched selections.
AC-OFFICE-AUTOMATION-MCP-004.11 is implemented in [the switch plan](../../../plans/builtin-mcp-switches/plan.md).
Return focus to the package row when Back to packages closes its detail. Keep the Configuration authority notice visible.
Workspace servers use the existing searchable checkbox picker and explicit inherited origins.
Keep plugin approval state and its live application notice intact.

Use shared Save/Discard contributors and dirty treatment. Track new selection edits independently.
Omit untouched selection fields when options fail or another form field changes.
Preserve post-submit edits when an older save response arrives.
Mask stale option results after workspace/profile/repository changes.
Unavailable selected items remain visible with a Remove action.

The phone entry point remains workspace Automations > editor.
Use the existing full-page editor with inline groups and one page scroll owner.
The package list/detail exemplar is `components/mcp/builtin-package-list.tsx` and `builtin-package-detail.tsx`.
Reuse the same inline composition on phones. The plugin section retains its existing mobile composition.
A second drawer is unnecessary for these short groups. Long catalogs use the existing bounded searchable list within the editor.
Retain the settings save surface and safe-area clearance. Use 44px phone/coarse targets and 28px ordinary desktop controls.
Keep one draft/controller across responsive changes, keyboard focus, visible labels, and wrapped names.
Reuse tokens and Tabler icons. Avoid new animation. Any press motion uses better-ui's 0.96 scale, 150ms ease-out, and reduced-motion handling.

Use the existing MCP plug viewer for MCP inspection. Do not render a separate MCP access summary above the run conversation.
Keep accepted package/setup evidence in the shared session APIs. Unknown historical evidence stays Unknown.
Private changes say Applies at the next agent process start. Reused turns may still use the previous selection.
Normal-template changes say Applies to newly created tasks.
All copy uses seven language catalogs and generated Traditional Chinese.

## Compatibility and failures

| Path | Contract and verification | Unsupported result |
| --- | --- | --- |
| ACP/native structured injection | Separate package entries, valid routes, automation binding round trips | Fail explicit package start before dispatch |
| CLI/project-file injection | Existing adapter strategies preserve exact package identities and provenance | Actionable unsupported delivery, no fake connection state |
| Local, worktree, supported remote executors | Shared prepared binding and helper negotiation tests | Old helper rejects explicit selection |
| Retained legacy process | Existing fixed tools and unknown historical choices | No automatic restart or reconstructed grant |
| Workspace stdio/HTTP/SSE | Existing executor transport filtering and secret resolution | Existing unavailable decision or resolver error |
| Managed conversation | Existing destination policy | Reject automation selection writes |

Missing automation ownership, corrupt explicit data, or a binding mismatch blocks a fresh start.
A selection save failure preserves the draft. A delivery failure does not undo a successful save.
Caller cancellation ends that save or launch through existing context handling. No new background waiters or automatic retries are introduced.
Log selection revision, package IDs, and bounded failure reasons with existing execution identifiers.
Do not log secrets, endpoint headers, tool arguments, or full prompts. No new metrics are required.

## Validation and implementation records

The [plan](../../../plans/automation-mcp-selection/plan.md) maps each criterion to targeted tests and work orders.
Tests must prove actual optional-tool invocation, raw-call denial, Configuration grant-field denial, workspace isolation, and retained-process behavior.
Real SQLite/PostgreSQL persistence tests cover migration, atomic save, conflicts, and guarded definition deletion.
Desktop/phone E2E covers save, reload, all optional packages, workspace servers, stale choices, and run evidence.
Mock-agent evidence does not certify installed third-party provider connectivity.
Public behavior is documented in `docs/public/automation-and-mcp.md` and `docs/public/builtin-mcp-packages.md`.
