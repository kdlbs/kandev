# TYPO-009: Centralize UI and technical font-family roles

- Priority: P1
- Status: Resolved
- Scope: Settings UI copy, technical values, terminal dialogs, code fields, and logs
- Related: TYPO-001, TYPO-006, TYPO-008

## Finding

The application has a base Figtree sans family and a global mono family, but settings does not name or enforce semantic family roles. Technical settings values use local `font-mono` classes, and the PTY terminal creates its own hard-coded family and size.

## Evidence

- `apps/web/app/globals.css:76` defines the base `--font-sans` family as Figtree followed by Geist and system fallbacks.
- `apps/web/app/globals.css:17-19` defines the global mono family and size variables.
- Settings technical values use local mono classes in `apps/web/components/settings/account/api-tokens.tsx:70`, `apps/web/components/settings/system/about-card.tsx:15`, `apps/web/components/settings/system/database-stats-card.tsx:58`, and `apps/web/components/settings/plugins/plugin-manifest-card.tsx:74`.
- `apps/web/components/settings/pty-terminal-view.tsx:56` hard-codes `ui-monospace, SFMono-Regular, Menlo, Monaco, "Courier New", monospace` and `fontSize: 13`, which is separate from the global mono family and from the terminal font preference rendered by `apps/web/components/settings/terminal-settings.tsx`.
- `apps/web/components/settings/profile-edit/script-editor.tsx:197-202` hard-codes Monaco `fontSize: 13` even though `apps/web/lib/theme/colors.ts:62-66` and `apps/web/lib/theme/editor-theme.ts:90-92` expose shared editor font tokens, including a 12px editor size.
- Settings code and value fields also choose their own sizes, for example `apps/web/components/settings/profile-edit/env-vars-card.tsx:65-179` and `apps/web/components/settings/repository-card.tsx:175-201`.
- Provider credential/value inputs split between mono `font-mono` in `apps/web/components/github/github-pat-form.tsx:44` and `apps/web/components/gitlab/gitlab-settings.tsx:150,303`, versus the sans defaults in Jira, Linear, Azure DevOps, and Sentry forms. The same credential role therefore changes family and size by provider.

## User impact

The sans family is generally consistent, but the family boundary is implicit. Technical values can look different depending on the component, and the PTY surface can look different from other terminal-related settings without an intentional reason.

## Proposed direction

Define explicit settings family and editor-size roles: UI copy uses the application sans family, technical values use the global mono family, and terminal/editor output uses configured terminal/editor tokens or a documented fallback. Decide one credential/value role for provider tokens and hosts, then apply it across all integrations. Keep code, paths, identifiers, secrets, and logs mono. Do not apply mono to explanatory copy merely to signal that a field is advanced.

## Verification

- Compare technical values in About, Account, System, Plugins, Executors, and Workspace settings.
- Verify configured terminal font behavior in both the settings preview and PTY login dialog.
- Check fallback behavior when a custom font is unavailable.
- Compare equivalent provider credential/value fields side by side for family, size, placeholder, and password/token readability.
- Verify PTY and Monaco/script editor sizes against the existing editor/theme tokens and terminal preferences.

## Progress

- 2026-08-16: Opened from the settings source audit.
