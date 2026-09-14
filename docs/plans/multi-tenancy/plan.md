---
spec: docs/specs/multi-tenancy/spec.md
created: 2026-08-22
status: draft
---

# Multi-Tenancy (Organizations) Plan

## Scope

Implement [`../../specs/multi-tenancy/spec.md`](../../specs/multi-tenancy/spec.md):
an organization tier above users, gated by the restart-required
`features.multiTenancy` toggle, with row-level scoping, two-tier
instance/org configuration, per-org filesystem roots and agent credentials,
tenant-pinned executors, and per-org background work.

**This plan runs after [team access](../team-access/plan.md)** — the scope
registry, org roles, and workspace visibility that
[#2824](https://github.com/kdlbs/kandev/issues/2824) actually asks for. That
work needs no org and ships first on today's auth model. Organizations are the
boundary above it. The coupling is narrow: `org` visibility must mean the
workspace's own org, org scopes are held in one org only, and cross-org
membership is refused — one check in the add path plus a migration that drops
cross-org rows, both owned by this plan in task 04.

Self-hosted deployments are the near-term target. The design is built to hold
for untrusted co-tenants because a hosted deployment is the eventual goal, but
every wave is validated on a self-hosted shape first.

## Current State

Nothing is implemented, and this plan assumes team access has landed: an
`internal/authz` scope registry with fixed org and workspace roles,
`workspaces.visibility`, `workspace_members`, and a `workspaceVisibleTo` that
already resolves owner / explicit row / org-visible. If tenancy were somehow
built first, every org check below still applies — it would just be bounding a
narrower predicate.

The existing base is the opt-in auth feature
(`docs/specs/auth/spec.md`, `internal/auth/`): users with `admin`/`member`,
sessions and PATs, `authn.Identity` in the request context, and per-user
workspace scoping through `workspaces.owner_id` enforced by the `authorize*`
helpers in `internal/task/service/service_access.go`, the WS gateway backstop
in `internal/gateway/websocket/dispatch_scope.go`, and `internal/mcp/scope`.

Three properties of that base define the shape of this work:

1. **An absent identity currently means "internal caller, unscoped."** Pollers,
   office schedulers, the orchestrator, and automation runners rely on it. That
   rule is a cross-tenant read the moment a second org exists, so inverting it —
   and migrating every background caller to an explicit per-org system identity
   — is the critical path, not a cleanup task.
2. **Configuration is instance-global today**, and a shared agent profile means
   a shared provider credential. The two-tier split is what makes org-level
   credential ownership possible at all, and `org.config.manage` is the scope
   that gates the org tier.
3. **The filesystem is one flat tree under `<home>`**, and agent CLIs
   authenticate as the backend's OS user. Row scoping does nothing about
   either; the isolation waves do.

## Architecture

- **The tenant is derived from the identity, never from the request.**
  `authn.Identity` gains `OrgID`; no route, payload, header, or WS frame may
  name an org. This is the same lesson as `internal/mcp/scope` resolving the
  owner from the `AgentExecution` rather than the agent-supplied payload.
- **One tenancy registry, no parallel maps.** `internal/tenancy/registry.go`
  classifies every table as `instance`, `tenant-root`, or `descendant` (with
  its FK path to `workspaces.org_id`), and records the reason for each
  denormalized `org_id`. A completeness test enumerates the live schema and
  fails on an unclassified table. This mirrors `internal/runtimeflags/registry.go`.
- **Scope at the service layer, backstop at the transport.** Org checks extend
  the existing `authorize*` helpers rather than adding a second mechanism.
  The WS dispatch backstop and the MCP scoper gain org resolution; neither
  replaces the service-layer check.
- **Denormalize `org_id` only where a query would otherwise leak.** A
  descendant table gets its own column when at least one production query
  reaches it without joining its root. The registry records which query.
- **Instance templates are shape, org rows are substance.** `org_id = ''` is a
  template; an org row shadows one by `template_id`. Every credential-bearing
  field is org-only, enforced at the API boundary and by a pinning test.
- **Filesystem tenancy is additive for upgrades.** The default org's
  `storage_root` stays `''`, meaning the legacy flat tree, permanently. Only
  new orgs get `<home>/orgs/<org_id>/`. No mass file move ships.
- **Standalone executors fail closed.** `local_pc` shares the backend's OS user,
  so `HOME` redirection is a convention, not a boundary. Multi-org standalone
  requires a per-org OS user or an explicit operator escape hatch.
- **Every wave lands behind the flag, off.** The feature is mergeable at any
  point; `features.multiTenancy` only becomes settable once wave 4 completes.

## Backend Touch Points

- New packages: `internal/tenancy/` (registry, classification test, org model,
  storage-root resolution), `internal/org/` (service, store, HTTP controller,
  lifecycle).
- `internal/auth/`: `authn.Identity.OrgID` / `.Instance`, `SystemIdentity`,
  `httpmw` org resolution and suspended-org fail-close, invite org binding,
  `users.org_id`, per-org email uniqueness.
- `internal/task/service/service_access.go`: org checks in every `authorize*`
  helper; identity-free denial under the flag.
- `internal/gateway/websocket/`: `BroadcastToOrg`, subscription org checks,
  `//ws:global` allowlist pinning test, port-proxy capability org binding.
- `internal/mcp/scope/`: resolve stream → task → workspace → org; deny on
  unresolvable, as it already does for owner.
- `internal/orchestrator/`, `internal/office/`, `internal/jira/`,
  `internal/linear/`, `internal/github/`, `internal/gitlab/`,
  `internal/azuredevops/`, `internal/sentry/`, `internal/automation/`: per-org
  cycle loops carrying `SystemIdentity`.
- `internal/agent/settings/store/`, `internal/agent/executor/`,
  `internal/task/repository/sqlite/`: `org_id` + `template_id` columns and
  effective-view queries for the eight config kinds.
- `internal/worktree/`, `internal/repoclone/`, `internal/task/service/attachment_service.go`,
  `internal/system/` storage maintenance: org storage-root resolution.
- `internal/agent/runtime/lifecycle/`, `internal/agentctl/`: per-org
  `HOME`/`XDG_*` for every ACP subprocess and agentctl child; Docker org labels,
  naming, volumes, and non-adoption on recovery; SSH executor org ownership;
  standalone fail-closed check placed before any process start.
- `internal/githubauth/` broker lease, automation webhook secret, office channel
  HMAC, plugin webhook secret: org binding and cross-org redemption refusal.
- `internal/runtimeflags/registry.go`, `profiles.yaml`: `features.multiTenancy`
  and `features.multiTenancyTrustedStandalone`, both `false` in every profile.
- `cmd/kandev/e2e_reset.go`: org-aware reset.

## Frontend Touch Points

- Boot payload `org` key and store hydration (`apps/web/src/boot-payload.ts`).
- Operator surfaces under Settings > System: Organizations list/create/suspend/
  delete (type-to-confirm on slug), per-org OS user, and instance Templates for
  the eight config kinds.
- Org admin surface: Settings > Organization (rename, members, invites already
  exist and become org-bound).
- Existing executor / profile / environment / agent / editor / prompt /
  notification-provider settings render `source: instance` rows as read-only
  with a "Use in this organization" action that creates the shadowing org row.
- Organization-unavailable route for `org_suspended`, distinct from `/login`
  so a suspended user is not told their credentials are wrong.
- All new copy through `t()` / `<Trans>` in five languages
  (`pnpm run i18n:zh-hant` for the Traditional Chinese pair), no em dashes.

## Task Waves

| Wave | Tasks | Theme |
|---|---|---|
| 1 | 01, 02 | Registry, flag, org table, identity carries the org |
| 2 | 03, 04, 05, 06 | Scoping: background work, service layer, transports, self-authenticating credentials |
| 3 | 07, 08 | Two-tier instance/org configuration |
| 4 | 09, 10, 11, 15 | Runtime isolation: filesystem, credentials, executors, secret keys |
| 5 | 12, 13, 14 | Org lifecycle, frontend, E2E and docs |

Task 03 is the critical path and the highest-risk item: it changes the meaning
of an identity-free context across the whole backend. It should land before any
wave-2 sibling and be reviewed as a security change.

## Tasks

- [ ] [01 — Tenancy registry and feature flag](task-01-tenancy-registry-and-flag.md)
- [ ] [02 — Org entity and identity binding](task-02-org-entity-and-identity.md)
- [ ] [03 — Per-org background work and system identity](task-03-system-identity-background-work.md)
- [ ] [04 — Service-layer org scoping](task-04-service-layer-org-scoping.md)
- [ ] [05 — Transport scoping: WS gateway and MCP](task-05-transport-scoping.md)
- [ ] [06 — Self-authenticating credential org binding](task-06-credential-org-binding.md)
- [ ] [07 — Two-tier configuration storage and effective view](task-07-two-tier-config-storage.md)
- [ ] [08 — Instance template API and operator role](task-08-instance-templates-and-operator.md)
- [ ] [09 — Per-org filesystem roots](task-09-per-org-filesystem-roots.md)
- [ ] [10 — Per-org agent credential home](task-10-per-org-agent-credentials.md)
- [ ] [11 — Executor tenant pinning](task-11-executor-tenant-pinning.md)
- [ ] [12 — Org lifecycle: create, suspend, delete](task-12-org-lifecycle.md)
- [ ] [15 — Per-org secret encryption keys](task-15-per-org-secret-keys.md)
- [ ] [13 — Frontend org and operator surfaces](task-13-frontend-surfaces.md)
- [ ] [14 — E2E, docs, and ADR](task-14-e2e-docs-adr.md)

## Risks

- **Inverting the identity-free rule (task 03) can silently break background
  work.** A poller that loses its grant fails quietly. Mitigation: the denial
  path logs at warn with the caller, and task 03 ships a test that runs each
  background entry point with an identity-free context and asserts denial —
  the mutation-test standard, not an observed-passing suite.
- **Missed leak surfaces.** The registry completeness test covers tables, not
  queries. Task 04 adds a second net: a repository-level test that runs each
  list/get method as a foreign org and asserts an empty result or a sentinel.
- **`local_pc` is not a real boundary.** Accepted and specced as fail-closed
  rather than papered over. The escape hatch is explicit, operator-only, and
  logged.
- **Upgrade path.** The default org keeps the legacy flat tree forever, so no
  mass file move ships. The migration aborts the boot rather than leaving an
  unassignable row reachable by every tenant; `persistence.Provide`'s
  pre-migration backup is the recovery path.
- **Scope creep in the other direction.** Team access already decides who sees
  what inside an org, via visibility and scopes. This plan must not add a second
  visibility rule keyed on the org; it only bounds the existing one. A request
  that starts "when multi-tenancy is on, org admins should also see…" is a
  change to `roles-and-scopes.md`, not to this plan.
- **Sequencing pressure from cloud.** If a hosted deployment moves up, the two
  items to pull forward are org suspension (already specced as the reversible
  billing lever, task 12) and per-org export, which is currently out of scope.
  Neither changes the isolation design.
