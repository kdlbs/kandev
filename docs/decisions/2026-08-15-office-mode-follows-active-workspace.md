# ADR-2026-08-15-office-mode-follows-active-workspace: Office Mode Follows the Active Workspace

**Status:** accepted
**Date:** 2026-08-15
**Area:** frontend, backend

## Context

Office-vs-kanban chrome was derived from the pathname: "in Office" meant "on an
`/office` route". Every shared surface (Settings, Stats, task pages,
integration dashboards) is outside that route family, so opening one silently
swapped an Office user's sidebar to kanban chrome. The two "home" affordances
disagreed about where home is. Visiting `/` with an office workspace active
reassigned the active workspace to the first kanban board the app could
render, and the settings bootstrap filtered office workspaces out of its
active-workspace resolution, so opening Settings could switch the workspace as
a side effect.

Office data (agents, projects, inbox) was loaded by the office route
bootstrap, so the store held it only while an `/office/*` route was mounted.
The office collections were flat single values, safe only under that same
route-owned loading.

## Decision

The active workspace decides the mode. The URL never does — **with one
explicit exception (amended 2026-08-17): an explicit `?workspaceId=` query
param on the office page is honored at page bootstrap** (route-first
resolution, matching ADR 0023's URL-params precedence), because the office
page resolves its data workspace from the URL. Navigation without an
explicit param cannot change the active workspace; when a route and the
active workspace disagree, the workspace wins and the URL follows (the
kanban board route redirects to the workspace's Office home instead of
reassigning the workspace).

Mode resolves through a `WorkspaceScopeProvider` context rather than direct
store reads, with one provider at the app root. Mode is tri-state:
`office`, `kanban`, or `unknown` while the workspace list has not hydrated,
and chrome holds on `unknown` instead of assuming kanban.

The office collections are keyed by workspace id and loaded by
`useOfficeWorkspaceData` from the always-mounted sidebar, so the data is a
property of the selected workspace rather than of the URL. The boot payload
names an active workspace on every authenticated SPA route, including
`/settings`, and no resolution path filters office workspaces out.

Workspace selection is a single write (`rememberWorkspaceSelection` plus the
store update in `useSelectWorkspace`). Switching modes is switching
workspaces: the workspace picker is the one control for it, and the footer's
dedicated Office/Kanban button is removed.

Shared kanban surfaces reached by URL (`/tasks`, `/stats`, integration
dashboards) render under the active workspace's chrome; they do not redirect.

## Consequences

- Shared surfaces keep the active workspace's chrome, and leaving them returns
  to that workspace's home.
- Navigation can no longer change the active workspace as a side effect; only
  an explicit selection does.
- Two workspaces' office data can coexist in the store across a switch; a late
  response lands under its own key instead of overwriting the current
  workspace's data. This is also the store shape a future side-by-side layout
  needs, together with per-subtree workspace scoping.
- Entering Settings still re-resolves the active workspace from the cookie and
  settings fallback, so a workspace activated only by visiting a URL (never
  explicitly selected) can snap back. Unchanged from before, minus the kanban
  bias.
- The legacy `office-active-workspace` cookie keeps its ADR-0023 role as a
  read-only fallback for the office boot paths; office selections now write
  the port-scoped name (amended 2026-08-17, see
  `docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md`).

## Alternatives Considered

- **Keep the pathname rule and patch each shared surface.** Rejected because
  every new shared surface would reintroduce the mode flip, and the home and
  workspace-reassignment defects are the rule itself, not its call sites.
- **Read the store directly in `useInOffice` instead of a scope context.**
  Rejected because it relocates the singleton: side-by-side panes need mode to
  be a property of a subtree, and retrofitting a context later means rewriting
  every consumer.
- **Redirect kanban surfaces such as `/tasks` under an office workspace.**
  Rejected because they are shared surfaces that render fine for any
  workspace; only the kanban board is a genuine mismatch.
- **Keep the flat office store and clear it on switches.** Rejected because
  route-independent loading makes two workspaces' data overlap in flight, and
  clearing loses the previous workspace's data on every switch.
