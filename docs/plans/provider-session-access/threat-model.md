# Provider session access threat model

Status: proposed, credential issuance disabled. This analysis does not approve
production redemption. It covers one managed Coordinator plugin session
recovering a failed GitHub Actions run on one linked pull request.

## Authority and exact scope

The Host authenticates the administrator and the connection-bound installed
plugin. A grant names one installation, workspace, managed conversation,
target task, canonical repository, provider, purpose, generation, and expiry.
Issuance and redemption independently resolve the active managed task/session,
H6 capability approval revision, target task repository attachment, linked PR,
base and head repository/ref/SHA, source run/check identity, and current GitHub
App connection. Caller-supplied IDs select records but do not establish
authority. A fork head is evidence to verify, never a token destination. The
plugin repeats current PR/run checks immediately before mutation and reads
back ambiguous outcomes; Kandev never performs the provider mutation.

For GitHub rerun, the only eligible principal is the verified workspace GitHub
App installation with access to the PR's canonical base repository. The Host
must request a **fresh, uncached installation token for exactly that one
repository** with `actions:write`, `pull_requests:read`, and `metadata:read`.
`contents:read` is excluded from rerun and requires a separately reviewed
operation profile if needed. `workflows:write`, `checks:write`, `contents:write`,
and other write permissions are excluded. The Host must verify the returned
repository and permissions are no broader than requested and reject a
principal mismatch. `InstallationTokenCache.GetForWorkspace` is prohibited:
its repository/permission key can share one token among leases and break
per-lease revocation. PAT, `gh`, executor, and ambient credentials are denied.
The current GitLab PAT/`glab` path is unsupported and returns no credential.

The lease expires within minutes and is bound to the exact session, grant and
approval generations, connection generation, operation, PR/head, and source
run. **Lease expiry only fences future redemption.** GitHub installation
tokens expire one hour after creation unless successfully revoked by GitHub;
the Host cannot shorten that provider-enforced lifetime. At redemption the
Host first durably records a one-shot `mint_started` intent for the exact lease
before contacting GitHub. Concurrent redemption or restart must not issue a
second mint for that lease. A timeout or crash after the provider may have
minted but before the Host receives the response leaves an `unknown_mint`
receipt, conservatively bounded by one hour from the attempt plus clock skew;
the Host has no token bytes with which to revoke that possible bearer.
After a successful response the Host must durably record only non-secret
principal, repository, permission, lease and provider-expiry metadata before
exporting the bearer. A write failure must prevent export and attempt
revocation of the freshly minted token. The token remains transient in Host
memory for exact-token revocation
on release/invalidation. A successful provider `DELETE /installation/token`
response is the only confirmed provider invalidation. Failure, timeout, or
Host crash leaves a non-secret `revocation_pending` / expiry-bounded residual
receipt until provider expiry; restart cannot recreate the lost bearer to
revoke it. The plugin may hold the bearer only in process memory and must not
put it in its ledger, agent output, logs, environment, or errors.

Provider references: [installation token creation and scope](https://docs.github.com/en/rest/apps/apps#create-an-installation-access-token-for-an-app),
[installation token expiry](https://docs.github.com/en/rest/apps/apps#about-github-apps),
and [token revocation](https://docs.github.com/en/rest/apps/installations#revoke-an-installation-access-token).

## Attack and failure cases

| Case | Required control | Remaining exposure |
| --- | --- | --- |
| Untrusted prompt asks for a different PR/run or repo | Host derives live link and exact target; plugin repeats provider checks | An already exported bearer is still repo scoped, not PR/run scoped. |
| Fork PR points to attacker-controlled head | Verify base repo, fork head repo/ref/SHA and source run; mint for canonical base only | Compromised plugin code could use Actions write elsewhere in that base repo. |
| Wrong plugin, task, session, workspace or stale H6 approval | Connection-bound installation plus exact managed-session/grant-generation admission on issue and redeem | No bearer is issued on a denial. |
| Replay, concurrent request or ambiguous mint | Persist `mint_started` before provider call; durable idempotency/one-shot redemption; unknown outcome fails closed and records uncertainty | A provider token created during an ambiguous mint may exist until provider expiry; no second mint to reconcile it. |
| Lease/grant expires or is revoked after export | Fence new issuance/redemption, request exact-token provider revocation, audit result | Failed revocation or Host crash leaves bearer usable until GitHub expiry. |
| Plugin process leaks bearer | Process-local handling, no persistence/log/output, restricted plugin admission and runtime privileges | GitHub bearer itself has no PR/run/session caveat; leaked token can perform other Actions-write operations in the repository. |
| Token cache shares bearer between leases | Fresh mint outside shared cache, per-token revocation state | Provider mint rate/cost rises; revocation is isolated per lease. |

## Generic egress enforcement alternative

A DNS/origin allowlist for the plugin does not narrow an exported GitHub
bearer: every GitHub API endpoint on that origin remains reachable. A network
gateway that terminates HTTPS and enforces method/path/body/target could
constrain an exact rerun. To enforce the current PR head and source attempt it
would also need authenticated provider reads and stateful policy. That gateway
would become a provider-operation intermediary, even if named a generic
egress service, and would reintroduce the action-specific proxy Carlos
objected to. A gateway that merely forwards opaque TLS cannot inspect or
constrain the request. The current plugin subprocess model has no proven
per-plugin network namespace and egress interception that could close this
gap without a new runtime security boundary. Therefore egress controls may
reduce destinations, but cannot be claimed as PR/run enforcement here.

## Human decision required

Option A: explicitly trust the reviewed plugin as a temporary one-repository
Actions-write principal, accept that an exported bearer can outlive lease
revocation failure or Host crash for up to GitHub's provider lifetime, and
enable the uncached Host redemption path only after independent security
review and complete Host/plugin tests.

Option B: require a separate plugin-owned GitHub App and credential lifecycle.
Kandev would still bind the plugin/session grant but would never export its
workspace App bearer. This moves provider-token risk and operator setup to
the plugin owner; the plugin's own token would still not be PR/run scoped.
The plugin program must review its permissions, installation ownership,
revocation and audit independently. Neither option makes GitLab's current
workspace PAT eligible.

The Coordinator routes this trust-boundary choice to the Human. Until it is
recorded, no live mint, redemption, token transport, provider mutation,
plugin admission, or ready-for-review transition is authorized by this plan.
