# TYPO-002: Consolidate duplicated settings page-shell typography

- Priority: P1
- Status: Resolved
- Scope: Page headers and top-level settings descriptions
- Related: TYPO-001

## Finding

Several settings shells independently implement the same page heading and description pattern. They currently happen to use similar values, but the duplication makes future changes easy to apply to one settings group and miss another.

## Evidence

- `apps/web/components/settings/settings-page-template.tsx:69-70` renders a `text-2xl font-bold` page title and a `text-sm text-muted-foreground` description.
- `apps/web/components/settings/system/system-page-shell.tsx:16-19` repeats the same title and description classes.
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx:74-79` repeats the same pair for executor profile pages.
- `apps/web/components/settings/workspaces/workspace-section-header.tsx:39-43` implements a related section header with `text-lg font-semibold` and `text-sm` description.
- `apps/web/components/settings/workspaces/workspace-settings-shell.tsx` gives the workspace switcher a separate `text-base font-semibold` treatment and falls back to a `text-2xl font-bold` heading when the workspace is missing.
- `apps/web/components/settings/utility-agents-section.tsx:185-189` renders the Utility Agents page title through a child `SettingsSection` as `text-lg font-semibold`, without the top-level `text-2xl font-bold` heading and separator used by the other settings hubs.
- `apps/web/components/integrations/integrations-index-page.tsx:136-141` uses `WorkspaceSectionHeader` for the legacy `/settings/integrations` route as well as workspace-scoped routes, so the global route receives the child-level `text-lg font-semibold` heading.
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx:70-91` and `apps/web/app/settings/executors/new/[type]/page.tsx:95-106` duplicate scoped page-header layouts instead of sharing the top-level/profile header contract.
- `apps/web/app/settings/executor/[id]/profile/[profileId]/page.tsx:460-480` has a second legacy executor profile header without the current shared icon, type badge, or responsive action wrapper; `components/settings/executor-profiles-card.tsx:88-91` still routes users to it.
- `apps/web/app/settings/agents/[agentId]/agent-setup-parts.tsx:67-77` and the profile-edit/scoped executor headers do not consistently apply the `min-w-0` and `wrap-break-word` behavior already present in `components/settings/agent-profile-page.tsx:95-103`, so long profile names and installation paths can overflow.
- `apps/web/components/settings/terminal-editors-settings.tsx:8-14` renders Terminal Settings and Editors Settings as siblings, but `terminal-settings.tsx:319-357` uses a section-level h3 while `editors-settings.tsx:435-450` introduces a second page-level h2 through `SettingsPageTemplate` on the same route.

## User impact

The same page-level concept is owned by multiple components. A title or description update can produce inconsistent settings pages, and the workspace shell can introduce a different hierarchy from the rest of settings.

## Proposed direction

Create or extend one settings page-header primitive with explicit variants for a page header, workspace header, profile header, and section header. Migrate Utility Agents to the top-level shell, give the legacy global Integrations route the top-level variant, and consolidate the legacy executor profile route onto the current shared header/cards or redirect it. Retain the section variant for workspace-scoped views. Ensure title groups use `min-w-0` and `wrap-break-word`, and bound or truncate path pills. Keep navigation controls and actions as slots, not as reasons to duplicate typography classes. Use the same semantic description role in all shells.

For the combined Terminal Editors route, render one page-level h2/description and two section-level h3 blocks, or intentionally split the route. Preserve render-time translation key resolution when moving the header.

## Verification

- Compare Appearance, Notifications, Agents, Executors, Workspace, and System pages at desktop and mobile widths.
- Include Utility Agents and the legacy global Integrations route in the comparison.
- Include long profile names, installation paths, current and legacy executor profile routes, and narrow tablet widths.
- Include the combined Terminal Editors route and verify there is only one page title per route.
- Confirm page titles remain one level above section titles and that long translated titles wrap cleanly.
- Confirm actions and workspace switchers do not change the title role.

## Progress

- 2026-08-16: Opened from the settings source audit.
