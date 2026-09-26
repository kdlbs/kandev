---
created: 2026-09-22
status: implemented
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-006
  - REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
  - ../../specs/tasks/system-design/mcp-create-task-agent-profile-validation.md
---

# Implementation Plan: Agent Permission Control Integrity

## Overview

A field report on v0.94.0 describes three independent defects in agent-session
permission control. All three share one shape: Kandev reports a control as
applied after writing it somewhere, not after the component that enforces it has
observed it.

This package makes each control either effective or honestly refused, and proves
the result in both directions: an unattended-permission profile must run a
state-changing Git command without a human, and the default profile must still
not.

## Specifications

| Document | Role |
| --- | --- |
| [`agent-permission-control-integrity.md`](../../specs/agents/requirements/agent-permission-control-integrity.md) | Requirements for the three profile permission controls |
| [`agent-permission-control-integrity.md`](../../specs/agents/system-design/agent-permission-control-integrity.md) | Design for flag destination, mode confirmation, auto-approve selection, contract cleanup, evidence |
| [`mcp-create-task-agent-profile-validation.md`](../../specs/tasks/requirements/mcp-create-task-agent-profile-validation.md) | Requirements for `create_task_kandev` profile validation |
| [`mcp-create-task-agent-profile-validation.md`](../../specs/tasks/system-design/mcp-create-task-agent-profile-validation.md) | Design for synchronous validation and async launch-failure visibility |

## Measured baseline

The reporter measured the permissive mode twice, once applied through the
profile at session start and once by toggling the mode on a live session. Both
runs produced the same three signals — Kandev logged `set profile mode on ACP
session` with `mode: bypassPermissions`, the agent stated in its own output that
the permissive mode was active, and the launched process carried
`--permission-mode default` — and both runs still refused the state-changing
commands.

In the same configuration the **default** profile raised a permission prompt and
ran the command once answered. The permissive mode was therefore strictly worse
than the default mode, not merely ineffective.

All four user-facing controls were measured. None of them produces an
unattended state-changing Git command today:

| Control | Measured result |
| --- | --- |
| `auto_approve: true` | No permission prompt reaches the user, and the command is refused in under a second. The interactive fallback is gone; nothing is approved. |
| `cli_flags` with `--dangerously-skip-permissions` | Ends at the ACP bridge argv; the agent CLI never receives it. |
| Profile `mode: bypassPermissions` | Set and logged; enforcement unchanged. |
| Live session mode switch | Reaches the agent's instruction layer only. |
| `mode: acceptEdits` + `auto_approve: true` | Same refusal pattern as every other run; the allowed/refused split did not move. |

The control set is therefore not merely incomplete — with `auto_approve` enabled
it is **worse than leaving every control off**, because the default profile at
least raises a prompt a person can answer (measured: a tool call held
`in_progress` for 2 m 6 s until a human selected `allow-with-updates`, then
completed). A control whose name promises automation must not be able to remove
the only working path.

No acceptance criterion in this package rests on a displayed mode, a Kandev log
line recording a mode as applied, or an agent-authored statement that a mode is
active. All three are downstream of the switch and none observes enforcement.
Acceptance is an executed state-changing Git command, verified by resolving the
resulting commit object.

### Open: the failing repository is the only perfect predictor

Across thirteen probes the outcome tracks the repository exactly and nothing
else does:

| Repository | Unattended-relevant probes | Succeeded |
| --- | --- | --- |
| sxBackend | A, D, F, G, H, I, J, K, L, L2, M | none |
| sxAiCoop | B, parent session | both |
| dev-standards | E | yes |

No other axis separates the results. The trust flag does not (A false → refused,
L true → refused, B true → succeeded, E false → succeeded). The agent profile
does not (A and B ran on identical profile IDs). The permission controls do not
(every combination failed in sxBackend). Probe A is the sharpest case: a human
answered the prompt and the state-changing commands were still refused, in the
same repository where every other probe also failed.

The reporter's matrix records "repository or organization" as excluded because
three repositories in three organizations were exercised. Varying a factor is
not controlling for it; the outcome tracks this one perfectly, so it remains the
leading open candidate.

The one known content difference between the failing and the succeeding
repository is the tracked `.claude/settings.local.json` in sxBackend. The
inverted correlation recorded above rules out the *absence* of that file as an
explanation. It does not rule out its *presence*, and a perfect inverse fit is
still a perfect fit.

This matters for the package's scope. Work orders 01 and 07 close provable
Kandev-side gaps and are worth landing regardless. If the repository-content
hypothesis holds, neither of them changes the reported outcome, and work order
06's instruction to stop and report rather than force the unattended direction
green is what keeps that visible.

### Open measurement: which side suppresses the request

Under `auto_approve` the absence of `responding to permission request` is
expected rather than diagnostic: that line belongs to the user-response path
(`orchestrator/handlers/handlers.go:448` → `manager_interaction.go:2535` →
`process/manager.go:3180`), and an auto-approved request is answered locally in
agentctl at `process/manager.go:2864` without ever using it.

Three agentctl-side lines discriminate, and for a host executor they are in the
agentctl process log rather than the backend log:

| Line | Site | Meaning |
| --- | --- | --- |
| `handling permission request` (carries `auto_approve`) | `process/manager.go:2756` | a request reached Kandev at all |
| `auto-approving permission request` (carries `option_id`, `kind`) | `process/manager.go:2864` | Kandev answered, and with which option |
| `no options available for auto-approve, cancelling` | `process/manager.go:2845` | empty option list; answered with a cancellation |
| `no options available, cancelling permission request` | `acp/client.go:141` | same, one layer earlier, before the handler runs |

Absence of all four means the agent never asked, which points at work order 07.
Presence of the third or fourth means Kandev converted an ask into a refusal,
which points at work order 01. Both work orders are in this package, so the
measurement changes their order and their acceptance evidence, not their scope.

## Confirmed root causes

**Report defect 1 — CLI flags never reach the agent.**
`lifecycle.CommandBuilder.BuildCommand` appends the profile's enabled
`cli_flags` to whatever `Agent.BuildCommand` returned. For every ACP agent that
is the bridge argv (`npx --yes --prefer-offline <bridge package>`), and no
supported bridge forwards unrecognized argv to the agent CLI it wraps. Only CLI
passthrough puts the flags on the real agent binary. Claude's
`dangerously_skip_permissions` permission setting already documents itself as
passthrough-only in prose, but nothing enforces that, so the flag can be saved
on an ACP profile and silently lands on the bridge.

**Report defect 2 — permission controls do not change enforcement.** Four
separate Kandev-side faults:

- **Kandev never configures the agent process with a permission mode.**
  `acp.Adapter.NewSession` sends only `Cwd` and `McpServers`; no `_meta`, no
  agent settings. The process starts in the runtime's default mode and Kandev
  issues `session/set_mode` afterwards. For the bundled Claude bridge that
  switch is a mid-session control request, which is exactly the shape the report
  measured: the mode reaches the agent's instruction layer while the process
  keeps enforcing what it started with. This is the fault that matches the
  reported symptom; work order 07 addresses it.
- `acp.Adapter.SetMode` emits its session-mode event from the **requested** mode
  and the cached mode list, never from the agent's reported current mode. A
  clamped or ineffective mode is logged identically to an applied one.
- The effective launch mode comes from `Manager.effectiveSessionMode`, where a
  persisted `session_mode` session-metadata value (written by the user toggle
  and by the workflow `set_session_mode` action, whose apply failure is
  downgraded to Debug) wins over the profile mode. Nothing records which source
  won.
- `process.Manager.autoApprovePermission` selects `req.Options[0]` when no
  option declares an allow kind, and answers `Cancelled` when the list is empty.
  Both return before `sendPermissionNotification`, so the agent sees a refusal,
  the user sees no prompt, and no permission request exists to inspect.
  `acp.Client.RequestPermission` has the same empty-option shortcut before the
  handler, and `acp.Adapter.handlePermissionRequest` has the same `Options[0]`
  fallback when no handler is installed.

Also confirmed: `approval_policy`, derived from the profile's `auto_approve` in
`resolveApprovalPolicyAndDisplayName`, is transmitted, stored on
`config.InstanceConfig.ApprovalPolicy`, logged, and read by nothing.

`auto_approve` reaches `config.InstanceConfig.AutoApprovePermissions` through
`ExecutorCreateRequest.AutoApprovePermissions`; that transport is intact in the
code. Transport is not approval. Measured end to end, enabling the control
removes the interactive path and approves nothing, so it must not be described
as the working control anywhere in this package.

**Report defect 3 — `agent_profile_id: "current_task"` creates a task with no
session.** `current_task` is a value of the per-user setting
`mcp_task_agent_profile_default`, not an argument value, but the tool
description names it inside the `agent_profile_id` property description. The
string is passed through unvalidated, counts as an explicit profile (defeating
every inheritance and workspace-default fallback), and fails in
`runtime.ValidateProfile` inside `launchAutoStartTask`'s fire-and-forget
goroutine, which logs the error and returns. The tool already reported success.

### Ruled out: a missing project permission list in the worktree

A plausible-looking explanation was that `.claude/settings.local.json` is
gitignored and therefore absent from a freshly created worktree, so an
MCP-created sub-task would lack the allow list that a hand-started session in
the main checkout has.

The reporter tested it and the correlation is inverted. In the repository where
sub-tasks **fail**, the file is tracked in git (3391 B on the base ref), so
`git worktree add` materializes it before any session hook runs; it carries 63
allow entries including `Bash(git commit:*)`, `Bash(git push:*)` and
`Bash(git checkout:*)`, with empty `deny`, empty `ask`, and no `defaultMode`. In
the repository where a sub-task **succeeded**, the file is not tracked at all
and a fresh worktree starts without it.

The absent-file explanation would therefore justify a fix that repairs nothing.
Seeding the file deterministically is still worth doing on its own terms — the
reporter's standards put `settings.local.json` in `.gitignore`, and the tracked
copy is the deviation — but it is not part of this package and must not be
presented as addressing the refusals.

### Where the denial comes from

The refusals are raised by the running agent itself, not by a Bash-tool
permission request that Kandev answers. That is consistent with the report's
measurement of zero permission requests for those sessions, and it means the
auto-approve faults above are a second, independent defect rather than the cause
of the reported symptom.

The agent denies internally because it is still running under the permission
mode it was launched with. Kandev supplies no mode at session creation, so the
process starts in the runtime default; the later `session/set_mode` changes what
the agent is told, and the report measured that it does not change what the
process enforces. Work order 07 closes that gap.

What this package does not claim: that the runtime's mid-session mode switch is
itself broken. That is the report's measurement, taken as evidence, and it
belongs to the provider. The Kandev-side gap — no initial mode at all — is
provable from the launch path and is the right fix either way. Work order 06 is
the check that decides whether anything survives it.

## Handover

[`handover.md`](handover.md) records the operational knowledge a fresh session
needs: environment setup, the traps that cost time, the pre-existing failures
with their measurements, and the questions this package does not settle.

## Work orders

| Order | Title | Wave | Depends on |
| --- | --- | --- | --- |
| [`task-07-initial-session-mode.md`](task-07-initial-session-mode.md) | Deliver the permission mode at session start | 1 | — |
| [`task-01-auto-approve-never-denies.md`](task-01-auto-approve-never-denies.md) | Auto-approve approves or prompts | 1 | — |
| [`task-02-confirm-session-mode.md`](task-02-confirm-session-mode.md) | Confirm and attribute the applied session mode | 1 | — |
| [`task-03-cli-flag-destination.md`](task-03-cli-flag-destination.md) | Declare and enforce the CLI flag destination | 1 | — |
| [`task-04-remove-dead-approval-policy.md`](task-04-remove-dead-approval-policy.md) | Remove the unread `approval_policy` field | 1 | — |
| [`task-05-mcp-create-task-profile-validation.md`](task-05-mcp-create-task-profile-validation.md) | Validate `create_task_kandev` agent profile | 1 | — |
| [`task-08-multi-repo-seed-reachability.md`](task-08-multi-repo-seed-reachability.md) | Report a repository seed that cannot reach the agent | 1 | — |
| [`task-06-unattended-permission-evidence.md`](task-06-unattended-permission-evidence.md) | Prove unattended and attended behavior end to end | 2 | 07, 01, 02 |

Work order 07 is the one that addresses the reported symptom. Start there.

Wave 1 work orders touch disjoint files and have no shared schema, generated
contract, or package config. That is planning information only; execute
sequentially unless the user explicitly authorizes subagents.

## ASCII UI previews

Two work orders change rendered UI. Spacing here is illustrative; the structural
requirements are the control order, the destination label, and the warning
placement. Copy is localized (`agents`, `settings`, `task` namespaces); the
English strings below are placeholders for the catalog keys the work orders add.

### UI-01: Profile editor, CLI flags section (work order 03)

Entry point: Settings → Agents → *agent* → Profiles → *profile*. State: a
passthrough-only flag enabled on a profile that launches over ACP.

Current behavior (save succeeds, flag silently lands on the bridge):

```text
+-- Agent CLI flags -------------------------------- 1 of 3 enabled --+
| [x] --dangerously-skip-permissions                            [Del] |
|     Pass --dangerously-skip-permissions so Claude Code does not     |
|     prompt for tool approvals.                                      |
| [ ] --verbose                                                 [Del] |
|                                            [ + Add flag ]           |
+---------------------------------------------------------------------+
                                                       [ Save profile ]
```

Proposed:

```text
+-- Agent CLI flags -------------------------------- 1 of 3 enabled --+
| Flags are passed to the launched ACP bridge process, not to the     |
| agent CLI it wraps.                                                 |
|                                                                     |
| [x] --dangerously-skip-permissions                            [Del] |
|     (!) Not available over ACP. Use Start mode > Bypass             |
|         permissions instead.                                        |
| [ ] --verbose                                                 [Del] |
|                                            [ + Add flag ]           |
+---------------------------------------------------------------------+
  (!) Remove or disable --dangerously-skip-permissions to save.
                                                       [ Save profile ]
                                                        ^ disabled
```

Command preview card, same page, gains a destination line:

```text
+-- Command preview --------------------------------------------[Copy]+
| Launched process (ACP bridge):                                      |
|   npx --yes --prefer-offline @agentclientprotocol/claude-agent-acp  |
|   --verbose                                                         |
+---------------------------------------------------------------------+
```

Phone composition is the same single-column stack; the inline flag warning wraps
under its row and the save-blocking message stays directly above the sticky save
action. No hover-only information, no horizontal scrolling.

### UI-02: Session mode warning (work order 02)

Entry point: task chat. State: the profile requested `bypassPermissions` and the
agent clamped it.

```text
  +-------------------------------------------------------------+
  | (!) Requested mode "Bypass permissions" is unavailable in    |
  |     this session. The session is running in "Manual".        |
  +-------------------------------------------------------------+

  [ ... conversation ... ]

  +-- chat input ----------------------------------------------+
  | > _                                                         |
  | [Model v]  [Mode: Manual v]                        [ Send ] |
  +-------------------------------------------------------------+
```

The mode selector shows the agent's reported mode, not the requested one. On
phone the same warning renders full width above the transcript and the selector
stays in the mobile toolbar row.

## Risks

- **Auto-approve fall-through changes unattended behavior.** A session that
  previously proceeded on an arbitrary option now blocks on a prompt. That is
  the intended correction, but it can surface as a task that stalls where it
  used to (wrongly) continue. Work order 01 covers it with explicit transcript
  entries so the stall is visible rather than mysterious.
- **Mode confirmation depends on a provider notification.** A provider that
  never publishes `current_mode_update` yields an unconfirmed result. The design
  warns and continues rather than failing the launch; the settle window must not
  add perceptible launch latency.
- **Save-time flag rejection can invalidate stored profiles.** Validation is
  write-only, so existing profiles keep launching; the message appears on next
  edit.
- **`approval_policy` removal crosses the backend/agentctl binary boundary.**
  The field is unread, and the receiving DTO keeps accepting it, so version skew
  in either direction is safe. The work order pins that with a test.

## Verification strategy

Each work order owns its targeted commands. The package is complete when work
order 06's two directions both pass; unit coverage alone does not close the
reported workflow failure.

## Verification Results

All eight work orders are `done`. Per work order:

| Order | Commit |
| --- | --- |
| 01 auto-approve approves or prompts | `f19a42945` |
| 05 create-task profile validation | `8c22037b7` |
| 07 permission mode at session start | `abd1d14c2` |
| 04 remove the unread `approval_policy` | `d29c43275` |
| 02 confirm and attribute the session mode | `fc6f8c352` |
| 06 unattended and attended evidence | `239593d8b` |
| 08 copy_files seed reachability | `b9f7ba0a9` |
| 03 CLI flag destination | `6cb16e75d` |

Work order 06 is the package's acceptance: against the mock agent, an
unattended profile runs a state-changing Git command with nobody answering and
the default profile holds the same call until a person does, both asserted
against a resolved commit object. 6 passed, 0 flaky over three repetitions.

`golangci-lint run ./... --new-from-rev=8690df2f7` reports `0 issues`.

### Pre-existing conditions, untouched by this package

- `TestHandleAgentCompleted_BlocksOnTurnCompleteWhileClarificationPending` is
  flaky at the base commit: 7/50 failures at base, 4/50 with these changes.
- `TestManagerRescanKeepsFocusedWorkspaceFast` and
  `TestRunAgentProcessAsync_ObservesStartingSiblingsBeforeProcessStart` fail
  only under full-sweep parallel load; 0/3 in isolation.
- `TestInstallSystemdWritesOwnerOnlyNativeMetadata` and
  `TestInstallLaunchdWritesNativeMetadata` read the developer machine's real
  runtime bundle directory, so they fail on any host with Kandev installed.
- `pnpm run i18n:check` reports 32 missing `ja` keys for SSH reachability and
  launch warnings, from `c7cc92382` landing after the Japanese catalog in
  `cd08c50ca`. No file this package touches is involved.

### What the package still does not settle

That the delivered mode changes what a real provider enforces. Work order 06
proves the Kandev side against the mock agent. The reporter's repository
correlation — every failure in one repository, every success in the other two —
remains the leading open candidate for their own case and is recorded above.
