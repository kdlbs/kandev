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
- An optional `UnattendedPermissionHandler` on the orchestrator, called from
  `handlePermissionRequest` (`internal/orchestrator/event_handlers_git.go`)
  after the permission message is stored, beside
  `failAutomationRunOnPermission`. Requests reaching it were not granted by
  agentctl's exact-name auto-approve (`manager_permission_policy.go`). For a
  request whose turn id equals an open unattended turn row's
  `session_turn_id`: record `(turn_id, pending_id)` in
  `coordinator_unattended_denials` and increment `denied_permissions` only
  when that insert added a row, then reject (or cancel) through
  `cancelAgentPermission` with a `PermissionResolutionAudit` of actor kind
  `coordinator_unattended`, source `coordinator_wake` and the turn id; the
  backstop re-resolves a recorded denial whose message is still pending
  ([Unattended permissions](../../specs/coordinator/system-design/containment.md#unattended-permissions)).
- `coordinator_containment_failed_total{condition}` and the state-change log.

## Out of scope

- Calling `Check` from admission (task 05) and the settings display (task 06).
- Any sandboxing or change to executors, auth or secrets.

## Acceptance

- Table test: each executor type yields `executor_isolated` met or not as
  the design lists (`mock_remote` met only under the e2e profile); auth modes
  `disabled` and `setup` fail; a key `KANDEV_API_KEY` or `KANDEV_RUN_TOKEN`, or
  a value starting `kandev_pat_` from a plain value or a secret, fails; any MCP
  server fails; each unreadable input fails with detail `unreadable`; no
  result is cached (two calls around a profile edit differ).
- A permission request during an open unattended turn that the exact-name
  rule does not approve is resolved at once with the audit above and the
  count incremented; an auto-approved one is untouched; the same request with
  no open turn, or from a later session turn than the unattended one (a
  drained queued manager message), waits for a person; the same `pending_id`
  delivered twice is counted and resolved once; a failed resolution is
  resolved again by the next backstop tick without a second count; a coordinator session started by delivery
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
  lifecycle manager's env builder through a narrow interface.
