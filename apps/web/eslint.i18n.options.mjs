/**
 * Options for `i18next/no-literal-string`, the guard against hardcoded
 * user-facing strings. Kept in its own module because the list is long enough to
 * bury the rest of eslint.config.mjs, and because it needs the explanatory
 * comments below to stay maintainable.
 */
export const noLiteralStringOptions = {
  // `jsx-only` (not `jsx-text-only`) so the guard also sees copy that
  // never appears as a JSX text node: ternary button labels
  // (`{saving ? "Saving..." : "Save"}`) and display props on internal
  // components (`label=`, `description=`, `tooltip=`). Those are the
  // majority of user-facing strings in this codebase, and the narrower
  // mode reported them as clean. The cost is that every attribute is
  // now checked, so the `words.exclude` list below has to carry the
  // weight of separating copy from prop enum values.
  mode: "jsx-only",
  // Template literals in JSX ARE copy: `Select task ${title}`, `${label} tasks,
  // over WIP limit`. Leaving them unchecked was the largest hole in the guard, and
  // turning it on measured cheap — +2 to +5 findings per un-migrated directory,
  // every one of them a real string, and zero new findings across the allowlist
  // (className templates are already covered by the Tailwind patterns below).
  "should-validate-template": true,
  // Brand/proper nouns and symbol-only strings are not translatable copy.
  //
  // NOTE: the plugin wraps each pattern as `^<pattern>$`
  // (helper/generateFullMatchRegExp), so every entry must match the WHOLE
  // literal. A prefix-only pattern like "^https?://" silently never
  // matches — add an explicit `.*` instead.
  words: {
    exclude: [
      "^\\s*$",
      "^[^A-Za-z]*$",
      "^(Kandev|GitHub|GitLab|Jira|Linear|Slack|Sentry|Azure DevOps|Docker|Codex|OpenCode|Claude|Copilot|Amp|Apprise|Sprites\\.dev)$",
      "^(ACP|MCP|SSH|URL|ID|PR|CI|AI|API|JSON|YAML|LSP|TLS|SQL|JQL)$",
      // Units, version prefixes, and keyboard glyphs — not translatable
      // copy; these show up as fragments beside an interpolated value.
      "^(v|ms\\)?|s|m|h|d|K|B|KB|MB|GB|TB|esc)$",
      "^\\+[A-Z]\\)?$",
      "^[·+\\-|/(),.:\\s]+$",
      // All-caps acronym badges (ENTRY, KAN, MTD, WIQL, JQL) label a
      // field or entity; they are identifiers, not prose.
      "^[A-Z][A-Z0-9_]+$",
      // Multi-word ALL-CAPS tokens are type-to-confirm phrases ("DELETE ALL NOW").
      // The user must type them verbatim and they are compared with `===`, so
      // translating one makes the dialog impossible to satisfy — see
      // docs/i18n.md ("Do not translate").
      "[A-Z][A-Z0-9_]*( [A-Z][A-Z0-9_]*)+",
      // Terminal control glyphs (^C, ^D) and repeat counts (3x) are
      // symbols, and "id · vN.N" is a version line, not prose.
      "^\\^[A-Z]$",
      "^·?\\s*v?$",
      // Keyboard key names label a physical key, so they are not translatable
      // copy — the spec lists keyboard glyphs as out of scope. They appear as
      // <Kbd> children and on keybar config.
      "(Esc|Escape|Tab|Home|End|PgUp|PgDn|Enter|Return|Space|Backspace|Delete|Del|Ins|Shift|Ctrl|Alt|Cmd|Meta|F\\d{1,2})",
      // URLs, home-relative paths, dotted placeholder tokens, and a
      // single letter (an avatar initial) are values, not prose.
      "(https?|file|ssh|git)://.*",
      "^~?/[\\w./~-]*$",
      "^[a-z][a-z0-9]*(\\.[a-z0-9<>]+)+$",
      "^[A-Za-z]$",
      // Tailwind class lists with variants/important modifiers.
      ".*[!\\[].*",
      // Example values shown in placeholders: emails, CSS functions,
      // inline JSON, and ALLCAPS filename stand-ins.
      "[\\w.+-]+@[\\w-]+\\.[\\w.-]+",
      // Wildcard domain globs (`*.example.com`) are the value shape a network
      // policy rule expects, not prose — the user types this pattern verbatim.
      "^\\*\\.[a-z0-9][a-z0-9.-]*$",
      ".*(calc|env|url|var)\\(.*",
      "\\{.*\\}",
      "[A-Z][A-Z0-9_]*\\.[a-z]{2,4}",
      // Static chunks of an interpolated DOM id. With
      // `should-validate-template` on, the plugin checks each literal chunk of a
      // template separately, so `startup-page-${v}-label` arrives as
      // "startup-page-" and "-label" — kebab/snake fragments with a dangling
      // separator, which the token patterns above do not cover. Copy chunks
      // ("Select task ", " tasks, over WIP limit") carry a capital, a space or
      // punctuation and so still get flagged.
      "[a-z0-9]+(?:[-_][a-z0-9]*)*|[-_][a-z0-9]+(?:[-_][a-z0-9]*)*",
      // Single lowercase/camel/kebab tokens are prop enum values,
      // classnames, and identifiers (variant="ghost", side="top",
      // value="work-items") — never display copy, which is capitalized
      // or multi-word.
      "^[a-z][a-zA-Z0-9]*$",
      "^[a-z0-9]+([-_][a-z0-9]+)+$",
      // CSS lengths, colors, Tailwind class lists, link rel/target
      // keywords, route paths, and `__sentinel__` option values.
      "^\\d+(\\.\\d+)?(px|rem|em|%|vh|vw|ch|fr|s|ms|d|h|m|w|y)$",
      "^#[0-9a-fA-F]{3,8}$",
      "^_(blank|self|parent|top)$",
      "^(noopener|noreferrer)( (noopener|noreferrer))*$",
      "^__[a-z_]+__$",
      "^/[\\w/\\-\\[\\]:.]*(\\?[\\w=&%.\\-]*)?$",
      "(?:-?[a-z0-9]+(?:[:/-][a-z0-9.]+)*\\s+)*-?[a-z0-9]+(?:[:/-][a-z0-9.]+)*",
    ],
  },
  "jsx-attributes": {
    // Attributes that carry display copy and must be translated.
    include: ["placeholder", "aria-label", "aria-description", "title", "alt"],
    exclude: [
      ".*[Cc]lassName$",
      "class",
      "id",
      "key",
      "type",
      "name",
      "role",
      "href",
      "src",
      "to",
      "htmlFor",
      "data-.*",
      // Identifiers and prefixes the caller composes into ids/testids.
      "id",
      "k",
      // Option/badge values are data the app compares and submits.
      "value",
      "cmd",
      ".*[Ii]dPrefix$",
      ".*[Ii]dSuffix$",
      ".*SaveId$",
      "aria-labelledby",
      "aria-controls",
      "aria-describedby",
    ],
  },
  callees: {
    // String args to these are identifiers/classnames, not copy.
    exclude: [
      "cn",
      "clsx",
      // `skipAll("User skipped")` records a reason sent to the server
      // alongside the skip; it is stored data, not rendered copy.
      ".*\\.skipAll",
      "cva",
      "tv",
      "t",
      "i18n(ext)?.*",
      "require",
      "console\\.\\w+",
      ".*\\.(getAttribute|setAttribute|matches|closest|querySelector)",
    ],
  },
};

/**
 * Allowlist of paths the `i18next/no-literal-string` ERROR applies to.
 *
 * Deliberately NOT `components/**` + `app/**`. A repo-wide error means every PR
 * that lands a hardcoded string anywhere — including PRs that have nothing to do
 * with i18n — breaks. The first attempt at this migration shipped the global
 * form and spent two days in conflict-resolution rounds because `main` kept
 * moving underneath it.
 *
 * Instead the guard is opt-in per path, so it can only ever tighten:
 *
 *   - Migrating a page or directory? Externalize its strings, then append its
 *     path here in the SAME PR. That path is now permanently protected.
 *   - Everything not listed is unaffected, so unrelated PRs never see this rule
 *     and there is no treadmill.
 *
 * Entries may be single files or directory globs — use a file glob while a
 * directory is partially migrated. Once it is done, LEAVE those entries alone:
 * **adding a path is the only safe edit to this list.**
 *
 * Do not collapse several file globs into one `dir/**`. That fails
 * `check-guard-allowlist.mjs`, which flags every entry that disappears while its
 * path still exists — globs included, resolved with `fs.globSync`. The check
 * cannot tell a tidy-up from quietly dropping protection, and refusing to guess
 * is the whole point of the ratchet: the two edits produce an identical diff to
 * the array, and only one of them is harmless. A broader glob added *alongside*
 * the existing entries passes; swapping them for it never does.
 *
 * Nor may a path be listed twice. An exact duplicate protects nothing new and
 * presents an earlier migration's work as yours; the same check rejects it.
 * (Entries a broader glob already covers are a different thing and are kept on
 * purpose — see the storage globs below.)
 *
 * A clean lint is NOT proof a path is fully migrated — the rule only sees plain
 * literals in JSX. Template literals, `confirm()`/`alert()` arguments, and copy
 * returned from plain `.ts` helpers are invisible to it. The pseudo-locale is
 * the completeness check. See docs/i18n.md.
 */
export const i18nGuardFiles = [
  // The i18n runtime itself.
  "lib/i18n/**/*.{ts,tsx}",
  // Settings → General → Appearance, migrated end-to-end as the worked example:
  // the page, the two sections it owns, and the settings chrome around them.
  "app/settings/general/appearance/**/*.{ts,tsx}",
  // Settings → General → Notifications; the shared settings chrome it renders
  // is migrated with the page that owns it, not here.
  "app/settings/general/notifications/**/*.{ts,tsx}",
  // Settings → General → Secrets.
  "app/settings/general/secrets/**/*.{ts,tsx}",
  // Settings → General → Terminal; the shared settings chrome is migrated with
  // the page that owns it, not here.
  "app/settings/general/terminal/**/*.{ts,tsx}",
  "components/app-sidebar/sections/settings/general-group.tsx",
  "components/settings/general-settings.tsx",
  "components/settings/general-nav.ts",
  "components/settings/language-settings.tsx",
  "components/settings/notification-events-table.tsx",
  "components/settings/notification-permission-section.tsx",
  "components/settings/notification-sound-section.tsx",
  // A `.ts` entry records that the file is migrated, but the rule cannot
  // enforce it: `mode: "jsx-only"` means a file with no JSX is never inspected,
  // so copy added here (toast/dialog/Notification arguments, thrown messages)
  // is caught only by the pseudo-locale, never by lint.
  "components/settings/notifications-settings-actions.ts",
  "components/settings/notifications-settings.tsx",
  "components/settings/secrets-settings.tsx",
  "components/settings/settings-floating-save.tsx",
  "components/settings/settings-layout-client.tsx",
  "components/settings/shell-settings-card.tsx",
  "components/settings/startup-page-settings-card.tsx",
  "components/settings/system-metrics-settings-card.tsx",
  "components/settings/terminal-settings.tsx",
  // Settings → General → Editors: the page, its state/section components, the
  // custom-editor form, and the editable-card shell that form renders inside.
  // `editable-card.tsx` is shared with repository-card.tsx, which is not
  // migrated — the guard is per-file, so that stays unaffected.
  "app/settings/general/editors/**/*.{ts,tsx}",
  "components/settings/editable-card.tsx",
  "components/settings/editor-form.tsx",
  "components/settings/editors-settings-state.tsx",
  "components/settings/editors-settings.tsx",
  "components/settings/lsp-language-options.ts",
  // Settings → General → Sprites.
  "app/settings/general/sprites/**/*.{ts,tsx}",
  "components/settings/sprites-settings.tsx",
  // Settings → General → Layouts: the page and the whole layouts component
  // directory. `layout-editor-actions.ts` and `use-layout-settings.ts` hold no
  // JSX, so `mode: "jsx-only"` never inspects them — the entries record that
  // they are migrated, but only the pseudo-locale can prove it stays that way.
  "app/settings/general/layouts/**/*.{ts,tsx}",
  "components/settings/layouts/**/*.{ts,tsx}",
  // Settings → General → Resource Metrics, Keyboard Shortcuts and Task Actions.
  // The pages render from general-settings.tsx (already listed above); these
  // entries add the routes plus the per-setting cards those two pages own.
  // Shortcut *names* still come from `lib/keyboard/shortcut-overrides.ts`, a
  // registry shared with the un-migrated voice-mode page — deliberately not
  // migrated here.
  "app/settings/general/resource-metrics/**/*.{ts,tsx}",
  "app/settings/general/keyboard-shortcuts/**/*.{ts,tsx}",
  "app/settings/general/task-actions/**/*.{ts,tsx}",
  "components/settings/anchored-prompt-bar-settings.tsx",
  "components/settings/archive-confirmation-settings.tsx",
  "components/settings/keyboard-shortcuts-card.tsx",
  "components/settings/mcp-task-agent-profile-default-settings.tsx",
  "components/settings/unread-divider-settings.tsx",
  // Settings → Integrations → GitHub, connection and authentication half: the
  // page, the settings shell, the status/identity panels, the connection dialog
  // and its PAT / CLI / App forms, and the App onboarding flow. The watch
  // dialogs, watch tables and My-GitHub default sections that the same page
  // renders are NOT migrated yet and are deliberately absent from this list.
  //
  // `github-app-onboarding-model.ts` and `github-access-help.tsx` hold no
  // literals of their own — the first is a plain `.ts` module that
  // `mode: "jsx-only"` never inspects, the second takes all its copy as props.
  // Both entries record that they are migrated; only the pseudo-locale can
  // prove they stay that way.
  "app/settings/integrations/github/**/*.{ts,tsx}",
  "components/github/github-access-help.tsx",
  "components/github/github-app-connection-panel.tsx",
  "components/github/github-app-create-form.tsx",
  "components/github/github-app-import-fields.tsx",
  "components/github/github-app-import-form.tsx",
  "components/github/github-app-import-guide.tsx",
  "components/github/github-app-onboarding-model.ts",
  "components/github/github-app-policy-dialog.tsx",
  "components/github/github-app-registration-list.tsx",
  "components/github/github-app-visibility-field.tsx",
  "components/github/github-auth-method-list.tsx",
  "components/github/github-callback-notice.tsx",
  "components/github/github-cli-form.tsx",
  "components/github/github-connection-dialog.tsx",
  "components/github/github-connection-settings-form.tsx",
  "components/github/github-pat-form.tsx",
  "components/github/github-permissions-dialog.tsx",
  "components/github/github-rate-limit.tsx",
  "components/github/github-repo-scope-section.tsx",
  "components/github/github-settings.tsx",
  "components/github/github-status.tsx",
  "components/github/github-task-credentials-section.tsx",
  // Shared with surfaces that are not migrated. The guard is per-file, so
  // `app/stats` and the Jira/Linear/Sentry/GitLab watch settings that also
  // render these are unaffected.
  "components/github/pr-stats.tsx",
  "components/integrations/workspace-scoped-section.tsx",
  "components/watches/reset-watch-dialog.tsx",
  // Settings → Integrations → GitHub, watches and My-GitHub defaults: the review
  // and issue watch dialogs and tables, the prompt field, and the quick-action /
  // default-query sections. With these the page is fully migrated.
  //
  // The three `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects
  // them; the entries record that they are migrated, and only the pseudo-locale
  // can prove it stays that way.
  "components/github/action-presets-section.tsx",
  "components/github/default-queries-section.tsx",
  "components/github/issue-watch-dialog.tsx",
  "components/github/issue-watch-placeholders.ts",
  "components/github/issue-watch-table.tsx",
  "components/github/review-watch-dialog.tsx",
  "components/github/review-watch-placeholders.ts",
  "components/github/review-watch-prompt-field.tsx",
  "components/github/review-watch-table.tsx",
  "components/github/watch-cleanup-policy.ts",
  // Shared with the My GitHub page (`app/github`) and the automations trigger
  // config, neither of which is migrated. Per-file guard, so they are
  // unaffected.
  "components/github/my-github/action-presets.ts",
  "components/github/my-github/search-bar.tsx",
  "components/github/my-github/use-default-query-presets.ts",
  "components/github/repo-filter-selector.tsx",
  // Settings → Integrations → GitLab: the page and the settings subtree of
  // `components/gitlab` — the connection card, the quick-action presets, the
  // review/issue watch sections, and the shared watch dialog, watch table and
  // delete-confirmation they render through. The merge-request task surface
  // (`mr-*`), `my-gitlab/**` and the task/quick-launcher components live in the
  // same directory and are NOT migrated, which is why this is a file list rather
  // than `components/gitlab/**`.
  //
  // `watch-form.ts` holds no JSX, so `mode: "jsx-only"` never inspects it; the
  // entry records that it is migrated (its two default prompt templates are
  // deliberately untranslated, being persisted and sent to the agent verbatim),
  // and only the pseudo-locale can prove it stays that way.
  "app/settings/integrations/gitlab/**/*.{ts,tsx}",
  "components/gitlab/action-presets-section.tsx",
  "components/gitlab/delete-watch-dialog.tsx",
  "components/gitlab/gitlab-settings.tsx",
  "components/gitlab/issue-watch-dialog.tsx",
  "components/gitlab/issue-watch-table.tsx",
  "components/gitlab/review-watch-dialog.tsx",
  "components/gitlab/review-watch-table.tsx",
  "components/gitlab/watch-dialog.tsx",
  "components/gitlab/watch-form.ts",
  "components/gitlab/watch-settings.tsx",
  "components/gitlab/watch-table.tsx",
  // Settings → Integrations → Jira: the page and the settings subtree of
  // `components/jira` — the connection card, the JIRA-watcher section with its
  // table and dialog, and the task-preset editor. The ticket task surface
  // (`jira-ticket-*`), the import bar and the `my-jira/**` browse UI live in the
  // same directory and are NOT migrated, which is why this is a file list rather
  // than `components/jira/**`.
  //
  // The three `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects
  // them; the entries record that they are migrated, and only the pseudo-locale
  // can prove it stays that way. Each carries copy that is deliberately left in
  // English because it is PERSISTED or sent to an agent verbatim — the default
  // JQL and issue-watch prompt in `jira-issue-watch-placeholders.ts`, the
  // seeded preset labels/hints/templates in `my-jira/presets.ts`.
  //
  // `my-jira/presets.ts` is shared with the un-migrated My Jira page, which the
  // per-file guard leaves unaffected; its icon labels now travel as `labelKey`
  // and resolve at render.
  "app/settings/integrations/jira/**/*.{ts,tsx}",
  "components/jira/jira-enabled-control.tsx",
  "components/jira/jira-issue-watch-dialog.tsx",
  "components/jira/jira-issue-watch-placeholders.ts",
  "components/jira/jira-issue-watch-table.tsx",
  "components/jira/jira-issue-watchers-section.tsx",
  "components/jira/jira-secret-help.tsx",
  "components/jira/jira-settings.tsx",
  "components/jira/my-jira/presets.ts",
  "components/jira/my-jira/use-task-presets.ts",
  "components/jira/task-presets-section.tsx",
  // Settings → Integrations → Linear: the page and the settings subtree of
  // `components/linear` — the connection card, the Linear-watcher section with
  // its table, and the watch dialog with its filter/settings field modules. The
  // issue task surface (`linear-issue-button`, `linear-issue-dialog`,
  // `linear-issue-common`), the import bar and the quick-task launcher live in
  // the same directory and are NOT migrated, which is why this is a file list
  // rather than `components/linear/**`.
  //
  // The two `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects
  // them; the entries record that they are migrated, and only the pseudo-locale
  // can prove it stays that way. Each carries copy that is deliberately left in
  // English because it is PERSISTED or sent to an agent verbatim — the
  // `DEFAULT_LINEAR_ISSUE_WATCH_PROMPT` in
  // `linear-issue-watch-placeholders.ts`, which must also keep matching
  // `apps/backend/config/prompts/linear-issue-watch-default.md`. The placeholder
  // `example` values in the same file, and the option `value`s in
  // `linear-issue-watch-form.ts`, are identifiers rather than copy; the option
  // labels there now travel as `labelKey` and resolve at render.
  "app/settings/integrations/linear/**/*.{ts,tsx}",
  "components/linear/linear-issue-watch-dialog.tsx",
  "components/linear/linear-issue-watch-fields.tsx",
  "components/linear/linear-issue-watch-filter-fields.tsx",
  "components/linear/linear-issue-watch-form.ts",
  "components/linear/linear-issue-watch-placeholders.ts",
  "components/linear/linear-issue-watch-table.tsx",
  "components/linear/linear-issue-watchers-section.tsx",
  "components/linear/linear-settings.tsx",
  // Settings → Integrations → Sentry: the page and the settings subtree of
  // `components/sentry` — the connection section, the per-instance card and
  // add/edit form, the Sentry-watcher section with its table, and the watch
  // dialog with its filter/throttle/multiselect field modules. The issue task
  // surface (`sentry-issue-button`, `sentry-issue-dialog`) and the quick-task
  // launcher live in the same directory and are NOT migrated, which is why this
  // is a file list rather than `components/sentry/**`.
  //
  // `sentry-issue-common.tsx` is deliberately absent. The settings subtree
  // reaches it only for `levelBadgeClass` / `statusBadgeClass`, which return CSS
  // class names and carry no copy; every string in that file belongs to the
  // un-migrated issue dialog, so listing it would pull the task surface into
  // this PR.
  //
  // The two `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects
  // them; the entries record that they are migrated, and only the pseudo-locale
  // can prove it stays that way. `sentry-issue-watch-placeholders.ts` carries
  // copy deliberately left in English because it is PERSISTED and sent to an
  // agent verbatim — `DEFAULT_SENTRY_ISSUE_WATCH_PROMPT`, which must also keep
  // matching `apps/backend/config/prompts/sentry-issue-watch-default.md`. The
  // placeholder `example` values in the same file, and the option `value`s in
  // `sentry-issue-watch-form.ts` (the `SentryLevel` / `SentryStatus` unions and
  // Sentry's `statsPeriod` tokens), are identifiers rather than copy; the option
  // labels there now travel as `labelKey` and resolve at render.
  "app/settings/integrations/sentry/**/*.{ts,tsx}",
  "components/sentry/sentry-instance-card.tsx",
  "components/sentry/sentry-instance-form.tsx",
  "components/sentry/sentry-issue-watch-dialog.tsx",
  "components/sentry/sentry-issue-watch-filter-fields.tsx",
  "components/sentry/sentry-issue-watch-form.ts",
  "components/sentry/sentry-issue-watch-multiselect.tsx",
  "components/sentry/sentry-issue-watch-placeholders.ts",
  "components/sentry/sentry-issue-watch-table.tsx",
  "components/sentry/sentry-issue-watch-throttle-field.tsx",
  "components/sentry/sentry-issue-watchers-section.tsx",
  "components/sentry/sentry-settings.tsx",
  // Settings → Integrations → Azure DevOps: the page and the settings subtree of
  // `components/azure-devops` — the connection card with its PAT help tooltip,
  // the work-item / pull-request watch settings, the quick-action presets and
  // the default-query editor. The board, work-item and pull-request task
  // surfaces (`azure-devops-board*`, `azure-devops-results`, `-filters`,
  // `-scope-bar`, `-task-*`, `-work-item-detail`, `-feedback-dialog`,
  // `-save-view-dialog`, `-task-launcher`) live in the same directory and are
  // NOT migrated, which is why this is a file list rather than
  // `components/azure-devops/**`.
  //
  // `azure-devops-workspace-defaults.ts` holds no JSX, so `mode: "jsx-only"`
  // never inspects it; the entry records that it has been reviewed, and only the
  // pseudo-locale can prove it stays that way. Everything in it is deliberately
  // left in English: the WIQL filters are Azure DevOps' query language, and the
  // preset `label` / `hint` / `promptTemplate` records are PERSISTED to
  // workspace settings and must keep matching the server-side defaults in
  // `apps/backend/internal/azuredevops/workspace_settings.go`, which has no
  // locale.
  "app/settings/integrations/azure-devops/**/*.{ts,tsx}",
  "components/azure-devops/azure-devops-default-queries.tsx",
  "components/azure-devops/azure-devops-quick-actions.tsx",
  "components/azure-devops/azure-devops-settings.tsx",
  "components/azure-devops/azure-devops-watch-settings.tsx",
  "components/azure-devops/azure-devops-workspace-defaults.ts",
  // Shared preset-icon catalog rendered by the Azure DevOps quick actions. Also
  // `.ts`-only, so the entry records the review rather than enforcing it; the
  // icon `key`s are the persisted enum and only the labels are copy, which now
  // travel as `labelKey` and resolve at render.
  "components/integrations/action-preset-icons.ts",
  // Settings → Integrations → Slack: the page and the whole of
  // `components/slack`, which holds nothing else. `slack-settings-state.ts` is
  // the form state and API actions split out of the component so it stays under
  // the 600-line cap; it holds no JSX, so `mode: "jsx-only"` never inspects it
  // and only the pseudo-locale can prove its toast/confirm copy stays migrated.
  //
  // Deliberately left in English, each a value the user must find or type
  // verbatim rather than copy: the `xoxc-` / `xoxd-` token prefixes, the `d`
  // cookie name, the `••••••••` mask, the `!kandev <instruction>` command
  // example, and the six `{{Slack…}}` prompt tokens. All are interpolated as
  // values so the pseudo-locale cannot turn them into dead pointers.
  "app/settings/integrations/slack/**/*.{ts,tsx}",
  "components/slack/**/*.{ts,tsx}",
  // Settings → Agents: the agents list, the per-agent setup page, the agent
  // profile editor, and the `/settings/agent/:id` redirect. The whole
  // `app/settings/agents` tree is migrated, plus the agent half of
  // `components/settings` — the loose `agent-*` / profile / CLI-flag files those
  // three routes render, and the shared model picker and PTY dialog they reach.
  //
  // Deliberately absent, each reached only from a different surface:
  // `components/settings/model-combobox.tsx` (the task-side CLI profile editor
  // and Utility Agents), `components/settings/agent-card.tsx` (Office), and
  // `components/settings/inference-agent-status.tsx` (Utility Agents). The
  // string counter listed all three for this route; walking the import closure
  // of the four pages shows none of them is in it.
  //
  // Also absent: `components/settings/profile-edit/**` (`env-vars-card.tsx` and
  // `profile-env-vars-section.tsx`). The profile editor does render the env-vars
  // card, but that directory is the *executor* profile tree and belongs to the
  // sibling migration; its strings are the only plain English left on the agent
  // profile page under the pseudo-locale.
  //
  // `agent-save-helpers.ts`, `use-profile-mcp-config.ts`,
  // `use-agent-profile-settings.ts`, `use-agent-update-dialog-state.ts` and
  // `agent-profile-page-state.ts` hold no JSX, so `mode: "jsx-only"` never
  // inspects them; the entries record that they are migrated (the first two
  // carry MCP parse errors and the last the profile save/delete toasts, all of
  // which reach the user) and only the pseudo-locale can prove it stays that
  // way. Same for the WebSocket error in `pty-terminal-dialog.tsx`, which is
  // set from a non-JSX handler.
  //
  // Deliberately left in English, each an identifier the user must read or type
  // verbatim, all interpolated as values so the pseudo-locale cannot turn them
  // into dead pointers: the `{prompt}` and `{{model}}` substitution tokens, the
  // `--my-flag` / `greywall --` / `/init` / `superclaude` examples, the
  // `mcpServers` JSON key, the MCP server product names (`Playwright MCP`,
  // `Chrome DevTools MCP`, `Context7 MCP`, `GitHub MCP`) and the Kandev MCP tool
  // list, and the whole of the assembled command preview. Wire values stay wire
  // values: the install/update job `status` unions, the capability-probe
  // `status`, the routing `tier`, the permission `apply_method`, the watcher
  // `kind`, and every model/mode `id` — only their labels are copy, and those
  // travel as catalog keys and resolve at render.
  "app/settings/agent/**/*.{ts,tsx}",
  "app/settings/agents/**/*.{ts,tsx}",
  "components/app-sidebar/sections/settings/agents-group.tsx",
  "components/settings/add-tui-agent-dialog.tsx",
  "components/settings/agent-login-dialog.tsx",
  "components/settings/agent-profile-delete-dialog.tsx",
  "components/settings/agent-profile-page-state.ts",
  "components/settings/agent-profile-page.tsx",
  "components/settings/agent-runtime-update-control.tsx",
  "components/settings/cli-flags-field.tsx",
  "components/settings/command-prefix-field.tsx",
  "components/settings/install-agent-card.tsx",
  "components/settings/installed-agent-card.tsx",
  "components/settings/mode-combobox.tsx",
  "components/settings/profile-capability-helpers.tsx",
  "components/settings/profile-form-fields.tsx",
  "components/settings/profile-status-panels.tsx",
  "components/settings/use-agent-update-dialog-state.ts",
  // Shared with surfaces that are not migrated. The guard is per-file, so the
  // task-side CLI profile editor, the onboarding dialog, Utility Agents, the
  // chat model selector and the session tabs are all unaffected.
  // `host-shell-dialog.tsx` is also opened from a chat action message;
  // `pty-terminal-dialog.tsx` is reached only through it and the agent login
  // dialog, both of which are migrated here.
  "components/model-config-selector.tsx",
  "components/settings/host-shell-dialog.tsx",
  "components/settings/pty-terminal-dialog.tsx",
  // Settings → System → Storage: the route and the whole
  // `components/settings/system/storage` directory, plus the page's own hook.
  // The other eight System routes and the cards they render from
  // `components/settings/system/*.tsx` are NOT migrated, which is why this stops
  // at the `storage/` subdirectory.
  //
  // Four `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects them;
  // the entries record that they are migrated and only the pseudo-locale can
  // prove it stays that way. `use-storage-maintenance.ts` is the load-bearing
  // one — it owns nine toast titles and the refresh-error message, none of which
  // lint can ever see. It imports the module-level `t` rather than the hook so
  // each string resolves when the callback fires. `storage-quarantine.ts` and
  // `storage-totals.ts` carry no copy (the first now formats its deadline
  // through `lib/i18n/formats` instead of the browser locale).
  //
  // Deliberately left in English, none of it copy:
  //   - The type-to-confirm phrases `DEDICATED` / `ADOPT` / `DELETE` /
  //     `DELETE ELIGIBLE` / `DELETE ALL NOW` in `storage-confirmation-dialogs`.
  //     They are a string-literal union compared with `===` to enable the
  //     confirm button, so translating one makes that dialog impossible to
  //     satisfy. They travel as an interpolated value into both the visible
  //     sentence and the input's aria-label.
  //   - `storage-units.ts` in full. `B` / `GB`, `-` and `<0.01 GB` are units and
  //     numeric formatting, not prose, which is why it is absent from this list
  //     rather than listed with a note.
  //   - `run.state` and `run.trigger` in `storage-run-history.tsx`. They are raw
  //     API enum tokens rendered verbatim beside the run's raw JSON result —
  //     `skipped_busy` is the proof — so they are values, not copy.
  //   - Every filesystem path: the adopted/managed Go cache paths, the quarantine
  //     `original_path` / `quarantine_path`, the Docker host, and the
  //     `/root/.cache/go-build` placeholder. All are interpolated as values so
  //     the pseudo-locale cannot turn them into dead pointers.
  //   - `StorageBusyResource.label`, which the backend renders.
  "app/settings/system/storage/**/*.{ts,tsx}",
  "components/settings/system/storage/**/*.{ts,tsx}",
  "hooks/domains/system/use-storage-maintenance.ts",
  // The shell every System route renders. It takes title/description/actions as
  // props and holds no literals of its own, so listing it costs nothing today
  // and pins that contract for the sibling migration of the other eight routes,
  // which will land their copy on top of it.
  "components/settings/system/system-page-shell.tsx",
  // Settings → Executors profile-editor tree: the executor detail route, the
  // new-executor route, the profile editor and all of its cards.
  //
  // Note the SINGULAR `app/settings/executor/`. The plural
  // `app/settings/executors/` is a different tree — the executor list, the
  // typed create routes and the SSH connection pages — and is NOT migrated;
  // it lands with the SSH follow-up. The two paths are one character apart.
  //
  // In `components/settings`, only the `profile-edit/` subtree and the two
  // loose `executor-profile*` files are migrated, which is why those are
  // listed individually rather than as `components/settings/**`. The
  // similarly-named `profile-form-fields.tsx`, `cli-flags-field.tsx` and
  // `agent-profile-*` belong to the AGENT profile editor and are a sibling
  // task's migration.
  //
  // Four entries in `profile-edit/` hold no JSX, or none that carries copy, so
  // `mode: "jsx-only"` never inspects them; the entries record that they were
  // reviewed by eye and only the pseudo-locale can prove they stay that way.
  // `executor-profile-baselines.ts` (the defaults/parsing table),
  // `profile-runtime-sections.tsx` and `profile-env-vars-section.tsx` (pure
  // prop-forwarding) genuinely carry none. `script-editor-completions.ts` is
  // the Monaco completion provider: every token it emits is a shell/script
  // identifier or comes from the placeholder API, so none of it is copy — only
  // the editor's own chrome in `script-editor.tsx` is.
  //
  // Deliberately left in English, each a value rather than copy:
  //   - The Docker/TLS placeholder values `ghp_...`, `/path/to/certs`,
  //     `unix:///var/run/docker.sock`, `tcp://remote:2376 or ssh://user@host`,
  //     `*.example.com` and `kandev/multi-agent:latest`, plus the whole
  //     `DEFAULT_DOCKERFILE` — all are values the user types or Docker parses.
  //   - `DELETE_CONFIRM_TOKEN` ("delete") in the executor delete dialog. It is
  //     compared with `!==`, so the copy interpolates it instead; translating
  //     it would make the dialog impossible to satisfy.
  //   - The seeded executor names "Local Docker" / "Remote Docker" in
  //     `executor/new`. They are PERSISTED as `payload.name` and rendered later
  //     on surfaces this PR does not own, so a stored name must not depend on
  //     the locale it was created in. The Select labels beside them are copy
  //     and do go through `t()`.
  //   - The `ERROR:` prefix `parseDockerLine` emits and matches with
  //     `startsWith`, and every line of `docker build` stdout it forwards.
  //
  // One known residual, recorded rather than folded in: the exported
  // `validateMcpPolicy` in `profile-edit/mcp-policy-card.tsx` still returns
  // English ("Invalid JSON", "MCP policy must be a JSON object"). Its only
  // callers are `app/settings/executors/{[profileId],new/[type]}/page.tsx` —
  // the un-migrated plural tree — which pass the result straight back in as the
  // card's `mcpPolicyError` prop and as `invalidReason`. Migrating it means
  // changing that prop to a catalog key and resolving it in both callers, so it
  // belongs to the SSH/executor-list PR that owns them. The executor route in
  // THIS PR renders its own local MCP policy card, whose validator does return
  // catalog keys, so no screen migrated here can reach the English strings.
  "app/settings/executor/**/*.{ts,tsx}",
  "components/settings/profile-edit/**/*.{ts,tsx}",
  "components/settings/executor-profile-dialog.tsx",
  "components/settings/executor-profiles-card.tsx",
  // The two hooks the Agents routes reach for install/update and capability
  // probing. Both are `.ts` with no JSX, and both carried copy the lint count
  // could never report: a thrown `Error` and an async `setError` fallback. Each
  // surfaces inside an already-translated wrapper (`unableToStartUpdate`,
  // `failedToRefresh`), so the English payload rendered *inside* accented copy —
  // the shape the pseudo-locale is easiest to skim past. Only the pseudo-locale
  // can prove they stay migrated.
  "hooks/domains/settings/use-agent-runtime-updates.ts",
  "hooks/domains/settings/use-dynamic-models.ts",
  // Settings → Workspace → Workflows: the workflow editor end to end — the
  // route, the list and its reorder/import/export chrome, the workflow card,
  // the pipeline/step editor with its WIP and transition controls, the
  // session-config editor and rule cards, the replay-cycle diagnostic, and the
  // GitHub-sync and export/import dialogs. Copy lives in a new `workflows`
  // namespace; the 27 workflow-editor keys that already sat in `settings` moved
  // there in this PR so the surface is not split across two catalogs.
  //
  // The closure was re-derived from the route's imports rather than from the
  // string counter. `components/workflow-selector-row.tsx` is deliberately
  // ABSENT: the counter attributed it here, but the only path to it from this
  // route runs through the settings save provider into the config-chat →
  // quick-chat → task-create-dialog tree, so it belongs to the task-create
  // surface, not this one. Same for `components/task-create-dialog-*` and
  // `components/task/chat/messages/workflow-step-message-badge.tsx`.
  //
  // `components/integrations/auth-status-banner.tsx` is also deliberately
  // absent. This surface reaches it only for `useTick`, the shared 30s
  // re-render hook, which carries no copy; every literal in that file belongs to
  // the integration status banner, so listing it would pull an un-migrated
  // component into this PR. It is the same call as `sentry-issue-common.tsx`
  // above.
  //
  // The `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects them;
  // the entries record that they are migrated (toast titles, the sync
  // `confirm()`, thrown save/order errors, and the carry-forward warning
  // sentence all reach the user) and only the pseudo-locale can prove it stays
  // that way.
  //
  // Deliberately left in English, because each is PERSISTED rather than
  // rendered: the seeded `New Step` name in `workflow-card-actions.ts` and
  // `workflow-step-mutations.ts`, the `New Workflow` fallback and the
  // `DEFAULT_CUSTOM_STEPS` names (`Todo` / `In Progress` / `Review` / `Done`) in
  // `use-workflow-creation.ts`, and the three `PROMPT_TEMPLATES` bodies in
  // `workflow-pipeline-editor-helpers.tsx` — the template button writes its
  // prompt into `WorkflowStep.prompt`, which is stored and sent to the agent
  // verbatim. Translating any of them would write a localized string into the
  // database. Same reasoning as the Jira/Linear/Sentry default watch prompts.
  //
  // Also English by design: the `kandev_workflow` YAML sample in the import
  // dialog (a wire format), the GitHub repository-link example, git's default
  // branch name `main`, the `{{task_prompt}}` substitution token, and the
  // `step_complete_kandev` MCP tool name — all interpolated as values so the
  // pseudo-locale cannot turn them into dead pointers.
  //
  // Wire values stay wire values, with only their labels as copy resolved at
  // render from a catalog key: the step `color` Tailwind classes
  // (`STEP_COLORS`), the transition action `type`s (`move_to_next` and
  // friends), the capability `key`s in `step-capability-icons.tsx`, the replay
  // diagnostic's `promptSource` / `trigger` / `actionKind`, the sync `state`
  // union, and the `configure_session` rule `operation`s. Workflow and step
  // NAMES are user data throughout and are always interpolated as values,
  // never built into a message by concatenation.
  // `[[]id[]]`, not `[id]`: glob brackets are a character class, so the
  // unescaped form matches nothing and the entry silently guards no files.
  // This one and the two under Workspaces were escaped by #2247, which hit the
  // same trap on its own dynamic route. `pnpm lint` is unchanged by the repair,
  // so these directories were always clean — just unguarded. See docs/i18n.md
  // ("An entry can be born dead").
  "app/settings/workspace/[[]id[]]/workflows/**/*.{ts,tsx}",
  "app/settings/workspace/use-workflow-creation.ts",
  "app/settings/workspace/workspace-workflows-client.tsx",
  "app/settings/workspace/workspace-workflows-dialogs.tsx",
  "components/settings/use-workflow-draft-contributor.ts",
  "components/settings/workflow-card-actions.ts",
  "components/settings/workflow-card-dialogs.tsx",
  "components/settings/workflow-card-header-actions.tsx",
  "components/settings/workflow-card.tsx",
  "components/settings/workflow-cycle-diagnostic.tsx",
  "components/settings/workflow-export-dialog.tsx",
  "components/settings/workflow-pipeline-editor-helpers.tsx",
  "components/settings/workflow-pipeline-editor-panels.tsx",
  "components/settings/workflow-pipeline-editor-step-actions.tsx",
  "components/settings/workflow-pipeline-editor-wip-controls.tsx",
  "components/settings/workflow-pipeline-editor.tsx",
  "components/settings/workflow-section-actions.tsx",
  "components/settings/workflow-session-config-carry-warning.tsx",
  "components/settings/workflow-session-config-editor.tsx",
  "components/settings/workflow-session-config-rule-card.tsx",
  "components/settings/workflow-step-mutations.ts",
  "components/settings/workflow-step-prompt-section.tsx",
  "components/settings/workflow-sync-dialog.tsx",
  "components/settings/workflow-sync-section.tsx",
  "components/settings/workflow-sync-status-banner.tsx",
  "components/settings/workflow-synced-badge.tsx",
  "hooks/domains/settings/use-workflow-sync.ts",
  "lib/workflows/session-config-carry-analysis.ts",
  // Shared with the task board's `components/task/workflow-stepper.tsx`, which
  // is not migrated. The guard is per-file, so that surface is unaffected; the
  // icons render here on every pipeline node, so leaving them would have left
  // plain English inside a card this PR claims is done.
  "components/step-capability-icons.tsx",
  // Settings → System, the remaining eight routes (about, backups, database,
  // feature-toggles, licenses, logs, status, updates) plus Users, which is a
  // ninth System route reachable only when the `auth` feature is on. This
  // completes the group: `components/settings/system/**` is now migrated in
  // full, so the storage-only globs above are the historical record of which
  // PR did which half rather than a boundary that still means anything.
  //
  // **The live titles and descriptions were not in `components/` at all.**
  // `app/settings/system/{about,backups,database,feature-toggles,licenses,
  // status,updates}/page.tsx` are unreferenced Next-era leftovers — nothing
  // imports them; `src/settings-routes.tsx` is the SPA's real route table and
  // `StoragePage` is the only `app/settings/system` page it still pulls in.
  // Those nine titles/descriptions live in `SETTINGS_ROUTES`, a SCREAMING_CASE
  // identifier that `i18next/no-literal-string` skips *entirely* — the guard
  // reported 5 findings in a 652-line file holding copy for every settings
  // route. They now travel as `titleKey`/`descriptionKey` through a
  // `SystemRouteShell` wrapper. The dead pages are migrated too rather than
  // deleted, which keeps this PR copy-only; deleting them is a separate call.
  //
  // `src/settings-routes.tsx` is deliberately NOT on this list. Only its System
  // entries are migrated — Executors, Workflows, Integrations, Workspaces and
  // the account routes still hold English titles there, and the file is one
  // file, so allowlisting it would claim a completeness this PR does not have.
  // Whichever sibling migration lands last should add it.
  //
  // Six entries hold no JSX, so `mode: "jsx-only"` never inspects them; they
  // record that the file is migrated and only the pseudo-locale can prove it
  // stays that way. Three are hooks that were in no lint-derived list at all
  // (`use-kandev-restart`, `use-self-update`, `use-desktop-updater` own seven
  // restart/update failure messages between them); all three import the
  // module-level `t` so each string resolves when its callback fires, keeping
  // `t` out of the callbacks' dependency arrays.
  //
  // Four more shapes the guard structurally cannot see were migrated here, and
  // every one was found by reading rather than by lint:
  //   - `job-progress-indicator.tsx` reported 0 and returned `Queued` /
  //     `Running` / `Done` / `Failed` from `stateLabel()`. It renders on four
  //     cards, one of which (`storage-quarantine-card.tsx`) is #2194's and is
  //     on a route this PR does not otherwise touch.
  //   - `action-button-content.tsx` reported 0: its `Running...` / `Done` /
  //     `Failed` were destructuring defaults, which evaluate before the
  //     component body, so they resolve inside the body now.
  //   - SCREAMING_CASE config tables: `ROWS` in `disk-usage-card.tsx` (7 row
  //     labels), `BASE_ITEMS`/`AUTH_ITEMS` in the sidebar's `system-group.tsx`
  //     (10 nav labels — the guard saw only `label="System"`), and the four
  //     `*_HELP` maintenance blurbs in `database-stats-card.tsx`.
  //   - `aria-label`s, which the pseudo-locale oracle cannot see either:
  //     `What is {label}?`, `Toggle {flag.label}`, `Restart support details`,
  //     `What's monitored`, and the three icon-only backup row actions, which
  //     had no accessible name at all before this PR.
  //
  // `bundle-customizer.tsx` (533 lines) and `log-viewer.tsx` (281) also report
  // 0 and are listed because they are genuinely already migrated — an earlier
  // diagnostics PR did them under `settings:diagnostic*`. Those 47 keys stay in
  // `settings.json` rather than moving to `system`: unlike the twelve
  // `storage*` keys #2194 relocated, they are already keyed, already correct,
  // and moving them would be churn with a rename-typo risk and no user-visible
  // gain.
  //
  // Deliberately left in English, none of it copy:
  //   - The type-to-confirm tokens `RESET` (`factory-reset-dialog.tsx`) and
  //     `RESTORE` (`restore-dialog.tsx`). Both gate their confirm button on
  //     `typed === CONFIRM_TOKEN` *and* are sent to the API (`resetDatabase`,
  //     `restoreBackup`), so translating either would make an irreversible
  //     dialog impossible to satisfy in that locale. Each travels as an
  //     interpolated value into the visible sentence, the placeholder and the
  //     input's aria-label, so shown and compared cannot drift.
  //   - Backend-owned copy, on the same contract as #2193's `PermissionSetting`:
  //     `RuntimeFlagState.label` / `.description` / `.risk_description` are
  //     authored in `runtimeflags/registry.go`; `HealthIssue.title` /
  //     `.message` / `.fix_label`, `HealthCheckSummary.name`, `SystemJob.message`,
  //     `RestartCapability.reason`, and `UpdatesResponse.apply_unsupported_reason`
  //     / `.manual_commands` are all rendered by the API. Localizing them needs
  //     a key/value split in Go, not a frontend change.
  //   - Wire values, each rendered beside its own translated label: the runtime
  //     flag `key` and `env_var` (the persisted registry identity — never
  //     translate either), the user `role` / `status` (the `value` on each
  //     `SelectItem` is the token posted to the API; only the child text is
  //     copy), the snapshot `kind`, the job `state`, the health `severity`, and
  //     the restart/update `phase` unions.
  //   - Identifiers and third-party content in `licenses-list.tsx`: package
  //     `name`, `version`, `ecosystem`, the SPDX `license` id (`MIT`,
  //     `Apache-2.0`) and the full `license_text`. None of it is ours to
  //     translate.
  //   - Build and release values in `about-card.tsx` / `version-summary-card.tsx`
  //     / `updates-card.tsx`: version strings, commit SHA, build time, Go
  //     version, OS and arch. `PostgreSQL` / `SQLite` in `formatDriver` are the
  //     products' own spellings, and `GitHub` is a brand noun.
  //   - Every filesystem path: the data directory, the database `path`, the
  //     backup filenames, and the `<data-dir>/backups/` placeholder — all
  //     interpolated as values so the pseudo-locale cannot turn them into dead
  //     pointers.
  "app/settings/system/**/*.{ts,tsx}",
  "components/app-sidebar/sections/settings/system-group.tsx",
  "components/settings/changelog-list.tsx",
  "components/settings/system/*.{ts,tsx}",
  "hooks/domains/system/use-desktop-updater.ts",
  "hooks/domains/system/use-kandev-restart.ts",
  "hooks/domains/system/use-self-update.ts",
  // The app sidebar, the settings nav tree and the app status bar. These mount
  // on EVERY screen, so their copy is what the pseudo-coverage spec sees wrapped
  // around each settings page it walks — leaving them was why that oracle had
  // never run green.
  //
  // Deliberately file globs, not `components/app-sidebar/**`. #2202 has landed,
  // so `sections/settings/system-group.tsx` is migrated and already listed in
  // the System block above — this directory is now covered in full, just by two
  // entries rather than one.
  //
  // Do NOT tidy that into `components/app-sidebar/**`.
  // `check-guard-allowlist.mjs` flags any entry that disappears while its path
  // still exists, globs included, so swapping these seven for one broader glob
  // reports seven removed entries and fails (verified by simulating it). A
  // redundant broader glob *alongside* them would pass; removing them never
  // does.
  //
  // `general-group.tsx` and `agents-group.tsx` are absent on purpose: earlier
  // migrations already listed them further up, and a second copy here would be
  // dead weight that reads as this PR's coverage.
  //
  // The four `.ts` entries hold no JSX, so `mode: "jsx-only"` never inspects
  // them. They are listed because they were read and carry no copy (section
  // ids, cookie names, drag geometry, a route predicate) — not because the rule
  // can keep them that way.
  "components/app-sidebar/*.{ts,tsx}",
  "components/app-sidebar/sections/*.tsx",
  "components/app-sidebar/sections/settings/account-group.tsx",
  "components/app-sidebar/sections/settings/executors-group.tsx",
  "components/app-sidebar/sections/settings/settings-nav-primitives.tsx",
  "components/app-sidebar/sections/settings/settings-tree.tsx",
  "components/app-sidebar/sections/settings/workspaces-group.tsx",
  "components/app-status-bar/**/*.{ts,tsx}",
  "components/theme-toggle.tsx",
  // The command palette's own copy. `group` doubles as the palette's Map key and
  // its rendered heading, so it must be translated in lockstep across every
  // producer; `components/task/recent-task-switcher-hooks.ts` is the other one
  // and had its `group` converted here, but the rest of that file belongs to the
  // components/task migration and is not listed.
  "components/global-commands.tsx",
  // Settings → Workspace: the workspace list and its inline create form, the
  // workspace edit page (settings card, links card and delete dialog), the
  // repositories page with its discover dialog, and the repository card with the
  // branch-template help, copy-files help, custom-scripts editor and delete
  // dialog it renders. Copy lives in a new `workspaces` namespace.
  //
  // This is a FILE list in `app/settings/workspace/`, not `**`, because the
  // sibling `workspace-workflows-client.tsx` / `workspace-workflows-dialogs.tsx`
  // and `use-workflow-creation.ts` belong to the workflow editor migrated in
  // #2201 and are already listed above, while `[id]/automations/**` is a
  // separate task.
  //
  // `components/settings/unsaved-indicator.tsx` is included: it is reached from
  // nowhere else in the app today, so its "Unsaved changes" badge and its Save
  // button were the only plain English left inside a repository card. Both
  // strings went to `common` rather than `workspaces`, since the component is a
  // generic settings primitive the remaining migrations will reuse.
  //
  // `components/settings/editable-card.tsx`, `settings-card.tsx` and
  // `settings-section.tsx` are all in the closure and all already correct —
  // the first two are pure render-prop/prop-forwarding shells and the third
  // takes title/description as props. `editable-card.tsx` is already listed
  // above from the Editors migration; the other two carry no literals at all.
  //
  // Two `.ts` files in this directory are deliberately ABSENT rather than
  // listed. `workspace-repositories-validation.ts` is a single boolean predicate
  // over the API response (`exists && is_git`) and
  // `workspace-repositories-dirty.ts` is field-comparison logic — neither holds
  // a string of any kind. Their user-facing validation MESSAGES live in
  // `workspace-repositories-client.tsx`'s `useDiscoverDialog`, which is listed
  // and migrated; `mode: "jsx-only"` could not have seen them there either,
  // because they are `setManualValidation` arguments rather than JSX.
  //
  // Deliberately left in English, because each is PERSISTED rather than
  // rendered: the seeded `New Repository` name in `buildDraftRepo`, and the
  // `New Script` name and `echo ""` command the save path writes for a script
  // left blank. Translating any of them would store a localized string in the
  // database and render it later on surfaces this PR does not own — the same
  // call as the `New Step` / `DEFAULT_CUSTOM_STEPS` names in #2201.
  //
  // Also English by design, each a VALUE the user types or a parser consumes,
  // and each interpolated so the pseudo-locale cannot turn it into a dead
  // pointer:
  //   - The six branch-template placeholders `{title}` / `{title_full}` /
  //     `{ticket}` / `{issue_key}` / `{task_id}` / `{suffix}`. `RenderTaskBranchName`
  //     in `apps/backend/internal/worktree/config.go` substitutes each by exact
  //     string match, so a translated token would land in the branch name
  //     verbatim. Only the description beside each one is copy. Same for the
  //     `feature/{ticket}-{title}` example and the default template
  //     `feature/{title}-{suffix}`.
  //   - Every copy-files glob: `.env`, `*`, `?`, `[abc]`, `**`, `**/.env`,
  //     `{a,b}`, `.env{,.local}`, and the `:symlink` / `::symlink` suffixes.
  //     The explanation around them is copy; the patterns are what the backend
  //     matches on.
  //   - `$PORT`, the environment variable a dev script reads.
  //   - Every script placeholder (`#!/bin/bash` plus a command). Shell samples
  //     the executor runs verbatim, the same call as `DEFAULT_DOCKERFILE` in the
  //     executor editor.
  //   - The `/absolute/path/to/repository`, `/path/to/repository` and `my-repo`
  //     placeholders — value shapes, not prose.
  //
  // Repository names, paths, `owner/name` slugs, branch names and workspace
  // names are user data throughout and are never built into a message by
  // concatenation. The workspace delete dialog compares the typed text with the
  // workspace's own name, so there is no translatable type-to-confirm token
  // here to break; the name travels into the sentence as an interpolated value.
  "app/settings/workspace/page.tsx",
  "app/settings/workspace/[[]id[]]/page.tsx",
  "app/settings/workspace/[[]id[]]/repositories/page.tsx",
  "app/settings/workspace/workspace-edit-client.tsx",
  "app/settings/workspace/workspace-not-found-card.tsx",
  "app/settings/workspace/workspace-repositories-client.tsx",
  "app/settings/workspace/workspace-repositories-dialog.tsx",
  "app/settings/workspace/workspaces-page-client.tsx",
  "components/settings/repository-branch-template-help.tsx",
  "components/settings/repository-card.tsx",
  "components/settings/repository-copy-files-help.tsx",
  "components/settings/repository-custom-scripts.tsx",
  "components/settings/repository-delete-dialog.tsx",
  "components/settings/unsaved-indicator.tsx",
  // Settings → External MCP, Prompts, Voice Mode and Utility Agents, plus the
  // two Changelog cards. Four small routes in one entry group because none of
  // them owns enough copy to be worth its own migration, and they share the
  // `settings` namespace: two of the four already had their page title there
  // (`settings:voiceMode`, `settings:utilityAgents`) and the other two in
  // `common` (`common:externalMcp`, `common:prompts`), all of which are reused
  // rather than twinned — `SEGMENT_LABEL_KEYS` in `settings-layout-client.tsx`
  // renders the same words as the breadcrumb on these very routes.
  //
  // `lib/settings/external-mcp-tools.ts` is the load-bearing entry: a `.ts`
  // catalog of 7 groups and 29 tools that `mode: "jsx-only"` never inspects, so
  // its 43 strings were invisible to every previous sweep. Titles and
  // descriptions now travel as `titleKey` / `descriptionKey` and resolve at
  // render; the tool `name`s stay literal because they are the protocol
  // identifiers an external agent calls. Two unit tests pin the keys to the
  // catalog, because nothing else can.
  //
  // Three more shapes here held copy no lint rule could see, all now migrated:
  // `buildEngineOptions` in `voice-mode-settings.tsx` (a plain builder
  // returning label/description object literals), `noteForStatus` in
  // `inference-agent-status.tsx` (a non-JSX switch over the probe status), and
  // the `placeholder = "Select a model"` parameter default in
  // `model-combobox.tsx`, which had to move into the component body — a
  // parameter list is evaluated before `useTranslation()` is in scope.
  //
  // `model-combobox.tsx` and `inference-agent-status.tsx` were listed for the
  // Settings → Agents migration and correctly left there, being absent from
  // that route's import closure. They belong here: the combobox is reached from
  // Utility Agents and from the task-side CLI profile editor
  // (`components/agent/cli-profile-editor.tsx`, not yet migrated — the guard is
  // per-file, so that surface is unaffected), and the status note only from
  // Utility Agents. `mode-combobox.tsx`, `pty-terminal-dialog.tsx` and
  // `changelog-list.tsx` are already listed above and were left alone.
  //
  // Five files here are DEAD TWINS — see "an existing key is not automatically
  // the right key" in docs/i18n.md, which is the shape that silently rewrote
  // English on a shipping System page. None of these had drifted, because none
  // of them holds any copy at all, but the next migration should not have to
  // rediscover which ones are live:
  //
  //   - `app/settings/prompts/page.tsx` and `app/settings/voice-mode/page.tsx`
  //     are unreferenced. `SETTINGS_ROUTES` in `src/settings-routes.tsx` renders
  //     `<PromptsSettings />` and `<VoiceModeSettings />` directly, so the SSR
  //     prefetch in those pages never runs. `external-mcp` and `utility-agents`
  //     go through their page, and are live.
  //   - `app/settings/changelog/page.tsx` is likewise unreferenced; the route
  //     table has its own `SettingsRedirect` to `/settings/system/updates`.
  //   - `changelog-settings.tsx` and `changelog-notification-card.tsx` are
  //     migrated but unreachable: nothing imports `ChangelogSettings`, and it is
  //     the only importer of `ChangelogNotificationCard`. The pseudo-locale
  //     therefore cannot verify either from any route — they were read by eye.
  //
  // That makes the whole changelog surface — the release-history list and the
  // topbar-notification card — currently unreachable, which is worth stating
  // plainly because it is easy to get wrong. `/settings/system/updates` renders
  // `renderUpdatesRoute` from the route table, which mounts `UpdatesCard` and
  // nothing else; it has never mounted `ChangelogList`. The
  // `app/settings/system/updates/page.tsx` that did was itself a dead twin, and
  // #2219 deleted it along with the other eight. So `changelog-list.tsx`, which
  // #2202 allowlisted as part of that route, has no live consumer either. It is
  // left listed here — an allowlist entry on unreachable code costs nothing and
  // removing one is the thing this file must never do.
  //
  // Deleting the five is a correctness change, not tidiness, and belongs in its
  // own PR rather than inside a copy-only migration.
  //
  // `use-inference-agents.ts` and `utility-dirty.ts` hold no JSX and no copy;
  // the entries record that the review happened. `external-mcp-snippets.ts` is
  // absent rather than noted: it is JSON/TOML config and CLI commands end to
  // end, with no prose at all.
  //
  // Deliberately left in English, none of it copy:
  //   - Every MCP tool `name` (`create_task_kandev`, …), the `parent_id`
  //     request field, and the `open, in_progress, complete, blocked,
  //     cancelled` task-state enum. The last two are interpolated as values so
  //     the pseudo-locale cannot turn them into dead pointers.
  //   - The agent product names and config paths in `SNIPPET_CARDS`
  //     (`Claude Code`, `~/.claude.json`, `~/.codex/config.toml`, …) and the
  //     whole of every generated snippet. All interpolated as values.
  //   - The `{{` prompt-template sigil and the `@name` chat mention token —
  //     both typed verbatim by the user, both interpolated.
  //   - Wire values: the voice `engine` / `mode` / `whisper_web_model` unions
  //     and the BCP-47 `language` tags, the `InferenceAgentStatus` probe
  //     states, the `USE_DEFAULT` sentinel, and every agent/model id. Only
  //     their labels are copy, and those travel as catalog keys.
  //   - `~40 MB` / `~75 MB` / `~240 MB` in `WHISPER_MODELS`: download sizes in
  //     binary units, not prose — the same call `storage-units.ts` made.
  //   - Utility agent `name`, `description` and `prompt`, and custom prompt
  //     `name` / `content`. The builtins' text is authored by the backend and
  //     the rest is user data; a prompt body is also sent to the agent verbatim.
  //   - `new Error("Voice settings require VoiceDraftProvider")`, a programming
  //     error that can only fire in a broken render tree.
  //
  // Every key added here was audited against all 15 en catalogs for English that
  // already exists under another key. Twelve matched. Ten are the established
  // per-namespace pattern the repo already runs on — `Refresh`, `Add`,
  // `Actions`, `Copied`, `Copy to clipboard`, `Tasks`, `Model`, `No default`,
  // `Not configured` each already exist in two to eight namespaces, because a
  // namespace is the unit a translator works in. Two needed a decision, both
  // because they appear in `OWNED_BY_ANOTHER_NAMESPACE` in
  // `settings-nav-copy.test.ts` and both because the nav renders the same word
  // on the same screen:
  //
  //   - `externalMcpGroupAgents` = "Agents" vs `common:agents`
  //   - `externalMcpGroupExecutors` = "Executors" vs `common:executors`
  //
  // These are kept separate, and that guard does not fire on them because it
  // scans `sidebar.json` only. The rule it encodes is that a nav label and the
  // page title of the destination it links to must share one key — same words,
  // same destination. These are not that: they are headings for MCP tool
  // categories mirroring the backend's grouping in
  // `apps/backend/internal/mcp/server`, not the settings routes. Reusing
  // `common:agents` would couple the tool catalog to the nav, so renaming the
  // Agents settings route would silently retitle a group of MCP tools.
  //
  // `promptDeleteDescription` also duplicates `thisWillPermanentlyRemoveSecret`
  // inside this namespace, sentence and tag index alike. Kept separate because
  // that key is named for the secrets dialog it belongs to; folding the prompts
  // dialog into it would leave a key whose name contradicts half its callers.
  //
  // Still English and NOT ours: `CONFIGURABLE_SHORTCUTS.VOICE_INPUT_TOGGLE.label`
  // ("Voice Input") from `lib/keyboard/shortcut-overrides.ts`, interpolated into
  // the migrated `settings:voiceShortcutTitle` frame. It is the shared keyboard
  // registry, owned by whoever migrates `lib/keyboard`; under the pseudo-locale
  // it reads as an English word inside accented copy, which is the oracle's
  // known weak spot rather than a miss here.
  "app/settings/changelog/**/*.{ts,tsx}",
  "app/settings/external-mcp/**/*.{ts,tsx}",
  "app/settings/prompts/**/*.{ts,tsx}",
  "app/settings/utility-agents/**/*.{ts,tsx}",
  "app/settings/voice-mode/**/*.{ts,tsx}",
  "components/settings/changelog-notification-card.tsx",
  "components/settings/changelog-settings.tsx",
  "components/settings/config-chat-agent-section.tsx",
  "components/settings/external-mcp-settings.tsx",
  "components/settings/inference-agent-status.tsx",
  "components/settings/model-combobox.tsx",
  "components/settings/prompts-settings.tsx",
  "components/settings/use-inference-agents.ts",
  "components/settings/utility-agent-dialog.tsx",
  "components/settings/utility-agents-section.tsx",
  "components/settings/utility-dirty.ts",
  "components/settings/utility-sections.tsx",
  "components/settings/voice-mode-settings.tsx",
  "lib/settings/external-mcp-tools.ts",
  // Configuration Chat. `ConfigChatProvider` mounts the FAB on EVERY /settings
  // route, so its `aria-label="Configuration Chat"` was the last text finding
  // the pseudo-coverage oracle reported across the whole migrated surface.
  //
  // `use-config-chat.ts` holds no JSX, so `mode: "jsx-only"` never inspects it;
  // the entry records that it was read by eye. The count from
  // `pnpm run lint:i18n` reported it as 0 while it owned two strings that reach
  // the user — the `setError` for a missing agent profile, and the fallback for
  // a throw with no `message`. It imports the module-level `t` rather than the
  // hook so each resolves when `startSession` runs, keeping `t` out of the
  // callback's dependency array.
  //
  // `SUGGESTION_PROMPT_KEYS` in `config-chat-setup.tsx` was `SUGGESTION_PROMPTS`,
  // four sentences the guard skipped entirely because the identifier is
  // SCREAMING_CASE. They now travel as catalog keys and resolve at render.
  //
  // Copy lives in a new `configChat` namespace, except the feature NAME
  // ("Configuration Chat"), which the command palette also renders and which is
  // therefore `common:configurationChat` — `common:commandConfigurationChat`
  // held the byte-identical string and was folded into it rather than left as a
  // twin free to drift.
  //
  // Deliberately left in English: the `"Config Chat"` session-name fallback in
  // `use-config-chat.ts`. `persistQuickChatRename` writes it to the task title,
  // so translating it would persist a locale-dependent value — the same call as
  // the built-in layout profile names and the seeded workflow step names.
  //
  // NOT listed, and not migrated here:
  //   - `components/settings/config-chat-agent-section.tsx`, the Configuration
  //     Chat card on Settings → Utility Agents. #2218 migrates it with the page
  //     that owns it, into `settings:configChatAgent*`.
  //   - `components/quick-chat/**`. `quick-chat-modal.tsx` renders
  //     `ConfigChatSetup` in its `presentation="dialog"` form and calls
  //     `useConfigChat`, so it inherits everything migrated here, but its own
  //     chrome — and `configuration-chat-toggle.tsx`, which `ConfigChatSetup`
  //     renders — is still English and belongs to the quick-chat migration.
  "components/config-chat/*.{ts,tsx}",

  // Automations — `components/automations/**` (incl. `trigger-configs/`) and
  // the three `app/settings/**/automations/**` routes. Copy lives in a new
  // `automations` namespace, except three strings reused verbatim from
  // `common`: `common:automations` (the word SEGMENT_LABEL_KEYS already names
  // as this settings route's owner), `common:cancel`, `common:status` and
  // `common:requestFailed` — each diffed byte-for-byte against what the live
  // surface rendered before reusing it. Inside the namespace the reverse call
  // was made for "Webhook": the badge, the card summary and the picker's group
  // heading get one key each rather than sharing one, because they are three
  // different grammatical contexts and a language that inflects them
  // differently would otherwise have nowhere to say so.
  //
  // Most of this directory's copy was invisible to the guard. `pnpm run
  // lint:i18n` reported 116; the migration converted 156. The extra 40 were in
  // shapes the rule does not inspect: SCREAMING_CASE config tables
  // (`TRIGGER_LABELS`, `STATUS_BADGE`, `EXECUTION_MODE_ITEMS`, `PRESETS` in two
  // files, `CRON_PRESETS`/`SIMPLE_SUMMARIES`/`TRIGGER_INFO`, `CI_CONCLUSIONS`,
  // `PR_EVENTS`, `CATEGORY_META`), plain functions returning copy
  // (`getTriggerSummary`, `getWorkflowStepHelpText`, `inputValueFor`), toasts
  // and a save-contributor `invalidReason` in `automation-editor.tsx`, an
  // `sr-only` "required" the guard's single-lowercase-token pattern skipped,
  // and `automation-repository-selection.ts` — a `.ts` module with no JSX that
  // reported 0 and held a picker option label.
  //
  // Persisted/protocol strings deliberately left in English, because a
  // translated one breaks a stored automation with nothing failing until a
  // locale switch: `DEFAULT_PROMPT` in `automation-editor.tsx` (persisted, sent
  // to the agent verbatim, and compared with `===`), the `"New Automation"` and
  // `"New Repository"` name fallbacks (both written to a record), every cron
  // expression and shorthand, every TriggerType / RunStatus / ExecutionMode /
  // PR-event / CI-conclusion id, `X-Webhook-Secret`, the `{{webhook.*}}`
  // placeholder paths, and the example values in `placeholder` attributes
  // (branch names, CI check names, GitHub usernames, a sample JSON body).
  // `run.trigger_type` renders raw in the runs table for the same reason.
  // Where syntax appears inside a sentence it travels as an interpolation
  // value, so the pseudo-locale cannot accent it into something that no longer
  // parses.
  //
  // NOT listed, and deliberately so:
  //   - `components/task-create-dialog-multi-repo-guard.ts`. Its three disabled
  //     -executor explanations render as a `title` in this editor's Executor
  //     Profile picker, so they are migrated here (into `common:multiRepo*`),
  //     but the module is shared with the un-migrated task-create dialog and
  //     belongs to neither. Allowlisting it would claim a completeness that
  //     dialog has not had; whichever migration takes the dialog adds it.
  //   - `formatRelativeTime` in `lib/utils.ts`, which the runs table and the
  //     automations table both render. It is a repo-wide formatter with 21
  //     consumers and `lib/i18n/formats.ts` already holds its intended
  //     replacement (`formatRelative`); swapping it is a cross-cutting change,
  //     not part of a copy migration.
  //
  // NOTE the `[[]id[]]` escaping on the dynamic route. Written as `[id]`, the
  // brackets are a glob CHARACTER CLASS matching a single `i` or `d`, so the
  // pattern matches nothing and the route is silently unguarded — verified by
  // putting a hardcoded literal in `automations/new/page.tsx` and watching
  // `pnpm lint` report 0 errors. `check-guard-allowlist.mjs` cannot catch this:
  // it only inspects entries that LEFT the array, so an entry that never
  // matched anything looks identical to a healthy one. `fs.globSync` on the
  // escaped form is what proves an entry is live.
  "components/automations/*.{ts,tsx}",
  "components/automations/trigger-configs/*.tsx",
  "app/settings/automations/page.tsx",
  "app/settings/workspace/[[]id[]]/automations/**/*.tsx",
  "hooks/domains/settings/use-automation-runs.ts",
];
