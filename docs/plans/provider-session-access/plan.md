---
created: 2026-09-26
status: in_progress
requirements:
  - REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001
system_design:
  - ../../specs/integrations/system-design/provider-session-access.md
legacy_specs: []
---

# Implementation Plan: Direct provider access for managed plugin sessions

## Overview

Replace PR #3165's server-performed CI rerun with a reusable Host grant and
credential lease. The installed Coordinator plugin calls GitHub directly and
owns the provider-action ledger. Build and test the grant before retiring the
old proxy so the draft branch always has a reviewable transition path.

## Scope

### In scope

- Authenticated, generation-scoped, expiring administrator grants for one
  plugin installation, managed conversation, workspace, target task and
  repository, provider, and permission purpose.
- Exact-session Host lease issuance/redemption/release with H6 approval,
  provider identity checks, redacted audit, and no agent-visible token.
- Fresh uncached, one-repository GitHub App token minting and explicit
  provider-side token revocation; GitLab fails closed pending an eligible
  short-lived token source.
- A separately owned Coordinator-plugin adapter that validates the live
  PR/fork/run, calls GitHub directly, and owns idempotent action receipts.
- Remove the old `request_fresh_ci_run_kandev` proxy, its server operation
  ledger, obsolete public docs and coverage claims. Keep PR #3165 draft until
  the replacement has current-head review, QA, and CI evidence.

### Out of scope

- General Actions administration, workflow dispatch from mutable refs,
  arbitrary repository or provider scopes, merge/deploy/release, a new UI,
  live consumer CI, and delivery of workspace PATs or App private keys.

## Technical approach

`internal/provideraccess` owns grant/lease/audit persistence, admission,
idempotency and revocation. It reuses the existing database and auth patterns,
not Git HTTPS path-matching as provider API authority. `internal/plugins`
adapts connection-bound Host RPCs and H6 `host.v2.write:provider_access`
approval; `pkg/pluginsdk` and `proto/kandev/plugin/v1/plugin.proto` expose
`provider-access/v1` as distinct exact methods. `internal/task/service`
managed-conversation ownership and primary-session lookup validate the
plugin-selected session. `internal/github` supplies a new uncached App token
minter/revoker through the verified workspace installation; `backendapp`
composes the dependencies and lifecycle revocation hooks.

The plugin implementation remains in its dedicated repository under the
existing plugin program owner. Its current coordinator-policy `1.1.0`
contract and digest remain unchanged. `provider-access/v1` is a separate
versioned addition, with one provider adapter/receipt slice. Host and plugin
owners exchange request/response fixtures and a minimum compatible SDK/Host
version before either side claims integration complete.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-001.1, AC-001.7 | SQLite/Postgres grant, generation, audit and idempotency tests in `internal/provideraccess` |
| AC-001.2, AC-001.3 | Host integration tests for installation/approval/session/link/head/fork/generation drift |
| AC-001.4, AC-001.5 | SDK wire round-trip and GitHub HTTP fixtures asserting exact repository/permissions and token redaction |
| AC-001.6 | Session/grant/plugin/connection revocation tests, including provider revoke failure and restart residual |
| AC-001.8, AC-001.9 | Negative MCP catalog tests, plugin direct-call/readback tests, GitLab PAT fail-closure |

## Work orders

- [ ] [Task 01: Persist and authorize provider grants](task-01-grant-ledger.md)
- [ ] [Task 02: Expose exact Host lease and GitHub credential adapter](task-02-host-lease.md)
- [ ] [Task 03: Integrate the Coordinator provider adapter](task-03-plugin-adapter.md)
- [ ] [Task 04: Retire the old proxy and document the direct path](task-04-retirement.md)

## Verification results

Pending replacement implementation. The current-main merge at local
`1a9388dbb12aeb028877329b80e8d12b6627fa31` passed focused GitHub/MCP
tests and public-doc validation; it is not replacement acceptance evidence.

## Risks

- GitHub's installation token is one-repository/permission scoped, not PR/run
  scoped. The plugin is trusted to enforce the lease target. An unrevoked token
  after Host crash may remain usable until provider expiry.
- Existing `InstallationTokenCache` shares tokens and cannot be used for this
  per-lease revocation path.
- The Coordinator plugin currently has no admitted integration implementation
  owner or branch. Host completion alone cannot satisfy AC-001.8.
- Current GitLab workspace PATs cannot satisfy the temporary scoped grant.
- Provider token issuance is disabled while the Human decides whether a
  repository-wide Actions token that can outlive Host revocation failure is
  acceptable to deliver to trusted plugin code.

## Open questions

- Human security decision, routed by the Coordinator, on whether the bounded
  GitHub bearer window under uncached minting/provider revocation fits the
  approved direct-access direction. Until recorded, do not enable redemption
  or publish a plugin adapter that can obtain the token.
- Plugin program owner admission and minimum SDK/Host version for the
  `provider-access/v1` adapter.
