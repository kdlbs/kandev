# Workspace-scoped integration settings

## Status

accepted

## Context

GitHub integration settings mixed install-wide authentication with operational
settings such as watch configuration, action presets, default queries, and repo
filters. That made multi-workspace installs confusing: a GitHub watch or query
configured for one workspace could appear beside settings for another, and the
`/github` PR/issue lists had no workspace boundary beyond ad hoc repo filters.

Jira, Linear, Sentry, Slack, GitLab, and GitHub all have settings that users
expect to vary by Kandev workspace. Keeping credentials or provider defaults
install-wide makes multi-workspace installs ambiguous: a "work" workspace and a
"personal" workspace may require different tokens, hosts, orgs, projects,
teams, or utility agents.

## Decision

Third-party integration configuration is workspace-owned by default. Provider
credentials, host URLs, default project/team/org fields, health status, query
presets, and watcher settings are read and written for an explicit
`workspace_id`.

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

For Jira, Linear, Sentry, Slack, and GitLab, singleton config tables migrate to
workspace-keyed config rows. Startup migration is deterministic and does not
prompt: copy the old singleton config and secret into the user's active
workspace when it exists, otherwise the first workspace by creation time. If no
workspace exists, leave the singleton data untouched until a workspace is
created or the user reconnects.

A follow-up should add a copy action that copies the current workspace's
provider config and credentials to another workspace. Copying automation/watch
rows should remain separate and opt-in so users do not accidentally duplicate
task-creating pollers.

## Consequences

- Existing installs migrate their singleton integration config to one
  deterministic workspace. If that is not the user's intended workspace, users
  can reconnect the integration in the correct workspace today; a future copy
  action should make that correction path faster.
- Existing GitHub installs default to `all` repository scope, preserving
  current behavior until a workspace changes its GitHub scope.
- Workspace-scoped GitHub searches have cache keys that include the workspace
  scope so scoped and unscoped result pages do not share cached data.
- Default query presets migrate opportunistically from the older global
  browser/user-settings path into the first workspace where the user opens
  GitHub settings or the `/github` page.
- Task creation repo/branch pickers remain governed by workspace repositories;
  GitHub repository scope only affects GitHub integration surfaces.
- Config/status APIs must either require `workspace_id` or derive it from the
  active workspace route state. Install-wide `/config` semantics are deprecated
  for integrations that support workspace-scoped settings.

## Alternatives Considered

### Keep GitHub settings global

Rejected. It would keep the current ambiguity and diverge from Jira/Linear,
where operational settings are already per-workspace.

### Keep credentials install-wide and only scope watchers

Rejected. It solves only part of the ambiguity. Different workspaces often map
to different external accounts or tenants, especially for GitLab self-managed
hosts, Jira sites, Sentry orgs, Linear teams, and Slack utility-agent routing.

### Prompt during migration

Rejected. Database migrations must be deterministic and able to run during
backend startup, CI, desktop launch, and headless server upgrades. A
copy-to-workspace affordance gives users a reversible correction path without
blocking startup.

### Treat selected repositories as workspace repositories

Rejected. GitHub repo scope is not the same as task execution repositories. A
workspace may monitor many GitHub repos for PRs/issues without attaching all of
them as local task repositories.
