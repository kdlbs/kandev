# ADR-2026-09-23-task-canvas-workspace-data: Separate canvas placement from data scope

**Status:** accepted
**Date:** 2026-09-23
**Area:** backend, frontend, protocol, security

## Context

Users create canvases from tasks and review their behavior before promotion.
The original canvas contract binds task placement and data scope together. A
task coordinator that calls the supported canvas task-list endpoint therefore
receives only its creating task, even when the package declares task reads and
the workspace has many tasks. Granting every capability would not remove that
scope filter.

The [owner-authorized creation decision](2026-09-10-canvas-creation-authority.md)
already treats the user's request as authority for the first release's exact
declared grants. The [same-origin decision](2026-09-19-trusted-same-origin-canvases.md)
also establishes that locally run canvas code has the viewer's ordinary browser
authority. The relative canvas API should provide a coherent supported path
without relying on direct first-party API calls.

## Decision

A task canvas has separate placement and data scopes. For a newly
owner-authorized local canvas, the first valid release receives only its
declared supported capabilities, with data scope limited to its current
workspace. It remains placed in the creating task until the user promotes it.
The task-list API can show every task in that workspace before promotion;
current user authorization still applies. Promotion governs placement and
sharing, and still requires human confirmation.

The data scope is server-owned, persisted, included in runtime bindings, and
checked alongside grants on every canvas-protocol request. Agent or manifest
input cannot choose a different workspace. Existing task-only releases retain
their scope until a workspace owner reviews their exact declared permissions
and explicitly enables workspace data access. Later permission increases and
imported packages continue through their existing review paths.

This amends the task-scoped first-grant ceiling in the 2026-09-10 decision and
the coupling of task canvas and task data scope in the 2026-08-26 decision. It
does not change the same-origin trust decision or grant undeclared capabilities.

## Consequences

- An owner-created canvas can be evaluated with representative live workspace
  data and supported actions before it appears in workspace navigation.
- Permission review remains tied to the exact capabilities a release uses;
  there is no blanket read, write, network, secret, or backend permission.
- The runtime, persistence, review projection, and authoring guidance must
  carry placement and data scopes independently.
- Existing task canvases require an explicit access upgrade or promotion to
  show workspace data. No retained release is silently broadened on upgrade.
- The upgrade path must check current ownership and a versioned review
  snapshot atomically, then invalidate older runtime tokens.

## Alternatives Considered

- **Grant all possible permissions by default:** Does not remove the task-list
  scope filter and gives unused writes and network access to generated code.
- **Require promotion before representative preview:** Leaves the user unable
  to judge the app before placing it in workspace navigation.
- **Call ordinary Kandev APIs from canvas code:** Works only with browser user
  authority and bypasses the supported capability contract; capability-only
  hosts would still show one task.
- **Silently widen all existing task canvases:** Changes retained release
  authority without a review of the code and exact declared capabilities.

## Related contracts

- [Workspace preview requirements](../specs/canvases/requirements/task-canvas-workspace-preview.md)
- [Workspace preview design](../specs/canvases/system-design/task-canvas-workspace-preview.md)
