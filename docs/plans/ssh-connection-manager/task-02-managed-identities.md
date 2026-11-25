---
id: "02-managed-identities"
title: "Managed identity settings"
status: done
wave: 2
depends_on:
  - "01-connection-foundation"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.1
  - AC-EXECUTORS-SSH-CONNECTIONS-001.2
  - AC-EXECUTORS-SSH-CONNECTIONS-001.3
  - AC-EXECUTORS-SSH-CONNECTIONS-001.4
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-001.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.1
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 02: Managed identity settings

## Summary

Deliver the first usable vertical slice: an administrator can generate/import a key, copy its public key, and manage safe metadata from desktop or phone.

## In scope

- Implement bounded generate/import/file/agent identity APIs and identity UI; no private-key export.
- Add hidden-secret exclusion regression coverage, command idempotency, transient input clearing, and reference-aware deletion.
- Wire settings route/discovery, scoped domain hooks/client/state, shared Save changes, permission gating, and all locales.

## Out of scope

Production work outside this order, remote Docker runtime implementation, and
changes to the confirmed credential ownership boundary. No publishing or delegation.

## Acceptance

- Generation/import succeeds without SSH_AUTH_SOCK; wrong passphrase creates nothing; ciphertext/private inputs are absent from user-facing projections.
- Desktop and phone can generate/import/copy and recover from errors, with secret clearing, safe focus/back behavior, and required target geometry.
- Only org.config.manage may manage identities; duplicate command UUIDs cannot issue a second key.

## ASCII UI preview

[Combined previews](plan.md#ascii-ui-preview). Applicable ACs are listed above;
retain the same structure and state rules in the rendered result.

### UI-01: Identity creation, desktop and phone

Entry: Settings > Executors > SSH connections > Identities. Empty or selected identity.

```text
Desktop
SSH connections   [Connections] [Identities]
No identities yet                         [Generate key] [Import key]
Selected: Build host key | Ed25519 | SHA256:... | Used by 2 connections
Public key [ssh-ed25519 AAAA...                     ] [Copy]
Protected by Kandev. Available after backend restart.

Phone: direct page, one scroll region
< SSH connections       Identities
No identities yet
[Generate key]
[Import key]

< Identities            Build host key
Ed25519   SHA256:...
[Public key, wrapped]
[Copy public key]
Protected by Kandev. Available after restart.
```

Maps to 001.1-001.3 and 004.1/004.3/004.4. A wrong import passphrase keeps the
non-secret label but clears key/passphrase inputs, shows an inline error, and
allows an explicit retry. No private-key reveal/export action exists.


## Verification

Use TDD for changed logic. Commands below are run from the repository root;
read the [prerequisites](plan.md#verification-prerequisites) first. Tests named
in this package are created by their owning order, not claimed to exist today.

```bash
(cd apps/backend && go test ./internal/sshconnections ./internal/secrets -count=1)
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections/identity-form.test.tsx lib/api/domains/ssh-connections-api.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/ssh-identities.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-ssh-identities.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/sshconnections/identity.go (new)`
- `apps/backend/internal/sshconnections/identity_test.go (new)`
- `apps/backend/internal/sshconnections/handlers.go (new)`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/web/components/settings/ssh-connections/ (new)`
- `apps/web/src/settings-routes.tsx`
- `apps/web/lib/settings-discovery/navigation.ts`
- `apps/web/lib/api/domains/ssh-connections-api.ts (new)`
- `apps/web/hooks/domains/settings/`
- `apps/web/lib/state/slices/settings/`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/ssh-identities.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-ssh-identities.spec.ts (new)`

## Dependencies

Task 01; see the [manifest](plan.md#work-orders).

## Risks

Generating/importing is an explicit command, not a shared-save side effect. Explain loss of original passphrase unlock protection before import. Verify secret data does not reach browser persistence.

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

Identity generation/import/reference UI and internal vault are restored. Protected import and secret-visibility unit tests pass. Desktop and phone browser coverage now generates and copies a key, rejects a wrong import passphrase, imports a protected key, and checks secret clearing after cancellation. Late identity responses cannot navigate after dismissal. Localization checks pass. Dependency details for refused deletion remain an implementation gap.

See [recovery provenance](recovery/recovery.md) and
[current validation](recovery/validation.md). Historical test reports are not
current acceptance. At that recovery handoff, this work order was not complete.


## Local closure (2026-10-25)

Identity reads and refused deletion now identify connection, executor, and retained-session dependencies, including bastion use. Desktop/phone links and concurrent refusal refresh are implemented and tested. Unused-key deletion removes local storage without changing remote authorization.

See [acceptance and validation](recovery/acceptance-2026-10-25.md) for exact
evidence, limits, and fixture cleanup. This is local implementation completion;
CI and PR review remain deferred.
