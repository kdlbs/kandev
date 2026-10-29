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
  - REQ-AGENTS-SANDBOX-007
  - REQ-AGENTS-SANDBOX-008
---

# Sandbox policy system design

## Ownership and baseline

Core owns admission, policy authority, process identity, cancellation, and persisted revisions.
The optional provider translates validated policy and provider evidence.
The [plugin contract](../../plugins/system-design/sandbox-provider-capability.md) owns negotiation and typed RPCs.
[Generic wrapping](generic-launch-wrappers.md) remains an independent core facility.
The [boundary decision](../../../decisions/2026-10-22-optional-sandbox-provider-boundary.md) records alternatives.

Source baseline is `main` at `1eb53576f496759b58ae73d4cba7bd96cbc91288`.
PR #4538 merged as `76c9ada7baaa6b15bf814aec2ef093cc7dc07610` and supplies core wrapping.
The [investigation](../../../plans/nono-sandbox-integration/investigation.md) distinguishes historical failures from current source and runtime experiments.
Policy storage, authority, plugin RPCs, and runtime wiring use a default-off flag. Operator UI and product acceptance remain in progress. Work orders record evidence and open gates.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-SANDBOX-001 | Core launch composition, Compatibility |
| REQ-AGENTS-SANDBOX-002 | Policy resolution, Authority, Persistence |
| REQ-AGENTS-SANDBOX-003 | Lifecycle ownership, Approval protocol |
| REQ-AGENTS-SANDBOX-004 | Reports and learning |
| REQ-AGENTS-SANDBOX-005 | Runtime endpoints |
| REQ-AGENTS-SANDBOX-006 | Operator surfaces, Acceptance |
| REQ-AGENTS-SANDBOX-007 | Execution settings, Projection and evidence |
| REQ-AGENTS-SANDBOX-008 | Execution details, Projection and evidence, Session indicator |

## Core launch composition

Keep `internal/common/launchwrapper.Spec` and `Prepare` provider-neutral.
Keep `resolveSpawnCommand` inner-command admission and execution-host managed npm preparation.
`internal/agent/sandbox` resolves policy and captures immutable attempts.
`internal/common/sandbox` carries them to agentctl without backend imports.

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

Executor bindings select a provider, required mode, global ceiling/default, and eligible revisions. A global executor can admit operator-accessible workspace revisions; selection stays bound to that workspace.
The executor mode is `disabled` or `required`. Optional describes plugin installation, not fallback after selection.
An absent reference means inherit, never disable. A required executor with no eligible default refuses launch.
Agent profiles, workflow steps, and tasks hold only optional revision references.
Resolve explicit task, workflow step, agent default, then executor default.
Task-free utilities inherit agent/executor defaults and omit task MCP. Capture reduced runtime grants without changing saved intent. Task sessions require a scoped MCP forwarder.
Policy-aware discovery requires its Local/Worktree executor context. Refuse missing context.
Once a higher-priority reference exists, invalidity fails admission. It never falls through to a weaker default.

The executor ceiling remains mandatory regardless of selection source.
Every imported revision receives a canonical permission representation and a resolved dependency graph.
Check filesystem access, network routes, privilege switches, environment effects, and runtime capability expansion against that ceiling.
Unknown provider policy fields or unprovable subset relationships are unsupported.
Do not use Nono `extends` as a restriction mechanism. Its list composition commonly adds grants.
Resolve inheritance, include implicit groups, and compare the effective result before authorizing it.
Freeze inherited contents and hashes. Do not follow mutable registry names at launch.

V1 permits exact path roots with read/write modes, logical `work_dir` and `agent_state` roots, and explicit route/service grants.
A filesystem entry contains an absolute `path` or logical `reference`, never both. Compare logical roots by identity and access mode. Core resolves `work_dir` from its owned workspace and checks protected roots. Missing or unsafe directories refuse launch. Attempts store immutable intent and resolved grants.
The `agent_state` grant binds a child home apart from supervisor state. Its scope includes organization, workspace, session, agent profile, provider installation, and key. Empty organization means organizations are disabled. Freeze scope in the attempt, retain files across attempts, and lock concurrent use. Require the same logical ceiling grant. Reject missing ownership, symlinks, closed leases, and literal protected paths. Deletion checks all attempts before removal, preserves siblings, and refuses uncertain cleanup. Archive and retained-workspace jobs preserve state.

Core supplies child home/config/state/temp paths as runtime references, without credentials. Providers bind the child environment separately from the supervisor. Never copy host home contents or operator credentials. No state grant means no host-home access. Utility state has a disposable execution scope.

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
The provider contract declares override variables and arguments that conflict with required policy.
Core enforces these declarations without matching provider IDs or embedding a Nono flag list.
The Nono plugin must reject overrides such as `--allow-net`, `--sandbox-policy external`, and `NONO_ALLOW_NET`.
Freeze the provider environment and configuration search roots before process creation.
Provider credentials need explicit grants. `environment_keys` contains sorted, unique uppercase names, never values or patterns. Each key must fit the executor ceiling and immutable revision. Core rejects configuration roots, loader/startup injection, and Kandev control keys. Providers reject their override keys. Child environments use approved keys plus the fixed baseline. Never copy retained-instance credentials.

Plugins are trusted installed host code. The extension permission does not isolate a malicious plugin from host files.
An unsandboxed same-user host process and the host administrator remain outside this confinement claim.

## Lifecycle ownership

Agentctl's existing `process.Manager` and `InteractiveRunner` own the launch process and cancellation handle.
Use foreground `nono run`. Do not use `--detached` in v1.
The Nono supervisor owns its sandboxed child, optional inner PTY, audit state, and proxy.
Core owns the supervisor process group, kernel descendant container, operation directory, callback listener lease, and terminal transport.

On Linux, create an owned cgroup v2 leaf under the execution host's delegated cgroup. Start with `clone3(CLONE_INTO_CGROUP)`. Record path, boot ID, and inode in the private snapshot. Refuse unsupported kernels or delegation without fallback. Never move the host or change controllers. Ordinary wrappers remain independent.

Deny grants and approvals for cgroup controls. Verify supervisor and child membership. Process sessions alone cannot track native PTYs or detached children. After supervisor exit, validate the captured kernel identity, terminate its cgroup, and require `populated 0` before removing artifacts. Missing or replaced ownership requires recovery. Never signal sibling services.

ACP starts with pipes. Verified Nono 0.80.0 behavior preserves piped stdin and stdout without terminal decoration under `--silent`.
Native terminal uses the outer Kandev PTY. Nono may add an inner PTY for terminal descriptors.
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

Persist policy intent, resolved revision, and provider/runtime versions. Adoption requires matching owner, generation, policy digest, and process identity. Names and mutable defaults prove nothing. Missing evidence requires recovery with Stop available.
Callback and egress leases end on detachment. Graceful shutdown stops sandbox executions despite agent survival. Before reconstructing commands or replaying prompts, recovery reads private attempts. Lost callback ownership requires bounded stop, not ordinary adoption. Replacement requires proof that the captured supervisor and kernel container ended; missing supervisor birth evidence blocks replacement and preserves private files.

Bound and authenticate startup receipts. `not_started` proves no spawn. `terminated` requires matching attempt, generation, policy, supervisor identity, and empty kernel container. Missing or mismatched receipts prove neither. Keep replay tombstones until file removal; reject excess receipts before dispatch. Public errors omit receipts.
Automatic replacement retains policy, provider package, and work directory. Manual restart reviews captured/current approved policy before stop. Current policy requires a browser operator, previous attempt, server-derived scope, and a digest of revisions, ceilings, and binding versions. Recheck admission, compilation, and atomic capture; reject stale choices. Capture operator, time, and digest before dispatch; consume the review before recovery. Admission can advance the private task guard, never saved policies. Phone uses a full-height view, fixed actions, and one scroller. Cancel sends no stop.

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
Existing `mcpconfig` strategies supply the MCP transport.
Pi MCP uses isolated child state without project trust. `PI_CODING_AGENT_DIR` must stay inside that home. Refuse missing state. Never copy credentials or change project trust.
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

Core owns `sandbox_policy_heads`, `sandbox_policy_revisions`, `sandbox_policy_bindings`, `sandbox_task_guards`, `sandbox_attempts`, `sandbox_reports`, `sandbox_learning_proposals`, and `sandbox_approval_requests`. Attempt snapshots freeze the resolved policy, ceiling, provider package, runtime digest, launch envelope, and resolved filesystem grants. Versioned lifecycle updates cannot rewrite those snapshots. A running lifecycle state records an owned process; it is not an active-confinement claim. Terminal attempts reject late running events.
Task guards retain the last admitted permission limit across context changes and service reloads. Compare-and-swap writes reject concurrent admission against a stale limit. Child admission checks the captured parent limit. These guards are policy authority records, not evidence that a process is sandboxed.
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
Suppressions hide matching denial kind, operation, and target in the current attempt view. Keep evidence unchanged and provide Show hidden denials. Closing the view clears these local entries. A live approval never writes a policy.
Revision publication is atomic. Selection changes are separate authorized writes.
Revoked revisions block new attempts. Explicitly stopping already-running attempts remains an operator action.

## Operator surfaces

Agents owns these surfaces because the underlying contract describes launch configuration and runtime evidence.
Tasks supplies session identity and presentation placement. Its lifecycle state remains independent of sandbox status.
Use generic settings and session details, without a security dashboard or provider-specific pages.
[Runtime identity display](runtime-identity-display.md) owns settings runtime information. It proves nothing about an existing session.

### Execution settings

`ProfileFormFields` places `CommandPrefixField` in Execution, with Command wrapper and Sandbox groups.
Preserve `command_prefix` storage and save/dirty behavior. Do not duplicate the editor in Advanced.
Raw launch diagnostics remain an initially closed disclosure. The command preview shows composition, not a shell command to execute.
Use the existing `launchwrapper` parser for validation and reserved references.
Show unresolved work-directory/runtime references symbolically until the operator selects execution context.
For diagnostic previews, show command identities and typed safe values only. Replace opaque arguments with an omitted-arguments label.
Do not interpolate inherited environment values or secrets into preview text, logs, telemetry, or session responses.

Sandbox displays the provider inherited from the executor, policy default selector, capability health, and permission summary.
Its sections are Readable locations, Writable locations, Network, Kandev MCP, and Mandatory restrictions.
Use canonical grants and declared capabilities from the validated provider contract.
Do not inspect command text for provider names or infer grants from arbitrary flags.
A custom prefix remains independent and runs inside structured confinement when both are configured.
No plugin shows an explanatory empty state while leaving the core wrapper editor available.
An installed but unavailable provider preserves its selection and shows the exact blocker.

Agent profiles can apply to multiple executors. Require an executor context to resolve availability, eligible policies, and the ceiling.
Before context selection, show the stored agent default and unresolved inherited provider without a health claim.
Executor selection in this preview does not assign the agent to that executor or change stored defaults.
An explicit Check availability action performs a bounded probe. Opening settings does not install software or start an agent.
`POST /api/v1/sandbox/availability` requires operator authority and a five-second deadline. It checks the saved selection and runtime artifact, then rechecks installation and binding identities. Return only availability and a stable blocker, never paths or provider errors. This proves no confinement. Context or binding changes invalidate the displayed result. Unsaved drafts hide it.
Reuse executor profile settings for provider and ceiling authority, and workflow/task editors for revision selection.
Show inherited source and ceiling restrictions beside each selector. An absent selection means inherit, not disable.
Existing authorization gates apply to all saves. Read-only users can inspect a permitted summary but cannot modify configuration.

### Projection and evidence

Capture an `ExecutionSnapshot` for every agent launch, including core wrappers and unwrapped agents.
Capture wrapper source and digest, safe command identities, executor context, selection source, and the resolved policy before process creation.
Persist immutable session, attempt, and generation identity.
Sandbox snapshots contain provider/runtime identity and policy digest. Only verified evidence supplies the mechanism.
Do not persist secret argv in this public-facing snapshot. Keep private launch material separate.
Legacy sessions without snapshots receive unknown history rather than data reconstructed from mutable profile settings.

Core projects execution details through authenticated session reads and WebSocket updates.
The DTO includes the snapshot, observed status, evidence timestamp, provider-service health, report completeness, and next-launch selection.
Next-launch selection is a separate current-policy resolution. A failed resolution leaves the recorded attempt readable.
Use existing session access checks. Shared/read-only surfaces receive only their permitted redacted fields and never operator actions.
No new unauthenticated diagnostics endpoint is permitted.

Provider runtime evidence uses the versioned `NormalizeSandboxStatus` contract.
Core reads only declared private artifacts and binds normalized evidence to its owned process identity, generation, and policy digest.
A provider's availability declaration or self-reported boolean cannot create active status.
The provider adapter must have conformance evidence for its host, mechanism, and transport before core accepts its evidence schema.
The initial Nono adapter's evidence source remains a Task 01 runtime gate. Missing reliable evidence yields unknown, not inferred active.
Do not insert a Nono-specific process or JSON parser into the core projection.

| Status | Entry condition | Exit or invalidation |
| --- | --- | --- |
| Not configured | Current known snapshot has no selected sandbox | New attempt with a selected policy |
| Configured | Required policy resolved, no process launch yet | Admission starts or fails |
| Starting | Current attempt launched, confinement evidence pending | Accepted evidence, launch failure, stop, or lost evidence |
| Active | Accepted current confinement evidence and live owned process tree | Stop, generation change, or uncertain evidence/ownership |
| Failed | Required launch or enforcement verification failed | Explicit new attempt |
| Unknown | Missing, stale, incompatible, or unbound evidence | Fresh accepted evidence or confirmed terminal outcome |
| Stopped | Core confirmed the attempt's process tree ended | New attempt only |

One core observer reads bounded pages of current sandbox attempts with bounded concurrency. Capture wakes it; a ten-second tick refreshes evidence. No per-tab polling.
Bound provider verification to a five-second call deadline and a 30-second maximum evidence freshness window.
Active status requires a live ownership observation and valid confinement evidence within that window.
Set unknown when the window expires, observation fails, or a required transport disconnects. Never extend freshness from a cached response.
Persist the last evidence for history, but invalidate active status across backend restart until recovery rebinds the live attempt.
The browser marks live status unknown on WebSocket loss and waits for a fresh server projection after reconnect.
Order updates by session ID, attempt generation, and monotonic projection revision. Reject stale and foreign updates.
Ignore a late active event after a terminal observation for the same attempt.

Keep provider-service health separate from confinement state.
If the plugin exits, existing kernel confinement may persist. Core can retain active only with still-valid independent runtime evidence.
Otherwise show unknown and provider unavailable, preserve history, deny new decisions, and retain Stop.
Changing the sandbox status never synthesizes task completion or resolves a launch-recovery error.

### Execution details

Use the same detail model from the session header, Dockview tab action, and mobile session status control.
Desktop opens a focused dialog with fixed title/close controls and one scrolling body.
Order the content as current status and observation time, This attempt, Protection scope, Next launch, then Approvals and Denials.
This attempt shows the captured wrapper, provider/runtime versions, immutable policy revision, and selection source.
Next launch shows current defaults and a diff when they differ. Configuration changes do not silently restart the session.
Protection scope names the agent process tree and the actual mechanism. List user shells, setup, and external MCP services as separate boundaries.
Kandev MCP indicates a narrowly allowed route, not confinement of the service itself.
Stopped attempts show historical status and time without a live shield.

The permission card shows the operation, deadline, and one-operation decision.
Details links to that same approval record instead of creating duplicate decision state.
Denial details and learning share the existing report/revision model with a diff and explicit persistence scope.
Absent reports say unavailable or unsupported, never zero denials. Provider failure preserves the configuration and any recorded report.

### Session indicator

Integrate a small button beside the title in `session-tab.tsx`. Keep task, session, and attempt identities distinct.
Use Tabler icons from the existing surface: neutral wrapper, shield for active confinement, and warning for a required failure.
Use a neutral status variant for configured, starting, unknown, and stopped. Do not reuse an active shield outline for these states.
When wrapping and confinement coexist, show one sandbox-status icon. Details describes both layers.
Sessions with neither can omit the decorative indicator but retain an Execution details menu/header action.
The accessible name includes the session and status. A tooltip supplements it but is not the only access path.
Activation opens details and does not close, rename, maximize, or initiate a drag of the tab.
Preserve tab keyboard navigation and focus return after closing details. Avoid a nested button within another button.
On coarse-pointer desktop/tablet layouts, use a 44px hit target or the existing touch action surface without overlapping tab actions.
Do not aggregate one session's shield into a task-wide protection claim or add badges to every board card.

### Phone composition and accessibility

Phone settings retain the same group order in one column with the page as scroll owner.
Use `MobilePickerSheet` for policy and executor choices. Return to the same draft after selection or Back.
Add a labelled Execution status control for the selected session in `session-mobile-top-bar.tsx` and the session picker actions.
Open a full-height detail surface with fixed title/Back, one scrolling body, and safe-area-aware actions.
Approval and learning open a next view within that surface, not a stack of drawers.
Reuse `session-task-switcher-sheet.tsx` for fixed-header and safe-area behavior.
Use one internal scroll owner, dynamic viewport height, visible Back, and fixed decision controls.
Share state, authority, stale-response guards, and action handlers across presentations.
Maintain 28px desktop controls and at least 44px phone/coarse-pointer targets.
Long policy names, provider names, and paths wrap within the surface. Browser zoom and keyboards must not hide actions.
Include keyboard, screen-reader, forced-colors, and reduced-motion checks. Do not rely on color or animated state alone.
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
