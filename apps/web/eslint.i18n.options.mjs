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
 * directory is partially migrated, and collapse to `dir/**` once it is done.
 * The list only shrinks by mistake; adding to it is the whole point.
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
  "app/settings/workspace/[id]/workflows/**/*.{ts,tsx}",
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
];
