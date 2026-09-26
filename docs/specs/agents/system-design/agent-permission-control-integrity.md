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
---

# Agent Permission Control Integrity System Design

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

The remediation of PR #3886 follows
[ADR-2026-09-25-session-mode-configuration-boundary](../../../decisions/2026-09-25-session-mode-configuration-boundary.md).
The start-mode artifact is a session-owned overlay, not an implicit request to
transfer host agent configuration. The executor profile's portable bundle and
authentication selections remain independent authorities.

The reported symptom is that a permission mode reaches the agent's instruction
layer without changing enforcement. That is consistent with what the launch path
does: Kandev never configures the agent process with a mode. It creates the
session in the runtime's default mode and then issues a mid-session switch.

Kandev therefore needs an initial-mode channel, resolved before the agent
process starts.

### Measured baseline

The reporter measured the switch reaching the agent twice, once through the
profile at session start and once by toggling the mode on a live session. In
both runs Kandev logged the mode as applied, the agent stated in its own output
that the permissive mode was active, and the state-changing commands were still
refused. The launched process carried `--permission-mode default` in both runs.

Two consequences for this design:

- The agent's own statement that a mode is active, the Kandev log line, and the
  displayed session mode are all downstream of the switch and none of them
  observes enforcement. No acceptance criterion may rest on them.
- In this configuration the permissive mode was worse than the default mode:
  the default profile raised a permission prompt and the command ran once
  answered, while the permissive profile refused without a prompt. Whatever
  else is true, delivering the mode after the process has started is not
  equivalent to starting the process in it.

Separately, the reporter's user-level `~/.claude/settings.json` carries no
`permissions` block at all, so nothing escalates from the user scope today. That
is the scope this design writes into, and it is currently empty rather than
conflicting.

### Agent seam

`agents.Agent` gains an optional `InitialModeDelivery` declaration describing how
that agent accepts a start mode. It has three shapes:

- **`settings`** — the agent resolves a start mode from a settings file in a
  configuration directory it reads from the environment. Kandev writes the mode
  into a per-session directory and points the agent at it.
- **`session_meta`** — the agent accepts a start mode in the `session/new`
  request metadata. Kandev populates it in `acp.Adapter.NewSession`.
- **absent** — no channel. Kandev keeps the post-creation switch as today and
  records the mode as best-effort rather than as delivered.

Only agents with a verified wire contract get a non-absent declaration. Claude
ACP uses `settings`, because the bundled bridge resolves its start mode from
`permissions.defaultMode` in the settings it merges, and overrides any
caller-supplied `permissionMode` in the session request. No speculative
declaration is added for an agent whose contract has not been read.

### Per-session configuration directory

For the `settings` shape the write must not touch the user's shared agent
configuration. Container executors already bind-mount an isolated per-instance
directory through `RuntimeConfig.SessionConfig.SessionDirTemplate` /
`SessionDirTarget`. Host executors do not: they inherit the user's real home,
so today a Claude ACP launch on a standalone executor reads the developer's own
`~/.claude`.

The lifecycle manager resolves the launch mode and creates one overlay from
empty settings or the settings bundle that the executor profile selected.
It changes only the declared mode key. A host executor may use the existing
per-instance directory (`CommandBuilder.ExpandSessionDir`) only when changing
the agent's configuration-directory environment preserves the selected
authentication and the caller's explicit directory setting. It must not link
the rest of the host agent directory as an implicit side effect. If the
provider has no safe channel for that launch, the mode is unavailable at
startup and the reason is session-visible.

The executor that launches the agent installs the resolved overlay after its
normal selected-bundle preparation and before the process starts. For Docker,
the final file resides in the mounted session directory at the declared
`SessionDirTarget`. For SSH, the uploader installs it in the remote session
home that is passed as the agent's configuration directory. For Kubernetes,
the pod transfer installs it in the resolved session home used by that launch;
it must not assume `/root/.claude` when the pod reads another home. Warm
resumes reuse the session-owned file and do not recopy unselected host data.
The executor reports the actual agent-visible path and installation result;
`initialModeOutcome.Delivered` becomes true only after that result succeeds.

The settings materializer accepts an object root only. A JSON `null`, scalar,
or array becomes a new object or a structured preparation error before nested
assignment; it never panics. The overlay is written atomically with private
permissions and does not mutate a selected source bundle.

### Escalation trust and process identity

Two runtime constraints must be handled rather than discovered at run time:

- The agent runtime may strip an escalating `permissions.defaultMode` that comes
  from a repository-committed settings source. Writing into the per-session
  configuration directory places the value in the non-committed source, which is
  why the previous section chooses that location rather than the workspace's
  `.claude/`.
- The agent runtime may disable a permissive mode entirely for the launched
  process identity — the bundled Claude bridge computes
  `ALLOW_BYPASS = !IS_ROOT || !!process.env.IS_SANDBOX` and then both omits
  `bypassPermissions` from the offered modes and downgrades a settings-supplied
  value to the default, with a log line as the only signal. For a container
  executor, whose isolation is exactly what that escape hatch is for, Kandev
  declares the sandbox environment. For any executor where the requested mode
  remains unavailable, the session reports it as unavailable with the reason
  instead of running in another mode silently.

`agents.Agent` carries the mode-availability precondition alongside the delivery
declaration, so this stays agent-owned data rather than a special case in the
launch path.

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
layout, because it writes into the per-session configuration directory and
points the agent at it through the environment rather than through the
workspace.

## Mode confirmation and attribution

### Confirmation

`acp.Adapter.SetMode` stops synthesizing the session-mode event from the
requested value. It captures a mode-observation sequence before sending
`session/set_mode` and serializes mode mutations per adapter session. A report
received after that capture belongs to the in-flight request even if it arrives
before the RPC response. A pre-request cached mode cannot confirm a new
request. The emitted `session_mode` event carries the agent's reported current
mode; if none is observed, it keeps the last reported effective value and
marks the request unconfirmed. A report from another ACP session does not
enter this adapter session's observation sequence.

Because a provider may publish `current_mode_update` asynchronously after
answering `session/set_mode`, the adapter waits for a bounded settle window for
a `current_mode_update` naming either the requested mode or a different one
before emitting. The window reuses the existing convergence pattern from
`emitSetModelEvent`; on expiry the adapter emits the last known current mode and
marks the result unconfirmed rather than assuming success.

`SetMode` returns a typed result carrying `requested`, `effective`, and
`confirmed`. `SessionManager.applyProfileSessionLayers` and
`applyRuntimeSessionLayers` log `set profile mode on ACP session` only for a
confirmed exact match. A clamp or an unconfirmed result logs at Warn and records
a session-visible warning message through the existing session message path.

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

Audit: `autoApprovePermission` already logs the selected option. Its decision
record crosses the agentctl-to-orchestrator boundary with the selected option
ID, option kind, and `auto_approve` source. The orchestrator persists these
fields in the permission message data before it marks the message approved;
updating only the status is insufficient. Delivery of the decision record must
be reliable or its failure must be visible, because a best-effort notification
can drop the only audit evidence. Reload and session replay read the same
durable fields. The delivery-timeout auto-cancel in
`sendPermissionNotification` records a distinct `timed_out` result.

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

- A provider that never publishes `current_mode_update` produces an unconfirmed
  mode result. Kandev warns and continues with the session; it does not fail the
  launch, because the mode may still have applied.
- An agent with no declared initial-mode channel keeps the post-creation switch.
  Its mode is recorded as best-effort, so the absence of a channel is visible
  rather than presented as delivery.
- A per-session settings overlay that cannot be created or installed is never
  reported as delivered. The session records the preparation reason and does
  not silently fall back to unselected host configuration. A launch whose
  safety depends on the requested start mode fails before its first turn.
- A requested mode the runtime disables for the process identity is reported as
  unavailable. The session still starts, in the runtime's effective mode, with
  that mode visible.
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
