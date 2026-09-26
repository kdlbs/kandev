---
id: "01-auto-approve-never-denies"
title: "Auto-approve approves or prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.5
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.6
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.7
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.8
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.9
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.10
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 01: Auto-approve approves or prompts

## Summary

Remove every path where an enabled `auto_approve` produces a refusal instead of
an approval. When no allow option is offered, fall back to the interactive
permission prompt.

## Scope

- `process.Manager.autoApprovePermission` returns a decision, not a response:
  an allow option was selected, or the caller must fall through to the pending
  permission flow.
- `process.Manager.handlePermissionRequest` treats both fall-through cases
  (no allow-kind option, empty option list) exactly like a non-auto-approve
  request: create the `PendingPermission`, send the notification, wait.
- Normalize option-kind comparison (trim, case-insensitive) so an unexpected
  casing is not read as an unknown kind.
- Remove the pre-handler empty-option shortcut in `acp.Client.RequestPermission`
  so the empty case reaches the handler.
- Change `acp.Adapter.handlePermissionRequest`'s no-handler `Options[0]`
  fallback to a cancellation with a Warn; a missing handler is a wiring failure,
  not a permission decision.
- Record an auto-approval in the permission transcript with option ID, option
  kind, and an `auto_approve` source; record the delivery-timeout auto-cancel
  with a distinct `timed_out` result.
- Make the request lifecycle decidable from recorded evidence without
  reproducing the session. Today the three discriminating lines
  (`handling permission request` at `process/manager.go:2756`,
  `auto-approving permission request` at `:2864`, and the two empty-option
  cancellations at `:2845` and `acp/client.go:141`) are agentctl-process logs,
  while `responding to permission request` — the line an operator reaches for —
  belongs to the user-response path and is absent by design whenever
  auto-approval answers locally. Surface the auto-approved and cancelled
  outcomes on the session transcript so "no request arrived" and "Kandev
  cancelled the request" are distinguishable there.

## Exclusions

- No change to `autoApproveInjectedKandevPermission` (injected Kandev MCP
  approval) beyond the shared kind-normalization helper.
- No change to the 5-second delivery-timeout duration or to the attached vs
  detached branching in `sendPermissionNotification`.
- No change to which commands the provider classifies as needing permission.

## Acceptance

1. With `auto_approve` enabled and an allow option offered anywhere in the list,
   Kandev answers with that allow option and creates no pending permission.
2. With `auto_approve` enabled and no allow-kind option (including an empty
   option list), Kandev creates a pending permission, emits the permission
   notification, and answers only when a response arrives.
3. An auto-approved request, a timed-out request, a cancelled request, and a
   session in which no request ever arrived are four distinguishable states in
   the permission transcript, without reading the agentctl process log.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_permission_policy.go`
- `apps/backend/internal/agentctl/server/process/manager_permission_policy_test.go`
- `apps/backend/internal/agentctl/server/acp/client.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_permissions.go`
- New `apps/backend/internal/agentctl/server/process/manager_auto_approve_test.go`
  (`manager.go` and the existing permission-policy test file are near the
  effective-line limits; add new tests in a new file)

## TDD sequence

1. Add failing tests: allow option listed after a reject option is selected;
   reject-only option list falls through to a pending permission; empty option
   list falls through to a pending permission; mixed-case `Allow_Once` is
   recognized; the auto-approval transcript entry carries option ID, kind, and
   source.
2. Run the focused command and confirm each fails for the expected reason (the
   current code answers with `Options[0]` or `Cancelled`).
3. Implement the decision split and the kind normalization.
4. Add the transcript entries for auto-approval and for the delivery timeout.
5. Re-run the focused command; all tests pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/agentctl/server/process/... ./internal/agentctl/server/acp/... ./internal/agentctl/server/adapter/transport/acp/... -race -count=1
```

## Dependencies

None.

## Risks

Sessions that previously continued on an arbitrary option now block on a
prompt. The transcript entries added here are what makes that stall legible.

The reporter measured `auto_approve: true` producing no prompt and an immediate
refusal, while the same setup with the control disabled held the call pending
for a person. Until the transcript work in this order lands, it is not settled
whether Kandev cancelled a request or the agent never sent one. Land the
observability first and read it before assuming which.

## Results

Done.

Implemented:
- `autoApprovePermission` returns a decision; `handlePermissionRequest` falls
  through to the pending flow when no option declares an allow kind, including
  for an empty option list.
- Option-kind comparison is normalized (trim, case-insensitive), shared with the
  injected-Kandev policy.
- `acp.Client.RequestPermission` no longer cancels an empty option list before
  the handler runs; its no-handler fallback cancels instead of selecting
  `Options[0]`.
- `acp.Adapter.handlePermissionRequest` cancels with a Warn when no handler is
  installed.
- An auto-approved request now emits a permission-request record carrying
  `auto_approved_option_id`. The orchestrator turns it into a transcript message
  settled as approved, and skips both `setSessionWaitingForInput` and the
  automation-run failure path, because the request was never pending.

Also fixed, because it is what made the field diagnosis impossible: the agentctl
launcher relayed the child's **stdout** unconditionally at DEBUG. agentctl logs
to stdout, and the backend file core defaults to INFO, so every agentctl record
— including every permission decision — was dropped before reaching any file.
`pipeOutput` now forwards a line at the level the child tagged it with on both
streams, keeping WARN as the unstructured fallback on stderr and DEBUG on
stdout.

Verification (2026-09-22):

```
go test ./internal/agentctl/server/process/... ./internal/agentctl/server/acp/... \
  ./internal/agentctl/server/adapter/transport/acp/... \
  ./internal/agent/runtime/agentctl/launcher/... -race -count=1
```

all `ok`. Regression sweep over `./internal/orchestrator/...` and
`./internal/agent/runtime/lifecycle/...` also `ok`.

Known pre-existing flake, not caused by this work order:
`TestHandleAgentCompleted_BlocksOnTurnCompleteWhileClarificationPending` fails
intermittently. Measured over 50 runs: 7/50 at the base commit, 4/50 with this
change. Its `require.Eventually` settles on the session state and then asserts
the active turn separately, so the two observations race.
