---
id: "03-trusted-connections"
title: "Trusted connection setup"
status: done
wave: 3
depends_on:
  - "02-managed-identities"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-002.1
  - AC-EXECUTORS-SSH-CONNECTIONS-002.2
  - AC-EXECUTORS-SSH-CONNECTIONS-002.3
  - AC-EXECUTORS-SSH-CONNECTIONS-002.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.1
  - AC-EXECUTORS-SSH-CONNECTIONS-004.2
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-002.7
  - AC-EXECUTORS-SSH-CONNECTIONS-002.8
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 03: Trusted connection setup

## Summary

An administrator can save a draft connection, establish explicit host trust, and diagnose/test the selected identity without binding an executor yet.

## In scope

- Implement connection CRUD, conditional route revisions, per-hop observation receipts, separate bastion/destination trust and identities, local preflight, and pinned tests. Bastion observation sends no credentials; destination observation may authenticate only the already trusted bastion.
- Complete the focused desktop/phone setup flow, identity picker, failure-code translations and stale-response rejection.
- Use in-process SSH servers in Go tests for real host-key/auth ordering; browser tests cover real backend metadata flow and unavailable prerequisite states.

## Out of scope

Production work outside this order, remote Docker runtime implementation, and
changes to the confirmed credential ownership boundary. No publishing or delegation.

## Acceptance

- No hop receives credentials before its own host trust; distinct identities and both pins are enforced, chain/cycle requests cause no dial, and bastion edits invalidate dependent routes; changed pins, expired/foreign receipts and stale results cannot authorize progress.
- Preflight distinguishes proven local failures from generic remote auth rejection; drafts survive navigation without retaining secrets.
- Phone has focused direct routes and an inset identity picker; no overflow, inaccessible action, or duplicated business logic.

## ASCII UI preview

[Combined previews](plan.md#ascii-ui-preview). Applicable ACs are listed above;
retain the same structure and state rules in the rendered result.

### UI-02: Connection setup, desktop and phone

Entry: Settings > Executors > SSH connections > New connection, or an executor-editor link.
State shown: target saved; host trust pending. Existing list rows show name,
remote user/host, identity, and readiness; tapping a phone row opens its route.

```text
Desktop
SSH connections                      [New connection]
Build box  | builder@build.example | Setup incomplete

Build box
[Target] -> [Host trust] -> [Identity] -> [Access]
Host [build.example] Port [22] User [builder]
Route [Via bastion v]  Bastion [Gateway: Ready v] [Create bastion]
Bastion: ops@gateway.example | Identity: Gateway key | Trusted SHA256:...
Destination fingerprint SHA256:...
Verify this fingerprint using a trusted source.
[Trust this host]                         [Continue]

Phone: focused full-page step
< SSH connections            Build box
Step 2 of 4: Host trust
builder@build.example:22
Via Gateway [Change]
Bastion: Trusted | Gateway key
Destination SHA256:...
Verify using a trusted source.
[Trust this host]
[Continue]
```

Maps to 002.1-002.3, 002.7-002.8 and 004.1-004.4. Target edits use the existing shared Save
changes coordinator; Continue saves through it before moving to the next step.
Phone identity/bastion selection uses temporary inset Drawers. Create bastion
opens a separate direct setup page and returns with the destination draft
preserved; no nested dialogs. Only ready direct bastions are selectable. A changed fingerprint shows OLD/NEW and blocks
authentication; an explicit replacement-trust action is separate from Test.


## Verification

Use TDD for changed logic. Commands below are run from the repository root;
read the [prerequisites](plan.md#verification-prerequisites) first. Tests named
in this package are created by their owning order, not claimed to exist today.

```bash
(cd apps/backend && go test ./internal/sshtransport ./internal/sshconnections -count=1)
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections/connection-steps.test.tsx lib/api/domains/ssh-connections-api.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/ssh-connections.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-ssh-connections.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/sshconnections/connection.go (new)`
- `apps/backend/internal/sshconnections/trust_test.go (new)`
- `apps/backend/internal/sshconnections/proxyjump_test.go (new)`
- `apps/backend/internal/sshconnections/preflight_test.go (new)`
- `apps/backend/internal/sshtransport/`
- `apps/web/components/settings/ssh-connections/connection-steps.tsx (new)`
- `apps/web/components/settings/ssh-connections/connection-steps.test.tsx (new)`
- `apps/web/hooks/domains/settings/`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/ssh-connections.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-ssh-connections.spec.ts (new)`

## Dependencies

Task 02; see the [manifest](plan.md#work-orders).

## Risks

Observation is untrusted evidence, not permission to authenticate. Config aliases resolve to displayed concrete values; do not silently follow later alias edits.

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

Connection editing, trust observations/receipts, route revisions, single-hop guards, and preflight errors are restored. Protocol tests reject changed host keys at either hop before credentials are sent. UI regressions now cover duplicate mutation admission and obsolete load responses. The full connection and executor browser matrix remains open.

See [recovery provenance](recovery/recovery.md) and
[current validation](recovery/validation.md). Historical test reports are not
current acceptance. At that recovery handoff, this work order was not complete.


## Local closure (2026-10-25)

Desktop and touch-phone migration now covers trusted direct access, one bastion, missing pins, cancellation, concurrent edits, and unsupported chains using real OpenSSH. Existing hop trust, stale-response, and diagnostic coverage remains applicable.

See [acceptance and validation](recovery/acceptance-2026-10-25.md) for exact
evidence, limits, and fixture cleanup. This is local implementation completion;
CI and PR review remain deferred.
