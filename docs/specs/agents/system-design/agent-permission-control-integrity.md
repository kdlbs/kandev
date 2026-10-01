---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-006
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
---

# Agent Permission Control Integrity System Design

[Explicit Auggie recovery](explicit-resume-settings.md) adds an attempt-scoped omission path. Ordinary mode application and confirmation below remain strict.

## Purpose and boundaries

This design covers the three profile permission controls (`cli_flags`, `mode`,
`auto_approve`) from the point where a profile is saved to the point where the
agent process observes the control. It owns the Kandev side of that path only.

It does not own the installed agent CLI's own permission classification. Which
commands a provider treats as requiring approval, and which requests remain
bypass-immune in a permissive mode, stay with the provider. This design makes
Kandev's own layer honest and observable so a provider decision is attributable
to the provider rather than to an unverifiable Kandev claim.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001` | [CLI flag destination](#cli-flag-destination) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002` | [Initial mode delivery](#initial-mode-delivery), [Mode confirmation and attribution](#mode-confirmation-and-attribution) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003` | [Auto-approve selection](#auto-approve-selection) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004` | [Configure contract cleanup](#configure-contract-cleanup) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005` | [End-to-end evidence](#end-to-end-evidence) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-006` | [Workspace-seeded agent configuration](#workspace-seeded-agent-configuration) |
| `REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007` | [Initial mode delivery](#initial-mode-delivery), [Mode confirmation and attribution](#mode-confirmation-and-attribution) |

## Baseline before PR #3886

This table records the behavior that led to the original permission-control
work. PR #3886 changes several rows; the remediation below describes the
remaining integration contract.

| Control | Where it is written | Where it is read |
| --- | --- | --- |
| `cli_flags` (ACP launch) | `lifecycle.CommandBuilder.BuildCommand` appends the tokens to the command `Agent.BuildCommand` returned, which for every ACP agent is the bridge argv (`npx --yes --prefer-offline <bridge package>`). | The bridge process. It forwards no unrecognized argv to the agent CLI it spawns. |
| `cli_flags` (passthrough launch) | `agents.StandardPassthrough.BuildPassthroughCommand` appends them to the agent CLI argv. | The agent CLI. Correct today. |
| `mode` (initial) | Nowhere. `acp.Adapter.NewSession` sends only `Cwd` and `McpServers`; Kandev supplies no `_meta` and writes no agent settings. | The agent process starts in the runtime's own default mode. |
| `mode` (switch) | `SessionManager.applyProfileSessionLayers` calls `client.SetMode` **after** `session/new` returned; `acp.Adapter.SetMode` issues `session/set_mode` and then emits a `session_mode` event built from the **requested** mode plus the cached mode list. | The agent applies it as a mid-session change. Kandev never compares the agent's reported current mode with the requested one. |
| `auto_approve` | Two carriers: `ExecutorCreateRequest.AutoApprovePermissions` / `…Override` and the `AGENTCTL_AUTO_APPROVE_PERMISSIONS` environment definition. Both converge on `config.InstanceConfig.AutoApprovePermissions`. | `process.Manager.handlePermissionRequest`. Working. |
| `approval_policy` | `resolveApprovalPolicyAndDisplayName` maps `AutoApprove` to `never`/`untrusted`; the value travels in the configure request and is stored on `config.InstanceConfig.ApprovalPolicy`. | Nothing. It is assigned and logged, never consulted. |

Three silent-denial sites exist on the permission path:

1. `process.Manager.autoApprovePermission` selects `req.Options[0]` when no
   option declares `allow_once` / `allow_always`, and answers `Cancelled` when
   the option list is empty. Both return before `sendPermissionNotification`, so
   no permission request is ever recorded.
2. `acp.Client.RequestPermission` answers `Cancelled` for an empty option list
   before the handler is consulted, and `forwardPermissionRequest` converts a
   handler error into `Cancelled`.
3. `acp.Adapter.handlePermissionRequest` selects `req.Options[0]` when no
   handler is installed.

A cancelled outcome reaches the agent as a refusal, which providers render as a
denial. From the user's side that is indistinguishable from a human denial.

The effective launch mode is resolved by `Manager.effectiveSessionMode`: a
persisted `session_mode` session-metadata value wins over the profile mode. That
value is written both by the user's session mode toggle and by the workflow
`set_session_mode` step action, whose apply failure is downgraded to Debug. None
of the three sources is recorded on the session, so a profile mode that lost is
invisible.

## CLI flag destination

Add one declaration to the agent contract rather than a per-agent forwarding
mechanism, because no supported ACP bridge offers a generic argv passthrough and
a speculative one would be untestable.

`agents.PermissionSetting` gains an explicit launch-mode scope. The existing
`PermissionApplyMethodCLIFlag` value keeps its meaning for passthrough launches;
a setting that only applies there declares `PassthroughOnly: true`. Claude's
`dangerously_skip_permissions` setting already carries that property in prose
(`claude_acp.go`); the change makes it machine-readable.

Two consumers use it:

- **Save-time validation.** `settings/controller` rejects saving a profile whose
  enabled `cli_flags` contain a token that the profile's agent declares
  passthrough-only while the profile does not use CLI passthrough. The error
  names the ACP equivalent from the same declaration (for Claude: the permission
  mode control). Validation runs on the resolved token list from
  `cliflags.Resolve`, so a flag reached through a multi-token entry is caught.
- **Command preview.** `settings/controller/agent_config.go` already builds a
  preview command. It gains a destination label for the flag segment so the
  preview states that the tokens are appended to the launched bridge process,
  not to the agent CLI it wraps.

Flags that are not declared passthrough-only keep today's behavior: appended to
the launched process argv. That remains useful (bridge-level flags exist) and is
now truthfully labelled.

`CommandBuilder.BuildCommand` is unchanged. The defect was the claim, not the
append.

## Initial mode delivery

The boundary follows
[ADR-2026-09-27-session-permissions-without-settings-mutation](../../../decisions/2026-09-27-session-permissions-without-settings-mutation.md).
The implementation now applies explicit modes through ACP before the first
prompt, preferring the advertised `mode` config option and its authoritative
returned options while retaining legacy `session/set_mode` support. It does not
write a mode overlay or redirect the configuration directory. The original
provider-level Git refusal remains unverified until the isolated real-Claude
acceptance check can run with a separately supplied test credential.

### Evidence and correction

Kandev pins Claude ACP to `0.81.2` in
`apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`.
The npm release identifies upstream commit
`5dbb453c63a89746627799b2b06b31ba01a1b674`.

At that commit:

- [`session-mode.ts`](https://github.com/agentclientprotocol/claude-agent-acp/blob/5dbb453c63a89746627799b2b06b31ba01a1b674/src/session-mode.ts#L211)
  calls `query.setPermissionMode` and awaits its result.
- [`acp-agent.ts`](https://github.com/agentclientprotocol/claude-agent-acp/blob/5dbb453c63a89746627799b2b06b31ba01a1b674/src/acp-agent.ts#L6889)
  handles `session/set_config_option`, applies the mode through that same SDK
  control, and returns `configOptions` with the resulting mode value.
- A normal legacy mode request emits `config_option_update`. It need not emit
  `current_mode_update`. Kandev currently waits only for the latter.
- Creation metadata spreads SDK options, then overrides `permissionMode` with
  the bridge's resolved initial mode. Direct `_meta.claudeCode.options.permissionMode`
  is therefore not a reliable startup control.
- SDK `settings`, `env`, and `extraArgs` can pass through creation metadata.
  They are provider-specific controls. They do not establish that a settings
  object overrides the bridge's explicit initial permission mode.
- [`index.ts`](https://github.com/agentclientprotocol/claude-agent-acp/blob/5dbb453c63a89746627799b2b06b31ba01a1b674/src/index.ts#L9)
  forwards arbitrary CLI arguments only in `--cli` mode. That mode does not
  retain the ACP connection. No dedicated permission-mode environment control
  was found in the reviewed bridge source.

The earlier claim that the switch only changes instructions was not established.
A process argument can retain its startup value after an SDK control changes
runtime state. Its unchanged value does not prove a failed permission change.
The original repository-specific refusal remains unproven against real Claude.
Mock-agent tests do not settle that question.

### Session flow and ownership

The flow is:

`profile/session override -> session/new or session/load -> advertised mode
control -> agent response -> confirmed session state -> first prompt`.

The lifecycle layer resolves the existing mode precedence. The ACP adapter owns
provider capability discovery, RPC selection, and interpretation of its response.
Executors own process isolation and credential delivery. They do not encode a
permission mode into a settings file.

`SessionManager.InitializeAndPrompt` already applies the profile and runtime
layers before `dispatchInitialPrompt`. Keep that order and propagate an unmet
explicit mode as a startup error before dispatch. Resolve the winning mode once
for enforcement. Preserve the profile-baseline snapshot separately.

Prefer the advertised select option with category `mode`. Use its actual ID,
including grouped choices. Do not hardcode the Claude option ID in generic code.
Use `session/set_config_option` and its returned current value. If no mode config
option exists, use advertised legacy `modes` with `session/set_mode`. A genuine
method-not-found response permits a legacy fallback when the agent advertises
it. A provider denial or invalid value does not permit a second escalation path.

Reuse this mode operation for profile application, user changes, resume, and
context reset. Mode-shaped generic settings changes must enter the same
serialization and observation path. Other settings keep their existing path.
No public API needs a Claude-specific control.

Keep requested mode separate from reported mode. Persist the selected override
in existing session metadata. Store neither permission mode nor its confirmation
in Claude user settings. No database migration or new profile option is needed
for this replacement.

### Process controls and settings fallback

Remove `InitialModeDelivery` settings-file machinery where it has no remaining
consumer, including executor uploads and mode-specific `CLAUDE_CONFIG_DIR`
changes. Preserve explicit portable settings transfers and credential delivery.
Do not erase historical session files automatically during an update.

Claude restricts bypass for root outside a sandbox. `IS_SANDBOX` controls that
availability, not the selected mode. Keep any verified container-only declaration
separate from mode delivery. Never set it on an ordinary host or SSH process
merely to make bypass available. Respect provider policy restrictions.

A process argument or environment value is an alternative only after its exact
bridge version and ACP behavior are verified. It must not affect other sessions.
No shared-settings fallback is needed on the available evidence, so none ships.
If a future provider gap requires one, it needs a separate profile opt-in that
defaults to false, including migration and import paths. Its design must address
shared scope, concurrent sessions, user edits, and recovery before implementation.

## Workspace-seeded agent configuration

Repository-scoped file seeding (`Repository.CopyFiles`, materialized by
`worktree.Manager.copyConfiguredFiles`) always writes into that repository's own
worktree root: `copyfiles.Copy(ctx, req.RepositoryPath, wt.Path, ...)`. Whether
that is the directory an agent reads depends on the workspace layout, which the
seeding surface does not currently mention.

| Layout | Agent working directory | Repository seed destination | Reaches the agent |
| --- | --- | --- | --- |
| One repository | the repository worktree root (`env_preparer_worktree.go:114`) | the same directory | yes |
| Two or more repositories | the parent task root (`env_preparer_worktree.go:507-512`) | `<task root>/<repo>/…`, one level below | no |

In the multi-repository layout the repositories are siblings under the task
root, and Kandev seeds only the ownership marker, workspace-source directory
links, and agent skills into that root. A repository-scoped seed intended to
configure the agent therefore lands where the agent never looks, and the
resulting session behaves as though the configuration were absent.

The design does not add arbitrary file seeding at the task root. It makes the
mismatch visible instead:

- The repository seeding surface states the destination directory and, for a
  workspace whose layout places the agent elsewhere, reports that the seed does
  not reach the agent.
- The check is a property of the resolved workspace layout, so it is evaluated
  when the workspace is prepared rather than discovered through agent behavior.

This is deliberately detection rather than relocation. Which files may be
promoted to a shared task root is a separate decision with its own blast radius
across repositories; this design only removes the silent case.

Note that [initial mode delivery](#initial-mode-delivery) is unaffected by the
layout, because it targets an ACP session ID rather than a workspace file.

## Mode confirmation and attribution

### Confirmation

`acp.Adapter.SetMode` returns requested, effective, and confirmed values.
A matching returned `configOptions` snapshot confirms the effective mode for
that request. It does not require an additional notification or a 750 ms delay.
A returned different value is a clamp. A missing mode value remains unconfirmed.
Do not reuse helpers that manufacture current values from the requested value.

For the legacy method, accept current-mode reports and mode values in
`config_option_update`. Capture the observation generation before the request.
Serialize mode changes, including changes through the generic config API.
Reject reports from another session and observations from before the request.
Keep bounded waiting and timeout ambiguity handling for asynchronous reports.
A correlated settings response does not inherit ambiguity from an earlier
uncorrelated legacy notification.

All consumers receive the same authoritative mode. This includes mode events,
settings snapshots, lifecycle caches, persistence, and the desktop/mobile selector.
A reset must not write the requested mode into its cache after an unconfirmed
response. An unmet explicit start mode stops prompt dispatch and records the
reason. A session without an explicit requested mode retains provider defaults.

### Already satisfied legacy modes

Criteria 007.9-007.11 extend confirmation without relaxing evidence for a mode
mutation. Agents owns this contract because provider observations and mode
enforcement are agent-facing runtime responsibilities, even when task startup
is the visible failure.

The active adapter can already know that the selected mode is in force. For a
legacy-only mode catalog, `Adapter.setSessionMode` shall check this after taking
the existing mode and config gates and validating the requested choice, before
`beginModeChange` or sending a provider RPC. Under `a.mu`, require all of:

- The connection exists, the adapter is open, and the active session is nonempty.
- No advertised select config option with category `mode` was selected. The
  authoritative config-option mutation path remains unchanged.
- `modeSessionID` equals the active `sessionID`, `currentModeID` is nonempty and
  equals the requested advertised legacy choice, and `modeOutcomeUncertain`
  is false. A request value alone is never a current-mode observation.
- The caller context has not been cancelled after acquiring the gates.

Return a confirmed `streams.ModeResult` using that observed value and route it
through the existing mode-event completion path. Preserve session-settings
generation ordering and the normal source attribution. Do not call
`noteCurrentMode` with the requested value, clear uncertainty, or manufacture a
new provider observation. The existing mode and config gates remain the
linearization boundary relative to other mutations and new/load/reset.

If any condition fails, retain the existing RPC selection, observation-generation
check, 750 ms settle window, and uncertainty handling. In particular, an empty
successful legacy RPC response still cannot confirm a genuine mode change. A
late report or a cached matching value after an uncertain request must not
activate the short path. A replacement session must establish its own report.

`NewSession`, `LoadSession`, and `ResetSession` already replace session-scoped
mode observations. Reuse those paths. `SessionManager.applyExplicitSessionMode`
and context-reset enforcement still require an applied result before dispatch;
they do not special-case Auggie or omit configured settings. The explicit
provider-restored recovery policy remains a separate, attempt-scoped exception.

The no-op result proves satisfaction, not a fresh mutation. Existing startup
confirmation logs remain valid; new diagnostics must not say an RPC was sent.
No schema, public API, settings-file write, profile option, or new runtime flag
is required. This preserves the existing ADR's evidence and isolation boundary;
the design records the local decision rather than introducing another ADR.

Deterministic adapter tests must assert zero provider mode RPCs and the emitted
confirmed result. A lifecycle-to-adapter wire fixture must prove prompt admission
for fresh and loaded matching sessions and non-admission for a different silent
mode. Config-option clamps and timeout ambiguity remain regression controls.

### Attribution

`Manager.effectiveSessionMode` returns the winning source alongside the mode:
`agent_profile`, `session_override`, or `workflow_step`. The source travels with
the mode into the session layers, is persisted on the session's runtime state
next to `session_mode`, and appears as a structured log field. The workflow
`set_session_mode` action's apply failure moves from Debug to Warn and records
the same session-visible warning, so a step that declared a mode and failed to
apply it is not silent.

No precedence changes. `session_override` continues to win over
`agent_profile`; this design only makes the winner observable.

## Auto-approve selection

`process.Manager.autoApprovePermission` returns a tri-state rather than a
response:

- an allow option was found — answer with it;
- options exist but none declares an allow kind — fall through to the pending
  permission flow;
- no options exist — fall through to the pending permission flow.

`handlePermissionRequest` treats the two fall-through cases exactly like a
non-auto-approve request: it creates the `PendingPermission`, sends the
notification, and waits. The user sees a prompt instead of an invisible denial.

Option-kind matching is normalized (trimmed, case-insensitive) so a provider that
sends `Allow_Once` is not read as an unknown kind.

`acp.Client.RequestPermission`'s pre-handler empty-option shortcut is removed so
the empty-option case reaches the handler and therefore the same pending flow.
`acp.Adapter.handlePermissionRequest`'s no-handler `Options[0]` fallback becomes
a cancellation with an explicit Warn, because a missing handler is a Kandev
wiring failure rather than a permission decision; it is unreachable in a wired
launch and must not silently approve.

Audit: `autoApprovePermission` selects the first allowed option but leaves the
provider request pending. Its candidate event carries the option ID, kind,
`auto_approve` source, and a pending marker. The orchestrator first writes a
permission message. It then uses the existing permission-resolution path to
claim the selected option in durable audit storage before it answers agentctl.
If the write or claim fails, the request remains pending for a person. A
legacy event without the pending marker remains a record of an approval that
the old agentctl already delivered. Reload and session replay read the durable
selection. The delivery-timeout auto-cancel in `sendPermissionNotification`
records a distinct `timed_out` result.

## Mode mismatch presentation

`mode-selector.tsx` shares the current mode, requested mode, and selection
handler between desktop and phone. Desktop keeps the compact dropdown and
focus/hover disclosure. A phone or coarse pointer opens the existing
`MobilePickerSheet` pattern from a visible selector. When the modes differ,
the sheet shows the requested and effective mode before its choices. The
warning stays visible until the reported mode matches or a later request
supersedes it. This is a short temporary choice, so the picker owns its one
scroll region and returns focus to the trigger on dismissal. Touch rows are
at least 44 CSS pixels. Localized labels and an accessible warning name do
not depend on the warning icon alone.

## Configure contract cleanup

`approval_policy` is removed from the sender
(`runtime/agentctl.Client.configureAgent`, `resolveApprovalPolicyAndDisplayName`
keeps only the display-name responsibility and is renamed accordingly) and from
`config.InstanceConfig`. The agentctl request DTO keeps the field as an accepted
but ignored value so a newer backend and an older agentctl, or the reverse, both
configure successfully. A test pins that an inbound `approval_policy` does not
fail the configure call and does not change behavior.

`auto_approve` keeps a single documented carrier. The
`AGENTCTL_AUTO_APPROVE_PERMISSIONS` environment definition and the
`AutoApprovePermissions` request field currently both exist;
`applyApprovalOverrides` already resolves them with the explicit override
winning. The design keeps the request field as the carrier and documents the
environment variable as the test and container-bootstrap override only.

## End-to-end evidence

The reported failure is a workflow outcome, so unit coverage alone does not
close it. A Playwright end-to-end check in `apps/web/e2e` runs the same task
twice against the mock agent, changing only the agent profile:

- with an unattended-permission profile: the mock agent's permission-requesting
  scenario completes with no pending permission request surfaced and no user
  interaction;
- with the default profile: the same scenario surfaces a pending permission
  request, the tool call stays pending, and it completes only after the test
  answers it.

The mock agent (`cmd/mock-agent`) gains a scenario that requests permission for
a state-changing shell command, so the check does not depend on a real provider
or on network access. Backend integration coverage asserts the same contract at
`process.Manager.handlePermissionRequest` for the option shapes listed in
`REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003`.

## Failure modes

- An authoritative settings response can confirm a mode without notifications.
  Without either source, an explicit start mode remains unconfirmed and the
  first prompt stays blocked with a visible reason.
- A provider refusal or unavailable mode never triggers a shared-settings write.
- A root process without the provider's required isolation cannot select bypass.
  Kandev reports the unavailable mode instead of changing the isolation claim.
- A provider that offers only reject options makes an `auto_approve` session
  block on a user prompt. That is the intended behavior: an unattended session
  stalls visibly rather than proceeding on a denial.
- Removing `approval_policy` from the sender while an older agentctl expects it
  is safe: the field was never read.
- Save-time rejection of a passthrough-only flag can invalidate an existing
  stored profile. Validation runs on write only; an existing profile continues to
  launch, and the profile editor shows the same message on next edit.

## Observability

- Structured log `session.mode.applied` with `requested`, `effective`,
  `confirmed`, and `source` replaces the current unconditional Info line.
- Structured log `permission.auto_approve` with `option_id`, `option_kind`, and
  an `outcome` of `selected` or `fell_back_to_prompt`.
- The existing permission transcript gains the auto-approval and timeout
  results, so a permission answered without a human is auditable.

## Implementation plans

- [Original permission-control plan](../../../plans/agent-permission-control-integrity/plan.md)
  records the initial implementation.
- [PR #3886 remediation plan](../../../plans/agent-permission-pr3886-remediation/plan.md)
  owns the start-mode transfer, ACP timing, audit, and phone corrections.

- [Session-control replacement plan](../../../plans/agent-permission-session-controls/plan.md)
  supersedes automatic mode overlays and completes config-option confirmation.
- [Auggie confirmed-mode startup repair](../../../plans/auggie-confirmed-mode-startup/plan.md)
  adds already satisfied legacy-mode handling without changing silent mutations.
