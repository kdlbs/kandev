---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001
created: 2026-09-26
owners:
  - kandev
---

# Provider access for managed plugin sessions

## Boundary and mapping

The Host implements `REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001` through an
administrator grant, an exact managed-session lease, a transient provider
credential redemption, and redacted audit. The installed plugin performs every
provider operation. [ADR-2026-09-26](../../../decisions/2026-09-26-plugin-direct-provider-access.md)
replaces the action-specific server rerun decision. The existing
[plugin Host boundary](../../../decisions/2026-08-31-generic-plugin-host-boundary.md)
supplies connection-bound installation identity and workspace capability
approval; the [Git credential broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
is a pattern for exact scope and revocation, not an API-token transport.
The [threat model](../../../plans/provider-session-access/threat-model.md)
records the exported-bearer risk. Credential issuance and redemption remain
disabled until the Coordinator records a Human security decision.

| Contract | Design sections |
| --- | --- |
| AC-001.1, AC-001.2 | Grant and exact session admission |
| AC-001.3, AC-001.8 | Target binding and plugin operation boundary |
| AC-001.4, AC-001.5, AC-001.9 | Lease, redemption, and provider adapters |
| AC-001.6, AC-001.7 | Revocation, failure, persistence, and audit |

## Grant and exact session admission

`internal/provideraccess` owns a provider-neutral grant and lease ledger in
the application database. An authenticated administrator creates a grant for
one plugin installation, workspace, managed conversation key, target task,
repository, provider, permission purpose, and bounded expiry. The controller
uses the existing `authn` administrator and workspace checks; task MCP cannot
create grants. An exact-scope replacement increments the generation and
revokes the old generation transactionally. Grant and lease reads use the
write DB transaction for admission; concurrent replacements cannot leave two
active generations. Workspace deletion removes leases and audits before grants.

The Host RPC is an optional `pluginsdk` extension under the existing
connection-bound `kandev.plugin.v1.Host` service. Methods have distinct exact
names and typed DTOs: `IssueProviderAccessLeaseExact`,
`RedeemProviderAccessLeaseExact`, and `ReleaseProviderAccessLeaseExact`.
The plugin manifest declares `api_write: [provider_access]`; current H6
approval must contain `host.v2.write:provider_access` at the supplied
capability revision. The Host derives installation identity from the plugin
connection. It rejects a request whose installation, manifest digest, approval,
workspace or grant generation is stale or missing. No v1 compatibility
fallback synthesizes approval.

The Host resolves the requested managed conversation's backing task from
`internal/task/service` ownership metadata, then loads its primary session
from the task repository. It compares exact plugin ID, workspace, conversation
key, task and session IDs, and a nonterminal session state on every issuance
and redemption. A plugin-provided task/session ID is a selector, never proof.
The grant's target task must remain in the same workspace with the exact
repository attachment. The Host resolves the canonical provider connection
and active linked PR/MR from durable state; prompts, profile names, task titles,
remotes, and plugin configuration cannot choose the provider principal.

## Target binding and plugin operation boundary

`provider-access/v1` requests carry request and idempotency IDs, workspace,
managed task/session, target task/repository, provider, permission purpose,
linked PR/MR number, expected base and head repository/ref/SHA, operation
class, and a run/check identity when the purpose requires it. The Host rejects
unknown or incomplete target fields and compares them with the authoritative
task link and a current provider read. For a GitHub fork PR, the credential is
minted only for the canonical base repository; the fork is evidence, not a
token target. The Host's provider reads can deny a stale target but cannot
claim the plugin's later action is safe; the plugin reads the provider again
immediately before mutation and after any ambiguous response.

The lease contains the exact target digest, grant generation, approval
revision, connection generation, session identity, expiry, and a hash of the
idempotency key. It exposes only opaque lease ID and non-secret target metadata.
The plugin owns the provider action ledger: its idempotency key includes grant
generation, repository, PR/MR head, operation class, and source run/check
identity. For GitHub CI recovery, it validates a completed failed
`pull_request` source run, canonical base, exact head repository/ref/SHA and
attempt, prefers rerun-failed-jobs, denies fork or mutable-ref dispatch, and
reconciles an ambiguous send by readback rather than resending. The Host
returns no action-success result and never calls the provider mutation API.

## Credential redemption and provider adapters

Redemption is one-shot and requires the same live approval, grant generation,
managed session, target digest, provider connection generation, and unexpired
lease. A durable mint intent is claimed before the provider call; concurrent
redemption or restart cannot mint a second token for the same lease. A timeout
or crash around the provider response records an unknown mint with a
conservative expiry bound and never retries the mint blindly. The Host creates
a distinct credential for that lease and returns it
only on the gRPC response to the installed plugin. It never puts the token in
an agent tool response, task MCP, log, audit, persistence row, or environment.
The plugin keeps it in memory only long enough for direct provider requests
and calls `ReleaseProviderAccessLeaseExact` afterward. The lease cannot be
redeemed again to mint another token. A repeated issuance with the same
idempotency identity returns its existing non-secret lease; a changed scope
conflicts.

The GitHub adapter uses the existing App registration/connection resolver but
calls the App token minter without `InstallationTokenCache`, because the cache
shares tokens across equal repository/permission scopes. It requests exactly
one canonical base repository and, for rerun, only `actions:write`,
`pull_requests:read`, and `metadata:read`. It excludes `contents:read` for
rerun; any different operation needs a separate reviewed permission profile.
It rejects PAT, named CLI, executor and
legacy credentials. It stores an issued token only in process memory so it
can call GitHub's token-revocation endpoint on release or invalidation.
GitHub's own token expiry is the final bound after a Host crash or failed
revocation. The plugin is trusted to enforce the lease's PR/run target because
GitHub installation tokens cannot encode that restriction.

The provider-neutral interface returns `UNSUPPORTED` for GitLab while the
workspace connection is a PAT or `glab` token. A later GitLab adapter must
prove short-lived project-level issuance and provider-side revocation before
it can deliver bearer material under this Host contract.

## Revocation, failure, persistence, and audit

The grant and lease ledger persists only IDs, scope digests, generation,
state, expiry, provider principal metadata, and redacted audit events. Token
bytes and authorization headers remain in memory. Grant revocation,
session/task/workspace teardown, provider disconnect/rotation, and plugin
disable/uninstall immediately fence future issuance/redemption. The same
events request revocation of every live issued token for the scope. A provider
`204` confirms invalidation; failure records `revocation_pending` with
provider expiry but never reports `revoked_at_provider`. After process loss,
the durable ledger still fences redemption and marks previously redeemed
tokens as expiry-bounded residuals until their recorded provider expiry.
The non-secret exposure row records the lease, App principal, canonical
repository, permission profile, export time, provider expiry, revocation
attempt, and confirmed provider revocation. Lease expiry or grant revocation
alone makes the bearer residual, not provider-revoked. An unsuccessful
provider revocation remains residual after Host restart; the Host has no
persisted token with which to retry exact-token revocation. It becomes
provider-expired only at the recorded provider expiry.

All admission and lifecycle paths emit non-secret audit receipts with grant,
lease, request, plugin installation, workspace, managed task/session, target,
capability and connection generations, provider principal, outcome, and
timestamp. Provider response bodies, tokens, headers, arbitrary caller input,
and raw idempotency keys are excluded. A provider timeout before any token
is minted returns `UNAVAILABLE`; an ambiguous mint or revocation records its
uncertainty rather than claiming success. The plugin's own provider-action
receipt correlates to the Host lease audit ID but remains a separate record.

## Verification

Focused database tests cover atomic grant replacement, idempotency,
cross-workspace/session/installation denial, expiry, and replay on SQLite and
PostgreSQL. Host RPC tests cover missing/unknown approval, stale revision,
foreign conversation/session, absent link, head and fork drift, token
redaction, plugin disable, and negative tool exposure in task MCP. GitHub
HTTP fixtures assert uncached one-repository mint, exact permission set,
provider token revocation, and no PAT fallback. Plugin integration tests in
its own repository prove direct calls, pre/post identity checks, concurrent
idempotency, ambiguous-send reconciliation, and non-secret receipts.
