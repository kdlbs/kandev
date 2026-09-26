# ADR-2026-09-26-plugin-direct-provider-access: Scoped direct provider access for managed plugin sessions

**Status:** proposed
**Date:** 2026-09-26
**Area:** backend, protocol, security, integrations

## Context

A managed Coordinator conversation may need to recover a CI run for a linked
pull request without changing its head. The initial design in
[ADR-2026-08-30](2026-08-30-server-owned-scoped-ci-runs.md) made Kandev perform
that GitHub operation through `request_fresh_ci_run_kandev`. Maintainer review
identified the architectural cost: every new provider operation would require
another Kandev intermediary. The Human chose direct GitHub/GitLab API calls in
the Coordinator plugin. The plugin is installed privileged code, but a task
agent, a prompt, and an ambient executor credential are not provider authority.

The existing provider-neutral Git credential broker issues exact task/session
leases for Git transport. It does not authorize provider APIs. Plugin Host RPCs
are connection-bound, and workspace capability approvals are versioned; neither
alone proves which managed agent session or repository may obtain provider
write authority.

## Decision

The direct-call ownership direction is Human-approved. The credential handoff
described below is a proposal and must remain disabled until the Human accepts
the residual GitHub bearer window described in Consequences.

Kandev owns a versioned, provider-neutral `provider-access/v1` Host contract
for an administrator-approved grant, an opaque short-lived lease, credential
redemption, and revocation. It does not own a rerun, workflow-dispatch, merge,
or other provider-action RPC. The plugin owns provider API requests, current
PR/MR and fork/run validation, idempotency, ambiguous-send reconciliation,
action receipts, and provider-specific policy. The agent sees only the plugin's
namespaced tool and non-secret result.

The Host derives plugin installation identity from the live connection and
checks the manifest, current workspace capability approval, administrator
grant generation, exact managed conversation task and active session, target
task's workspace/repository/link, provider connection generation, and requested
purpose on issuance and redemption. A lease binds the exact session, target
repository and PR/MR head, operation class, and provider target identity. It
is neither a provider credential nor evidence that an action succeeded.
Credential material is returned transiently only to the installed plugin at
redemption; it is not persisted, logged, sent to the agent, or exposed through
task MCP. Revocation prevents subsequent issuance and redemption immediately.
The Host requests provider-side revocation of already minted credentials and
records whether it was confirmed. A failed or interrupted provider revocation
is reported as a residual bearer window, never as confirmed invalidation.

For GitHub, only a verified workspace App installation may mint a fresh,
uncached installation token for one canonical repository and the minimum
permission profile. No PAT, ambient `gh` account, executor profile, or shared
cached token may satisfy this contract. GitHub cannot constrain an installation
token to one PR or run, so the trusted plugin enforces that target from the
lease and revalidates live provider identity before and after mutation. The
Host records issuance/redemption/revocation without claiming to attest the
provider action. A token that escaped before a Host crash may remain valid
until GitHub's expiry if provider revocation could not be confirmed.

The current GitLab workspace credential is a potentially long-lived PAT or
`glab` token. It is not eligible for this lease. GitLab direct actions fail
closed until an independently reviewed short-lived, project-scoped credential
source and revocation contract are available; the plugin may add that provider
adapter without changing Kandev into an action proxy.

## Consequences

The Coordinator plugin gains provider-specific adapter and audit responsibility.
Kandev retains reusable credential admission and revocation rather than an
expanding list of provider operations. A review must treat the plugin as a
privileged recipient of one-repository provider authority during the token's
remaining lifetime; an exact PR/run lease narrows policy and audit, but cannot
cryptographically narrow GitHub's bearer token to that PR/run. The old
action-specific MCP tool, server rerun state machine, and their public contract
must be removed or superseded before the replacement is ready.

## Alternatives Considered

- Keep or rename Kandev's rerun proxy as Host RPC: rejected because Kandev
  would still perform each provider operation on behalf of the plugin.
- Expose workspace PATs, App private keys, or ambient CLI tokens: rejected
  because they are long-lived or too broad and cannot be revoked by the exact
  session grant.
- Let the plugin own a separate provider App installation: viable later, but
  requires a second operator credential lifecycle and does not reuse the
  verified workspace connection.
- Reuse the Git HTTPS lease as an API token: rejected because its host/path
  authorization and resolver contract do not cover provider API permissions.
