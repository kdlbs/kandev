# TYPO-005: Normalize descriptions, helpers, and status copy

- Priority: P1
- Status: In progress
- Scope: Page descriptions, card descriptions, field helpers, empty states, and status/error copy
- Related: TYPO-001, TYPO-002, TYPO-004

## Finding

Helper and description text is split between `text-xs`, `text-sm`, custom line heights, and the defaults of `CardDescription`. The same muted explanatory role therefore changes size across settings pages. Error, empty, and loading states also use different sizes without a clear semantic rule.

## Evidence

- `apps/packages/ui/src/card.tsx:44` defaults `CardDescription` to `text-xs/relaxed`.
- `apps/web/components/settings/settings-page-template.tsx:70` and `apps/web/components/settings/settings-section.tsx:51` use `text-sm text-muted-foreground` for page and section descriptions.
- `apps/web/components/settings/notification-events-table.tsx:155-156` pairs a `font-medium` heading with a `text-xs` description, while `apps/web/components/settings/notification-permission-section.tsx:50-51` uses a `text-sm` heading with a `text-xs` description.
- `apps/web/components/settings/system/feature-toggle-card.tsx:41` uses a `text-sm` description and line `54` uses `text-sm leading-6` for risk copy.
- `apps/web/components/settings/system/database-stats-card.tsx:131-143` uses `text-sm` operation labels with `text-xs` descriptions.
- Across the settings tree, `text-xs text-muted-foreground` is repeated for helpers in components such as `apps/web/components/settings/terminal-settings.tsx:132-134`, `apps/web/components/settings/repository-card.tsx:178-204`, and `apps/web/components/settings/system/storage/storage-policy-fields.tsx:30-56`.
- Profile helpers vary between `text-xs` without `leading-relaxed` in `apps/web/components/settings/profile-form-fields.tsx:49,142-146,232-234,381`, `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx:176-178,242-258,343-375`, and `apps/web/components/settings/profile-edit/mcp-policy-card.tsx:92-94`; inline errors vary between `text-xs` and `text-sm` across the same surfaces.
- Automation helper copy is smaller than its labels: `apps/web/components/automations/config-section.tsx:542` and `automation-repository-rows.tsx:90` use `text-[10px]`, and the workflow-load error at `config-section.tsx:462` is also `text-[10px]` beside `text-xs` labels.
- Settings shells and discovery models repeat description classes instead of naming a shared role: `apps/web/components/settings/settings-page-template.tsx:65-71`, `system/system-page-shell.tsx:13-20`, and `apps/web/components/settings/settings-section.tsx:38-56` accept or render resolved description strings, while `apps/web/lib/settings-discovery/types.ts:3-15` models labels but not a description role. Long locale descriptions in `apps/web/src/locales/en/settings.json:455-466,557-560,663-683` can therefore receive different size and leading decisions by route.
- Account token and security tables explicitly use `text-xs` for rows and cells at `apps/web/components/settings/account/api-tokens.tsx:257-273` and `security-settings.tsx:164-185`, while loading/empty states use `text-sm`; define a deliberate dense-table role or promote primary readable columns to `text-sm`.
- Primary account/system errors use `text-xs text-destructive` in `api-tokens.tsx:125-128,246-249`, `security-settings.tsx:85-88,153-156`, `system/create-user-dialog.tsx:102-105`, `invite-dialog.tsx:81-84`, `users-table.tsx:295-299`, and `backups-table.tsx:216-219`, while comparable user-facing errors use `text-sm`.
- Visible guidance in `apps/web/components/settings/system/backups-table.tsx:212-215`, `account/api-tokens.tsx:244-245`, and storage policy descriptions remains `text-xs` beside page/section explanatory copy at `text-sm`.

## User impact

Important explanatory text can look like metadata on one page and like readable body copy on another. Long translated descriptions are more likely to wrap or become hard to scan when a page uses an unplanned compact role.

## Proposed direction

Separate the roles instead of making every paragraph the same size:

- page and section descriptions: readable muted body role;
- card descriptions and field helpers: compact but readable role;
- loading and empty states: body role when they are the main content;
- validation and error copy: compact status role with stable line height;
- technical metadata: the documented micro-type exception.

Provide shared `SettingsDescription`, `SettingsHelpText`, and `SettingsErrorText` roles, or equivalent class constants, with documented inline-versus-page error sizing. Normalize helper copy to a readable `text-xs/relaxed` role and remove direct combinations where possible. Keep translation keys and render-time `t()` resolution intact rather than moving translated strings to module scope.

Define a deliberate dense-table role for account/security rows, promote primary user-facing columns and errors when they are the main content, and retain compact treatment for IPs, timestamps, user agents, tokens, and diagnostics.

## Verification

- Test long translated and pseudo-locale descriptions.
- Verify helper text remains readable next to compact controls.
- Verify errors and empty states do not become smaller than the content they explain.
- Compare profile, MCP, executor, and command-preview helpers and errors in the same viewport.
- Include automation helpers and workflow-load errors; reserve 10px for dense metadata, status, or chips rather than ordinary field help.
- Compare account tables, system dialogs, backups, and storage guidance against the shared helper/error roles.

## Progress

- 2026-08-16: Opened from the settings source audit.
