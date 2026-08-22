# ADR-2026-08-05-plugin-native-integration-settings: Plugins Contribute Native Integration Settings

**Status:** accepted
**Date:** 2026-08-05
**Area:** frontend, protocol

## Context

An integration plugin can register an arbitrary plugin settings route, but that route
appears under Settings > Plugins and does not participate in Kandev's native
Settings > Integrations index, workspace navigation, or workspace-scoped URL model.
Official integrations such as Bitbucket should use the same discovery and settings
structure as built-in integrations without adding provider-specific rendering branches
to the host.

## Decision

The frontend plugin registry adds
`registerIntegrationSettings({ id, label, description, icon?, Component, action? })`.
`Component` receives `{ workspaceId?: string }`. An optional `action` receives the
routed workspace and a `surface` value. The host renders the action in the detail
section header and the native integrations index card. The host renders the main
component in the existing
settings shell at both `/settings/integrations/{id}` and
`/settings/workspace/{workspaceId}/integrations/{id}`, and adds the contribution to
the native integrations index and workspace settings navigation. The host wraps the
component's plugin-owned cards and controls in the shared `SettingsSection`, using the
registered label, description, and resolved icon, and uses the label in settings
topbar chrome.

Integration IDs are URL-safe slugs. First-party integration IDs remain reserved, one
active plugin owns each contributed ID, collisions fail without replacing the current
owner, and unload revokes the contribution. Labels, descriptions, icons, and rendering
stay plugin-owned; the host contains no provider-specific settings branch. Curated
brand icons remain a host-owned string-to-component map, including `bitbucket`.

Operational plugin routes remain separate. An integration's queue, review, or browse
workbench may live at its normal plugin route, while credentials, connection health,
and watch configuration use the native integration settings contribution.

## Consequences

Official and third-party integrations can feel first-party in Settings while retaining
independent release ownership. Plugins must choose a stable non-reserved slug and make
their settings component and action work with either an explicit workspace or the
legacy global route. Actions must tolerate the detail and index surfaces. The host
owns navigation, responsive settings chrome, error containment, and lifecycle. The
plugin owns all provider-specific fields and behavior.

## Alternatives Considered

- Keep integration configuration under Settings > Plugins: rejected because it makes
  an installed integration use a different information architecture from built-ins.
- Let plugins register arbitrary `/settings/integrations/...` routes: rejected because
  route registration alone cannot safely contribute index metadata, workspace
  navigation, ownership, or collision handling.
- Add a Bitbucket-specific settings page to the host: rejected because future provider
  plugins would repeat the same core coupling.
