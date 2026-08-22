---
status: draft
created: 2026-08-22
owner: tbd
---

# Workspace Visibility and Membership

Tracks [issue #2824](https://github.com/kdlbs/kandev/issues/2824). Depends on
[org roles and scopes](../auth/roles-and-scopes.md).

## Why

A team running one Kandev server wants a shared board: everyone sees what
everyone is working on, anyone can open a colleague's task, read the agent
conversation, test the preview, and take it over. Today every workspace has
exactly one owner and returns 404 to everyone else, so the only way to get a
team board is a shared login — which forfeits per-person sessions, audit,
per-user API tokens, and per-human GitHub identity.

The fix is a **default**, not an invitation flow. A Kandev workspace is coarse:
it owns the default executor, default environment, default agent profile, the
task prefix and sequence, its Kanban workflow, and the workspace integrations
resolve against. A team has one or two. Asking that team to invite each
colleague into each workspace would just be their org roster, retyped by hand.

## What

- **A workspace has a visibility: `org` or `private`.** `org` means every
  member of the org reaches it with the scopes their org role grants. `private`
  means only the owner and explicitly added members reach it. Visibility is the
  primary mechanism; membership is the exception path.
- **The org has a default visibility for new workspaces**, settable by anyone
  with `org.settings.manage`. A team install leaves it `org` and gets the
  shared board with zero invitations. An install that is several individuals
  privately sharing one server sets it to `private` and gets exactly today's
  behavior. Both are first-class.
- **Existing workspaces migrate to `private`.** An upgrade never widens access
  to data that was private the moment before. Turning on the shared board is an
  explicit act by each workspace owner, or a one-click "make all my workspaces
  org-visible" action offered once.
- **Membership is the exception mechanism, with a role.** A
  `workspace_members` row names a user and a workspace role (`owner`,
  `collaborator`, `viewer`). It serves three jobs: populating a `private`
  workspace, letting a `guest` into exactly one workspace, and **narrowing** a
  member to `viewer` on a sensitive org-visible workspace. An explicit row
  always outranks the org default, in both directions.
- **What a member can do is decided by scopes, not by membership.** Reach and
  permission are separate questions;
  [roles and scopes](../auth/roles-and-scopes.md) owns the second. This spec
  owns only which workspaces a user can reach.
- **Tasks gain a human assignee.** `tasks.assignee_user_id` is new and distinct
  from `assignee_agent_profile_id`, which names an Office agent, not a person.
  Assignment is advisory: it gates nothing. **Takeover is reassign plus
  continue** — no lock is taken and no session is torn down.
- **Every human action records its actor.** Session messages, queued messages,
  workflow-step transitions, task state changes, and agent stop/cancel record
  the acting `user_id` and the UI attributes them. This is the audit layer a
  shared login cannot provide, and no path falls back to the workspace owner
  when the actor is unknown.
- **Concurrent prompting is serialized, not blocked.** Two members prompting one
  session both enter the existing message queue in arrival order, each
  attributed. Kandev does not add a per-session human lock: it would strand a
  session when someone shuts their laptop, and the queue already provides
  ordering.
- **Per-human GitHub identity already composes.** `github_user_connections` is
  keyed `(workspace_id, user_id)`, so "Approve as X" and PR actions run under
  the acting member's own connection as soon as they can reach the workspace.
  That package does not change.
- **Membership and visibility are confined to one org.** When
  [multi-tenancy](../multi-tenancy/spec.md) is enabled, `org` visibility means
  the workspace's own org and nothing wider, and adding a member from another
  org is refused with 404.

## Data model

```
workspaces
  + visibility     enum  org | private     NOT NULL DEFAULT 'private'

workspace_members
  workspace_id  string     PK part, FK -> workspaces.id  (cascade delete)
  user_id       string     PK part, FK -> users.id       (cascade delete)
  role          enum       owner | collaborator | viewer
  added_by      string     acting user id ('' for the seeded owner row)
  created_at    timestamp
```

- `PRIMARY KEY (workspace_id, user_id)`.
- The workspace creator gets an `owner` row, kept in sync with
  `workspaces.owner_id`. A workspace may hold more than one `owner` row;
  `owner_id` names the single accountable owner.
- The migration sets `visibility = 'private'` for every existing workspace and
  writes an `owner` row for each one with a non-empty `owner_id`.
- A pre-auth workspace (`owner_id = ''`) gets no rows and keeps today's
  visible-to-everyone behavior until the setup wizard claims it.

Changed existing tables:

| Table | Change | Meaning |
|---|---|---|
| `tasks` | `+ assignee_user_id TEXT NOT NULL DEFAULT ''` | human assignee, independent of `assignee_agent_profile_id` |
| `task_session_messages` | `+ author_user_id TEXT NOT NULL DEFAULT ''` | who sent a human message; `''` means agent output |
| `queued_messages` | `+ author_user_id TEXT NOT NULL DEFAULT ''` | attribution survives queueing |
| `task_step_transitions` | `+ actor_user_id TEXT NOT NULL DEFAULT ''` | who moved the task; `''` means engine-driven |

Empty always means "not a human", never "unknown human".

## API surface

```
PATCH  /api/v1/workspaces/{id}                    -> accepts visibility (workspace.manage)
POST   /api/v1/workspaces/visibility/bulk         -> {visibility} for the caller's own workspaces
GET    /api/v1/workspaces/{id}/members            -> list (workspace.read)
PUT    /api/v1/workspaces/{id}/members/{userId}   -> {role} add or re-role (member.manage)
DELETE /api/v1/workspaces/{id}/members/{userId}   -> remove (member.manage)
POST   /api/v1/workspaces/{id}/transfer-ownership -> {user_id} (member.manage)

PATCH  /api/v1/orgs/current/settings              -> accepts default_workspace_visibility
PATCH  /api/v1/tasks/{id}                         -> accepts assignee_user_id (task.write)
GET    /api/v1/users/directory                    -> id + display name of active users
```

`/api/v1/users/directory` returns only what a member picker needs — ID and
display name, never email, role, or status. Full user records stay behind
`org.members.manage`.

**Changed contracts**

- Workspace DTOs gain `visibility`, `member_count`, `viewer_role`, and
  `scopes` (see [roles and scopes](../auth/roles-and-scopes.md)).
- Task DTOs gain `assignee_user_id`.
- Session message DTOs gain `author_user_id` and its resolved display name.
- WS: workspace-scoped events reach everyone who can reach the workspace, which
  for an `org`-visible workspace is the whole org minus guests.
  `workspace.access.updated` is emitted when visibility or membership changes so
  an open client re-evaluates without a reload; losing access is delivered as a
  subscription drop.

## Permissions

Reach is this spec; permission is
[roles and scopes](../auth/roles-and-scopes.md). The two compose as:

| Situation | Reaches the workspace? | With which scopes |
|---|---|---|
| `workspaces.owner_id` is the caller | yes | workspace `owner` |
| explicit `workspace_members` row | yes | that row's role |
| `visibility = 'org'`, caller in the org, org role is not `guest` | yes | the org role's default workspace role |
| `visibility = 'org'`, caller is a `guest` | no | — |
| `visibility = 'private'`, no row | no | — |
| different org | no | — |

Changing visibility, managing members, and transferring ownership require
`workspace.manage` / `member.manage`, which only a workspace `owner` holds.
Org admins gain no reach here: an admin sees an org-visible workspace because
it is org-visible, and never sees a private one they are not in.

## Failure modes

- **Adding a user who does not exist, is disabled, or already has a row** — 400
  with a distinct code per case; nothing is written.
- **Adding a user from another org** (tenancy enabled) — 404, so org membership
  is not enumerable.
- **Removing the last `owner` row, or removing `workspaces.owner_id`'s row** —
  refused; ownership must be transferred first.
- **Transferring ownership to a user with no row** — refused; add them first.
- **Setting `visibility = 'org'` on a workspace whose owner is a `guest`** —
  refused; a guest cannot publish to an org they cannot see.
- **A user loses access mid-session** (removal, visibility narrowed, role
  narrowed) — their next request 404s or 403s, WS subscriptions drop, and any
  agent turn they started keeps running and stays attributed to them. Nothing is
  rolled back.
- **A user is disabled** — rows are retained but grant nothing.
- **Two members prompt one session simultaneously** — both messages queue in
  arrival order and both are attributed; neither is dropped or cancelled.
- **A member deletes a task another member is viewing** — the viewer receives
  the existing task-deleted event and is routed out; there is no handshake.
- **Workspace deletion** — membership rows cascade and are also removed
  explicitly by the workspace-deleted handler, per the side-table rule.

## Persistence guarantees

- Visibility, membership rows, and attribution survive restart and are
  unaffected by disabling `features.auth` (they become inert, as ownership does
  today).
- Attribution is permanent: removing a member never rewrites their past
  messages, transitions, or assignments to anonymous.
- No worktree, session, or preview is per-member. Members share the workspace's
  server-side resources, which is what makes takeover work at all.
- The upgrade never widens access: every pre-existing workspace lands
  `private`.

## Scenarios

- **GIVEN** an upgraded instance, **WHEN** the migration runs, **THEN** every
  existing workspace is `private`, every non-owner still gets 404, and behavior
  is byte-identical to the build before this feature.
- **GIVEN** an org whose default visibility is `org` and members Ana, Bruno and
  Carla, **WHEN** Ana creates a workspace and a task, **THEN** Bruno and Carla
  see it on their board with no invitation, and can open its session
  transcript and port preview.
- **GIVEN** the same org set to default `private`, **WHEN** Ana creates a
  workspace, **THEN** Bruno gets 404 until Ana adds him or changes visibility.
- **GIVEN** an org-visible workspace, **WHEN** Bruno reassigns a task to
  himself and prompts, **THEN** the assignee is Bruno, the session continues in
  the same worktree with the same execution, and the message is attributed to
  Bruno.
- **GIVEN** an org-visible workspace and a `viewer` row for Carla, **WHEN**
  Carla opens the board, **THEN** she reads tasks and transcripts; **WHEN** she
  opens a terminal or prompts, **THEN** she gets 403.
- **GIVEN** a `guest` user, **WHEN** they list workspaces, **THEN** they see
  only workspaces they hold a row on, including none of the org-visible ones.
- **GIVEN** Bruno viewing a task in an org-visible workspace, **WHEN** the owner
  sets visibility to `private`, **THEN** Bruno's next request 404s, his
  subscriptions drop, and his earlier messages remain attributed to him.
- **GIVEN** member Bruno with his own GitHub connection, **WHEN** he approves a
  PR from the task panel, **THEN** the approval is made as Bruno's GitHub
  identity, not the owner's.
- **GIVEN** members Ana and Bruno on one session, **WHEN** both prompt within
  the same second, **THEN** both messages appear in arrival order, each
  attributed, and neither is dropped.
- **GIVEN** an owner, **WHEN** they transfer ownership to Carla, **THEN** Carla
  becomes owner, the previous owner becomes `collaborator`, and `owner_id` and
  the `owner` row agree.
- **GIVEN** multi-tenancy enabled, **WHEN** an owner tries to add a user from
  another org, **THEN** the response is 404.
- **GIVEN** a workspace with three members, **WHEN** it is deleted, **THEN** all
  rows are removed and the E2E reset leaves no orphan.

## Out of scope

- **Email or link invites to new people.** This adds existing deployment users;
  account creation stays with the auth invite flow.
- **Group or team objects.** Members are added individually; the org is the
  group.
- **Nested or per-project scoping below the workspace.** The workspace is the
  unit.
- **Sharing workspace secrets or integration credentials with members.**
- **Real-time presence, cursors, or typing indicators.**
- **Per-member worktrees or previews.** The shared server-side resource is the
  point.
- **Locking a session to one human.**
- **Human reviewer/approver workflow participants.** Existing participants name
  agent profiles.
