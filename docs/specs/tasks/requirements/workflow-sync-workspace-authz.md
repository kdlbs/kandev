---
status: active
system: tasks
created: 2026-08-06
owners:
  - nova28
---
# Workflow Sync — Per-User Workspace Authorization Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-WORKFLOW-SYNC-WORKSPACE-AUTHZ-001: Workflow Sync — Per-User Workspace Authorization

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-SYNC-WORKSPACE-AUTHZ-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why
Every other per-workspace integration service (GitHub, Jira, Linear, Slack, Azure DevOps,
Automation) enforces that a caller-supplied `workspace_id` belongs to the requesting user *inside
the service itself* — `SetWorkspaceAuthorizer` + a check at the top of every user-facing method —
in addition to the global `integrationWorkspaceScopeMiddleware`
(`internal/backendapp/main.go`/`helpers.go`) that already gates their HTTP routes by path prefix.
`internal/workflowsync` is the one integration service that never adopted the service-layer half of
that pattern: its `Service` methods take a caller-supplied `workspace_id` straight from the caller
with no ownership check of their own.

**Correction (found during review):** an earlier draft of this spec claimed the HTTP endpoints
(`/api/v1/workflow-sync/config`, `/api/v1/workflow-sync/sync`) had no ownership check at all and were
directly exploitable by an authenticated member. That was wrong — those paths are already listed in
`integrationWorkspacePrefixes` and covered by the global middleware, which authorizes before any
workflow-sync handler runs, exactly like `/api/v1/jira/`, `/api/v1/github/`, etc. The actual gap this
spec closes is narrower: `workflowsync.Service` has no authorization boundary of its own, unlike
every sibling integration service, so (a) it is the one integration whose safety depends entirely on
the route-level middleware never being bypassed, misconfigured, or missing this path prefix, with no
independent second layer, and (b) any future non-HTTP caller of the service (a WS handler, an MCP
tool, a plugin, a different mount point) would inherit no protection at all, unlike every sibling
service where the constructor-level check still applies regardless of how the call arrived. This is
a defense-in-depth and consistency fix, not a closure of a currently open HTTP exploit.

## What
- The workflow-sync config read endpoint (`GET /api/v1/workflow-sync/config`) SHALL deny a request
  for a `workspace_id` the caller does not own.
- The workflow-sync config write endpoint (`POST /api/v1/workflow-sync/config`) SHALL deny writing a
  config for a `workspace_id` the caller does not own.
- The workflow-sync config delete endpoint (`DELETE /api/v1/workflow-sync/config`) SHALL deny
  deleting a config for a `workspace_id` the caller does not own.
- The force-sync endpoint (`POST /api/v1/workflow-sync/sync`) SHALL deny triggering a sync for a
  `workspace_id` the caller does not own.
- A denied request SHALL be indistinguishable from a request against a nonexistent workspace (404,
  no existence leak) — consistent with every other per-user-scoped integration in this codebase.
- Requests from an **unscoped caller** — no identity in context (internal callers: the periodic sync
  poller, the event bus) or a synthetic identity (auth disabled) — SHALL continue to work exactly as
  before this fix; this feature only closes the gap for caller-supplied `workspace_id` values
  reaching these endpoints from an authenticated HTTP request.
- Workspaces with no owner (`owner_id = ''`, pre-auth rows not yet claimed by the setup wizard)
  SHALL remain visible to any authenticated member, matching the existing workspace-visibility rule
  used everywhere else.

## Permissions
Reuses the existing per-user workspace scoping rule documented in
`apps/backend/AGENTS.md` ("Opt-in authentication & per-user scoping"):
- No identity in context, or a synthetic identity → unscoped (today's pre-auth behavior).
- Real identity → the workspace is visible only if its `owner_id` is empty or matches the caller.
- Denial uses the `ErrWorkspaceNotFound` sentinel — the same value the task service, GitHub, Jira,
  Linear, Slack, and Azure DevOps services already return for this class of denial — so a foreign
  workspace and a missing workspace produce the same observable response.

This spec does not introduce a new permission concept; it applies the existing one to a service that
missed it.

## Failure modes
- **Caller lacks access to the target workspace:** the request is denied with the sanitized
  not-found response before the config store (or the sync/apply pipeline) is touched. No config
  contents, repo identity, or sync status for the foreign workspace are ever included in the denial.
- **Workspace authorizer not wired** (e.g. a unit test constructing `workflowsync.Service` directly
  without calling `SetWorkspaceAuthorizer`): the service behaves as unscoped, matching every other
  integration service's nil-safe default — this preserves existing tests and call sites that don't
  go through the full app-boot wiring.

## Scenarios
- **GIVEN** two workspaces A (owned by user `alice`) and B (owned by user `bob`) each with their own
  workflow-sync config, **WHEN** `bob` calls `GET /api/v1/workflow-sync/config?workspace_id=<A>`,
  **THEN** the response is 404 and contains no data about workspace A's config.
- **GIVEN** the same setup, **WHEN** `bob` calls
  `POST /api/v1/workflow-sync/config?workspace_id=<A>` with an attacker-controlled repo payload,
  **THEN** the request is denied (404) and workspace A's stored config is unchanged.
- **GIVEN** the same setup, **WHEN** `bob` calls `DELETE /api/v1/workflow-sync/config?workspace_id=<A>`
  or `POST /api/v1/workflow-sync/sync?workspace_id=<A>`, **THEN** both are denied (404) and workspace
  A's config/sync state is unaffected.
- **GIVEN** workspace A owned by `alice`, **WHEN** `alice` calls any of the four endpoints against her
  own workspace, **THEN** the request succeeds exactly as before this fix.
- **GIVEN** `features.auth` is disabled (synthetic identity) or the caller is an internal caller (no
  identity in context, e.g. the periodic sync poller), **WHEN** any workflow-sync entry point runs,
  **THEN** it is unaffected — no denial, same behavior as before this fix.

## Out of scope
- Changing the periodic poller (`SyncDueConfigs`/`SyncWorkspace` invoked internally) to carry or
  require an identity — it is a correct internal caller today and stays unscoped by the existing
  `callerScope` rule.
- Any change to GitHub or GitLab repository-level authorization (App installation scope, PAT scope)
  — this spec is strictly about the `workspace_id` ownership boundary on the workflow-sync HTTP
  surface.
- Adding workspace authorization to any other integration package — all of them already have it.
- New data model or schema changes — no persistent state changes.