---
status: draft
system: agents
created: 2026-10-22
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-002
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-004
  - REQ-AGENTS-SANDBOX-005
  - REQ-AGENTS-SANDBOX-006
---

# Sandbox policy system design

## Ownership and baseline

Core owns admission, policy authority, process identity, cancellation, and persisted revisions.
The optional provider translates validated policy and provider evidence.
The [plugin contract](../../plugins/system-design/sandbox-provider-capability.md) owns negotiation and typed RPCs.
[Generic wrapping](generic-launch-wrappers.md) remains an independent core facility.
The [boundary decision](../../../decisions/2026-10-22-optional-sandbox-provider-boundary.md) records alternatives.

Source baseline is parent `fff43904827d8b44bf2a33439bce7fee022fc893`.
The [investigation](../../../plans/nono-sandbox-integration/investigation.md) distinguishes historical failures from current source and runtime experiments.
All new types, tables, capability names, and routes below are proposed. They do not exist yet.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-SANDBOX-001 | Core launch composition, Compatibility |
| REQ-AGENTS-SANDBOX-002 | Policy resolution, Authority, Persistence |
| REQ-AGENTS-SANDBOX-003 | Lifecycle ownership, Approval protocol |
| REQ-AGENTS-SANDBOX-004 | Reports and learning |
| REQ-AGENTS-SANDBOX-005 | Runtime endpoints |
| REQ-AGENTS-SANDBOX-006 | Operator surfaces, Acceptance |

## Core launch composition

Keep `internal/common/launchwrapper.Spec` and `Prepare` provider-neutral.
Keep `resolveSpawnCommand` inner-command admission and execution-host managed npm preparation.
A proposed `internal/agent/sandbox` service resolves policy and creates immutable attempt snapshots.
A neutral `internal/common/sandbox` envelope carries those snapshots to agentctl without importing backend-owned packages.

Composition is deployment guard, selected sandbox, existing profile prefix, then admitted child argv.
Core inserts each layer once. A provider returns sandbox prefix tokens and artifacts, never replacement child argv.
Ordinary prefix execution needs neither sandbox negotiation nor a plugin.
When structured sandbox policy is selected, a custom prefix runs inside that confinement.
Reject provider output that requests shell evaluation, detached ownership, arbitrary host writes, or unknown references.
Provider absence never removes the selected outer layer.

Integrate after final environment resolution in `hostutility/profile_probe.go`, lifecycle launch/utility paths, and agentctl utility/process launch paths.
Discovery uses an operation identity without creating a task session.
The cache key includes policy revision, resolved inheritance digest, runtime version, provider version, and executor identity.
Disposable paths and callback secrets do not enter cache keys or public projections.

## Policy resolution

Proposed executor binding contains `provider_id`, required mode, ceiling revision, eligible revision IDs, and default revision ID.
The executor mode is `disabled` or `required`. Optional describes plugin installation, not fallback after selection.
An absent reference means inherit, never disable. A required executor with no eligible default refuses launch.
Agent profiles, workflow steps, and tasks hold only optional revision references.
Resolve explicit task, workflow step, agent default, then executor default.
Task-free utility operations use agent and executor defaults only.
A policy-aware discovery request must identify its Local/Worktree executor context. Missing context fails rather than certifying an unrelated host default.
Once a higher-priority reference exists, invalidity fails admission. It never falls through to a weaker default.

The executor ceiling remains mandatory regardless of selection source.
Every imported revision receives a canonical permission representation and a resolved dependency graph.
Check filesystem access, network routes, privilege switches, environment effects, and runtime capability expansion against that ceiling.
Unknown provider policy fields or unprovable subset relationships are unsupported.
Do not use Nono `extends` as a restriction mechanism. Its list composition commonly adds grants.
Resolve inheritance, include implicit groups, and compare the effective result before authorizing it.
Freeze inherited contents and hashes. Do not follow mutable registry names at launch.

V1 permits exact path roots with read/write modes and explicit route/service grants.
Canonicalize paths on the execution host. Reject symlink escapes, overlapping protected roots, and unresolved paths.
Where the provider cannot enforce a nested exclusion, reject the policy instead of claiming subtraction from a broader grant.
Elevation can approve only operations inside an operator-approved expansion envelope, which itself stays within the executor ceiling.
A policy mutation requires a revision compare-and-swap and a human operator identity.

## Authority

Existing profile mutation handlers use `authz.ScopeOrgConfigManage`.
Synthetic identity currently receives broad single-user customization authority through `authz.SubjectFromGin`.
Preserve that existing generic-prefix behavior. Require a real authenticated human for structured enforcement management.
An auth-disabled install can use basic wrapping, but cannot activate the new operator-only enforcement workflow.
Reject agent and Office credentials even if a caller supplies a policy field through a generic task or workflow update route.
Validate imports, duplicate/copy operations, HTTP and WS handlers, draft probes, and task tools at their common service boundary.
Guard indirect changes through agent/executor switching and workflow movement as well as explicit policy fields.
An agent-triggered context change cannot weaken the captured policy. Preserve it or require operator selection before the next launch.
A trusted workflow transition can apply an operator-configured destination policy only through its recorded transition authority.

The sandbox child must not read Kandev configuration, database files, operator sessions, callback secrets, or provider control state.
Grant runtime directories individually. Never grant the whole real home, temporary root, or Kandev home.
Remove provider override variables, loader injection, and unapproved configuration roots from the supervised environment.
Treat agent profile environment as untrusted relative to the executor ceiling.
Reject `--allow-net`, `--sandbox-policy external`, and `NONO_ALLOW_NET` in required-policy mode.
Freeze the provider environment and configuration search roots before process creation.
Approved provider credentials remain a separate explicit grant. Do not copy them from retained test instances.

Plugins are trusted installed host code. The extension permission does not isolate a malicious plugin from host files.
An unsandboxed same-user host process and the host administrator remain outside this confinement claim.

## Lifecycle ownership

Agentctl's existing `process.Manager` and `InteractiveRunner` own the launch process and cancellation handle.
Use foreground `nono run`. Do not use `--detached` in v1.
The Nono supervisor owns its sandboxed child, optional inner PTY, audit state, and proxy.
Core owns the supervisor process group, operation directory, callback listener lease, and terminal transport.

ACP starts with pipes. Verified Nono 0.80.0 behavior preserves piped stdin and stdout without terminal decoration under `--silent`.
Native terminal starts through the existing outer Kandev PTY.
Nono may create an inner PTY when all descriptors are terminals.
Verify resize, input, signals, EOF, exit codes, and output draining across both PTYs before enabling terminal support.
No approval UI reads from the agent's terminal. Use the separate webhook channel.

Capture supervisor PID and process birth identity, child identity, attempt ID, Nono session ID, and policy digest.
Do not equate a user-chosen `--name` with Nono's generated session identity.
Learn the latter from a bounded private registry record and validate it against the owned process.
A callback arriving before this binding is complete remains denied.

Cancellation closes prompt admission, denies pending approvals, requests graceful supervisor termination, and drains output.
Escalate within the existing stop budget, then verify owned descendants and private listeners are gone.
Refresh descendant evidence during termination and verify PID birth identity before sending signals.
A browser disconnect affects only its UI connection.
Plugin loss disables new launches and pending approval grants. Static confinement stays active until core stops the owned process.
Do not let plugin removal delete artifacts needed by a running attempt.

Persist unresolved policy intent plus the immutable resolved revision and provider/runtime versions.
Recovery must bind the retained runtime to the same owner, generation, policy digest, and process identity before adoption.
Never adopt an unknown supervisor by name or replace it using mutable defaults.
Missing evidence produces recovery-required status with Stop available.
Automatic replacement reuses the snapshot. Explicit restart uses the reviewed current selection only after showing its difference.

## Approval protocol

Nono 0.80.0 implements the pinned [approval webhook v1](https://github.com/nolabs-ai/nono/blob/8bb041356d48f771a221e58c98d9b1ff442e76fe/docs/protocols/approval-webhook-v1.md).
Actual binary probes verified file-capability grant, denial, malformed decision, and timeout outcomes.
`inspect --json` is a report source, never an approval transport.

For initial Local/Worktree support, core creates a private loopback callback with a random per-attempt 256-bit route secret.
The URL is a bearer capability. Never log it, expose it in child argv/environment, or publish it in task metadata.
Nono reads it from a generated protected profile before sandboxing the child.
Place that profile beneath the operation's protected Nono state root and deny child access to that root and the callback listener.
Verify `/proc`, symlink, inherited-descriptor, and proxy-route attempts cannot expose the secret or call the listener.
If this isolation cannot be demonstrated, keep interactive approval unsupported. Unsigned loopback alone is not caller authentication.
Signed platform enrollment remains a possible later transport, not an assumed implemented dependency.

The webhook POST contains `backend` and a tagged `request` with request ID, session ID, child PID, and operation details.
Authenticate the route secret, then validate the bound attempt, Nono session, process ancestry, request type, and ceiling.
Reject requests over 64 KiB. Ignore caller-supplied authority and any attempt to select another session.
V1 enables only verified file-capability requests on Linux. Other request types fail with explicit unsupported status until separately proven.

Core persists a pending request before displaying it through the existing permission-card presentation.
Use a separate sandbox decision domain, so profile `auto_approve` and ordinary ACP permissions cannot resolve it.
Deduplicate by attempt and request ID. Reject a repeated ID with different content.
The operator endpoint requires authenticated configuration authority and the current request revision.
Return exactly `{"decision":"granted"}` or `{"decision":"denied"}` to Nono.
Use a 60-second maximum budget with a smaller remaining budget after transport overhead.
Cancellation, deadline, connection loss, or backend restart resolves pending requests as denied or expired.
A late click cannot grant. A delivered response is not proof that the child received it, so no automatic replay follows uncertain delivery.

## Runtime endpoints

Core supplies logical endpoint descriptors after actual runtime allocation.
Each descriptor binds service kind, task/session, attempt, scheme, address, port or socket, allowed methods, path, and expiration.
Policies select the logical service `kandev_mcp`, not an arbitrary URL string.
Existing `mcpconfig` strategies and passthrough artifacts supply the actual MCP transport.
A provider translates approved descriptors into explicit routes. It cannot invent destinations.

For HTTP MCP, expose an operation-owned MCP-only relay that validates method, route, and session authentication.
It forwards only the resolved MCP service. Settings, generic API paths, redirects, CONNECT, and other sessions remain inaccessible.
Grant only the relay endpoint through Nono's verified network proxy rules.
Do not grant the entire backend port as an unrestricted network destination.
The approval callback uses a separate port and remains unreachable to the child.
Keep ordinary MCP session credentials scoped to their existing tools. They never become policy-management credentials.

Verify DNS changes, direct-IP bypass, IPv4/IPv6 loopback, proxy bypass variables, redirects, and unrelated local ports.
On endpoint rotation, close the old relay and revoke its lease before authorizing a new attempt mapping.
An in-place rebind requires a provider-supported atomic rule update. Otherwise require restart with the same logical policy.
For stdio MCP bridges, allow only generated bridge/config artifacts and their exact upstream route.
Do not claim external MCP servers inherit confinement merely because the agent is wrapped.

## Persistence and learning

Core owns proposed tables `sandbox_policy_revisions`, `sandbox_policy_bindings`, `sandbox_attempts`, and `sandbox_approval_requests`.
Use the existing database abstraction and migrations for SQLite and PostgreSQL.
Store schema version, content hash, dependency hashes, source kind, operator ID, timestamps, ceiling identity, and selection source.
Attempt records contain non-secret endpoint intent and resource identities. Protected runtime files contain transient callback material.
Old rows without sandbox fields retain generic launch behavior. Downgrade cannot start an attempt marked sandbox-required.

Provider reports contain bounded typed denials, source offsets, finalization state, and integrity/completeness status.
Read private audit artifacts through core-owned paths. Reject foreign paths, malformed records, and oversized payloads.
Do not interpret `command_policy_summary` as a complete filesystem-denial list.
A provider that cannot observe a denial class reports that limit. It must not infer denials from arbitrary agent text.

Learning produces a proposed diff against an exact revision.
Core validates it against the ceiling and presents added access, inherited sources, and persistence scope.
An operator can create a workspace-scoped or installation-scoped revision.
Nono compiles immutable JSONC beneath the managed policy directory, outside agent-writable worktrees.
Repository profiles are import inputs only. They never become live authority through an agent edit.
Suppressions are presentation-only entries. A live approval never writes a policy.
Revision publication is atomic. Selection changes are separate authorized writes.
Revoked revisions block new attempts. Explicitly stopping already-running attempts remains an operator action.

## Operator surfaces

Use existing agent/executor profile settings and workflow/task editors for reference selection.
Show inherited source and ceiling summary beside the effective revision.
The permission card shows the denied operation, deadline, and one-operation decision.
Denial details and learning use a focused review surface with a diff and explicit persistence scope.

Desktop uses existing settings cards, selectors, and session cards.
Phone uses `MobilePickerSheet` for temporary selection and a full-height review surface for denial details.
Reuse `session-task-switcher-sheet.tsx` for fixed-header and safe-area behavior.
Use one internal scroll owner, dynamic viewport height, visible Back, and fixed decision controls.
Share state, authority, stale-response guards, and action handlers across presentations.
Maintain 28px desktop controls and at least 44px phone/coarse-pointer targets.
Use existing UI tokens. Any press motion uses 0.96 scale for 150ms and respects reduced motion.
Errors retain drafts. Empty lists explain missing provider or policy setup without enabling an unsafe default.
All copy uses localization across the seven maintained locales.

## Compatibility and observability

Initial support targets Linux Local/Worktree with verified Nono 0.80.0 and Pi ACP/native transports.
Other agents require their own transport evidence. macOS static policy support remains gated on actual Seatbelt tests.
Remote executors and Windows Nono remain unsupported. Host discovery cannot certify them.
Agentctl must advertise proposed `sandbox-launch.v1` before receiving required sandbox envelopes.
Old peers refuse required launches. Existing `launch-wrapper.v1` retains its meaning.

Emit bounded failure classes for policy resolution, provider availability, bootstrap authentication, approval timeout, and cleanup failure.
Task/session IDs and policy hashes belong in structured logs, not metric labels.
Never persist raw argv, callback URLs, provider credentials, or unfiltered stderr in public errors.

## Acceptance

[Work orders](../../../plans/nono-sandbox-integration/plan.md) map all criteria to exact tests.
Tests must pair allowed work with denied reads, writes, network routes, and policy mutation attempts.
A wrapper receipt proves invocation only. Kernel denial evidence proves only the tested resource and host configuration.
