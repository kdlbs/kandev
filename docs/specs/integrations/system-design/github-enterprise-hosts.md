---
status: current
system: integrations
created: 2026-10-14
owners:
  - Kandev
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-002
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-005
---

# GitHub Enterprise host system design

## Purpose and boundaries

Extend the existing GitHub integration across GitHub.com, GHES, and GHE.com.
Integrations owns host selection, credentials, provider operations, and settings.
Repository/task systems consume the resulting provider identity; they do not
infer a new GitHub host from owner/name. Keep one automation connection per
workspace and the existing optional personal connection per workspace/user.

The [authentication design](github-authentication-01.md), its
[transport contracts](github-authentication-02.md), and its
[scenarios](github-authentication-03.md) remain the foundation. This design
defines the host extension and explicit GHES OAuth exception. Those contracts
now reference this host policy while retaining their credential ownership and
executor inheritance boundaries.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001 | Host value; HTTP transport; CLI connection |
| REQ-INTEGRATIONS-GITHUB-ENTERPRISE-002 | App and OAuth lifecycle; capabilities |
| REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003 | Durable identity; transitions; workflow consumers |
| REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004 | Task execution |
| REQ-INTEGRATIONS-GITHUB-ENTERPRISE-005 | Settings and mobile; failure reporting |

## Original failure evidence

At investigation, `components/github/github-cli-form.tsx` already listed `login (host)` for every
discovered CLI account. `buildConnectionRequest` in
`github-connection-settings-form.tsx` sent the selected host/login. In
`internal/github/gh_accounts.go`, `resolveGHAccountToken` rejected any host other
than `github.com`. Even after removing that rejection,
`validateAndCommitWorkspaceGHCLI` validated the token through the public-host
client. The failure was a backend contract gap, not missing account discovery.

`WorkspaceConnection.GitHubHost` and App registration `github_host` already
exist. App registrations have uniqueness on `(github_host, app_id)`.
`TokenClient` lacked a host; `graphql.go` constructed a public GraphQL URL directly.
`AppClient`, manifest conversion, personal authorization, installation callbacks,
issue URL parsing, fork/comparison identities, and remote contribution handling
contained additional public-host assumptions. `gitcredentials` already validated
explicit provider origins, while GitHub's broker adapter rejected non-public hosts.

## Host value and endpoints

Introduce a small pure `internal/common/githubhost` package for normalization,
classification, endpoint derivation, and CLI token-family selection. It has no
database or HTTP dependencies and can be imported by GitHub, clone, and agentctl
code without introducing runtime-tier imports. Use a validated immutable value;
do not distribute parallel hostname switches across clients.

Accept a DNS hostname or HTTPS origin with an optional port and optional trailing
slash. Normalize lowercase DNS and default port 443. Reject userinfo, non-HTTPS
schemes, embedded controls/whitespace, query, fragment, non-root path, malformed
port, IP literals, and empty host. Preserve non-default ports in authority and
identity. A bare host uses HTTPS. Persist `github_host` as authority, matching
the existing field; repository `provider_host` remains a canonical HTTPS origin.
Never infer provider type from an arbitrary hostname substring.

| Family | Web origin | REST base | GraphQL endpoint | CLI token variable |
| --- | --- | --- | --- | --- |
| GitHub.com | `https://github.com` | `https://api.github.com` | `https://api.github.com/graphql` | `GH_TOKEN` |
| GHE.com | `https://tenant.ghe.com` | `https://api.tenant.ghe.com` | `https://api.tenant.ghe.com/graphql` | `GH_TOKEN` |
| GHES | `https://git.example.com` | `https://git.example.com/api/v3` | `https://git.example.com/api/graphql` | `GH_ENTERPRISE_TOKEN` |

Recognize GHE.com only as one non-empty tenant label followed by `.ghe.com`,
without a non-default port. Reject bare `ghe.com` and API-host input such as
`api.tenant.ghe.com`; setup asks for the web host. GitHub.com similarly uses its
canonical authority, without arbitrary ports or `api.github.com` as a web host.
GHES authorities retain supported ports in all derived endpoints.

The initial compatibility test baseline is GHES 3.20's documented core APIs.
Do not advertise older-version compatibility without fixtures and provider
evidence. Newer versions remain subject to operation capability checks; this is
not a promise that every GitHub.com feature exists on every GHES release.

## HTTP transport

Add host-aware constructors for `TokenClient`, `AppClient`, the OAuth client,
and manifest conversion client. Keep existing default constructors as explicit
GitHub.com compatibility wrappers, but migrate every workspace-resolved call to
the host-aware constructor. Pass host into App verifier factories and token
validation. REST and GraphQL must use separately derived endpoints; never append
`/graphql` to the GHES REST base.

Centralize authenticated URL construction and validation within the GitHub
transport. Relative REST paths stay under the exact REST origin and base path.
Validate absolute pagination URLs against the same boundary, including decoded
path traversal, scheme-relative URLs, encoded separators, and userinfo. Keep
existing pagination and response-size bounds. Authenticated API and token
exchange requests reject redirects rather than relying on Go's default
cross-subdomain authorization behavior. A signed artifact download, if an
existing operation needs one, uses a separate unauthenticated request with no
provider headers and explicit download bounds.

Apply normal TLS verification and the runtime's OS trust roots. Private DNS and
RFC1918 addresses are legitimate GHES destinations. Block loopback, link-local,
unspecified, and multicast destinations at the direct dial boundary; validate
every resolved address used for dialing to avoid DNS rebinding. Existing trusted
operator proxy configuration remains an operator-controlled network boundary.
Test-only injected transports may route fixture hostnames to local TLS servers;
production validation gains no insecure testing switch. Do not add a CA-upload
UI or disable certificate verification.

New host selection uses existing workspace integration-write authorization;
App root-credential management remains operator-only. Network probes happen only
after authorization and explicit setup, never while parsing an arbitrary task URL.

## CLI connection

Keep discovery and the existing picker. Normalize the selected account host,
resolve `gh auth token --hostname <host> --user <login>` with argument arrays,
then verify `/user` on that host. The older CLI fallback must verify the active
account on that exact host and fail if another account would be selected.

`githubCLIEnvironment` must strip all four token variables (`GH_TOKEN`,
`GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`) for named-account
discovery and resolution. Set or clear `GH_HOST` explicitly per operation; do
not inherit a default that contradicts the selected host. Never run `gh auth switch`.
Keep deadlines, cancellation, token redaction, and memory-only derived tokens.

PAT save accepts optional `host`; omission preserves public-host compatibility,
never the meaning of the current Enterprise connection. The UI always submits
the selected host. Existing CLI request `host` is already the authoritative
selection. `legacy_shared` and automatic new-workspace public defaults keep
their existing meaning; Enterprise is an explicit connection choice.

Stored PAT keys include the canonical host for Enterprise connections. Public
GitHub retains the existing workspace key for upgrade compatibility; Enterprise
never falls back to that public key. A resolver uses the host from its captured
connection when reading a secret, so a concurrent host replacement cannot pair
new-host token material with an old-host client. Old-host secret cleanup happens
after a successful replacement and cannot roll back the committed host scope.

## App and OAuth lifecycle

Carry host through manifest start, import preparation, verifier construction,
installation start, personal authorization, refresh, and revocation. Extend
the existing request models with `github_host` where absent. Once a preparation
or flow exists, its saved host is immutable; a callback cannot supply an override.
App selection is filtered to the chosen host, with shared registration secrets
retaining their existing deployment ownership.

Build App creation and installation links from the web origin. Convert manifest
codes and mint installation tokens through that host's REST base. Kandev's public
callback/webhook base is a separate setting and must not be confused with the
GitHub host. Validate returned App owner, ID, permissions, webhook settings, and
registration host before storing credentials. Preserve the current permission
policy; reject unsupported policy with a specific diagnostic rather than silently
creating an underprivileged App.

Persist an explicit OAuth policy (`pkce_s256` or `confidential_code`) in each
authorization flow. Public GitHub/GHE.com select PKCE; the GHES compatibility
profile selects GitHub's documented client-secret flow. Both use single-use
state, expiry, authenticated initiating user, fixed redirect URI, registration,
host, and workspace connection generation. GHES does not require a fabricated
PKCE verifier at callback. Never retry an exchange with a different policy.
Capture policy at start so an upgrade cannot reinterpret an outstanding flow.

App user tokens may be expiring or non-expiring according to the registration.
Refresh only when a refresh token/expiry exists; do not invent an expiry or
silently require refresh material absent from a valid provider response. Preserve
atomic refresh, revocation, and encrypted secret ownership. Personal identity is
always intersected with automation repository access on the same host.

Personal access and refresh token keys also bind to the host derived from the
captured connection's immutable registration. Token reads use that connection
snapshot rather than reloading a replacement registration by workspace/user.
Keep the public legacy keys, with no Enterprise fallback. A valid non-expiring
user token does not require a refresh-token secret. Refresh, revocation and
compensation use the same captured host identity.

Webhook routing uses registration ID from the Kandev route, authenticates before
parsing, and derives host from that registration. Never trust a payload URL or
installation ID alone to select the host. Keep deduplication keyed by registration
and delivery ID, and verify installation/workspace association before effects.

## Durable identity and persistence

Reuse `github_workspace_connections.github_host` and
`github_app_registrations.github_host`. Add host snapshots to auth/registration
flows and import preparations where missing; add OAuth policy to auth flows.
Derive personal host from its immutable registration and require it to match
the active automation registration. Include host in `AuthPrincipal`,
`ResolvedCredential`, cache namespaces, and rate-limit buckets where workspace
generation alone is currently implicit protection.

Persist host on GitHub work records that otherwise contain only owner/name:
`github_pr_watches`, `github_task_prs`, `github_review_watches`,
`github_review_pr_tasks`, `github_issue_watches`, and `github_issue_watch_tasks`.
Child records must agree with their parent watch and repository identity. Add
host to owner/name/resource-number lookup keys; keep task/repository-based
uniqueness where repository identity already provides the host boundary. Never
globally key GitHub resources by numeric IDs.

Give `github_workspace_settings` an explicit host scope for repository filters
and saved presets. Views, query caches, and API projections consume that scope.
Workspace PR and issue search responses include the resolved operation's
`github_host`, including cached pages. Add response metadata to resource copies
so shared provider/cache entries remain unchanged.
Task metadata URLs and contribution target/source identities must retain and
validate host; avoid adding duplicate mutable identity when a canonical
repository/connection reference already supplies it.

Use idempotent, transactional SQLite and PostgreSQL schema changes. Historical
integration-generated records belong to GitHub.com because the old integration
only operated there. Backfill that host only when persisted URLs/origins do not
contradict it. Preserve explicit valid provider origins; mark contradictory or
unattributable records ineligible for execution and expose a repair error.
Do not use the currently selected connection to relabel historical work. Test
fresh schema, upgrade, repeated migration, rollback-on-error, and restart reads.

### Connection replacement

Validate a replacement before changing the active connection. Under the existing
workspace mutation lock and expected-generation check, atomically commit the new
host/connection, increment credential generation, invalidate pending flows and
personal bindings, and reset host-bound repository filters to an explicit empty
selected-repository scope. Preserve task-access mode. Notify the UI that repository
scope must be selected; do not silently widen old filters to all repositories.
Disconnect retains settings. Reconnect compares the retained settings host in
the connection transaction: a different host resets scope even when no active
connection remains, while reconnecting the same host preserves that scope.

Old repositories, links, and watches remain bound to their original host. Pollers
and mutations compare record host with the operation's resolved credential before
provider access and before projecting results. A mismatch produces a paused or
unavailable outcome, not a request to the new host. Reconnecting the old host
requires current permissions and valid credentials before those records resume.
Workspace batches exclude other-host watches before provider lookup, and fallback
groups include the watch host so retained records cannot suppress current watches.
Do not stop unrelated agent processes; reject stale broker redemption instead.

Fence asynchronous results by captured host and generation. In-flight requests
already admitted to the old provider can finish externally, but their results
must not populate the new connection's cache or trigger follow-on operations.
Host replacement is not a distributed cancellation guarantee.

## Workflow consumers and capabilities

Thread resolved host through repository listing/import, issue URL parsing, PR
discovery, review watches, issue watches, task associations, comments/reviews,
merge/queue actions, checks, fork creation, comparison targets, and remote
contributions. The starting inventory includes `service_task_issue.go`,
`service_remote_contribution.go`, `service_fork.go`,
`service_comparison_target.go`, and `service_routing.go`.

A pasted Enterprise URL is accepted as GitHub only when it matches the workspace's
configured GitHub host or an already verified repository identity. Arbitrary
custom-host URLs are not auto-classified as GitHub. Short owner/name references
are scoped to the selected workspace connection. Cross-host forks and associations
fail explicitly. Browser links use validated provider-returned web URLs or the
host value, never string concatenation with GitHub.com.

Audit frontend `lib/github/parse-url.ts`, `lib/utils/github-repo-url.ts`,
`lib/utils/external-vcs-file-url.ts`, `use-remote-repositories.ts`,
`use-branches-by-url.ts`, and PR/issue-to-task cache keys. Include host in keys or
use canonical repository IDs whose host was verified. Keep Kandev release links
and demo content outside this migration.

Distinguish provider capability from credential permissions. Basic PR discovery
must not depend on optional merge-queue GraphQL fields existing. Use bounded,
cached schema/capability evidence or known compatibility profiles, with separate
queries for optional features. A transport failure, 401, or permission denial is
not evidence that a feature is unsupported. Do not retry failed mutations with
alternative endpoints or automatically switch hosts. Preserve retry/backoff and
circuit behavior within the host/principal scope.

### Workflow sync

Workflow sync configurations persist `github_host` as a canonical web authority.
Legacy rows and omitted GitHub request fields mean GitHub.com; GitLab configurations
clear this field. Configuration saves validate against the workspace connection.
Each sync captures the connection host and generation before repository reads, and
serializes its local apply with connection replacement. A stale capture fails before
the workflow applier runs. No connection mutation lock is held during provider I/O.
The repository URL editor preserves this host across save/reload and draft changes.

## Task execution

GitHub's adapter in `credential_broker.go` compares scope host to the resolved
connection instead of comparing it to the public constant. Preserve the provider-
neutral broker's exact task/session/repository/path/generation checks, including
destination bindings and lease reissue. Installation tokens are minted on the
registration host and narrowed to the correct provider repository ID.

The agentctl CLI shim obtains host from the authorized redeemed scope. Set
`GH_HOST`, isolate CLI config, remove all inherited public/Enterprise token
variables, then set only the token variable for that host family. Check explicit
`--hostname`, `--repo`, URLs, `GH_REPO`, and repository-derived targets against
the selected authorized scope before launching the real CLI. An invocation that
targets another host without a matching lease fails before credentials enter
the child environment. Preserve extension recursion bounds and real-CLI lookup.

Reuse `gitcredentials.ResolveRepositoryIdentity`, clone origin validation, and
task access contracts. Generalize managed clone URLs and host-specific protocol
lookup in `repoclone`; preserve host and explicit HTTPS port. SSH aliases and
independent SSH-port configuration are not inferred. Executor inheritance keeps
user-managed origins untouched and probes only eligible attached hosts. Correct
the existing host bridge's public-versus-Enterprise split so GHE.com uses public
token variable precedence, as the CLI documents.

Audit launcher/container/script paths only where they participate in managed
GitHub task checkout. Reuse scoped helper configuration, avoid global rewrites,
and remove Enterprise credential leakage through old token-bearing clone URL
paths. Local/Worktree and remote runtimes need network reachability and trust
roots independently. Native executor/provider contracts do not gain automatic
CA installation or host-user credential inheritance.

## Settings and mobile

Entry point remains `/settings/workspaces/:id/integrations/github` and its
existing Connect/Change connection action. CLI mode retains the current account
dropdown and derives a read-only host summary from the selected account. PAT
and App modes add a host field before credentials/App selection. Do not require
Enterprise CLI users to duplicate host entry or hide their existing accounts.

Changing host clears incompatible in-memory credential drafts and App selections,
invalidates stale lookup responses, and regenerates setup instructions. Show
current and proposed host plus the host replacement effects before Save. Keep
non-secret values after a failure; never copy saved secret material into inputs.
App root credentials remain ephemeral form data and are cleared after submission.
An open connection draft owns its selected method across status refreshes. Choose
the latest saved method when opening the dialog. A refresh must not interrupt
pending-save completion or failure recovery.

Use `GitHubConnectionDialog` as the nearest shipped feature exemplar and the
curated mobile menu sheet's fixed-header/one-scroll-body pattern. Desktop keeps
the dialog; phone keeps the existing full-height inset Drawer because App setup
is a deep temporary flow. Host, method/account, credentials/App actions, and task
access appear in that order, except CLI host is derived from its account field.
Use a fixed safe-area-aware footer and one internal scrolling body with `100dvh`
containment. Share the draft above the responsive branches so resizing does not
discard it. Preserve 28px ordinary desktop controls and >=44px phone/coarse-pointer
targets. Focus returns to the opener; error text is associated with its field.

`UI-01` and `UI-02` in the [plan](../../../plans/github-enterprise-hosts/plan.md#ascii-ui-preview)
define required grouping and changed states. All new labels, status messages,
and errors use i18next and all seven catalogs; Traditional Chinese uses the
repository generation command. Hostnames and CLI command values are untranslated data.

## Failure reporting and observability

Expose stable error codes for invalid host, unreachable host, TLS failure,
invalid credentials, unavailable capability, and host mismatch through existing
integration errors and localized UI. Log sanitized host family, operation,
workspace/registration correlation, and error class; never raw OAuth responses,
tokens, secrets, codes, or credential URLs. Keep dynamic hosts/IDs out of metric
labels. Existing rate limits/circuits remain scoped to the resolved principal.

## Sources and decisions

Verified during planning on 2026-10-14:

- [ADR: host and OAuth boundaries](../../../decisions/2026-10-14-github-enterprise-host-boundaries.md)
- [GitHub Cloud data residency API destinations](https://docs.github.com/en/enterprise-cloud%40latest/admin/data-residency/about-github-enterprise-cloud-with-data-residency)
- [GHES GraphQL endpoint](https://docs.github.com/en/enterprise-server%403.20/graphql/guides/introduction-to-graphql)
- [GHES App user OAuth and refresh response](https://docs.github.com/en/enterprise-server%403.20/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app)
- [CLI host/token environment semantics](https://cli.github.com/manual/gh_help_environment)

Live GHES/GHE.com access has not been exercised during planning. Fixture tests
prove routing and isolation; actual provider smoke evidence is a separate
compatibility claim and must state the server version and App policy used.
