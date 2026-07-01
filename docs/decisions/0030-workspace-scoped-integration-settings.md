# Workspace-scoped integration settings

## Status

accepted

## Context

GitHub integration settings mixed install-wide authentication with operational
settings such as watch configuration, action presets, default queries, and repo
filters. That made multi-workspace installs confusing: a GitHub watch or query
configured for one workspace could appear beside settings for another, and the
`/github` PR/issue lists had no workspace boundary beyond ad hoc repo filters.

Jira and Linear already model operational integration state as workspace-owned.
GitHub needs to follow the same ownership rule while keeping authentication
shared, because the authenticated GitHub account/token is still install-wide.

## Decision

Third-party integration authentication can remain global when the external
provider account is global, but operational integration settings are owned by a
workspace.

For GitHub, each workspace has a `github_workspace_settings` row containing:

- repository scope mode: all repositories, selected organizations, or selected
  repositories;
- selected org/repo scope values;
- workspace-owned GitHub query presets.

The settings UI exposes a topbar workspace switcher on integration settings
routes. GitHub settings render for the active workspace, and review/issue watch
dialogs are locked to that workspace from the settings page.

The backend enforces GitHub repository scope on `/github` PR/issue searches and
watch polling results. The frontend also uses the scope to narrow repo selector
options, but that is only a usability layer; backend filtering is the source of
truth.

## Consequences

- Existing installs default to `all` repository scope, preserving current
  behavior until a workspace changes its GitHub scope.
- Workspace-scoped GitHub searches have cache keys that include the workspace
  scope so scoped and unscoped result pages do not share cached data.
- Default query presets migrate opportunistically from the older global
  browser/user-settings path into the first workspace where the user opens
  GitHub settings or the `/github` page.
- Task creation repo/branch pickers remain governed by workspace repositories;
  GitHub repository scope only affects GitHub integration surfaces.

## Alternatives Considered

### Keep GitHub settings global

Rejected. It would keep the current ambiguity and diverge from Jira/Linear,
where operational settings are already per-workspace.

### Treat selected repositories as workspace repositories

Rejected. GitHub repo scope is not the same as task execution repositories. A
workspace may monitor many GitHub repos for PRs/issues without attaching all of
them as local task repositories.
