---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006
created: 2026-09-14
owners:
  - kandev
---

# Task change request MCP system design

## Purpose and boundaries

The integration system owns the shared contribution contract. Provider services
retain their persistence, credentials, automation algorithms, and events.
The MCP layer translates explicit agent intent into those existing services.
This design replaces the MCP surface after PR #3506. It describes the
implementation introduced for the planned 0.95.0 stable release.

## Requirement mapping

| Requirement suffix in `REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-*` | Design sections |
| --- | --- |
| 001, 003 | Management, Security and identity, Failure and recovery |
| 002 | Read contract, Capabilities and discovery |
| 004 | Automation targeting, Security and identity |
| 005 | Bound outcome reporting |
| 006 | Compatibility and migration |

## Verified baseline

PR [#3506](https://github.com/kdlbs/kandev/pull/3506) merged on
2026-09-14 at 14:00:36 UTC as `262ecdf270b5171538a68e9937dda7894ee01673`.
That commit was both HEAD and fetched `origin/main` during investigation.

The merged coordinator preserves same-identity no-op, same-provider replacement,
new-link-first ordering, and compensation only for newly created links.
Its tests cover stale unlink, workspace boundaries, host spoofing, and failed compensation.
GitHub detach persists tombstones and exact automation cleanup. GitLab unlink
removes exact automation state and publishes a workspace event after persistence.

`TaskCIOptionsPatch` supports one PR internally. The current MCP server handler
forwards only switches and prompt, so it cannot target that PR through MCP.
The GitLab MCP handler forwards `repository_id`, `project_path`, and `mr_iid`.
Both provider services keep five switches per association and a prompt per task/provider.

`orchestrator.Service.ReportTaskPRAutoFixOutcome` resolves the active turn and
calls the GitHub attempt store. GitLab has fix-attempt counters and checkpoints,
but no equivalent bound outcome reporter. A neutral name does not add that capability.

## Components and responsibilities

Existing source boundaries:

- `internal/mcp/server/server.go`: profile registration and tool schemas.
- `internal/mcp/server/handlers.go`: tool-to-backend dispatch and bound task identity.
- `internal/mcp/server/tool_argument_validation.go`: schema validation inside `wrapHandler`.
- `internal/mcp/handlers/handlers.go`: backend action registration.
- `internal/mcp/handlers/task_change_link.go`: association identity and principal checks.
- `internal/mcp/handlers/task_change_request.go`: shared read and automation DTOs and validation.
- `internal/backendapp/task_change_link_coordinator.go`: association operations and provider dispatch.
- `internal/backendapp/task_change_request_read.go`: provider-neutral read projection and capabilities.
- `internal/backendapp/task_change_request_automation.go`: explicit target preflight and provider fan-out.
- `internal/mcp/handlers/task_pr_automation.go` and `task_mr_automation.go`: existing provider service interfaces and events.
- `internal/mcp/handlers/task_pr_enrich.go`: existing task-list projections.
- `internal/orchestrator/ci_automation_attempt.go`: trusted active-turn resolution.
- `pkg/websocket/actions.go`: internal action names, independent from MCP tool names.

Paths in this section are relative to `apps/backend/`.
Add a small shared automation/read coordinator at the `backendapp` composition boundary.
Use concrete GitHub and GitLab adapters behind a narrow shared interface.
Keep the existing association coordinator and provider services.
Do not move provider tables, dedupe state, prompt defaults, or event ownership.

Add neutral backend actions and typed request/response DTOs for the four tools.
Keep provider DTOs behind the adapters. Share identity validation between operations.
Do not expose provider-specific request unions to agents.

## Schema rules

Use raw JSON Schema with object roots, `additionalProperties: false`, and nested
closed objects. Require nonempty strings and integers with minimum 1.
Declare every root property. Use enum values for providers and operations.
Reject null instead of treating it as omission.

Tool schemas stay within the portable subset defined in the
[MCP tool schema portability design](mcp-tool-schema-portability.md): the schema
root declares no `oneOf`, `allOf`, or `anyOf`. Operation-, target-, and
prompt-exclusivity constraints that the root cannot express are enforced in the
handlers before any backend side effect, not through top-level combinators.
`manage_task_change_request_kandev` rejects `old_*` fields on link and unlink and
requires the full old identity on replace. `update_task_change_request_automation_kandev`
rejects a mixed association/task target and rejects `auto_fix_prompt_override` on
an association target. Schema validation and handler validation must agree,
including direct backend calls. Backend authorization must not depend on
successful agentctl validation.

## GitHub caller identity

GitHub associations use the existing workspace-scoped personal-read resolver.
The user identity comes from trusted host context, never tool arguments.
An existing non-empty caller identity is preserved. When an in-session MCP
request has no identity, host wiring may supply the same synthetic single-user
identity used by HTTP requests only when the auth service explicitly reports
disabled mode. Setup, enabled, and unavailable auth state do not permit this
fallback. An explicitly present but empty identity is rejected.

This fallback is local to the GitHub association operation. It does not attach
an identity to all MCP dispatches or change task reach, repository validation,
provider credential selection, or the auth scope resolver's disabled-mode
contract. The coordinator consumes a host-provided identity resolver; auth mode
selection remains in backend composition. Link and replacement use the same
resolution path before provider work. Failed resolution leaves associations
unchanged.

This completes the technical path for
`AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1` while preserving `.2` and `.4`.
It follows the existing
[opt-in authentication decision](../../../decisions/2026-07-24-opt-in-authentication.md)
and [GitHub identity ownership](../../../decisions/0047-github-authentication-ownership.md).
Delivery is tracked in the
[single-user PR-link repair plan](../../../plans/mcp-pr-link-single-user/plan.md).

## Tool example

```json
{
  "task_id": "<task-id>",
  "provider": "github",
  "repository_id": "<repository-id>",
  "number": 3506
}
```

`link_task_pr_kandev` and `unlink_task_pr_kandev` accept this shape;
`replace_task_pr_kandev` additionally requires the old association triple
(`old_provider`, `old_repository_id`, `old_number`). See
[docs/public/automation-and-mcp.md](../../../public/automation-and-mcp.md)
for the operator-facing contract.

## Read contract

`get_task_change_requests_kandev` takes `{}`. It reads only the bound current task.
It returns this stable envelope:

```json
{
  "task_id": "task-1",
  "complete": true,
  "change_requests": [],
  "provider_settings": [],
  "provider_capabilities": [],
  "errors": []
}
```

Each `change_requests` item includes `provider`, `repository_id`, `number`,
`url`, `title`, `state`, `draft`, `base_ref`, `head_ref`, and `head_sha`.
State is `open | closed | merged | unknown`; preserve the provider state separately
as `provider_state` when normalization loses information.
Include nullable `merged_at`, `closed_at`, and `updated_at` where available.
An absent observation stays null or unknown; never invent a fresh provider read.

Each item includes `identity_status: resolved | unresolved | ambiguous`, the five
`automation` switches, and `capabilities`. Missing settings are null on read failure.
Default false is valid only when the provider service confirms its default.
An unresolved historical `repository_id` is null, never a guessed canonical identifier.
Duplicate canonical triples with different GitLab paths are marked ambiguous.
All mutations on an ambiguous triple fail before selecting a row.

`provider_settings` has one item per available provider: `provider`,
`auto_fix_prompt_override`, `effective_auto_fix_prompt`, `using_default_prompt`,
`auto_fix_max_rounds`, and `prompt_scope: task_provider`.
Return available automation state such as round count and last error in a
provider-labelled `automation_status` object. Do not rename opaque internal checkpoints into shared semantics.
Lifecycle prompt text stays excluded. Do not combine different provider prompts or aggregate switches into misleading booleans.

An installed provider read failure sets `complete: false` and adds a sanitized
provider error. Successful provider results remain usable. An unconfigured provider
is marked unavailable rather than represented as an empty successful read.
The read is a persisted snapshot; it does not trigger provider synchronization.
Task-list `change_requests` and GitHub-only `prs` retain their existing shapes.

## Management

`manage_task_change_request_kandev` takes the existing flat identity fields plus
`operation`. `task_id` remains explicit and can identify another task in the caller workspace.

```json
{
  "operation": "replace",
  "task_id": "task-1",
  "provider": "gitlab",
  "repository_id": "repo-new",
  "number": 42,
  "old_provider": "gitlab",
  "old_repository_id": "repo-old",
  "old_number": 7
}
```

Link and unlink forbid all `old_*` fields. Replace requires all three and equal
providers. Preserve `TaskChangeLinkRequest` and dispatch to its three service methods.
Return `task_id` and `links` containing the resulting canonical triples.
This preserves the merged association response without requiring a second automation read.

Reuse `taskChangeLinkCoordinator.ReplaceTaskChange` and its compensation order.
Validate operation and cross-provider mismatch before mutation.
An identical old/new identity returns current links without detachment.
An absent old link preserves existing idempotent behavior: establish the new link.
A new link that already existed is never compensation-owned by this request.

## Automation targeting

`update_task_change_request_automation_kandev` takes exactly `target` and `patch`.
Task, session, and caller identifiers are forbidden public arguments.

Association target:

```json
{
  "target": {"scope": "association", "provider": "github", "repository_id": "repo-gh", "number": 3506},
  "patch": {"auto_fix_enabled": false}
}
```

Task target:

```json
{
  "target": {"scope": "task", "providers": ["github", "gitlab"]},
  "patch": {"prompt_on_merged": true, "auto_fix_prompt_override": "Run the focused checks before pushing."}
}
```

Task `providers` is required, nonempty, unique, and drawn from the supported enum.
It is a deliberate selection, not an omitted identity or a wildcard that changes
meaning when another provider is added. A single provider selects all current
links for that provider. Both values explicitly select a mixed task.

`patch` allows these optional fields and requires at least one:

| Field | Type | Association target | Task target |
| --- | --- | --- | --- |
| `auto_fix_enabled` | boolean | Exact link | Current links of selected providers |
| `auto_merge_enabled` | boolean | Exact link | Current links of selected providers |
| `prompt_on_review_requested` | boolean | Exact link | Current links of selected providers |
| `prompt_on_merged` | boolean | Exact link | Current links of selected providers |
| `prompt_on_closed` | boolean | Exact link | Current links of selected providers |
| `auto_fix_prompt_override` | string | Forbidden | Task/provider prompt; empty clears |

A task prompt applies to present and future links for that provider. Switches
apply only to links present at the provider transaction. They do not become defaults.
Prompt-only updates may select an available provider without linked contributions.
A switch patch requires at least one link for every selected provider.
For a mixed prompt/switch patch, an empty selected provider fails preflight for the entire request.

Preflight checks the full request, principal, provider availability, capabilities,
link identities, and known credential prerequisites before any write.
Each provider service still revalidates authoritative membership and credentials.
Task fan-out uses each provider's existing transaction and membership checks.
A link added between provider transactions may be included only in that provider's write;
return actual affected identities rather than claiming a cross-provider snapshot.

Apply selected providers in lexical order (`github`, then `gitlab`). Stop after
first runtime failure. Keep each provider's existing atomic prompt/switch transaction.
Do not add cross-store transactions or reverse successful automation writes;
reversal could reset checkpoints or race concurrent user changes.

Return `status: applied | partial | failed`, provider results with
`status: applied | failed | not_attempted`, affected identities, and resulting
settings where known. A failed provider readback uses `state_known: false`.
A write followed by failed readback must not appear as a guaranteed rolled-back write.
Return an MCP error result for partial/failed requests while preserving this structured
payload in text as well as structured content. Never discard it at the WS adapter.
The caller reads state and retries only failed/unattempted targets.

## Security and identity

Derive caller task, session, and workspace from `mcpscope.PrincipalFromContext`.
Recheck target task workspace in backend handlers. Association operations retain
same-workspace reach; read, automation, and outcome operations retain current-task binding.
Reject injected public identity fields. Internal envelope identity must match the principal.
Retain live permission/profile checks and provider-origin validation for direct calls.

Link and replacement's new link require the repository attached to the target task,
its workspace match, and verified provider origin. Keep workspace-scoped GitHub
user credentials and GitLab credentials in the existing services.
Never construct a provider URL from an agent-supplied host or project path.

Unlink resolves the exact stored association without requiring the repository to
remain attached. This preserves stale removal and avoids a new remote dependency.
For GitLab automation, resolve `ProjectPath` from the unique stored `TaskMR` selected
by the canonical triple. Verify it against stored origin/repository evidence.
If a legacy row lacks repository identity, resolve only one verified workspace
repository matching its host and full project path. Otherwise report unresolved identity.
Do not persist guessed backfills or select by number alone.

## Capabilities and discovery

Expose provider support and current availability separately. Capabilities use
`link`, `unlink`, `replace_same_provider`, `auto_fix`, `auto_merge`,
`lifecycle_notifications`, `custom_auto_fix_prompt`, and `auto_fix_outcome_reporting`.
Both implemented providers support the first seven. Only GitHub supports the last.
Per-association eligibility can be false for unresolved identity, stale link
operations, unavailable services, or credentials. Include a stable reason code.
Feature support does not promise successful merge or provider access.

Retain backend-owned provider union, launch propagation, dynamic registry rebuild,
and tool-list-changed notification from the provider-scoped discovery ADR.
Register the first three tools once in task mode when GitHub or GitLab is present.
Register the outcome tool only when GitHub is present. Local/unknown-only tasks
receive none of these tools. Other modes retain their existing exclusion rules.
A same-workspace association target still receives independent backend checks.
Stale cleanup remains callable through the registered backend action and existing
UI even when a local-only task catalog hides contribution tools.

A later provider adds one adapter and capability entry, not another tool family.
A provider with different capabilities must reject unsupported patches explicitly.

## Bound outcome reporting

`report_change_request_auto_fix_outcome_kandev` accepts only:

```json
{"outcome": "action_taken", "summary": "Pushed a fix for the failing integration check."}
```

Preserve the existing handler principal check and orchestrator active-turn lookup.
Route only to the provider of a server-bound unresolved attempt. Today this path
has one implementation: GitHub. Do not accept a provider selector or fall back
from an unmatched GitLab turn to an arbitrary GitHub association.

Keep first-write/replay handling, attempt identity, feedback signature, provider
progress confirmation, retry rules, and round caps in the existing GitHub service.
Preserve the unmatched-turn message and its instruction not to retry or enable auto-fix.
Rename the tool reference in the immutable server-owned outcome protocol.
Keep structured hidden context and passthrough terminal context equivalent.
Manual review, sibling messages, and copied old instructions do not establish an obligation.

## Failure and recovery

Association errors preserve committed state. Add a narrow typed result at the
coordinator/handler seam where current joined errors cannot carry machine-readable state.
Report `operation_error`, `rollback_error`, `links`, and `state_known` on replacement failure.
Sanitize provider errors and log full causes server-side.
When final listing fails after a committed mutation, report uncertain readback rather
than implying that no mutation occurred. Do not automatically repeat replacement.

Keep GitHub tombstones, exact automation retirement, GitLab refresh-watch cleanup,
and committed provider deletion events. Event publication failure does not undo
persistence; preserve existing provider logging and recovery behavior.
No new database schema, cross-provider transaction, or metrics family is required.
Log operation, provider, failure stage, and compensation status without prompt bodies or credentials.

## Compatibility and migration

The released catalog contains no legacy aliases. The SDK's `WithToolFilter`
also filters invocation, so it cannot safely implement callable hidden aliases.
Do not fork the protocol or add a second registry to hide these eight names.

Remove these tool registrations in the same release as caller migration:

- `link_task_pr_kandev`, `unlink_task_pr_kandev`, `replace_task_pr_kandev`.
- `get_task_pr_automation_kandev`, `update_task_pr_automation_kandev`.
- `get_task_mr_automation_kandev`, `update_task_mr_automation_kandev`.
- `report_pr_auto_fix_outcome_kandev`.

Old names on a new runtime return the normal unknown-tool result without mutation.
Document rediscovery and session resume with the new runtime. Do not claim that
registry refresh updates the executable of an already-running old agentctl.

Retain existing backend WS action handlers for one release transition to serve
old agentctl processes. These are internal transport compatibility, not new MCP aliases.
They keep their original provider scope and response shape. Do not translate omitted
legacy identity into a mixed-provider update. Apply trusted principal checks there too.
Remove these compatibility handlers in the first stable release after the
planned 0.95.0 release that introduces the neutral tools. The removal obligation
is recorded in the implementation plan.
Old agentctl processes must resume on a current runtime before that removal release.

Migrate `config/workflows/pr-review.yml`, its loader tests, and
`internal/orchestrator/event_handlers_github_ci_automation.go` with its protocol tests.
Search `.agents`, built-in prompt sources, public docs, and all runtime source for old names.
The inspected `.agents` skills contain no matches; repeat the scan at implementation.
Preserve edited workflow/prompt records. Use existing guarded seed refresh patterns
only where a persisted built-in has a known historical value.

Select the outcome tool name from the current execution's Kandev tool observation.
The existing `SessionMetaKeyMCPAttachmentState` and `LoadMCPAttachmentHistory`
provide execution-scoped tool evidence. This evidence selects wording only, never authorization.
Ignore previous attempts and foreign MCP servers. Prefer the neutral name when present.
If only the legacy name exists, retain legacy wording for that old runtime.
If current evidence is missing, leave dispatch retryable without consuming a round
or binding an attempt to an undelivered prompt. Retry through the existing dispatcher,
not a new polling loop. Surface the missing-catalog reason in existing automation errors.

Queued trusted protocol blocks must use the new name at delivery on a new runtime.
Do not rewrite user text or stored transcripts. An already-delivered old block
can remain in history; the new catalog and resume guidance identify the replacement.
Test an old binary against retained actions and a resumed session against the new catalog.
Do not emit instructions to a tool absent from that runtime's catalog.

`docs/public/automation-and-mcp.md`, `docs/public/integrations.md`, and
`docs/public/coverage.json` were updated during implementation and describe the
neutral catalog after cutover.

The provider-aware runtime requirement's old tool-name criteria were marked
superseded by this contract's 002.2 at cutover, preserving the backend union and
transport requirements.
Replace duplicate legacy discovery prose with a link to this design.
The existing explicit-outcome ADR remains authoritative for attempt semantics;
only its tool name changes through the new ADR.

## Related decisions and implementation

- [Neutral contribution contract decision](../../../decisions/2026-09-14-provider-neutral-change-request-mcp.md).
- [Provider-scoped runtime](../../../decisions/2026-08-03-provider-scoped-task-mcp-tools.md).
- [Explicit outcomes](../../../decisions/2026-09-06-explicit-pr-auto-fix-outcomes.md).
- [Dual-era MCP](../../../decisions/2026-08-30-dual-era-mcp-protocol.md).
- [Implementation package](../../../plans/provider-neutral-change-request-mcp/plan.md).

## Implementation record

The neutral catalog and caller migration are implemented for the planned
0.95.0 stable release. The legacy backend WebSocket actions remain during that
release transition and are scheduled for removal in the first stable release
after 0.95.0.
