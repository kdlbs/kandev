---
id: "04-assisted-enrollment"
title: "Assisted enrollment"
status: done
wave: 4
depends_on:
  - "03-trusted-connections"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-003.1
  - AC-EXECUTORS-SSH-CONNECTIONS-003.2
  - AC-EXECUTORS-SSH-CONNECTIONS-003.3
  - AC-EXECUTORS-SSH-CONNECTIONS-003.4
  - AC-EXECUTORS-SSH-CONNECTIONS-003.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.2
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-005.4
  - AC-EXECUTORS-SSH-CONNECTIONS-003.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 04: Assisted enrollment

## Summary

Install a public key using explicitly supplied bootstrap access, then prove access with the destination identity. The flow preserves remote authorization and reports partial/unknown outcomes truthfully.

## In scope

- Implement durable idempotent enrollment state, bounded destination-only password/existing-identity bootstrap over direct or trusted single-hop routes, and append-only authorized_keys procedure with unsafe-path checks and lock handling.
- Require independent destination authentication; handle duplicate entries with options, no trailing newline, mid-operation disconnect, audit failures, cancellation, and restart without password replay.
- Add enrollment/recovery UI in the existing access step, translated messages, no secret persistence, and isolated sshd fixture coverage.

## Out of scope

Production work outside this order, remote Docker runtime implementation, and
changes to the confirmed credential ownership boundary. No publishing or delegation.

## Acceptance

- Host-pin check precedes bootstrap credential use; existing key restrictions survive and no password/private key enters remote command text or persisted state.
- Success requires fresh destination auth; duplicate requests, unknown outcomes and audit failures follow the specified state machine without silent rollback/retry.
- Desktop/phone access forms expose the same controls and clear transient inputs; real-host tests prove password and existing-identity enrollment over both routes, with the bastion authorized_keys unchanged.

## ASCII UI preview

[Combined previews](plan.md#ascii-ui-preview). Applicable ACs are listed above;
retain the same structure and state rules in the rendered result.

### UI-03: Enrollment and recovery

Entry: saved connection > Access. Target and trust fixed to the saved revision.

```text
Desktop
Build box: builder@build.example:22    Host SHA256:...
Via Gateway: ops@gateway.example       Bastion SHA256:...
Identity: Build host key              Key SHA256:...
[Copy public key]   Install using: ( ) Password ( ) Existing identity
Password [................]   Used once; never saved.
[Install public key and verify]
Status: Checking access...    [action disabled while request runs]

Phone: full-page Access step
< Identity                    Build box
Step 4 of 4: Access
builder@build.example:22
Via Gateway (trusted, Gateway key)
Host SHA256:...  Key SHA256:...
[Copy public key]
Install using [Password v]
Password [................]
Used once; never saved.
[Install public key and verify]

Recovery, both compositions
Key may already be installed. Verification failed.
[Check access again]   [Copy public key]
```

Maps to 003.1-003.6 and 004.2-004.4. Bootstrap applies to Build box only;
Gateway uses its saved identity. Bastion failures identify that hop and block
destination authentication/enrollment. Unknown outcome never displays Ready.
Wrong password clears the input and offers a new explicit attempt. No
credentials appear in URLs, logs, persisted form state, or event payloads.


## Verification

Use TDD for changed logic. Commands below are run from the repository root;
read the [prerequisites](plan.md#verification-prerequisites) first. Tests named
in this package are created by their owning order, not claimed to exist today.

```bash
(cd apps/backend && go test ./internal/sshconnections -run 'Test(Enrollment|Audit|HostChange|ManagedProxyJump)' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections/enrollment-form.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-ssh-connections.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/ssh/managed-enrollment.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/sshconnections/enrollment.go (new)`
- `apps/backend/internal/sshconnections/enrollment_test.go (new)`
- `apps/backend/internal/sshconnections/enrollment_recovery_test.go (new)`
- `apps/web/components/settings/ssh-connections/enrollment-form.tsx (new)`
- `apps/web/components/settings/ssh-connections/enrollment-form.test.tsx (new)`
- `apps/web/src/locales/`
- `apps/web/e2e/fixtures/ssh-test-base.ts`
- `apps/web/e2e/helpers/ssh.ts`
- `apps/web/e2e/tests/ssh/managed-enrollment.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-ssh-connections.spec.ts (new)`

## Dependencies

Task 03; see the [manifest](plan.md#work-orders).

## Risks

The remote operation cannot be atomic with local audit storage. Preserve started/outcome_unknown records, never infer no mutation from a timeout, and never break a remote lock automatically.

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

The production enrollment script now runs through disposable SSH protocol fixtures for password and identity bootstrap over direct and bastion routes. Tests preserve destination authorization bytes, leave bastion authorization unchanged, and retain uncertain outcomes without replay. Enrollment retains the failed hop and authentication diagnostic. Desktop/phone component tests cover explicit confirmation and password clearing. Real OpenSSH direct and bastion password bootstrap, bastion identity bootstrap, and direct identity bootstrap now pass. They preserve existing authorization bytes, leave bastion keys unchanged, make retries idempotent, and authenticate with the selected managed key after restart. The complete browser and failure-path acceptance matrix still needs final reconciliation.

See [recovery provenance](recovery/recovery.md) and
[current validation](recovery/validation.md). Historical test reports are not
current acceptance. At that recovery handoff, this work order was not complete.


## Local closure (2026-10-25)

Current protocol, PostgreSQL, and real OpenSSH checks cover both bootstrap modes, direct/bastion routes, authorization preservation, audit/partial failures, and no credential replay. The ten-case integration run passes alongside the earlier focused safety evidence.

See [acceptance and validation](recovery/acceptance-2026-10-25.md) for exact
evidence, limits, and fixture cleanup. This is local implementation completion;
CI and PR review remain deferred.
