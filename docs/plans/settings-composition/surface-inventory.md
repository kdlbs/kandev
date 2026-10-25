# Settings Surface Inventory

This inventory records the original migration coverage. It is a delivery record, not another product contract.
The 2026-09-22 [Task 06 refinement](task-06-concise-help-and-tabs.md) is pending.
It supersedes the completed Task behavior row's collapsed-runtime presentation with tabs and concise optional help.
Original completion claims below do not verify this refinement.
The route source is `apps/web/src/settings-routes.tsx`, including its dynamic routes and integration route adapter.
For each row, record changed components, retained specialized layouts, and exact rendered evidence during implementation.
A shared primitive change alone does not prove that every caller conforms.

| Family | Entry files or directories | Composition | Work order | Result |
| --- | --- | --- | --- | --- |
| Task behavior | `components/settings/task-behavior-settings.tsx`, task controls, `system/message-queue-settings.tsx`, `system/session-capacity-settings.tsx`, `sleep-inhibition-settings.tsx` | Preference groups and runtime form disclosure | 01 | Complete. Four ordered groups keep runtime mounted behind a closed disclosure. Load, validation, and asynchronous save attention from queue, session, and sleep owners reveal it once while preserving later manual collapse. Queue/session contributors feed the group dirty marker, and focused component tests cover dirty, reset, save, and disclosure transitions. |
| Appearance and keyboard shortcuts | `components/settings/general-settings.tsx`, `appearance-account-sections.tsx`, `startup-page-settings-card.tsx` | Preference groups | 02 | Complete. Simple preference cards are rows inside one group frame, while menu/startup/metrics controls keep their specialized form bodies. Route targets, dirty state, and existing controls remain intact; the Appearance structural check verifies no row-owned card is nested in the group frame. |
| Notifications | `components/settings/notifications-settings.tsx`, external-provider and event sections | Preference groups and provider forms | 02 | Complete. The page template is frameless and sound, permission, provider, and event surfaces own their shared group frames. The component type-scale test and desktop/mobile structural checks verify grouped content without an extra page card. |
| Layouts and terminal/editors | `components/settings/layouts/`, `terminal-editors-settings.tsx`, `terminal-settings.tsx`, `editors-settings.tsx` | Preference/form groups, retained previews | 02 | Complete. The terminal/editors page is frameless with two group frames; terminal, editor, preview, and language-server bodies retain specialized geometry. The desktop structural check verifies the page-level frame is not duplicated. |
| Agents and profiles | `app/settings/agents/`, `components/settings/agent-profile-page.tsx`, agent setup and profile children | Resource and form groups | 03 | Complete. Installed-agent resource chrome and profile children preserve actions and dialogs; desktop and mobile agent layout checks passed. |
| Executors and profiles | `app/settings/executor/`, `app/settings/executors/`, `components/settings/profile-edit/`, SSH and Kubernetes settings | Resource/form groups, retained diagnostics | 03 | Complete. Executor/profile groups retain SSH and Kubernetes diagnostics; desktop and mobile executor spacing checks passed. |
| Utility agents, external MCP, prompts | Corresponding routes under `app/settings/`, `components/settings/prompts-settings.tsx` | Resource/form groups, retained editors | 03 | Complete. Utility, MCP, prompt, and editor surfaces use shared surrounding groups while retaining their specialized bodies; component and family matrices passed. |
| Workspaces and repositories | `app/settings/workspace/`, `components/settings/workspaces/`, repository cards and forms | Resource and form groups | 04 | Complete. Workspace settings and route sections use shared group chrome; the repository resource section is frameless so each repository card remains the single bordered surface; tabs, forms, and repository children retain their behavior; desktop/mobile family and workspace-tab checks passed. |
| Workflows and automations | Workspace workflow routes, workflow forms, automation routes | Group chrome around retained editors | 04 | Complete. Shared route sections are frameless around retained workflow editor cards, while repository-set rows and automation forms keep their owning surfaces; workspace/integration family coverage and the full settings matrix passed. |
| Integration connections and watches | `src/integration-settings-route.tsx`, `components/github/`, `gitlab/`, `jira/`, `linear/`, `azure-devops/`, `sentry/`, native integration host forms | Credential/form and resource groups | 04 | Complete. Integration route sections use shared group semantics; GitHub/Jira desktop checks and GitHub mobile checks passed, including repository-scope heading and save behavior. |
| Global and workspace secrets | `components/settings/secrets-settings.tsx` and secret dialogs | Resource/form groups | 04 | Complete. Secret list/form chrome is grouped while dialogs and credential behavior remain domain-owned; component and family matrices passed. |
| System, storage, status, updates, feature toggles | `components/settings/system/` and system route shells | Resource/form groups, retained diagnostics | 05 | Complete. Data/logs, About, and system route surfaces use shared groups around retained diagnostics and tables; system family desktop/mobile checks passed. |
| Account, users, organizations, units | `components/settings/account/`, `system/users-table.tsx`, `system/organizations/`, `units/` | Resource/form groups | 05 | Complete. Token, user, organization, and unit resource chrome is grouped while permissions and dialogs remain intact; system/account family desktop/mobile checks passed. |
| Plugin listing/details and host-provided settings | `app/settings/plugins/`, `components/settings/plugins/`, native host settings chrome | Resource/form groups with plugin-owned frame opt-out | 05 | Complete. Installed plugin and detail settings use shared groups; browse/canvas/plugin-owned bodies remain specialized, and integration routes explicitly preserve frameless plugin-owned content; full component and family matrices passed. |

Paths in the table are relative to `apps/web/`.
Existing redirect routes and the Settings index retain navigation behavior.
Existing dialogs inherit field and action presentation without replacing their submission flow.
Plugin-owned content is outside this repository's migration scope.

## Retained specialized layouts

- Workflow pipeline, sidebar/layout preview, drag ordering, and automation editors retain their interaction geometry.
- Monaco, terminal, prompt, code, query, and log content retain technical fonts and editor sizing.
- Tables retain columns and overflow within their owning surface. Their headings, helpers, and actions use the shared settings contract.
- Health, connection, update, and permission states retain their semantic alert colors.
- Destructive operations remain visibly separate and retain confirmation behavior.
- Settings header tabs retain their current placement and URL behavior.

## Evidence protocol

For each row, inspect its imported first-party forms and identify every composition exception.
Mark a row complete only after its work-order commands pass and its desktop/phone evidence is recorded.
Use seeded mock resources for empty and populated states. Do not mutate a developer instance.
Record exact commands and outcomes, not screenshots alone or class-name inspection alone.
