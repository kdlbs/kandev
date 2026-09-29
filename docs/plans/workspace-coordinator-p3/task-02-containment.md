---
id: "02-containment"
title: "Containment check and unattended permissions"
status: pending
wave: 2
depends_on:
  - "01-flag-schema-settings"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-CONTAINMENT-001
  - REQ-COORDINATOR-CONTAINMENT-002
  - REQ-COORDINATOR-CONTAINMENT-003
acceptance_criteria:
  - AC-COORDINATOR-CONTAINMENT-001.1
  - AC-COORDINATOR-CONTAINMENT-001.2
  - AC-COORDINATOR-CONTAINMENT-001.3
  - AC-COORDINATOR-CONTAINMENT-002.1
  - AC-COORDINATOR-CONTAINMENT-002.2
  - AC-COORDINATOR-CONTAINMENT-003.1
  - AC-COORDINATOR-CONTAINMENT-003.2
system_design:
  - ../../specs/coordinator/system-design/containment.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 02: Containment Check And Unattended Permissions (WP-11)

## Summary

Builds `containment.Check`, the four fail-closed conditions admission step 2
runs, and the orchestrator hook that denies non-auto-approved permissions
during an unattended turn and counts them on the turn row.

## In scope

- `internal/coordinator/containment.go`: `Check(ctx, coordinator) Result`
  over the executor profile and executor type, `auth.Service.Mode()`, the
  resolved launch environment (secrets resolved through the secrets store,
  compared and discarded), and `AgentProfileMcpConfig`
  ([Check](../../specs/coordinator/system-design/containment.md#check)).
- One exported, read-only lifecycle function,
  `ResolveLaunchEnvironmentDefinitions`, returning the resolved definitions in
  resolver order for (executor profile, agent profile, task id). It is a
  behaviour-preserving extraction that the launch path also calls; existing
  launch tests pass unchanged and one new test shows launch and `Check` see the
  same set. Nothing else in the launch path is refactored. `Check` also
  resolves `ProviderAPIKeySecretID`.
- Launch currency includes the `updated_at` of every secret and repository
  binding feeding condition 3; a missing MCP row, and a deleted or disabled
  configuration, use the agent profile's `updated_at`.
- New constants `models.PermissionActorCoordinatorUnattended` and
  `models.PermissionSourceCoordinatorWake`; optional `ActorKind` and
  `UnattendedTurnID` on `ResolveAgentPermissionRequest`; optional
  `PermissionResolutionAudit.UnattendedTurnID`. No schema column: the audit is
  message metadata JSON.
- `CheckForAdmission` in `containment.go`: runs `Check`, increments the counter
  and writes the state-change log keyed by (Contained, first unmet condition).
- Fix copy for `changed_since_launch` and `unreadable` in all six locales,
  wired by task 06 into the settings display.
- An optional `UnattendedPermissionHandler` on the orchestrator, called from
  `handlePermissionRequest` (`internal/orchestrator/event_handlers_git.go`)
  after the permission message is stored, beside
  `failAutomationRunOnPermission`. Requests reaching it were not granted by
  agentctl's exact-name auto-approve (`manager_permission_policy.go`). For a
  request whose turn id equals an open unattended turn row's
  `session_turn_id`: record `(turn_id, pending_id)` in
  `coordinator_unattended_denials` and increment `denied_permissions` only
  when that insert added a row, then reject through the
  `ResolveAgentPermission` path (or, with no reject option,
  `cancelAgentPermission` plus `markSessionRunningAfterPermission`) with a
  `PermissionResolutionAudit` of actor kind
  `coordinator_unattended`, source `coordinator_wake` and the turn id; 
  `ReresolveRecordedDenials(ctx, coordinatorID)` re-resolves, for one
  coordinator, a recorded denial whose turn is open and whose message is still
  pending; the resolver and message reader are injected once at wiring
  ([Unattended permissions](../../specs/coordinator/system-design/containment.md#unattended-permissions)).
- `coordinator_containment_failed_total{condition}` and the state-change log, inside `CheckForAdmission`, tested with a fake caller.
- The denial only removes capability: it never adds a tool to the bound list
  or approves a request the allowlist left open
  ([integration Tool list](../../specs/coordinator/system-design/integration.md#tool-list);
  `AC-COORDINATOR-INTEGRATION-003.2` is owned and tested by task 05).

## Out of scope

- Calling `CheckForAdmission` from admission (task 05; it only calls the helper) and the settings display (task 06, which owns rendering the fix copy).
- Residual sources with no `updated_at` (agent-runtime defaults, credentials-manager values): not detected by launch currency, named under Out of scope in the design.
- Any sandboxing or change to executors, auth or secrets.
- Wiring `ReresolveRecordedDenials` into the backstop tick (task 04). Admission holding wakes `pending` with hold reason `containment` (task 05). For `AC-COORDINATOR-CONTAINMENT-002.1` this card pins only that `Check` returns the four conditions in order and that the first unmet one is identifiable; `002.2` is pinned by the phase 1 tests passing with every condition failing.

## Acceptance

- Table test: each executor type yields `executor_isolated` met or not as
  the design lists (`mock_remote` met only under the e2e profile); auth modes
  `disabled` and `setup` fail; a key `KANDEV_API_KEY` or `KANDEV_RUN_TOKEN`, or
  a value starting `kandev_pat_` from a plain value or a secret, fails; any MCP
  server fails; each unreadable input fails with detail `unreadable`; a profile with no MCP row is met; a `kandev`-named MCP entry fails; a profile updated after the session's `started_at` fails conditions 3 and 4 with `changed_since_launch`; a secret or repository binding updated after `started_at` fails condition 3 with `changed_since_launch`; a missing MCP row is met and uses the agent profile's `updated_at`; `ProviderAPIKeySecretID` holding a `kandev_pat_` value fails; a `ConflictError` is `unreadable`; no
  result is cached (two calls around a profile edit differ).
- Lifecycle test: launch and `Check` see the same definition set. `CheckForAdmission` counts each unmet condition and logs only when (Contained, first unmet) changes.
- A permission request during an open unattended turn that the exact-name
  rule does not approve is resolved at once with the audit above and the
  count incremented; an auto-approved one is untouched; the same request with
  no open turn, or from a later session turn than the unattended one (a
  drained queued manager message), waits for a person; the same `pending_id`
  delivered twice is counted and resolved once; a failed resolution is
  resolved again by a direct call of `ReresolveRecordedDenials(ctx, coordinatorID)` without a second count, only for that coordinator's open turns, never for a settled turn; with a reject option the session is not left `WAITING_FOR_INPUT`, with none the cancel path restores it; a failed message write records and counts nothing; a coordinator session started by delivery
  has profile and environment auto-approve forced off.
- Attended paths never call `Check`: the phase 1 conversation and message
  tests pass unchanged with every condition failing.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Containment' -count=1
cd apps/backend && go test ./internal/orchestrator/... -run 'Coordinator.*Permission' -count=1
make -C apps/backend lint
```

## Risks

- Resolving the launch environment the lifecycle manager would build must use
  the same resolver, not a copy; a divergence is a containment hole. Reuse the
  lifecycle manager's env builder through the one extracted function and a narrow interface.
