---
title: "Agent-authored Canvases"
description: "Create, review, promote, edit, recover, and remove isolated web-app canvases."
status: experimental
---

# Agent-authored Canvases

This guide explains how users and task agents create and manage isolated web-app canvases for tasks and workspaces.

## Availability

> [!EXPERIMENTAL]
> Agent-authored canvases are in progress. `features.canvases` is off in every shipped profile, and this page describes the current contract.

An administrator can enable `features.canvases` in **Settings > System > Feature Toggles** or set `KANDEV_FEATURES_CANVASES=true`. Restart Kandev after changing the toggle. The restart is required because the feature changes MCP tool registration and backend composition.

With the flag off, Kandev does not expose canvas tools, routes, events, background work, or navigation. Database migrations can still exist, but Kandev does not read or change canvas data.

## Create a task canvas

1. Open the task that owns the canvas.
2. Ask the task agent to create a canvas for the current task.
3. Let the agent read the version-matched canvas authoring skill.
4. Review the first release after the agent publishes it.

Kandev binds a new task canvas to the current task. The agent cannot choose a different task, workspace, or owner through canvas input. A task can hold up to 10 non-removed task canvases.

The agent creates a packaged static web app. Kandev does not provide a blank canvas builder, native blocks, source upload, or a direct source editor. The authoring skill explains the package layout and the relative `./_kandev/v1` protocol. Kandev does not inject a JavaScript API into the app.

Kandev validates the package before it stores or runs the release. An invalid draft keeps the current active release. A valid release becomes active only after its permissions fit the current grants.

## Use a canvas

A task canvas belongs to one task. A workspace canvas appears in workspace navigation and uses workspace scope. Promotion changes the same canvas from task scope to workspace scope. It does not copy the canvas.

The app uses relative requests such as `./_kandev/v1/data/tasks` and `./_kandev/v1/state`. Kandev remains the source of truth for task, workflow, and message data. Canvas state stores app-specific shared state. It does not replace Kandev domain data.

The host shows canvas controls outside the app frame. The app runs in a sandboxed iframe with an opaque browser origin:

- The frame allows packaged scripts and forms.
- The app cannot use the host DOM, cookies, host authentication headers, popups, or top-level navigation.
- `localStorage`, `sessionStorage`, IndexedDB, and service workers are not available.
- Use Kandev instance state for small shared values. Keep temporary values in memory.
- External network access uses exact HTTPS origins that a user approved.
- Remote scripts are not allowed. Scripts and styles must come from the package.

The **Releases and permissions** control is available on task and workspace
canvas hosts. It shows each release's declared reads, writes, events, shared
state, exact external origins, missing grants, protocol version, and safe
source provenance. Use it to approve or reject the first task release. A task
canvas cannot run until its pending permissions are approved.

The host receives canvas lifecycle notifications through WebSocket. It
refreshes visible task, workspace, release, and direct-host projections after
creation, release activation, promotion, archive, restore, or removal. It
tears down the old iframe before it loads a replacement runtime after an
authority change. This also limits the lifetime of direct browser requests to
approved external origins.

Kandev calculates effective access from the package declaration, instance grant, trusted task or workspace scope, and current caller authorization. A release receives only the intersection of those permissions. See [Security and trust](security.md#isolated-web-applications) for the security boundary.

## Promote a task canvas

Only a user can promote a task canvas. An agent cannot promote, demote, grant permissions, approve a release, archive, restore, roll back, or remove a canvas.

1. Open the task canvas and choose its promotion action.
2. Read the review dialog before you continue.
3. Review every Kandev data read and write, event subscription, shared-state permission, and exact external HTTPS origin.
4. Confirm the task-to-workspace scope change and workspace placement.

The confirmation includes the active release ID, permission declaration
digest, and grant generation that you reviewed. If any of these changes before
confirmation, Kandev rejects the request as stale and requires a new review.

The promotion keeps the canvas identity, active release, state, and release history. A workspace canvas then appears in workspace navigation. Canceling the review leaves the task canvas unchanged.

If promotion adds a permission, Kandev keeps the current active release until a user approves the new grant. The new release stays pending permission until that review finishes.

## Review permissions and releases

Every published release is immutable. Kandev retains the active release, one prior valid release, and a pending release when one exists.

Use the release review to inspect the manifest, source actor, declared Kandev access, and exact network origins. A release that requests no new access can replace the active release after validation. A release that requests more access stays pending and does not change the active app.

Reject a pending release to keep the current app. Use rollback to select the retained prior release. Rollback does not restore grants that a user already revoked. Kandev starts a new permission review if the selected release needs access that is not currently granted.

## Edit a canvas with Quick Chat

1. Open the canvas host controls and choose **Edit**.
2. Use the new Quick Chat to describe the change.
3. Review the draft and its validation result.
4. Publish the draft after its permissions are acceptable.

Kandev starts a normal Quick Chat with a trusted canvas target. The agent receives the canvas identity, manifest, active source, validation result, and current grants. The target is fixed to the selected canvas.

The agent publishes through the canvas release flow. The active release changes
only after validation and permission review. A new permission request uses the
same review as promotion. If another edit publishes first, the stale edit is
rejected and cannot silently replace the newer release. An expired Quick Chat
does not remove the active or prior release.

## Recover, archive, and remove a canvas

Kandev keeps the active release across page reloads and backend restarts. A missing or changed artifact makes the release unavailable before Kandev runs it. Kandev does not silently replace or execute that artifact.

Use these recovery actions:

- Reload the page after a temporary connection loss. The host reconnects and keeps the last known release.
- Fix an invalid draft and publish it again. The active release stays available while the draft is invalid.
- Roll back to the retained prior release after a failed publish.
- Restore the database and matching artifact directory after a storage loss. See [Canvas artifacts and recovery](operations.md#canvas-artifacts-and-recovery).
- Archive a canvas to hide it from normal discovery. Restore it from the canvas controls.
- Remove a task canvas to remove its task-scoped data. Removing a task does not remove a canvas already promoted to a workspace.
- Remove a workspace canvas to remove its grants, state, tokens, releases, and artifacts after cleanup completes.

Kandev records artifact cleanup before it removes release ownership. A worker completes cleanup after the database transaction and retries it after a restart.

## Limits and device behavior

Kandev allows up to 100 workspace canvas instances across scopes. Archived canvases count toward instance and storage limits. A workspace can retain up to 2 GiB of canvas artifacts. One Kandev installation can retain up to 10 GiB.

On desktop, workspace canvases use the workspace Canvases area. On phones, Kandev opens a full-height canvas route and keeps canvas controls in an inset bottom drawer.

## Related guides

- [Plugin manifest reference](plugins-manifest.md#isolated-web-applications) defines the `ui.web_apps` manifest fields.
- [Authoring a plugin](plugins-authoring.md#build-an-isolated-web-application) explains package authoring without an injected JavaScript API.
- [Configuration](configuration.md#runtime-feature-toggles) explains the feature flag and restart rule.
- [Security and trust](security.md#isolated-web-applications) explains sandboxing, capabilities, network access, and opaque storage.
- [Operations](operations.md#canvas-artifacts-and-recovery) explains the database and artifact backup boundary.
- [Feature status](feature-status.md) records the public support status.
