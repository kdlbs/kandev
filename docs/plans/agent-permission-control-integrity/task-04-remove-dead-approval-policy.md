---
id: "04-remove-dead-approval-policy"
title: "Remove the unread approval_policy field"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.3
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 04: Remove the unread `approval_policy` field

## Summary

`approval_policy` is derived from the profile's `auto_approve`, sent on the
agent-configure request, stored on `config.InstanceConfig`, logged, and read by
nothing. Remove it from the sender and the stored config, keep the receiving DTO
tolerant, and document `auto_approve`'s single carrier.

## Scope

- Stop sending `approval_policy` from `runtime/agentctl.Client.configureAgent`
  and its two wrappers.
- Reduce `resolveApprovalPolicyAndDisplayName` to its display-name
  responsibility and rename it accordingly; update the three call sites
  (`manager_startup.go`, `manager_interaction.go`, `managed_runtime_startup.go`)
  and the Kubernetes refresh path.
- Remove `ApprovalPolicy` from `config.InstanceConfig` and
  `applyApprovalOverrides`; keep the field on the inbound request DTO in
  `agentctl/server/api/server.go` as accepted and ignored.
- Document in `apps/backend/internal/agentctl/AGENTS.md` that `auto_approve`
  travels on `CreateInstanceRequest.AutoApprovePermissions`, with
  `AGENTCTL_AUTO_APPROVE_PERMISSIONS` as the test and container-bootstrap
  override only.

## Exclusions

- No change to `auto_approve` resolution, to `applyApprovalOverrides`'s
  explicit-override precedence, or to the environment definition in
  `environment_resolution.go`.
- No change to the permission flow itself; that is work order 01.

## Acceptance

1. No non-test code assigns, sends, or reads `approval_policy`, and
   `auto_approve` still reaches `process.Manager` through
   `CreateInstanceRequest.AutoApprovePermissions`.
2. A configure request that still carries `approval_policy` succeeds and changes
   no behavior, pinned by a test.
3. The agentctl scoped guidance names the single carrier.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_refresh.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/adapter/e2e/harness.go`
- `apps/backend/internal/agentctl/AGENTS.md`

## TDD sequence

1. Add a failing test that a configure request carrying `approval_policy`
   succeeds and leaves auto-approve behavior determined solely by
   `AutoApprovePermissions`.
2. Add a failing test (or extend the existing auto-approve config tests) that
   the profile's `auto_approve` still reaches `InstanceConfig`.
3. Run the focused command and confirm the expected failures.
4. Remove the field from sender and stored config; update call sites.
5. Re-run the focused command; all pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/agentctl/server/... ./internal/agent/runtime/... -race -count=1 && rg -n 'ApprovalPolicy|approval_policy' --glob '!*_test.go' internal/ | grep -v 'server/api/server.go'
```

The `rg` check must return no other match; the retained inbound DTO field in
`server/api/server.go` is the only permitted occurrence.

## Dependencies

None. Disjoint from work orders 01, 02, 03, and 05.

## Risks

The change crosses the backend/agentctl binary boundary, where version skew is
possible in both directions. The field is unread on the receiving side and the
DTO keeps accepting it, so both directions are safe; the pinning test is what
keeps that true.

## Results

Done.

Implemented:
- `approval_policy` removed from the sender: `Client.ConfigureAgent`,
  `ConfigureAgentWithEnvironment` and `configureAgent` no longer take or send it.
- `resolveApprovalPolicyAndDisplayName` reduced to `resolveAgentDisplayName`;
  the parameter is gone from `configureAndStartAgent`, `initializeAgentSession`,
  `startManagedRuntimeRetry`, `retryManagedRuntimeStartup` and the Kubernetes
  refresh path.
- `ApprovalPolicy` removed from `config.InstanceConfig`, `InstanceOverrides` and
  `applyApprovalOverrides`; `process.Manager.Configure` /
  `ConfigureWithEnvironment` / `configure` no longer take it, and it is gone
  from the "agent configured" log fields.
- The inbound DTO in `agentctl/server/api/server.go` keeps the field, documented
  as accepted and ignored, so an older backend configuring a newer agentctl
  still succeeds.
- `internal/agentctl/AGENTS.md` gains "Permission auto-approval has one
  carrier".

Verification (2026-09-22):

```
go test ./internal/agentctl/server/... ./internal/agent/runtime/... -race -count=1
golangci-lint run ./... --new-from-rev=8690df2f7
grep -rn 'ApprovalPolicy|approval_policy' --include='*.go' internal/ | grep -v _test.go | grep -v server/api/server.go
```

Tests `ok`, lint `0 issues`, and the grep returns nothing outside the retained
DTO field.

Note: one full-package run of `./internal/agent/runtime/lifecycle` under
parallel load showed `TestPreflightRemoteContributionPushesUsesOneBudgetForAllRepositories`
failing. It passed 5/5 in isolation and 2/2 on a repeat full-package run, so it
is load-sensitive rather than a regression from this work order.
