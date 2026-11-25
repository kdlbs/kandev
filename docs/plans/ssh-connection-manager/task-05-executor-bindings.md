---
id: "05-executor-bindings"
title: "Executor binding and explicit adoption"
status: done
wave: 5
depends_on:
  - "04-assisted-enrollment"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-001.6
  - AC-EXECUTORS-SSH-CONNECTIONS-002.4
  - AC-EXECUTORS-SSH-CONNECTIONS-002.5
  - AC-EXECUTORS-SSH-CONNECTIONS-002.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.5
  - AC-EXECUTORS-SSH-CONNECTIONS-005.1
  - AC-EXECUTORS-SSH-CONNECTIONS-005.2
  - AC-EXECUTORS-SSH-CONNECTIONS-005.3
  - AC-EXECUTORS-SSH-CONNECTIONS-002.7
  - AC-EXECUTORS-SSH-CONNECTIONS-002.8
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 05: Executor binding and explicit adoption

## Summary

The [navigation follow-up](../ssh-connections-executor-navigation/plan.md) owns
the completed contextual manager return and draft-preservation refinement. This
order retains backend binding/adoption acceptance and its current status.

Connect the new manager to real SSH task execution and make legacy adoption explicit. Bound executor settings cannot substitute another target or identity.

## In scope

- Wire the resolver into preparation/launch/readiness/status/resume/cleanup; retain the complete route stamp and both identity generations. Validate dependency revisions before remote effects; reject direct fallback.
- Implement atomic profile binding/adoption against current revision/config digest, target-change conflicts, reference deletion protections, and managed-selector enforcement in legacy HTTP/WS routes.
- Add executor selector/adoption UI and auth tests; recheck #3783 and migrate its consumers if it has landed without implementing that runtime here.

## Out of scope

Production work outside this order, remote Docker runtime implementation, and
changes to the confirmed credential ownership boundary. No publishing or delegation.

## Acceptance

- Two profiles reuse one connection; auth readiness and executor readiness stay distinct, and non-admin task use cannot override credentials or target.
- Legacy direct and single-hop adoption retains the exact target/pin, resolves each hop identity, and requires explicit bastion trust; failed route checks leave the profile unchanged. Refuse transport edits while retained runtimes depend on either route record; stale recorded generations never reconnect to a changed target.
- Enabled/disabled and HTTP/WS/runtime paths obey the same gate and authority, including background probes and authenticated task use.

## ASCII UI preview

[Combined previews](plan.md#ascii-ui-preview). Applicable ACs are listed above;
retain the same structure and state rules in the rendered result.

### UI-04: Executor binding

Entry: SSH executor editor, shared composition with phone picker Drawer.

```text
Connection [Build box: Ready v]     [Manage connections]
Connection: Ready | Agent readiness: Not checked
[Test executor]
Shared Save changes: disabled until executor check succeeds

Legacy profile
Connection: Existing inline SSH settings
[Move to shared connection]
Preview: destination + optional bastion, each user/identity/fingerprint
[Create connection and check]
```

Maps to 002.4-002.8, 004.5, and 005.1-005.2. Converted forms cannot override the
connection target or pin. Conflicting edits keep current server data and show a
reload/retry action. Single-hop adoption adds explicit bastion trust and route
checks; unsupported chained settings leave the legacy editor untouched.

Structural requirements are control order, explicit trust before credential
use at each hop, focused phone navigation, and a distinct recovery state. Labels and spacing
are illustrative and must be localized. Reuse SettingsLayoutClient's single
scroll owner and save bar. Phone/coarse targets are >=44px; desktop controls
are 28px. Safe-area/keyboard space keeps the current action reachable; no
horizontal document scroll. Test around 767/768px and on a coarse tablet.


## Verification

Use TDD for changed logic. Commands below are run from the repository root;
read the [prerequisites](plan.md#verification-prerequisites) first. Tests named
in this package are created by their owning order, not claimed to exist today.

```bash
(cd apps/backend && go test ./internal/sshconnections ./internal/ssh ./internal/task/service ./internal/task/handlers -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(ManagedSSH|ConnectionReady|SSH|BuildAuth|IdentityAgent)' -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'Test(SSHConnections|ManagedSelectors)' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections/managed-binding.test.tsx components/settings/ssh-connection-card.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project auth tests/auth/ssh-connections-permissions.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/ssh/managed-connections.spec.ts tests/ssh/auth-methods.spec.ts tests/ssh/proxy-jump.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_managed_test.go (new)`
- `apps/backend/internal/ssh/handlers.go`
- `apps/backend/internal/sshconnections/adoption.go (new)`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/task/handlers/executor_handlers.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/ssh_connections_test.go (new)`
- `apps/web/components/settings/ssh-connections/managed-binding.tsx (new)`
- `apps/web/components/settings/ssh-connection-card.tsx`
- `apps/web/app/settings/executors/`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/auth/ssh-connections-permissions.spec.ts (new)`
- `apps/web/e2e/tests/ssh/managed-connections.spec.ts (new)`

## Dependencies

Task 04; see the [manifest](plan.md#work-orders).

## Risks

Connection edits and task admission race across network calls. Recheck revision before workload effects and protect references transactionally. Do not treat management scope as workspace reach.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/ssh-connection-manager.md), IDs above.
- [System design](../../specs/executors/system-design/ssh-connection-manager.md), corresponding sections.
- [Custody ADR](../../decisions/2026-09-19-managed-ssh-credential-custody.md).
- Existing source seams, test cases, and fixtures identified in the manifest and design.

## Results

Recovery resumed on 2026-10-20 under the preserved implementation approval.
The following results describe that recovery handoff; the later closure is below.

Managed executor binding/adoption, saved route stamps, runtime recovery references, and reachability integration restored and reconciled with current ownership/save coordination. Targeted tests pass. Real OpenSSH direct and bastion adoption now preserves the target and pin, refuses unverified or stale configuration changes, and authenticates after restart. Managed executor launch/reconnect and the legacy launch/recovery cases also have fresh container evidence. The complete browser adoption matrix, dependency details for refused deletion, and current-main orphan-sweep integration for managed connections remain open.

See [recovery provenance](recovery/recovery.md) and
[current validation](recovery/validation.md). Historical test reports are not
current acceptance. At that recovery handoff, this work order was not complete.


## Local closure (2026-10-25)

Managed orphan cleanup now holds the shared saved-executor connection until cleanup ends, with no inline fallback. Composition, route protection, task/process fences, real workload restart, retained-session dependency navigation, and browser adoption checks pass.

See [acceptance and validation](recovery/acceptance-2026-10-25.md) for exact
evidence, limits, and fixture cleanup. This is local implementation completion;
CI and PR review remain deferred.
