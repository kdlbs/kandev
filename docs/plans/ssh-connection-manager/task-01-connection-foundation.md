---
id: "01-connection-foundation"
title: "Shared transport and protected connection store"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.1
  - AC-EXECUTORS-SSH-CONNECTIONS-001.4
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-001.6
  - AC-EXECUTORS-SSH-CONNECTIONS-002.6
  - AC-EXECUTORS-SSH-CONNECTIONS-005.1
  - AC-EXECUTORS-SSH-CONNECTIONS-005.3
  - AC-EXECUTORS-SSH-CONNECTIONS-005.4
  - AC-EXECUTORS-SSH-CONNECTIONS-005.5
  - AC-EXECUTORS-SSH-CONNECTIONS-002.7
  - AC-EXECUTORS-SSH-CONNECTIONS-002.8
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 01: Shared transport and protected connection store

## Summary

Provide the shared transport and persistence boundary on which managed setup can operate. Preserve legacy behavior while making internal credential storage transactional and inaccessible through user-secret APIs.

## In scope

- Extract transport with adapters and move low-level tests without duplicating implementations. Keep agentctl/workload logic in lifecycle.
- Implement optional direct-bastion references with cycle/chain rejection, route revision and deletion guards; metadata/audit schema and prefix-constrained internal-secret transaction support on SQLite and PostgreSQL; use existing DB composition.
- Add default-off feature identity through profiles, config, registry/config binding, frontend defaults, and disabled service construction. Add role authorization, deletion/revision guards, audit redaction, and protection-key mismatch handling.

## Out of scope

Production work outside this order, remote Docker runtime implementation, and
changes to the confirmed credential ownership boundary. No publishing or delegation.

## Acceptance

- Legacy auth/config/ProxyJump regressions pass through the shared transport; managed single-hop transport accepts separately pinned/authenticated hops and closes the tunnel and bastion on success/failure.
- Metadata/backing-secret operations roll back together, internal secrets cannot be listed/revealed/copied/moved/bound, and creating-user deletion/auth setup does not break approved runtime access.
- Disabled service entry rejects before secret access/dial; default/retired flag contracts and audit failure behavior are covered.

## Verification

Use TDD for changed logic. Commands below are run from the repository root;
read the [prerequisites](plan.md#verification-prerequisites) first. Tests named
in this package are created by their owning order, not claimed to exist today.

```bash
(cd apps/backend && go test ./internal/sshtransport ./internal/sshconnections ./internal/secrets ./internal/authz ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(InheritFromSSHConfig|ApplyTargetDefaults|BuildAuthMethods|BuildClientConfig|DialSSH|DialViaJump|ResolveBastion|BastionHostKey|IdentityAgent|EffectiveSSHConfig)' -count=1)
(test -n "$KANDEV_TEST_POSTGRES_DSN" && cd apps/backend && go test ./internal/sshconnections ./internal/secrets -run TestPostgres -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/features/features-contract.test.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/sshtransport/ (new)`
- `apps/backend/internal/sshconnections/ (new)`
- `apps/backend/internal/secrets/`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_dial_test.go`
- `apps/backend/internal/backendapp/storage.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/runtimeflags/config.go`
- `apps/backend/internal/common/config/config.go`
- `profiles.yaml`
- `apps/web/lib/state/slices/features/types.ts`

## Dependencies

None. Recheck main and related PR heads before extracting transport.

## Risks

Preserve existing exported lifecycle seams while moving internals. Do not treat ScopeGlobal user secrets as installation secrets or drop an HTTP identity to bypass authorization.

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

Shared transport, protected identity storage, SQL schema, audit, runtime references, and disabled feature registration restored. Focused Go tests pass. Both recovered PostgreSQL cases pass on a disposable server. Complete acceptance mapping remains open.

See [recovery provenance](recovery/recovery.md) and
[current validation](recovery/validation.md). Historical test reports are not
current acceptance. At that recovery handoff, this work order was not complete.


## Local closure (2026-10-25)

The remaining PostgreSQL transaction and concurrency cases pass. Both-dialect store conformance, SQL guard, focused transport/credential tests, and changed-package lint pass. The acceptance record maps the recovered implementation to its contracts.

See [acceptance and validation](recovery/acceptance-2026-10-25.md) for exact
evidence, limits, and fixture cleanup. This is local implementation completion;
CI and PR review remain deferred.
