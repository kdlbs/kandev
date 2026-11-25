---
created: 2026-09-19
status: done
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
legacy_specs: []
---

# Implementation Plan: SSH connection manager

## Current local state

Local implementation and acceptance of the six work orders are complete as of
2026-10-25. The continuation closes managed orphan cleanup, identity dependency
details, browser adoption coverage, native desktop restart, and PostgreSQL
transactions. [Final acceptance](recovery/acceptance-2026-10-25.md) records the
mapping, command results, environment failures, cleanup, and platform limits.
The recovered implementation approval remains authoritative; original source
IDs and artifacts remain in the [recovery note](recovery/recovery.md).

The [navigation follow-up](../ssh-connections-executor-navigation/plan.md) is also
implemented and locally verified. Draft PR [#4778](https://github.com/kdlbs/kandev/pull/4778)
contained commits through `20661f71af0c8ac42f233d8de418411088ced639` at the
2026-10-25 handoff. The user authorized committing and pushing the continuation
on 2026-10-26. CI and review comments remain deferred by the user.
No running main or seeded instance was changed. The feature remains disabled in
shipped profiles; this completion does not promote or release it.

## Overview

Deliver reusable, admin-managed SSH connections with unattended managed keys
and explicit assisted enrollment. Implement sequentially: shared transport and
storage, identity management, trusted connections, enrollment, executor
consumption, then integrated restart/compatibility evidence and public docs.
Implementation was authorized on 2026-09-22 and resumed from recovered evidence
on 2026-10-20. All six work orders contain recovered implementation, with remaining local
acceptance completed on 2026-10-25. See the [recovery record](recovery/recovery.md)
for approvals, reconstruction limits, current validation, and remaining work.

Sources: [#3735](https://github.com/kdlbs/kandev/issues/3735),
[requirements](../../specs/executors/requirements/ssh-connection-manager.md),
[design](../../specs/executors/system-design/ssh-connection-manager.md), and
[custody ADR](../../decisions/2026-09-19-managed-ssh-credential-custody.md).

## Provenance and moving dependencies

- Intake owner: zeval, reverified sole assignee on 2026-09-19. Claim timestamp:
  2026-09-19T14:32:04.959823+00:00; eyes reaction ID: 514704201.
- Existing task: 74961be3-8c1a-4c16-ac36-3eff5f7455e8; session:
  e60c5f55-efb3-43f9-926d-0a985337679e; intake task:
  4b90f67b-010a-454e-9f68-74fe712cf037. No duplicate tasks.
- Original design baseline: 794773dd6887ae5c326ee77757dedd7bafb1ecf7. Fetched main:
  9e48a71dbe502408f3678f6bfefd9ee87490c3cc. Its intervening repository-checkout
  options changes do not add SSH custody; do not reset or rebase user work.
- Related remote Docker PR #3783 head:
  b7edaee8f00bf31ee0d0f52467842cbccc0d3312. Reachability PR #3747 head:
  67c76e43f31549b68656c608fa4cb25d4f2c6301. These are snapshots, not pinned
  dependencies. Re-resolve before implementation and reconcile newly landed
  consumers; do not push to contributor branches.
- Existing companion contracts: SSH executor design and
  docs/plans/ssh-executor/e2e-plan.md, ssh-transport-liveness,
  ssh-dead-runtime-resume, and repository-secrets plans. Preserve their behavior;
  no historic completion status is changed by this design package.

On recovery, this isolated branch was fast-forwarded to verified remote main
`e34761417d28e561ce9bd616cfa78c1b723df9ad`. Remote Docker has landed.
The recovered transport adapters preserve its existing consumers. Managed
Remote Docker binding remains outside this package's approved scope. Current
executor ownership, startup sequencing, and settings save coordination remain
authoritative.

## Scope

In scope: managed generation/import, external identity references, connection
CRUD/trust/readiness including single-hop ProxyJump, public-key copy and assisted
enrollment through the selected route, SSH profile
binding/adoption, frontend/mobile flows, internal audit, and default-off gate.
Out of scope: remote Docker runtime implementation, chained ProxyJump, private
key export, automatic remote revocation/rotation, per-user/workspace sharing,
interactive unlock after restart, sudo and keyboard-interactive bootstrap.

## Technical approach

Follow the paired design's proposed packages internal/sshtransport and
internal/sshconnections. Extract existing low-level SSH machinery with adapters;
reuse the secrets encryption provider with a purpose-specific transactional
internal-secret capability. Extend the existing scope resolver and feature
registry instead of adding parallel authority/config maps. Runtime consumers
receive a connection dial capability, never raw secret IDs or key bytes.

Tables: ssh_identities, ssh_connections, enrollment operation state, and
ssh_connection_audit in the existing DB, with SQLite/PostgreSQL parity.
Reserved secret prefix: ssh-identity:. Gate: features.sshConnectionManager /
KANDEV_FEATURES_SSH_CONNECTION_MANAGER, false in prod/dev/e2e, restart required.
Settings metadata uses shared Save changes; remote commands have explicit
buttons. Implement all catalog changes and translations alongside each UI slice.

Existing public SSH requirement .4 remains the legacy agent/file contract;
managed behavior is owned by the new pair. The current requirement .5 mentions
IdentitiesOnly despite code/design/docs excluding it. Record that pre-existing
drift; do not implement IdentitiesOnly or silently broaden this package to fix it.

## ASCII UI preview

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

## Tests

All test names below are proposed unless identified as existing. Go names are
in the named package; frontend names are in the new settings/ssh-connections
component family. Every work order writes its regression tests before behavior.

| ACs (prefix AC-EXECUTORS-SSH-CONNECTIONS-) | Test file and proposed cases | Work |
| --- | --- | --- |
| 001.1-001.4 | sshconnections/identity_test.go: TestIdentityGenerateImport, TestIdentityUnavailable; secrets/user_visible_store_test.go: TestSSHIdentityHidden | 01, 02 |
| 001.5 | sshconnections/authorization_test.go: TestManagementAuthorization, TestRuntimeUseBoundToExecutor; backendapp/ssh_connections_test.go: TestSSHConnectionsAuthSetup | 01, 05 |
| 001.6, 002.6 | sshconnections/store_test.go: TestReferenceDeletion, TestConditionalMutations; postgres_store_test.go: TestPostgresSSHConnectionsTransactions | 01, 05 |
| 002.1-002.3 | sshconnections/trust_test.go: TestObservationTrustReceipt, TestStaleReadiness, TestHostChangeBeforeCredentials | 03 |
| 002.7-002.8 | sshconnections/proxyjump_test.go: TestManagedProxyJumpDistinctIdentitiesAndPins, TestManagedProxyJumpDependencyInvalidation, TestManagedProxyJumpRejectsChainsAndCycles; sshtransport/proxyjump_test.go: TestManagedProxyJumpClosesBothHops | 01, 03, 05 |
| 002.4-002.5 | lifecycle/executor_ssh_managed_test.go: TestManagedSSHBinding, TestManagedSSHRevisionMismatch | 05 |
| 003.1-003.4 | sshconnections/enrollment_test.go: TestEnrollmentModes, TestEnrollmentPreservesAuthorizedKeys, TestEnrollmentFreshAuthentication | 04 |
| 003.6 | sshconnections/enrollment_test.go: TestEnrollmentViaBastionIsDestinationOnly; recovery_test.go: TestManagedProxyJumpRestart | 04, 06 |
| 003.5 | sshconnections/enrollment_recovery_test.go: TestEnrollmentUnknownOutcome, TestEnrollmentIdempotencyAndRestart | 04, 06 |
| 004.1, 004.3 | identity-form.test.tsx, connection-steps.test.tsx, enrollment-form.test.tsx: stale responses, explicit effects, secret clearing | 02, 03, 04 |
| 004.2 | sshconnections/preflight_test.go: TestPreflightClassification; connection-steps.test.tsx: safe code-to-action mapping | 03 |
| 004.4 | settings and mobile browser files below; pseudo-locale and computed target geometry | 02-05 |
| 004.5 | lifecycle/executor_ssh_managed_test.go: TestConnectionReadyIsNotExecutorReady; managed-binding.test.tsx | 05 |
| 005.1-005.2 | sshconnections/adoption_test.go: TestLegacyAdoptionPreservesConfig; existing lifecycle SSH connection/dial tests | 01, 05 |
| 005.3 | sshconnections/feature_gate_test.go: TestDisabledNoSideEffects; backendapp/ssh_connections_test.go: TestManagedSelectorsCannotBypassGate; existing feature contract tests | 01, 05, 06 |
| 005.4 | sshconnections/audit_test.go: TestAuditRedactionAndFailure; enrollment_recovery_test.go: final audit failure | 01, 04 |
| 005.5 | sshconnections/recovery_test.go: TestProtectionKeyMismatch; real runtime restart test below | 01, 06 |

## E2E tests

Browser paths below are relative to apps/web/e2e. All use a real isolated backend.
Settings files cover key creation/import, draft navigation, public-key copy,
error handling, and local management; real remote operations use SSH fixtures.
Phone coverage proves the same generate/copy/setup value and the enrollment
form's navigation/state; wire-level enrollment correctness is also proven by
backend tests and the real-host containers suite. No backend response stubs.

| File | Project | Scenarios / ACs |
| --- | --- | --- |
| tests/settings/ssh-identities.spec.ts | chromium | generate/import/copy, secret clearing and unavailable identity; 001.1-001.4, 004.1/3 |
| tests/settings/mobile-ssh-identities.spec.ts | mobile-chrome | same generate/import/copy, back/focus, geometry; 004.4 |
| tests/settings/ssh-connections.spec.ts | chromium | save/reopen draft, missing prerequisites, conflicting edits; 002.1/3/6, 004.1-004.3 |
| tests/settings/mobile-ssh-connections.spec.ts | mobile-chrome | focused steps, identity/bastion Drawers, create-bastion return, disabled enrollment without trust, no overflow; 002.2/7/8, 004.4 |
| tests/auth/ssh-connections-permissions.spec.ts | auth | member cannot manage/reveal/override, authorized task consumption; 001.5 |
| tests/ssh/managed-enrollment.spec.ts | containers | disposable sshd password/existing identity over direct/single-hop routes, fresh auth, each changed host, unchanged bastion authorized_keys; 002.2/3/7, 003.1-003.6 |
| tests/ssh/managed-connections.spec.ts | containers | multiple profiles, distinct hop identities/pins, direct/jump adoption, bastion revision conflict and chain rejection; 002.4-002.8, 004.5, 005.1/2 |
| tests/ssh/managed-restart.spec.ts | containers | no SSH_AUTH_SOCK over either route, both hop records preserved, restart outcome unknown, missing master key; 001.3/4, 002.7, 003.5/6, 005.5 |
| tests/settings/ssh-connections-disabled.spec.ts | chromium | hidden settings and direct managed API rejection; legacy unaffected; 005.3 |

Use existing fixtures/ssh-test-base.ts and helpers/ssh.ts, extended only for
isolated password-enabled/unavailable targets and a two-sshd bastion/target
route. Make the destination inaccessible directly from the backend so tests
cannot pass by silently bypassing the bastion. Never enroll against a developer
host. Gate is enabled explicitly per worker/spec and reset on teardown. New
mobile files remain outside tests/ssh so ordinary mobile runs do not acquire a
Docker dependency. Run projects sequentially with the managed resource budget.

## Work orders

- [x] [Task 01: Shared transport and protected connection store](task-01-connection-foundation.md)
- [x] [Task 02: Managed identity settings](task-02-managed-identities.md)
- [x] [Task 03: Trusted connection setup](task-03-trusted-connections.md)
- [x] [Task 04: Assisted enrollment](task-04-assisted-enrollment.md)
- [x] [Task 05: Executor binding and explicit adoption](task-05-executor-bindings.md)
- [x] [Task 06: Restart and disabled-path integration](task-06-recovery-and-rollout.md)

Dependency order: 01 -> 02 -> 03 -> 04 -> 05 -> 06. Work is sequential; no
subagents or persistent child tasks are authorized. Each order names exact
commands and acceptance IDs. Run relevant commands once after its final change;
repeat prior tests only when later integration changes the paths they cover.

## Verification prerequisites

Commands run from the repository root and use isolated subshells for package
paths. Put Go on PATH; this cloud image uses /usr/local/go/bin/go. In a fresh
worktree first run `(cd apps && pnpm install --frozen-lockfile)` once. PostgreSQL
checks require KANDEV_TEST_POSTGRES_DSN pointing to a disposable server and must
not be reported as passed when skipped. Real SSH tests require Docker and use
KANDEV_E2E_CONTAINERS=1. Managed e2e:run rebuilds backend/web/fixtures; do not
reuse stale artifacts or overlap full suites. Use disposable local services for validation; do not change live SSH access.

## Verification results

Historical design checks on 2026-09-19:

- `python3 scripts/list-docs.py validate`: passed, 292 decisions and 1040 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/ssh-connection-manager`: passed.
- Revalidated after adding managed single-hop ProxyJump; linter tests: 36 passed.
- Local link/traceability audit: five requirement IDs, 30 acceptance criteria,
  six pending work orders; every AC assigned and every artifact link resolves.
- Status and whitespace checks included all new untracked artifacts.

Those checks preceded implementation. Recovered implementation and fresh checks
are recorded in [current validation](recovery/validation.md). Proposed test names
in this manifest are acceptance targets; a missing test is not a passed test.

## Risks

- Internal installation secrets must bypass only user-secret ownership, never
  caller authorization, and remain hidden from all generic secret consumers.
- A bastion change invalidates every dependent route. Validate both revisions
  at admission, trust, enrollment and resume; never fall back to a direct dial.
- A trusted but wrong host can receive a bootstrap password; independent host
  fingerprint verification is mandatory and cannot be automated away.
- Remote authorized_keys edits are not a distributed transaction. Preserve
  unknown outcomes, idempotent explicit retry, and external editor limitations.
- Transport edits are blocked while live/resumable references remain. Surface
  their existing stop/reset/archive actions; stale generation mismatches still
  fail closed rather than redirecting a runtime.
- Unattended custody relies on backend host and master-key backup protection.
- Main/#3783 can move. Reconcile actual call sites and their plans before work
  05; this package is not authorization to copy or publish contributor work.
- Identity generation/import and remote enrollment are explicit effects; shared
  settings save must never replay them as ordinary form persistence.


## Proposed UX follow-up (2026-10-26)

The user requested a less crowded SSH manager after testing this implementation.
The [focused management package](../ssh-connections-ux/plan.md) proposes a new
collection/overview/access composition while preserving this package's verified
security and continuation work. The follow-up remains pending. The completed
results above describe this package, not the proposed UI.
