---
status: current
system: auth
requirements:
  - REQ-AUTH-ROLES-AND-SCOPES-001
---

# Editor authorization boundary

## Purpose and boundaries

Auth owns named permission scopes and their server-side enforcement for editor
management and launch. The [roles and scopes requirement](../requirements/roles-and-scopes.md)
defines the behavior. The task service owns workspace reach, and the
[workspaces editor design](../../workspaces/system-design/editor-file-containment.md)
owns file-target containment. This design composes those boundaries without
changing them.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AUTH-ROLES-AND-SCOPES-001` | [Scope enforcement](#scope-enforcement) and [Client visibility](#client-visibility) |

## Components and responsibilities

The authz scope registry defines `org.config.manage` for editor management and
`session.exec` for opening a session editor or folder. Editor handlers guard
create, update, and delete routes with `org.config.manage`; listing remains
available to all callers so they can choose an editor.

The editor service checks session access before reading a session or starting
a process. The backend wiring resolves the session to its task and asks the task
service to authorize `session.exec` on that task. This preserves workspace reach
without admitting access to the session's executor configuration: editor
launches run on the backend host and do not use that executor.

The settings UI hides custom-editor management controls from callers who lack
the admin role. This is a presentation aid; the server-side scope guards remain
the enforcement boundary.

## Scope enforcement

Management writes require `org.config.manage` on the request. Session editor
and folder opens require `session.exec` before repository access, including the
embedded VS Code empty-file-path path. A denied scope maps to HTTP 403. Missing
session or workspace authorization data keeps the existing not-found response.
Authentication-off synthetic callers retain local behavior.

The session check resolves session-to-task before asking the task service to
authorize workspace reach. It does not authorize executor configuration,
because the editor process runs on the backend host. File-path containment and
custom-command argument construction remain owned by their existing workspace
designs.

## Client visibility

The settings UI keeps the editor list visible and hides add, edit, and remove
controls from non-admin callers. These controls mirror the management boundary;
they do not grant permission or replace the handler guards.
