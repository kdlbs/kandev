---
status: draft
created: 2026-08-22
owner: tbd
---

# Multi-Tenancy (Organizations)

## Why

Kandev's opt-in authentication gives an instance users, roles, and per-user
workspace privacy, but no boundary above the user: every account on an instance
shares one filesystem tree, one set of agent CLI credentials, one executor pool,
and one instance-wide configuration surface. That is acceptable for colleagues
who already trust each other and unacceptable for anyone who wants to run one
Kandev deployment for two teams that must not reach each other's code, or to
host Kandev for customers at all. Users need to belong to an **organization**
that is a real isolation boundary, not just a label.

**Sequencing.** Self-hosted is the near-term priority and a hosted deployment is
the reason the boundary must be real rather than cosmetic. Concretely: the
design is built to be strong enough for untrusted co-tenants, but it ships and
is validated on self-hosted instances first. What a self-hosted team actually
asks for today ([#2824](https://github.com/kdlbs/kandev/issues/2824)) is
[org roles and scopes](../auth/roles-and-scopes.md) plus
[workspace visibility](../workspaces/membership.md) — neither needs an org, and
both ship ahead of this spec. Organizations are what make that team board safe
to put on a box shared with another team, and later on a box shared with
another customer.

## What

- **An organization ("org") is the tenant.** It owns users, filesystem roots,
  agent credentials, executors, and configuration. Cross-org reach is a bug,
  not a permission level.
- **Every user belongs to exactly one org.** `users.org_id` is non-empty and
  immutable after creation. There is no org switcher, no "acting as org" request
  parameter, and no way for a session or token to name an org. The tenant is a
  total function of the authenticated identity, so no request payload can move
  a caller between tenants.
- **The org is the outer edge of sharing, not a sharing mechanism of its own.**
  Who sees what inside an org is decided by
  [org roles and scopes](../auth/roles-and-scopes.md) and
  [workspace visibility](../workspaces/membership.md), exactly as on a
  single-org instance. This spec adds no visibility rule; it bounds every
  existing one.
- **`org` visibility means the workspace's own org and nothing wider.** A
  workspace marked visible to the organization is reachable by members of *its*
  org only. Adding a member from another org is refused with 404, so org
  membership is not enumerable, and the tenancy migration drops any pre-existing
  cross-org membership row it finds, logging each one without aborting.
- **Org scopes are per-org.** A user holding `org.members.manage` or
  `org.config.manage` holds it in their own org and nowhere else. The
  instance-level `operator` tier is separate and holds no org scopes at all.
  Membership and roles are the boundary below the user; tenancy is the boundary
  above; neither blocks the other.
- **Multi-tenancy is a runtime feature toggle.** `features.multiTenancy`
  ("Organizations", `KANDEV_FEATURES_MULTI_TENANCY`) is restart-required and
  OFF in every shipped profile. It requires `features.auth`; enabling it with
  auth disabled is refused at startup with a named error. With the flag off, an
  instance behaves byte-identically to today — no org UI, no org routes, no
  filesystem layout change.
- **A third role tier above the org.** `operator` is instance-level and manages
  orgs, feature toggles, instance configuration templates, backups, and storage
  maintenance. The org roles (`owner`, `admin`, `member`, `guest`) and their
  scopes are defined by [roles and scopes](../auth/roles-and-scopes.md) and are
  re-scoped to one org. An operator holds **no org scopes** and MUST NOT gain
  read access to any org's workspaces, tasks, sessions, transcripts, or
  secrets; operator is an administration role, not a visibility role, exactly
  as org `admin` is.
- **Configuration is two-tier: instance template → org row.** Executors,
  executor profiles, environments, agents, agent profiles, editors, custom
  prompts, and notification providers each exist as instance-level templates
  (`org_id = ''`, managed by an operator) and org-level rows (managed by an org
  admin). An org sees the union of its own rows plus every template it has not
  shadowed. An org admin may shadow, override, or ignore a template; an
  operator cannot see or edit an org's rows.
- **An instance template MUST NOT carry a credential.** Templates hold shape
  only: executor type, image reference, agent binary, model, flags, env-var
  *names*. Every value that authenticates (secret, token, API key, env-var
  value marked sensitive, integration config) is org-owned. This is a hard
  invariant with a pinning test, because a template is the one row two orgs
  both read.
- **Runtime isolation is tenant-pinned, not just row-scoped.** An agent
  launched for org A SHALL NOT run in any process, container, or host that
  holds org B's credentials or working tree:
  - **Filesystem.** Each org gets a private root (`<home>/orgs/<org_id>/`)
    containing its worktrees, clones, attachments, and agent credential home.
    Directories are created mode `0700`. The pre-existing flat tree becomes the
    default org's root in place, so upgrading an instance moves no files.
  - **Agent credentials.** Every ACP subprocess and agentctl child for org A
    receives `HOME`/`XDG_CONFIG_HOME`/`XDG_DATA_HOME` pointed at org A's
    credential home, so `gh auth`, `claude login`, and provider API keys are
    per-org rather than per-OS-user.
  - **Containers.** Docker executions are labelled `kandev.org=<org_id>`,
    named and volume-namespaced by org, and never reused across orgs. A
    container whose label does not match the launching org is not adopted on
    recovery; it is treated as foreign and left alone.
  - **Remote/SSH executors.** An executor row is org-owned and reachable only
    by its org. Two orgs cannot both target one remote host through Kandev.
  - **Standalone (`local_pc`) executors fail closed.** A standalone execution
    shares the backend's OS user, so `HOME` redirection is a convention an
    agent can walk out of. When more than one org is active, standalone
    executions are refused with a named error unless the instance declares a
    per-org OS user for the launching org, or an operator has set the
    `features.multiTenancyTrustedStandalone` escape hatch for a
    mutually-trusting self-hosted deployment.
- **Secrets are sealed under a per-org key.** Row scoping decides who may
  *read* a secret; it does nothing about the ciphertext. Today one AES-256
  master key at `<data-dir>/master.key` seals every row on the instance, so at
  rest every tenant collapses to one key that sits beside the database. Under
  organizations each org gets its own data encryption key (DEK), wrapped by
  that same master key:
  - The master key and `MasterKeyProvider` are unchanged, so a self-hosted
    upgrade changes nothing operationally and no KMS is required.
  - A secret is sealed under its own org's DEK, so compromising one org's
    plaintext does not yield another's.
  - Deleting an org destroys its DEK, so the org's secrets are unreadable **in
    the live database**. This is deliberately NOT a crypto-shred against
    backups: a backup is `VACUUM INTO` of the database, `org_keys` is a table,
    so the wrapped DEK travels inside the same snapshot as the ciphertext it
    protects, and `master.key` is untouched by restore. Restoring a
    pre-deletion backup on the same host unwraps it again. See Out of scope
    for the change that would make deletion a real shred and what it costs.
  - The tenancy migration re-wraps existing secrets under the default org's DEK
    in the same first-boot pass that assigns `org_id`.
- **Background work runs per-org, never identity-free.** Today an absent
  identity means "internal caller, unscoped". Under tenancy that is a
  cross-tenant read. Pollers, office schedulers, the orchestrator, watchers,
  automation runners, and storage maintenance SHALL iterate orgs and carry a
  **system identity bearing an `OrgID`**. An identity-free context reaching a
  tenant-scoped service entry point is a denial, not a grant.
- **Migration is automatic and lossless.** On first boot with the flag enabled,
  every existing user, workspace, secret, executor, profile, environment,
  editor, prompt, notification provider, and integration config is assigned to
  a single **default org** named after the instance. No row is orphaned and no
  file moves.
- **Deleting an org is explicit and destructive.** An operator deletes an org
  only through a type-to-confirm flow naming the org slug. Deletion revokes
  sessions and tokens, stops executions, removes org rows, and removes the org
  filesystem root. The default org cannot be deleted while it is the only org.

## Data model

New and changed tables. Timestamp types follow the existing SQLite/Postgres
substitution (`internal/db/dialect`); all DDL is replay-safe (ADR 0027) and new
columns arrive via idempotent `ADD COLUMN` migrations, never by editing a
`CREATE TABLE` alone.

```
orgs
  id            string     PK
  name          string     display name, non-empty
  slug          string     UNIQUE, lowercase [a-z0-9-], used in confirm flows
  status        enum       active | suspended
  is_default    bool       exactly one row true; the migration target
  storage_root  string     abs path; '' means the legacy flat tree
  created_at    timestamp
  updated_at    timestamp
```

```
org_keys                           -- per-org data encryption key, wrapped
  org_id        string     PK, FK -> orgs.id (cascade delete)
  wrapped_dek   bytes      the org's DEK sealed under the instance master key
  nonce         bytes      AEAD nonce for the wrap
  created_at    timestamp
```

Deleting the row destroys the only copy of the wrapped DEK. The plaintext DEK
is never persisted and is held in memory only for the life of a request.

```
org_os_users                       -- optional per-org OS identity, operator-managed
  org_id        string     PK, FK -> orgs.id (cascade delete)
  os_user       string     POSIX user name that standalone executions run as
  created_at    timestamp
```

Changed existing tables:

| Table | Change | Meaning |
|---|---|---|
| `users` | `+ org_id TEXT NOT NULL DEFAULT ''` | the user's tenant; immutable after create |
| `users` | `email` unique constraint becomes `UNIQUE (org_id, email)` | the same address may exist in two orgs |
| `auth_invites` | `+ org_id TEXT NOT NULL DEFAULT ''` | an invite mints a member of one org |
| `workspaces` | `+ org_id TEXT NOT NULL DEFAULT ''` | the tenant root for all workspace-descended data |
| `secrets` | `+ org_id TEXT NOT NULL DEFAULT ''` | org-owned; no instance tier. Sealed under that org's DEK, not the instance master key |
| `executors`, `executor_profiles`, `environments`, `agents`, `agent_profiles`, `editors`, `custom_prompts`, `notification_providers` | `+ org_id TEXT NOT NULL DEFAULT ''`, `+ template_id TEXT NOT NULL DEFAULT ''` | `org_id = ''` is an instance template; `template_id` names the template this org row shadows |
| `executors_running` | `+ org_id TEXT NOT NULL DEFAULT ''` | recovery must not adopt another org's execution |
| `task_message_attachments` | `+ org_id TEXT NOT NULL DEFAULT ''` | reached by `owner_id` without a workspace join |
| `plugin_settings`, `plugin_state` | `+ org_id TEXT NOT NULL DEFAULT ''` | plugin config is per-org; `plugin_user_state` inherits from its user |
| `settings`, `runtime_flag_overrides`, `kandev_meta`, `storage_*`, `telemetry_activations`, `notification_migrations` | unchanged | instance-global by design |

**Tenancy classification is exhaustive and pinned.** Every table is exactly one
of:

- `instance` — one row set for the whole deployment, operator-managed.
- `tenant-root` — carries `org_id` directly.
- `descendant` — reaches its org through a documented FK path (for example
  `task_sessions -> tasks -> workspaces.org_id`).

The classification lives in one registry (`internal/tenancy/registry.go`) and a
completeness test enumerates the live schema and fails when a table is absent
from it. Adding a table without classifying it is a build failure, not a
runtime leak.

**Denormalization rule.** A descendant table gains its own `org_id` column only
when at least one production query reaches it without joining its root — that
is the query that would otherwise leak. The registry records the reason.

## API surface

```
GET    /api/v1/orgs/current                       -> current caller's org (any member)
PATCH  /api/v1/orgs/current                       -> rename own org (org admin)

GET    /api/v1/instance/orgs                      -> list orgs (operator)
POST   /api/v1/instance/orgs                      -> create org + first admin invite (operator)
PATCH  /api/v1/instance/orgs/{id}                 -> name, status (operator)
DELETE /api/v1/instance/orgs/{id}                 -> destructive, requires {"slug": "<slug>"} (operator)
PUT    /api/v1/instance/orgs/{id}/os-user         -> set/clear the per-org OS user (operator)

GET    /api/v1/instance/templates/{kind}          -> list instance templates (operator)
POST   /api/v1/instance/templates/{kind}          -> create template (operator)
PATCH  /api/v1/instance/templates/{kind}/{id}     -> update template (operator)
DELETE /api/v1/instance/templates/{kind}/{id}     -> delete template (operator)
```

`{kind}` is one of `executors`, `executor-profiles`, `environments`, `agents`,
`agent-profiles`, `editors`, `prompts`, `notification-providers`.

Existing collection routes for those kinds are unchanged in shape and return
the org's effective view: own rows first, then every template not shadowed by a
`template_id` match. Each item carries `"source": "instance" | "org"` and
templates are returned with `"editable": false`.

`POST /api/v1/instance/templates/{kind}` and its `PATCH` reject a body carrying
a secret value, a sensitive env-var value, or an integration credential with
`400 template_may_not_carry_credentials`.

**Changed contracts**

- `authn.Identity` gains `OrgID string` and `Instance bool` (the operator tier).
  `IdentityFromContext` returning an identity with an empty `OrgID` while the
  flag is on is a denial at every tenant-scoped entry point.
- A new `authn.SystemIdentity(orgID)` constructor is the only way background
  work obtains a context; it is not reachable from any HTTP or WS path.
- `/api/v1/app-state` gains `org: {id, name, slug}` for authenticated callers
  and continues to return only `{features, auth}` anonymously. It never
  enumerates other orgs.
- WS: `Hub.BroadcastToWorkspace` is unchanged. `Hub.Broadcast` (global) is
  replaced by `Hub.BroadcastToOrg(orgID, …)` for everything tenant-derived. The
  `//ws:global` justification comment survives only for genuinely
  instance-global events (release notification, feature-toggle change,
  restart), and a lint-style test pins that allowlist.
- The GitHub credential broker lease, the port-proxy capability HMAC, automation
  webhook secrets, office channel HMACs, and plugin webhook secrets each bind
  the issuing org into their payload and refuse redemption under another org.

## Permissions

This spec adds only the instance tier. Org-level authority is the scope set in
[roles and scopes](../auth/roles-and-scopes.md).

| Action | operator | org owner | org admin | member / guest |
|---|---|---|---|---|
| Create / suspend / delete an org | yes | no | no | no |
| Feature toggles, backups, storage maintenance, restart | yes | no | no | no |
| Manage instance templates | yes | no | no | no |
| Set a per-org OS user | yes | no | no | no |
| Rename own org (`org.settings.manage`) | no | yes | yes | no |
| Manage org config: executors, profiles, environments, agents, editors, prompts, notification providers (`org.config.manage`) | no | yes | yes | no |
| Manage org users, invites, roles (`org.members.manage`) | no | yes | yes | no |
| Delete own org (`org.delete`) | no | yes | no | no |
| Read a workspace they cannot reach | no | no | no | no |

An operator is a user like any other: they belong to an org, they reach
workspaces there by the ordinary rules, and the instance tier grants them
nothing inside any org. On a self-hosted single-org instance the first admin
created by the auth setup wizard holds `operator` and org `owner`.

## State machine

An org has three states:

| State | Entered by | Effect |
|---|---|---|
| `active` | creation, or an operator resuming a suspended org | normal operation |
| `suspended` | operator | sessions and tokens fail closed with `org_suspended`; running executions are stopped; background work skips the org; rows and files are retained |
| *(deleted)* | operator, type-to-confirm | rows removed, filesystem root removed, sessions and tokens revoked; not reversible |

Suspension is reversible and is the intended lever for a cloud billing lapse.
Deletion is the only destructive path and is never reached implicitly.

## Failure modes

- **`features.multiTenancy` on with `features.auth` off** — startup aborts with
  a named configuration error rather than serving an instance whose tenant
  boundary has no identity behind it.
- **A tenant-scoped service entry point receives a context with no identity**
  (flag on) — denied with the existing `*NotFound` sentinels. This inverts
  today's "no identity means internal caller" rule and is the single most
  important behavioral change; every background caller must be migrated to
  `authn.SystemIdentity` before the flag can be enabled.
- **An identity's `OrgID` names a suspended or missing org** — the session and
  any token fail closed on the next request; the browser is sent to a "your
  organization is unavailable" page, not to `/login`, so the user is not told
  their credentials are wrong.
- **A standalone execution is requested with more than one active org and no
  per-org OS user** — refused before the process starts, with an error naming
  the missing `org_os_users` row and the escape-hatch flag. No partial launch.
- **A Docker container or `executors_running` row carries a different
  `kandev.org` label than the recovering backend expects** — it is not adopted,
  not stopped, and not deleted; it is logged and left for its owning org's
  recovery pass.
- **An org's storage root is missing or not writable** — task creation and
  session launch in that org fail with a storage error; other orgs are
  unaffected. The backend does not fall back to the shared tree.
- **The org migration cannot assign a row** (an orphan whose FK path is broken)
  — migration aborts and the boot fails rather than leaving a row reachable by
  every tenant. The pre-migration backup taken by `persistence.Provide` is the
  recovery path.
- **An org's DEK cannot be unwrapped** (missing `org_keys` row, or a master key
  that no longer decrypts it) — every secret read and write in that org fails
  with a named error. It does NOT fall back to the master key: a silent
  fallback would re-create exactly the shared-key property this removes.
  The error names the recovery route, because a rule with no recovery story is
  a rule the next person deletes under pressure: `org_keys` is an ordinary
  table, so a lost row is restored from a database backup like any other row,
  and secrets stay readable afterwards because the master key that unwraps it
  is not part of the snapshot and did not change.
- **A template edit would introduce a credential** — rejected at the API
  boundary; no partial write.
- **An org is deleted while an execution is running** — executions are stopped
  first; if a stop fails the deletion aborts and reports the execution, so no
  filesystem root is removed under a live agent.

## Persistence guarantees

- Org rows, memberships, per-org OS users, and org filesystem roots survive
  restart.
- Org storage roots are only removed by explicit org deletion. Storage
  maintenance and GC operate per-org and never traverse another org's root.
- The default org's `storage_root` stays `''` (the legacy flat tree) for an
  upgraded instance, permanently. A fresh install writes
  `<home>/orgs/<org_id>/` for every org including the default. Both shapes are
  supported forever; the empty value is not a migration to finish later.
- Backups remain instance-wide (one database), so a backup contains every org.
  Per-org export is out of scope.
- **Deleting an org does not erase it from backups already taken**, including
  its secrets. The wrapped DEK lives in `org_keys`, which is inside the
  snapshot, and the master key that unwraps it is not part of the snapshot and
  survives restore. Deletion is irreversible on the live instance; it is not
  retroactive across retained backups.
- Suspending an org does not delete anything; deletion is the only data loss.
- Turning `features.multiTenancy` off retains every `org_id`, exactly as
  disabling auth retains `owner_id`. Re-enabling restores the previous
  boundary without a re-migration.

## Scenarios

- **GIVEN** an instance with `features.multiTenancy` off, **WHEN** a user loads
  any page or calls any API, **THEN** no org route exists, no `org` key appears
  in the boot payload, and every response is byte-identical to the same build
  with the tenancy code absent.
- **GIVEN** an instance with `features.auth` off, **WHEN** the operator sets
  `features.multiTenancy` on and restarts, **THEN** startup aborts with a named
  configuration error and the previous binary state is untouched.
- **GIVEN** an existing auth-enabled instance with three users and eight
  workspaces, **WHEN** multi-tenancy is enabled and the backend restarts,
  **THEN** one default org exists, all three users and all eight workspaces
  carry its `org_id`, no file under `<home>` has moved, and every user's
  visible workspace list is unchanged.
- **GIVEN** users A (org 1) and B (org 2) and a workspace owned by B, **WHEN**
  A requests that workspace, task, session, transcript, secret, or attachment
  by ID over HTTP, WS, or MCP, **THEN** the response is 404 and no row is
  serialized.
- **GIVEN** org 1 and org 2, **WHEN** an agent runs for org 1 and reads
  `$HOME`, **THEN** it sees org 1's credential home, and no path under org 2's
  storage root is readable from that process.
- **GIVEN** an org-1 agent that runs `gh auth status`, **WHEN** only org 2 has
  authenticated `gh`, **THEN** the agent reports no credential rather than
  authenticating as org 2.
- **GIVEN** two active orgs and no `org_os_users` row, **WHEN** a user launches
  a session on a `local_pc` executor, **THEN** the launch is refused before any
  process starts and the error names the missing per-org OS user.
- **GIVEN** two active orgs and the `features.multiTenancyTrustedStandalone`
  override set, **WHEN** the same launch is attempted, **THEN** it proceeds with
  the org's redirected `HOME` and a warning is logged once per org per boot.
- **GIVEN** a Docker container labelled `kandev.org=org-2`, **WHEN** the backend
  recovers executions for org 1, **THEN** the container is not adopted, not
  stopped, and not removed.
- **GIVEN** an org-scoped poller, scheduler, or automation runner, **WHEN** it
  executes a cycle, **THEN** every service call it makes carries a system
  identity bearing that org's ID, and a call made with no identity is denied.
- **GIVEN** a workspace-carrying event in org 1, **WHEN** the WS gateway
  broadcasts it, **THEN** no client authenticated to org 2 receives a frame.
- **GIVEN** an instance template for an executor profile, **WHEN** an org admin
  lists executor profiles, **THEN** the template appears with
  `"source": "instance"` and `"editable": false`, and after the org creates a
  row with that `template_id` the template no longer appears.
- **GIVEN** an operator, **WHEN** they attempt to save an instance template
  whose body contains a secret or sensitive env-var value, **THEN** the request
  is rejected with `template_may_not_carry_credentials` and nothing is written.
- **GIVEN** an operator who belongs to org 1, **WHEN** they request any
  workspace, task, session, or secret belonging to org 2, **THEN** the response
  is 404 — the instance tier grants no org scopes and no org visibility.
- **GIVEN** an org-visible workspace in org 1, **WHEN** a member of org 2
  requests it, **THEN** the response is 404 — `org` visibility means the
  workspace's own org.
- **GIVEN** a workspace owner in org 1, **WHEN** they try to add a user from
  org 2 as a workspace member, **THEN** the response is 404 and no membership
  row is written.
- **GIVEN** an instance already using org-visible workspaces and workspace
  members, **WHEN** multi-tenancy is enabled and every user lands in the default
  org, **THEN** every visibility setting and membership row is retained and
  every shared board keeps working exactly as before.
- **GIVEN** a membership row whose member and workspace would land in different
  orgs, **WHEN** the tenancy migration runs, **THEN** the row is dropped and
  logged, and the migration does not abort.
- **GIVEN** a GitHub credential-broker lease minted for an org-1 task, **WHEN**
  it is redeemed against an org-2 task, **THEN** redemption fails and no token
  is returned. The same holds for a port-proxy capability, an automation webhook
  secret, an office channel HMAC, and a plugin webhook secret.
- **GIVEN** an active user in org 1, **WHEN** an operator suspends org 1,
  **THEN** the user's next request fails closed with `org_suspended`, running
  executions are stopped, and the browser shows the organization-unavailable
  page rather than the sign-in page.
- **GIVEN** two orgs each holding a secret, **WHEN** org A's DEK is used to
  decrypt org B's ciphertext, **THEN** the decryption fails: the rows are not
  sealed under a shared key.
- **GIVEN** org B deleted from the live instance, **WHEN** any of its secrets
  is read, **THEN** the read fails: its DEK row is gone.
- **GIVEN** a backup taken while org B existed, **WHEN** it is restored onto the
  same host after B was deleted, **THEN** B's secrets ARE readable again. This
  is the documented consequence of `org_keys` living inside the snapshot while
  the key that unwraps it lives outside; a test asserts it so nobody mistakes
  deletion for a backup-spanning shred.
- **GIVEN** an org whose `org_keys` row is missing, **WHEN** any secret in it is
  read, **THEN** the read fails with a named error rather than falling back to
  the instance master key.
- **GIVEN** an org with a running execution, **WHEN** an operator confirms
  deletion by typing the slug, **THEN** executions are stopped first, all org
  rows are removed, the org storage root is removed, and every session and
  token for its users is revoked.
- **GIVEN** an org whose execution cannot be stopped, **WHEN** deletion is
  confirmed, **THEN** the deletion aborts, the storage root still exists, and
  the error names the execution.
- **GIVEN** a new table added to the schema, **WHEN** the tenancy completeness
  test runs, **THEN** it fails until the table is classified as `instance`,
  `tenant-root`, or `descendant` in the tenancy registry.

## Out of scope

- **Workspace sharing, roles, and scopes.** Those are
  [roles and scopes](../auth/roles-and-scopes.md) and
  [workspace visibility and membership](../workspaces/membership.md), which ship
  first and do not require an org. This spec only bounds them to one org.
- **Multiple orgs per user.** One org per user is the design, not a stopgap: it
  makes the tenant a total function of the identity and removes an entire class
  of "acting as" confusion. Someone who needs two orgs creates two accounts. If
  multi-org is ever added, `users.org_id` becomes the primary-org pointer and a
  membership table joins it; no data migration is required.
- **Schema-per-tenant or database-per-tenant.** Row-level scoping plus
  filesystem and runtime pinning is the boundary. Revisit only if a hosted
  deployment demands physical separation.
- **Billing, plans, quotas, and usage metering.** The org is the anchor these
  will hang from; none of them ship here.
- **Per-org SSO / IdP pinning and domain-based auto-join.** Plugin-provided
  external login (ADR 0050) stays instance-level for now.
- **Per-org data export or per-org backup/restore.** Backups stay instance-wide.
- **Cross-org anything** — no shared workspaces, no transfer of a workspace or
  user between orgs, no cross-org search, no cross-org analytics.
- **Crypto-shredding an org out of existing backups.** Per-org DEKs buy two
  things: a partial compromise (a leaked key, a bad export, memory disclosure
  in one org's context) exposes that org rather than all of them, and the
  envelope structure is the prerequisite for a real shred. They do not deliver
  the shred, because the wrapping key is in the same snapshot boundary as the
  ciphertext.

  The change that would deliver it is specific: wrap each org's DEK with a key
  that lives **outside** the database snapshot and dies with the org. The
  per-org filesystem root is the natural home, since deletion already removes
  it and backups already do not capture it. It is out of scope here because the
  cost is not small and lands on every self-hosted operator: a database backup
  alone stops being sufficient for recovery, and an org root lost to disk
  failure or a partial restore takes that org's secrets with it, permanently.
  That tradeoff deserves its own decision rather than riding in on this one.
- **Protecting the root key itself.** `master.key` remains a mode-0600 file in
  the data directory beside `kandev.db`, so anyone who can read that directory
  can unwrap every org's DEK. `MasterKeyProvider` is the seam a KMS, HSM or
  env-supplied root key plugs into. Envelope encryption stops one org reaching
  another's plaintext; it does not defend the instance against someone who
  already has its disk.
- **Sandboxing an agent from the OS user it runs as.** Tenant pinning stops
  org A's agent from reading org B's tree; it does not stop an agent from
  escaping its own org's tree if the OS permits it. That remains the executor's
  job.
