---
id: "06-recovery-and-rollout"
title: "Restart and disabled-path integration"
status: done
wave: 6
depends_on:
  - "05-executor-bindings"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.3
  - AC-EXECUTORS-SSH-CONNECTIONS-001.4
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-003.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-005.1
  - AC-EXECUTORS-SSH-CONNECTIONS-005.2
  - AC-EXECUTORS-SSH-CONNECTIONS-005.3
  - AC-EXECUTORS-SSH-CONNECTIONS-005.4
  - AC-EXECUTORS-SSH-CONNECTIONS-005.5
  - AC-EXECUTORS-SSH-CONNECTIONS-003.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 06: Restart and disabled-path integration

## Summary

Complete the cross-process recovery and rollout contract with executable real-host evidence and operator documentation. This is the final feature integration slice, not an extra generic QA phase.

## In scope

- Add real managed direct/single-hop runtime restart/no-agent and missing-key recovery tests, including retained bastion pin/identity and dependent-route conflicts; interrupted enrollment must remain unknown and never replay.
- Complete off-state route/direct-call/runtime coverage, feature registry contract checks, and all final changed-path checks.
- Update public executor how-to and secret protection/recovery reference alongside behavior, backend scoped guidance for new ownership, and spec statuses only after actual implementation passes.

## Out of scope

Production work outside this order, remote Docker runtime implementation, and
changes to the confirmed credential ownership boundary. No publishing or delegation.

## Acceptance

- A managed task reconnects after backend restart without an inherited agent; key loss, pin changes, and connection revision conflicts fail before remote workload action.
- Disabled paths retain data, refuse managed remote actions/decryption, and preserve legacy behavior; browser and runtime evidence agree.
- Public docs explain bootstrap prerequisites, backup custody, deletion versus revocation, feature enable/restart, manual-copy fallback, and compatibility limits; final evidence records all required checks without counting skips as passes.

## Verification

Use TDD for changed logic. Commands below are run from the repository root;
read the [prerequisites](plan.md#verification-prerequisites) first. Tests named
in this package are created by their owning order, not claimed to exist today.

```bash
(cd apps/backend && go test ./internal/sshtransport ./internal/sshconnections ./internal/secrets ./internal/ssh ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(test -n "$KANDEV_TEST_POSTGRES_DSN" && cd apps/backend && go test ./internal/sshconnections ./internal/secrets -run TestPostgres -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(ManagedSSH|SSH|ConnectionReady)' -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'Test(SSHConnections|ManagedSelectors)' -count=1)
make -C apps/backend lint
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections lib/api/domains/ssh-connections-api.test.ts lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/ssh-identities.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-connections-disabled.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-ssh-identities.spec.ts tests/settings/mobile-ssh-connections.spec.ts)
(cd apps/web && pnpm e2e:run --project auth tests/auth/ssh-connections-permissions.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/ssh/managed-enrollment.spec.ts tests/ssh/managed-connections.spec.ts tests/ssh/managed-restart.spec.ts tests/ssh/auth-methods.spec.ts tests/ssh/proxy-jump.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/sshconnections/recovery_test.go (new)`
- `apps/backend/internal/backendapp/ssh_connections_test.go (new)`
- `apps/web/e2e/tests/ssh/managed-restart.spec.ts (new)`
- `apps/web/e2e/tests/settings/ssh-connections-disabled.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-ssh-connections.spec.ts (new)`
- `docs/public/executors.md`
- `docs/public/configuration.md`
- `docs/public/operations.md`
- `apps/backend/AGENTS.md`
- `docs/specs/executors/requirements/ssh-connection-manager.md`
- `docs/specs/executors/system-design/ssh-connection-manager.md`
- `docs/plans/ssh-connection-manager/`

## Dependencies

Task 05; see the [manifest](plan.md#work-orders).

## Risks

Use disposable backend state, master keys, and SSH hosts only. Test authoring must not delete a real protection key. No new gate promotion, publication, or automatic GitHub actions are included.

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

Recovery hooks, default-off gates, settings gates, and public documentation are restored. A fresh process reopens the database/vault and authenticates retained direct and bastion snapshots without SSH agent variables. Missing ciphertext and a different protection key fail before authentication without identity replacement. Authenticated browser coverage verifies administrator access and member rejection before management actions. A rebuilt real OpenSSH test now proves managed workload launch and backend restart without an inherited SSH agent, including health through the restored tunnel. That test exposed and verified a fix for startup recovery bypassing the managed resolver. The new Go regression checks valid, missing-stamp, stale-generation, and disabled cases without legacy fallback. Desktop-specific acceptance and the full failure matrix remain open.

See [recovery provenance](recovery/recovery.md) and
[current validation](recovery/validation.md). Historical test reports are not
current acceptance. At that recovery handoff, this work order was not complete.


## Local closure (2026-10-25)

The native desktop process test authenticates a generated managed identity before and after a real backend restart without SSH agent variables. Managed and legacy OpenSSH restart checks pass. Remaining acceptance is reconciled, public documentation is current, and shipped feature defaults remain off.

See [acceptance and validation](recovery/acceptance-2026-10-25.md) for exact
evidence, limits, and fixture cleanup. This is local implementation completion;
CI and PR review remain deferred.
