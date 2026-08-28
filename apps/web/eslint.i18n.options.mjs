/* eslint-disable max-lines -- `i18nGuardFiles` below is a 638-entry data list,
   not logic. Splitting it into another module would break
   `check-guard-allowlist.mjs`, which imports the BASE revision of this single
   file from a temp directory to diff the array; a relative import of a sibling
   would not resolve there. */
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
      //
      // The outer `(?:…)` is load-bearing. `^` and `$` bind tighter than `|`, so
      // the ungrouped form compiled to `(^A)|(B$)` — a first branch with no end
      // anchor, which matched ANY string starting with a lowercase letter or a
      // digit. That swallowed every `<Trans>` split fragment that happens to
      // begin lowercase ("open pull requests assigned to you") in silence, for
      // the whole repo, for as long as the pattern has existed. See the compiled
      // -pattern test in `eslint.i18n.options.test.ts`, which fails without the
      // group.
      "(?:[a-z0-9]+(?:[-_][a-z0-9]*)*|[-_][a-z0-9]+(?:[-_][a-z0-9]*)*)",
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
      // Whitespace-separated token lists: Tailwind class lists that reach the
      // guard as an object property (`{ className: "h-4 w-4" }`) rather than as
      // a `className` JSX attribute, plus `owner/repo` and `ns:key` values.
      //
      // At least ONE token must carry a `-`, `:` or `/` (the middle branch).
      // Without that requirement the pattern was "any run of lowercase words
      // separated by spaces", which is a description of TYPOGRAPHY, not of
      // syntax — and so it silently excluded every lowercase English sentence
      // that happens to carry no punctuation ("open pull requests assigned to
      // you"). Separators are what make a token a class name or a path rather
      // than a word; English copy that needs one is rare, English copy that
      // needs none is most of it.
      "(?:-?[a-z0-9]+(?:[:/-][a-z0-9.]+)*\\s+)*-?[a-z0-9]+(?:[:/-][a-z0-9.]+)+(?:\\s+-?[a-z0-9]+(?:[:/-][a-z0-9.]+)*)*",
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
      // `PluginErrorBoundary`'s console-log identifier (`slot "task-sidebar"`,
      // `route "/plugins/hello"`). It is written to console.error in
      // componentDidCatch and never reaches the DOM, so it is a developer
      // diagnostic rather than copy. The only literal `context=` values in the
      // tree are that boundary's four call sites.
      "context",
      // Identifiers and prefixes the caller composes into ids/testids.
      "id",
      "k",
      // A prop whose NAME ends in `Key` carries a catalog key, not copy:
      // `titleKey`, `descriptionKey`, `labelKey`, `i18nKey`. Migrating a surface
      // that resolves its copy at render (`SystemRouteShell`, the preset-icon
      // catalogs) produces exactly this shape, and the value is by construction
      // `namespace:someKey` — already-migrated copy, flagged only for being a
      // string. This keys off the prop NAME, a syntactic category, not off what
      // the value happens to look like.
      ".*[Kk]ey$",
      // Option/badge values are data the app compares and submits.
      "value",
      "cmd",
      ".*[Ii]dPrefix$",
      ".*[Ii]dSuffix$",
      // Matches the bare `saveId` prop as well as composed `fooSaveId` names.
      // The former `.*SaveId$` required at least one leading character, so it
      // never matched the prop the codebase actually uses (`saveId`).
      ".*[Ss]aveId$",
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
  // Dev-server preview control (added with the feature, per the same-PR rule).
  "components/task/dev-server-preview-button.tsx",
  // Task dependencies (added with the feature, per the same-PR rule).
  "components/task/task-dependency-chip.tsx",
  "components/task-create-dialog-dependencies.tsx",
  // The i18n runtime itself.
  "lib/i18n/**/*.{ts,tsx}",
  // Settings → Preferences — Appearance, Notifications, Terminal & Editors and
  // friends render straight from components/settings via the SPA route table.
  "components/settings/app-status-bar-settings-card.tsx",
  "components/settings/general-settings.tsx",
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
  // Settings → Preferences → Terminal & Editors: the merged page, its
  // state/section components, the custom-editor form, and the editable-card
  // shell that form renders inside. `editable-card.tsx` is shared with
  // repository-card.tsx, which is not migrated — the guard is per-file, so
  // that stays unaffected.
  "components/settings/editable-card.tsx",
  "components/settings/editor-form.tsx",
  "components/settings/editors-settings-state.tsx",
  "components/settings/editors-settings.tsx",
  "components/settings/lsp-status-location-setting.tsx",
  "components/settings/lsp-language-options.ts",
  // Sprites.dev config, now a section on the Executors page.
  "components/settings/sprites-settings.tsx",
  // Settings → Preferences → Layouts: the whole layouts component directory.
  // `layout-editor-actions.ts` and `use-layout-settings.ts` hold no
  // JSX, so `mode: "jsx-only"` never inspects them — the entries record that
  // they are migrated, but only the pseudo-locale can prove it stays that way.
  "components/settings/layouts/**/*.{ts,tsx}",
  // Settings → Preferences → Appearance, Keyboard Shortcuts and Task behavior.
  // The pages render from general-settings.tsx (already listed above); these
  // entries add the merged pages plus the per-setting cards those pages own.
  // Shortcut *names* still come from `lib/keyboard/shortcut-overrides.ts`,
  // which is deliberately not migrated here.
  "components/settings/task-behavior-settings.tsx",
  "components/settings/terminal-editors-settings.tsx",
  "components/settings/anchored-prompt-bar-settings.tsx",
  "components/settings/archive-confirmation-settings.tsx",
  "components/settings/keyboard-shortcuts-card.tsx",
  "components/settings/mcp-task-agent-profile-default-settings.tsx",
  "components/settings/settings-menu-mode-card.tsx",
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
  "app/settings/agents/**/*.{ts,tsx}",
  "components/settings/agents/**/*.{ts,tsx}",
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
  "components/settings/mcp-strategy-select.tsx",
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
  "components/settings/changelog-list.tsx",
  "components/settings/system/*.{ts,tsx}",
  "components/settings/system/**/*.{ts,tsx}",
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
  "components/app-sidebar/sections/settings/settings-nav-primitives.tsx",
  "components/app-sidebar/sections/settings/settings-tree.tsx",
  // The optional tree modes the menu grows under Workspaces, Agents and
  // Executors. `settings-menu-node.tsx` is the only one with JSX; the three
  // `.ts` entries hold branch data, expansion state and the store read, and
  // carry catalog *keys* rather than copy — `mode: "jsx-only"` never inspects
  // them, so the entries record that they were read, not that lint guards them.
  "components/app-sidebar/sections/settings/settings-menu-branches.ts",
  "components/app-sidebar/sections/settings/settings-menu-node.tsx",
  "components/app-sidebar/sections/settings/use-settings-menu-branches.ts",
  "components/app-sidebar/sections/settings/use-settings-menu-expansion.ts",
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
  "app/settings/workspace/workspace-edit-client.tsx",
  "app/settings/workspace/workspace-not-found-card.tsx",
  "app/settings/workspace/workspace-repositories-client.tsx",
  "app/settings/workspace/workspace-repositories-dialog.tsx",
  "app/settings/workspace/workspaces-page-client.tsx",
  "components/settings/workspaces/**/*.{ts,tsx}",
  "components/settings/repository-branch-template-help.tsx",
  "components/settings/repository-card.tsx",
  "components/settings/repository-copy-files-help.tsx",
  "components/settings/repository-custom-scripts.tsx",
  "components/settings/repository-delete-dialog.tsx",
  "components/settings/unsaved-indicator.tsx",
  // Settings → External MCP, Prompts and Utility Agents, plus the two
  // Changelog cards. Small routes in one entry group because none of them owns
  // enough copy to be worth its own migration, and they share the `settings`
  // namespace: one already had its page title there
  // (`settings:utilityAgents`) and the others in
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
  // Two more shapes here held copy no lint rule could see, both now migrated:
  // `noteForStatus` in
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
  //   - `app/settings/prompts/page.tsx`
  //     are unreferenced. `SETTINGS_ROUTES` in `src/settings-routes.tsx` renders
  //     `<PromptsSettings />` directly, so the SSR prefetch in that page never
  //     runs. `external-mcp` and `utility-agents`
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
  //   - Wire values: the `InferenceAgentStatus` probe states, the
  //     `USE_DEFAULT` sentinel, and every agent/model id. Only their labels
  //     are copy, and those travel as catalog keys.
  //   - Utility agent `name`, `description` and `prompt`, and custom prompt
  //     `name` / `content`. The builtins' text is authored by the backend and
  //     the rest is user data; a prompt body is also sent to the agent verbatim.
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
  // Shortcut labels in `CONFIGURABLE_SHORTCUTS`
  // (`lib/keyboard/shortcut-overrides.ts`) are still English and NOT ours:
  // that is the shared keyboard registry, owned by whoever migrates
  // `lib/keyboard`. Under the pseudo-locale they read as English words inside
  // accented copy, which is the oracle's known weak spot rather than a miss
  // here.
  "app/settings/external-mcp/**/*.{ts,tsx}",
  "app/settings/prompts/**/*.{ts,tsx}",
  "app/settings/utility-agents/**/*.{ts,tsx}",
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
  //
  // `components/quick-chat/**` was the other entry here until #2300 migrated it
  // — `quick-chat-modal.tsx` renders `ConfigChatSetup` in its
  // `presentation="dialog"` form and calls `useConfigChat`, so it inherited
  // everything migrated here, and its own chrome (plus
  // `configuration-chat-toggle.tsx`, which `ConfigChatSetup` renders) now lands
  // on the same `chat:` namespace. It is listed at the end of this file.
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
  "app/settings/workspace/[[]id[]]/automations/**/*.tsx",
  "hooks/domains/settings/use-automation-runs.ts",
  // Settings → Plugins, Settings → Account, and the plural
  // `app/settings/executors/` tree (the executor list, the typed create
  // routes, and the SSH connection pages). Copy lives in two new namespaces,
  // `plugins` and `account`, plus additions to `executors`.
  //
  // `components/plugins/**` is reached from OUTSIDE Settings — the app
  // sidebar, the mobile menu sheet, the chat top bar, the chat input, the task
  // sidebar and the kanban top bar all render `PluginSlot`, and
  // `PluginModalHost` mounts at the app root. It is listed here because this
  // migration owns the Plugins feature end to end, not because those routes
  // are migrated; only the two strings those files own (`PluginRouteFallback`
  // and the mobile nav heading) went through `t()`. Everything else they
  // render — a slot component, a modal title, a nav item's label — is
  // plugin-authored and is third-party data, not our copy.
  //
  // Nine entries hold no JSX, or none that carries copy, so `mode: "jsx-only"`
  // never inspects them and the string count reported them as ZERO while they
  // owned copy that reaches the user. Each records that it was read by eye,
  // and only the pseudo-locale can prove it stays that way:
  //   - `app/settings/executors/new/[type]/executor-types.ts` — reported 0 and
  //     carried ELEVEN strings, the six type labels and their descriptions.
  //   - `lib/executor-icons.ts` and `components/settings/executor-description.ts`
  //     — plain functions returning copy, the shape one step out from a config
  //     table and just as invisible.
  //   - `components/settings/plugins/use-plugin-actions.ts` (eight toasts),
  //     `use-plugin-config-form.ts` (five toasts plus the required-fields
  //     reason), `lib/plugins/sync-summary.ts` (the sync toast), and the three
  //     `hooks/domains/plugins/*.ts` error fallbacks. All import the
  //     module-level `t` rather than the hook, so each resolves when its
  //     callback fires and stays out of the dependency array.
  //   - `app/settings/executors/new/[type]/ssh-config.ts` carries no copy at
  //     all (snake_case config mapping); it is listed to pin that.
  //
  // This PR also closes the residual #2218 recorded against it:
  // `validateMcpPolicy` in `profile-edit/mcp-policy-card.tsx` returned English
  // ("Invalid JSON", "MCP policy must be a JSON object") because its only
  // callers were the un-migrated plural tree. It now returns catalog keys and
  // the card's prop is `mcpPolicyErrorKey`, matching the local validator in
  // `app/settings/executor/[id]/page.tsx`.
  //
  // `src/settings-routes.tsx` is listed for the first time. docs/i18n.md's
  // "Copy that belongs to no directory" rule says the LAST area to migrate
  // adds it, because allowlisting it earlier claims a completeness nobody has:
  // the two Account routes were the final inline English entries in
  // `SETTINGS_ROUTES`, so every entry in that table now resolves from a
  // catalog. Read the entry as a claim about that FILE, not about all of
  // Settings — `app/settings/automations/page.tsx`,
  // `app/settings/integrations/page.tsx` and `components/settings/agent-card.tsx`
  // still hold English and belong to their own migrations. They render no
  // route-table copy, which is why they do not block this entry.
  //
  // THREE surfaces describe the same six executor types and had already
  // drifted from one another: the hub cards use the imperative mood ("Run
  // agents directly…"), while the create and edit headers use the third person
  // ("Runs agents directly…") — and the edit header's SSH sentence differs
  // from the create header's again. Sentences that are byte-identical share a
  // key (the create page reuses `executors:description*`, which
  // `/settings/executor/:id` already owned); the ones that differ keep their
  // own. Folding them together would have rewritten shipping English on two of
  // the three with every gate green.
  // `components/settings/executor-copy.test.ts` pins all three tables
  // byte-for-byte, and `account/account-route-copy.test.ts` does the same for
  // the two route headers. De-duplicating the tables is a behaviour change and
  // belongs in its own PR.
  //
  // Deliberately left in English, none of it copy:
  //   - The executor type keys `local` / `worktree` / `local_docker` /
  //     `remote_docker` / `sprites` / `ssh`. They are the persisted enum, the
  //     create route's path segment, and a `===` comparison in four places.
  //     The `exec-*` executorIds are backend row ids. Only the labels beside
  //     them are copy.
  //   - The brand and protocol names `Docker`, `Sprites.dev` and `SSH` when
  //     they ARE the whole label. They read the same in every locale (the
  //     guard's own `words.exclude` lists all three), and keeping them out of
  //     the catalog stops the pseudo-locale transliterating a name the user
  //     must match against their SSH config or Sprites dashboard.
  //   - Every SSH fingerprint and known-hosts value. `ssh-fingerprint-trust-block`
  //     is a security surface: the user compares those strings
  //     character-for-character against what their own host reports, so both
  //     the observed and the pinned fingerprint are interpolated as values.
  //   - The SSH example values `prod`, `dev.example.com`, `22`, `ubuntu`,
  //     `~/.ssh/id_ed25519` and `bastion.example.com`, and the
  //     `ssh-agent (SSH_AUTH_SOCK)` option — a program name plus the
  //     environment variable it reads. `My VPS` beside them IS copy and does
  //     go through `t()`. `~/.ssh/config`, `$PATH`, `.tar.gz` and `index.json`
  //     are interpolated into their sentences for the same reason.
  //   - The throw in `ssh-connection-card`'s `handleSave`. It is unreachable
  //     while `canSave` gates the button and it signals the settings save
  //     coordinator, not the user — the same shape docs/i18n.md records for
  //     the migrated GitHub and Jira settings.
  //   - Plugin ids, versions, manifest keys (`config_schema`), permission
  //     scopes (`events:*`, `read:*`, `write:*`), webhook keys, marketplace
  //     source URLs, and the raw manifest JSON dump. Those are the contract.
  //     A plugin's display name, description, categories, icon, config-field
  //     labels and modal titles come from its manifest or an untrusted
  //     index.json, so they are third-party data — interpolated as values,
  //     never written into a message.
  //   - Account data: emails, usernames, user agents, IP addresses, and every
  //     API token VALUE, which never passes through `t()` at all. Token names
  //     are user data; only the column headers and role labels are copy.
  //   - `PluginErrorBoundary`'s `context` prop, now in `jsx-attributes.exclude`
  //     above — it is a console.error identifier that never reaches the DOM.
  //
  // `getExecutorLabel` returns catalog text now, so a caller that does not
  // subscribe to i18n renders the previous locale until something else makes it
  // re-render. All three consumers are covered: `ProfileCard` subscribes,
  // `use-filter-value-options.ts` (the task sidebar's executor filter) carries
  // `i18n.language` in its memo deps, and both `applyView` consumers — the
  // desktop sidebar and the mobile task-switcher sheet — do the same for
  // `applyGroup`'s executorType heading in `lib/sidebar/apply-view.ts`.
  //
  // This bug class is invisible to BOTH the guard and the pseudo-locale: the
  // text is translated, just frozen at the previous locale, so it renders
  // accented and reads as done. `check-module-scope-t.mjs` does not fire
  // either — the `t()` call sits inside a function, which is correct, since it
  // does resolve at call time. Only a locale-switch test covers it, which is
  // what `use-filter-value-options-locale.test.tsx` is for.
  //
  // `UNASSIGNED_LABEL` and `MULTI_REPO_LABEL` in `lib/sidebar/apply-view.ts`
  // were the English remainder noted here; they are now
  // `sidebar:groupUnassigned` / `sidebar:groupMultiRepo`, resolved at call time
  // alongside the `__all__` heading. `lib/sidebar` is still NOT on this list —
  // the rest of the directory was not swept, so whoever migrates it adds the
  // entry.
  //
  // One deliberate English change, the only one in this PR: the SSH
  // running-sessions confirm read "This executor has 3 running session(s)."
  // The `(s)` is the inline-plural shape docs/i18n.md rejects — the plural rule
  // cannot be expressed at the call site in a language with three or six forms
  // — so it is now `_one`/`_other` and reads "1 running session" /
  // "3 running sessions".
  //
  // Four dates moved off `toLocaleString()` onto `lib/i18n/formats`'
  // `formatDateTime`, and the marketplace star count off `toLocaleString()`
  // onto `formatNumber`, so they follow the active locale rather than the
  // browser's — the same call `storage-quarantine.ts` made.
  "app/settings/executors/**/*.{ts,tsx}",
  "components/settings/account/**/*.{ts,tsx}",
  "components/settings/executor-description.ts",
  "components/settings/plugins/**/*.{ts,tsx}",
  "components/settings/ssh-agent-readiness-card.tsx",
  "components/settings/ssh-connection-card.tsx",
  "components/settings/ssh-connection-form.tsx",
  "components/settings/ssh-fingerprint-trust-block.tsx",
  "components/settings/ssh-sessions-card.tsx",
  "components/settings/ssh-settings.tsx",
  "components/plugins/**/*.{ts,tsx}",
  "hooks/domains/plugins/*.ts",
  "lib/executor-icons.ts",
  "lib/plugins/sync-summary.ts",
  "src/settings-routes.tsx",
  // GitHub + GitLab TASK surfaces: the PR/MR detail panels and their sections,
  // the topbar buttons and CI popovers, the My-GitHub / My-GitLab browse lists
  // and their preset sidebars and dialogs, and the two `/github` + `/gitlab`
  // page clients. Together with the settings entries listed far above, this
  // completes both directories; the file lists are kept rather than collapsed to
  // `components/github/**` because collapsing would mean REMOVING entries, which
  // `check-guard-allowlist.mjs` cannot distinguish from un-protecting a path.
  //
  // `hooks/domains/gitlab/use-mr-actions.ts` is listed because the task surface
  // reaches its toasts. Its `run()` first argument used to be the human label —
  // one string serving as both `pendingAction` state and the interpolated
  // failure title `${label} failed`. That is the dual-use shape AGENTS.md warns
  // about, so it is now an `MRActionKind` id plus a per-action catalog key.
  // `mr-reviewer-control.tsx` had the same shape in a prop typed
  // `"Reviewers" | "Assignees"`, which TypeScript caught the moment the codemod
  // translated it; it is now a `kind` discriminant.
  //
  // Deliberately left in English, all of it protocol or provider data: GitHub
  // and GitLab state values (`open`/`merged`/`closed`/`opened`, review states,
  // check conclusions) which are compared with `===`; every id, branch name,
  // repo/project path, label and username from the provider API; and the
  // `PR_FEEDBACK_PLACEHOLDER` token, which the agent prompt matches verbatim and
  // which therefore travels as an interpolation value rather than as catalog
  // text. The example values in `placeholder` attributes (`kdlbs, example-org`,
  // `group/api, group/web`, `state=opened`) are data a user types, not copy.
  "app/github/github-page-client.tsx",
  "app/gitlab/gitlab-page-client.tsx",
  "components/github/issue-task-icon.tsx",
  "components/github/multi-pr-ci-popover.tsx",
  "components/github/my-github/issue-list.tsx",
  "components/github/my-github/list-toolbar.tsx",
  "components/github/my-github/pr-list.tsx",
  "components/github/my-github/pr-row-task-indicator.tsx",
  "components/github/my-github/pr-status-badges.tsx",
  "components/github/my-github/presets-sidebar.tsx",
  "components/github/my-github/repo-filter-combobox.tsx",
  "components/github/my-github/save-preset-dialog.tsx",
  "components/github/my-github/task-row-indicator.tsx",
  "components/github/my-github/use-pr-statuses.ts",
  "components/github/pr-checks-section.tsx",
  "components/github/pr-ci-automation-controls.tsx",
  "components/github/pr-ci-automation-prompt-dialog.tsx",
  "components/github/pr-ci-automation-rows.tsx",
  "components/github/pr-ci-popover.tsx",
  "components/github/pr-comments-section.tsx",
  "components/github/pr-detail-panel.tsx",
  "components/github/pr-merge-button.tsx",
  "components/github/pr-mergeability-notice.tsx",
  "components/github/pr-mergeability-row.tsx",
  "components/github/pr-reviews-section.tsx",
  "components/github/pr-shared.tsx",
  "components/github/pr-status-chip.tsx",
  "components/github/pr-topbar-button.tsx",
  "components/gitlab/mr-auto-fix-prompt-dialog.tsx",
  "components/gitlab/mr-automation-controls.tsx",
  "components/gitlab/mr-automation-rows.tsx",
  "components/gitlab/mr-commits-section.tsx",
  "components/gitlab/mr-task-status-summary.tsx",
  "components/gitlab/mr-detail-panel.tsx",
  "components/gitlab/mr-discussions-section.tsx",
  "components/gitlab/mr-files-section.tsx",
  "components/gitlab/mr-overview-section.tsx",
  "components/gitlab/mr-reviewer-control.tsx",
  "components/gitlab/mr-status-chip.tsx",
  "components/gitlab/mr-status-chip-drawer.tsx",
  "components/gitlab/mr-status-chip-popover.tsx",
  "components/gitlab/mr-status-chip-selection.ts",
  "components/gitlab/mr-status-chip-trigger.tsx",
  "components/gitlab/mr-topbar-button.tsx",
  "components/gitlab/my-gitlab/issue-list.tsx",
  "components/gitlab/my-gitlab/list-toolbar.tsx",
  "components/gitlab/my-gitlab/mr-list.tsx",
  "components/gitlab/my-gitlab/mr-row-task-indicator.tsx",
  "components/gitlab/my-gitlab/presets-sidebar.tsx",
  "components/gitlab/my-gitlab/save-preset-dialog.tsx",
  "components/gitlab/my-gitlab/start-task-menu.tsx",
  "components/gitlab/subscription-toggle.tsx",
  "components/gitlab/task-mr-link-dialog.tsx",
  "hooks/domains/gitlab/use-mr-actions.ts",
  // The shared scope bar the `/github` and `/gitlab` dashboards render through
  // their thin wrappers. Listed because this migration converts it COMPLETELY —
  // all four of its own strings — not because integration chrome belongs to this
  // PR. `azure-devops-scope-bar.tsx` also wraps it and is untouched; it passes
  // its own labels in and inherits the base already done. Copy lives in a new
  // `integrations` namespace, the home `NAMESPACE_RULES` in
  // externalize-strings.mjs already designates for `components/integrations/`.
  //
  // Review caught this: the pseudo walk of `/github` and `/gitlab` reported them
  // clean because both pages rendered the NOT-CONNECTED alert, so the scope bar
  // never mounted. A surface that does not render cannot be verified by looking
  // at it, and "clean" there meant "absent", not "migrated".
  //
  // The `KINDS` tables in the two wrappers are SCREAMING_CASE, so the guard
  // skipped them entirely; their labels now travel as `labelKey`.
  "components/integrations/presets-scope-bar-base.tsx",
  "components/github/my-github/presets-scope-bar.tsx",
  "components/gitlab/my-gitlab/presets-scope-bar.tsx",
  // Azure DevOps, Jira, Linear and Sentry TASK surfaces: the board / work-item /
  // PR views, the ticket and issue dialogs and their launchers, the My-Jira and
  // My-Linear browse pages, and the four `/azure-devops`, `/jira`, `/linear`
  // page clients. With the settings halves listed far above, this completes all
  // four directories.
  //
  // Two shared files ride along because each is now converted COMPLETELY, which
  // is the criterion for listing — shared-ness is not:
  //   - `components/integrations/auth-error-message.tsx`: props-driven apart from
  //     one heading and one link label, rendered by the Jira, Linear and Sentry
  //     issue-common surfaces.
  //   - `components/task-create-dialog-multi-repo-guard.ts`: migrated by the
  //     Automations PR, which then argued itself out of listing it. See the
  //     corrected comment in that file.
  //
  // `components/jira/my-jira/filter-pills.tsx` carried the same defect review
  // found in the GitLab sidebar: `data-testid` was built as
  // `jira-filter-pill-${label.toLowerCase()}` from a label this PR translates.
  // Here it was a LIVE break — `e2e/tests/integrations/jira-default-project.spec.ts`
  // selects `jira-filter-pill-project` and `-status` by name. The pill id and the
  // visible label are now separate props.
  //
  // Deliberately left in English: JQL and WIQL (query languages, and the bare
  // acronyms label them), every Azure DevOps / Jira / Linear / Sentry state,
  // level, priority and work-item type, all provider ids, keys and names, and
  // the persisted prompt templates the settings halves already recorded.
  "app/azure-devops/azure-devops-page-client.tsx",
  "app/jira/jira-page-client.tsx",
  "app/linear/linear-page-client.tsx",
  "components/azure-devops/azure-devops-board.tsx",
  "components/azure-devops/azure-devops-feedback-dialog.tsx",
  "components/azure-devops/azure-devops-filters.tsx",
  "components/azure-devops/azure-devops-pull-request-pagination.tsx",
  "components/azure-devops/azure-devops-results.tsx",
  "components/azure-devops/azure-devops-save-view-dialog.tsx",
  "components/azure-devops/azure-devops-work-item-detail.tsx",
  "components/integrations/auth-error-message.tsx",
  "components/jira/jira-import-bar.tsx",
  "components/jira/jira-ticket-button.tsx",
  "components/jira/jira-ticket-common.tsx",
  "components/jira/jira-ticket-dialog.tsx",
  "components/jira/my-jira/filter-pills.tsx",
  "components/jira/my-jira/jql-editor.tsx",
  "components/jira/my-jira/list-toolbar.tsx",
  "components/jira/my-jira/results-pagination.tsx",
  "components/jira/my-jira/ticket-row.tsx",
  "components/linear/linear-import-bar.tsx",
  "components/linear/linear-issue-button.tsx",
  "components/linear/linear-issue-common.tsx",
  "components/linear/linear-issue-dialog.tsx",
  "components/sentry/sentry-issue-button.tsx",
  "components/sentry/sentry-issue-common.tsx",
  "components/jira/my-jira/filter-bar.tsx",
  "components/sentry/sentry-issue-dialog.tsx",
  "components/task-create-dialog-multi-repo-guard.ts",
  // Stats dashboard.
  "app/stats/stats-sections.tsx",
  "app/stats/stats-page-client.tsx",
  "app/stats/stats-charts.tsx",
  // Tasks list view.
  "app/tasks/tasks-list-view.tsx",
  "app/tasks/tasks-list-controls.tsx",
  "app/tasks/tasks-pagination.tsx",
  "app/tasks/rich-task-list-row.tsx",
  "app/tasks/[[]id[]]/kanban-task-shell.tsx",
  "app/tasks/columns.tsx",
  // Auth: login, invite, first-run setup.
  "app/auth/invite-page.tsx",
  "app/auth/setup-wizard.tsx",
  "app/auth/login-page.tsx",
  // Settings → Integrations index.
  "app/settings/integrations/page.tsx",
  // Settings → Agents: already fully migrated to the `agents:` namespace by
  // #2193 and #2281, before this list caught up. No `settings:` copy here —
  // recorded so the guard covers them going forward.
  "app/settings/agents/[[]agentId[]]/use-profile-mcp-config.ts",
  "app/settings/agents/page.tsx",
  "app/settings/agents/[[]agentId[]]/profiles/[[]profileId[]]/command-preview-card.tsx",
  // Home route redirect shell.
  "app/page-client.tsx",
  // Review dialog: top bar, file tree, diff toolbar/header/list, walkthrough.
  "components/review/review-comments-overview.tsx",
  "components/review/review-dialog-pr-state.tsx",
  "components/review/review-dialog-surface.tsx",
  "components/review/review-diff-header.tsx",
  "components/review/review-diff-list.tsx",
  "components/review/review-diff-toolbar.tsx",
  "components/review/review-file-tree.tsx",
  "components/review/review-findings-button.tsx",
  "components/review/review-findings-overview.tsx",
  "components/review/review-fix-comments-button.tsx",
  "components/review/review-markdown-diff-preview-content.tsx",
  "components/review/review-pr-selector.tsx",
  "components/review/review-run-button.tsx",
  "components/review/review-top-bar.tsx",
  "components/review/walkthrough-overlay.tsx",
  // Plain-TS label helpers the review diff list renders — guard-blind, so they
  // are listed to keep the entries above honest rather than because lint sees
  // anything here.
  "components/review/types.ts",
  // Kanban board: swimlanes, graph pipeline, and the mobile board shell.
  "components/kanban/graph2-task-pipeline.tsx",
  "components/kanban/kanban-header-mobile.tsx",
  "components/kanban/kanban-header.tsx",
  "components/kanban/mobile-column-tabs.tsx",
  "components/kanban/mobile-drop-targets.tsx",
  "components/kanban/mobile-fab.tsx",
  "components/kanban/mobile-menu-sheet.tsx",
  // Extracted out of mobile-menu-sheet.tsx to stay under the 600-line limit.
  "components/kanban/mobile-menu-styles.ts",
  "components/kanban/mobile-menu-utility-actions.tsx",
  "components/kanban/mobile-menu-task-list-options.tsx",
  "components/kanban/mobile-search-bar.tsx",
  "components/kanban/swimlane-graph-content.tsx",
  "components/kanban/swimlane-graph2-content.tsx",
  "components/kanban/swimlane-header.tsx",
  "components/kanban/swimlane-kanban-content.tsx",
  "components/kanban/columns-menu.tsx",
  "components/kanban/task-multi-select-toolbar.tsx",
  // Kanban board: loose card/column/dropdown components on the same namespace.
  "components/kanban-board.tsx",
  "components/kanban-card-content.tsx",
  "components/kanban-card-menu-items.tsx",
  // Extracted out of kanban-card-content.tsx, which is guarded: without its own
  // entry the card title markup would silently leave whole-file guard coverage.
  "components/kanban-card-title.tsx",
  "components/kanban-column.tsx",
  "components/kanban-display-dropdown.tsx",
  "components/kanban-with-preview.tsx",
  // Shared by the desktop dropdown and the mobile sheet; returns catalog keys.
  "lib/kanban/repository-placeholder.ts",
  // The editor surfaces: Monaco/CodeMirror editors and diff viewers, the Shiki
  // and Monaco code blocks, the TipTap plan editor, and the file-actions menu.
  // Copy lands in the `editors:` namespace.
  //
  // Two of these files hold copy the rule cannot see, so it is recorded here
  // rather than enforced: `tiptap-mermaid-extension.ts` and the CodeMirror
  // gutter marker in `use-codemirror-editor-state.ts` build DOM imperatively
  // from ProseMirror/CodeMirror callbacks, which have no hook scope, so both
  // resolve through the module-level `t` from `@/lib/i18n`.
  "components/editors/**/*.{ts,tsx}",
  // Quick chat: the modal, its tabs, the setup form and the delete dialog.
  // Copy lands in the existing `chat:` namespace, shared with config chat.
  "components/quick-chat/**/*.{ts,tsx}",
  // Diff viewer: the pierre-backed viewer shell, its toolbar, the inline
  // comment/finding surfaces and the walkthrough overlay. Whole directory —
  // every file in it is migrated, and the `diff` namespace is not shared with
  // anything outside it.
  //
  // Git statuses, file modes, hunk markers and the pierre option keys in here
  // are protocol values, not copy, and stay in English by design; only their
  // labels are translated.
  "components/diff/**/*.{ts,tsx}",
  // Change-request (PR/MR) and commit dialogs, plus the shared integration
  // settings widgets: the auth-status banner, the copy-config menu, the
  // watcher card shell and the integrations nav menu. Both directories share
  // the `integrations` namespace.
  //
  // `change-request-feedback.ts` holds no JSX, so `mode: "jsx-only"` never
  // inspects it; the glob records that it is migrated, and only the
  // pseudo-locale can prove it stays that way. Branch names, remotes and VCS
  // provider ids travel through it as data and are never translated.
  "components/vcs/**/*.{ts,tsx}",
  "components/integrations/**/*.{ts,tsx}",
  // The CLI agent-profile editor. `components/agent/` has no namespace rule of
  // its own, so its copy lives in `common`.
  "components/agent/**/*.{ts,tsx}",
  // SPA entry: the route tables, their Suspense/loading placeholders and the
  // root/route error boundaries. `src/**` has no namespace rule either, so this
  // copy is also on `common`. Route names reach `RouteLoading` as catalog keys
  // rather than resolved copy — see the comment there.
  "src/*.{ts,tsx}",
  // Task chat surface: the composer, the transcript, the message renderers and
  // the Kandev tool-call renderers. A directory glob rather than a file list —
  // the whole tree is migrated, including the `.ts` helpers that hold copy
  // (`subagent-meta.ts`, `use-attachment-file-feedback.ts`, `agent-error-label.ts`)
  // which `mode: "jsx-only"` never inspects; only the pseudo-locale can prove
  // those stay clean.
  //
  // Deliberately left in English inside this tree, because they are protocol
  // rather than copy: message `author_type` values, the `prompt:<id>` context
  // path, the `null` rendered in the debug-metadata dialog, the highlight CSS
  // in `message-comment-decorations.tsx`, and the programming-language names in
  // `tiptap-code-block-view.tsx`.
  "components/task/chat/**/*.{ts,tsx}",
  // Seven task subdirectories, one glob each. Whole trees — including the
  // `.ts` helpers and the module-scope config tables that `mode: "jsx-only"`
  // never inspects, so only the pseudo-locale can prove those stay clean.
  //
  // The pattern used throughout these trees for copy the rule cannot see: a
  // module-level table stores a `labelKey` (a catalog key) rather than a
  // resolved string, and the component resolves it at render. That keeps the
  // sibling `value` / `key` field — which is persisted or compared with `===`
  // — in English while the label follows the locale.
  //
  // Deliberately left in English inside these trees, because they are data or
  // protocol rather than copy: sidebar-filter *values* (`state`, `in_progress`,
  // …) and sort/group keys, which are persisted in a saved view; the share
  // API's `applied_rules` redaction ids and share URLs/tokens; terminal
  // key sequences and key-cap glyphs in `mobile-terminal-keybar-helpers.tsx`
  // (only their spoken aria-labels are translated); and the `group:` bucket on
  // the command-palette entry in `sidebar-filter-bar.tsx`, which is shared
  // taxonomy owned by `components/session-commands.tsx` and migrates with it.
  "components/task/simple/**/*.{ts,tsx}",
  "components/task/mobile/**/*.{ts,tsx}",
  "components/task/add-workspace-sources/**/*.{ts,tsx}",
  "components/task/sidebar-filter/**/*.{ts,tsx}",
  "components/task/share/**/*.{ts,tsx}",
  "components/task/inspector/**/*.{ts,tsx}",
  "components/task/document/**/*.{ts,tsx}",
  // Loose components directly under `components/`. Individual file entries
  // rather than a `components/*.tsx` glob: the sibling `components/task-*.tsx`
  // files match the `components/(task\/|task-)` namespace rule and belong with
  // the `components/task` batches, so a glob here would falsely claim them.
  //
  // These have no namespace rule of their own, so they are on `common` — except
  // the two `vcs-*` menus and `workflow-selector-row`, which were placed on
  // `integrations` / `workflows` by hand to sit with the copy they share.
  //
  // Deliberately left in English inside these files, because they are protocol
  // or data rather than copy: the `gh` CLI binary name and the `kdlbs/kandev`
  // repository slug in the Improve Kandev dialogs, the `esc` key name in the
  // onboarding preview and the command-panel footer, the `make install`/`/commit`
  // shell and slash commands, git refs reaching `ontoBranch`/`fromBranch`, and
  // the `ActionDef.key` / `RUNTIMES[].id` discriminants that keep React keys
  // locale-independent.
  "components/branch-refresh-button.tsx",
  "components/cli-mode-icon.tsx",
  "components/command-panel-footer.tsx",
  "components/command-panel-scope-switcher.tsx",
  "components/create-local-repository-surface.tsx",
  "components/discard-local-changes-dialog.tsx",
  "components/enhance-prompt-button.tsx",
  "components/folder-picker.tsx",
  "components/grid-spinner.tsx",
  "components/improve-kandev-dialog-create.tsx",
  "components/improve-kandev-dialog.tsx",
  "components/onboarding-dialog.tsx",
  "components/prompt-result-recovery.tsx",
  "components/vcs-multi-repo-menu.tsx",
  "components/vcs-split-button.tsx",
  "components/watcher-repository-fields.tsx",
  "components/workflow-selector-row.tsx",
  "components/workspace-content-search.tsx",
  // The loose `components/task/*.tsx` level and the `components/task-*.tsx`
  // create-dialog / preview files — the last of the task area. A single `*`,
  // not `**`: the subdirectory globs above are separate migrations, and a `**`
  // here would claim credit for trees this PR never touched.
  //
  // `chat-context-items.ts` is listed on its own because it holds no JSX;
  // `mode: "jsx-only"` never inspects it, so the entry records that its three
  // plural labels are migrated but only the pseudo-locale can prove it stays
  // that way. The other loose `.ts` files at this level are deliberately
  // absent — they are hooks and layout helpers whose copy has not been
  // migrated, and claiming them would be false.
  //
  // Deliberately left in English inside this tree, because they are protocol
  // or data rather than copy: the dockview panel ids built by
  // `sessionPanelId()` in `session-reopen-menu.tsx` and its `.ts` siblings;
  // ports, hosts and proxy/tunnel URLs in `port-forward-dialog.tsx`, whose
  // `badge` prop was split into a `"detected" | "manual"` discriminant plus a
  // translated label so the `===` comparison stays locale-independent; commit
  // SHAs, refs and author names in `commit-row.tsx` / `commit-detail-panel.tsx`;
  // terminal commands and the `</ kandev-system>` marker in the passthrough
  // components; the `#1470`-style GitHub ref *shape* inside
  // `githubIssueRefPlaceholder` / `githubPrRefPlaceholder`, which is one key
  // each so a translator can localize the "or" without touching the URL; and
  // console-only diagnostic prefixes such as `[ModeSelector] …`.
  "components/task/*.tsx",
  "components/task/chat-context-items.ts",
  "components/task-create-dialog-footer.tsx",
  "components/task-create-dialog-form-body.tsx",
  "components/task-create-dialog-fresh-branch.tsx",
  "components/task-create-dialog-header.tsx",
  "components/task-create-dialog-options.tsx",
  "components/task-create-dialog-remote-repo-chip.tsx",
  "components/task-create-dialog-remote-repo-chips.tsx",
  "components/task-create-dialog-repo-chips.tsx",
  "components/task-create-dialog-selectors.tsx",
  "components/task-create-dialog-source-mode.tsx",
  "components/task-create-dialog-workspace-repo-chips.tsx",
  "components/task-preview-panel.tsx",

  // The last batch of live user-facing surfaces before `app/office`: the session
  // prepare panel, the onboarding agents step, the host system-metrics readout
  // and system-health issues dialog, the shared panel search bar, the mermaid
  // diagram block, the settings agent card, the two release-notes surfaces, and
  // the shared data-table pair.
  //
  // Listed FILE BY FILE rather than by directory. Every one of these directories
  // still holds un-migrated files — `components/settings/` alone has hundreds —
  // so a `components/settings/*.tsx` glob would claim work this PR did not do.
  //
  // Deliberately NOT migrated in these files, because each is protocol or data
  // rather than copy: the `%`/`s`/`ms`/`m` unit suffixes and the `-` empty marker
  // in the metrics readout, the `{workspaceId}` URL template token in the health
  // dialog, the `"ellipsis"` page sentinel in the data-table pagination, the
  // `v` version prefix and the `0 / 0` match counter, the `[-]`/`[+]` toggle
  // glyphs in the prepare panel, and every mermaid theme/security identifier and
  // CSS value in the diagram block. `metricLabel`'s wire ids stay untranslated
  // on the `?? id` fallback path for the same reason.
  "components/onboarding/step-agents.tsx",
  "components/release-notes/release-notes-button.tsx",
  "components/release-notes/release-notes-dialog.tsx",
  "components/search/panel-search-bar.tsx",
  "components/session/prepare-progress.tsx",
  "components/settings/agent-card.tsx",
  "components/shared/mermaid-block.tsx",
  "components/system-health/health-indicator.tsx",
  "components/system-metrics/status-surface-metrics.tsx",
  "components/ui/data-table-pagination.tsx",
  "components/ui/data-table.tsx",

  // Office, in full — the last un-migrated area of the app. One glob rather than
  // a file list because every one of the 212 files under it is migrated; `**`
  // traverses the `[id]` / `[runId]` route directories because the brackets are
  // in the PATH, not in the pattern (verified: the entry resolves to 212 files,
  // 69 of them inside dynamic routes).
  //
  // Office ships behind `KANDEV_FEATURES_OFFICE` and is marked
  // `StabilityExperimental`, so its copy is less settled than the rest of the
  // app. Everything here was migrated as written; nothing was reworded.
  //
  // Deliberately NOT translated inside this tree, because each is protocol,
  // persisted data, or an identifier rather than copy:
  //
  //   - Wire enums used as VALUES: task/agent/run statuses and priorities,
  //     tiers, wake reasons, route-attempt outcomes, provider ids, skill source
  //     types, concurrency and catch-up policies. Their LABELS are copy and live
  //     in `app/office/lib/label-keys.ts` and per-file `*_LABEL_KEYS` maps; the
  //     record keys beside them are the wire values and never move.
  //   - Persisted content: `suggestWorkspaceName`'s "Default" (a workspace
  //     name), `DEFAULT_ONBOARDING_TASK_TITLE`, the `"CEO"` / `"KAN"` / `"local_pc"`
  //     fallbacks `submitOnboarding` writes, the default git commit message in
  //     `git-section.tsx`, and the `"unnamed"` filename stem in the export
  //     bundle. Translating any of these would store a different value per
  //     boot locale.
  //   - Agent-facing text: `DEFAULT_ONBOARDING_TASK_DESCRIPTION` is sent to the
  //     coordinator verbatim as its prompt.
  //   - Syntax the user types: the `{{name}} - {{date}}` routine-title template
  //     (routed through `t()` it would become an i18next interpolation and
  //     resolve to nothing) and the cron expressions, which travel as `values`
  //     so they never enter the catalog.
  //   - Backend strings matched or echoed: the `"already exists"` / `"duplicate"`
  //     / `"unique"` substrings in `skills-page-client.tsx`, and the field names
  //     and messages `extractValidationDetails` reads off an API error.
  //   - Two derived verbs with no closed union on the wire: `activityActionVerb`
  //     in `app/office/tasks/[id]/page.tsx` and `formatReason` in
  //     `runs-list-view.tsx`. A key map would silently fall through for any
  //     action the backend adds; both are commented in place.
  //   - `"Agent"` in `agent-avatar.tsx`, which seeds the initials and the tint
  //     hash rather than being rendered, and the `?? role` / `?? status`
  //     fallbacks in `agent-role-badge.tsx` / `agent-status-dot.tsx`, which show
  //     the raw wire value only before `office.meta` hydrates.
  //   - Units, symbols and acronym badges: `m`/`h`/`d`/`s`/`ms`/`B`/`KB`, `%`,
  //     the `—` empty marker, `MTD`, `KEY`/`MODE`, and the debug field names in
  //     the route-attempt panel.
  //
  // Also here: `app/office/lib/label-keys.ts` is new in this change — the shared
  // wire-value-to-catalog-key maps the status icon, board, filters, new-task
  // chips, routing cards and activity rows all resolve through, so the same
  // vocabulary cannot drift apart across surfaces.
  "app/office/**/*.{ts,tsx}",
  // The command palette's two un-migrated producers. `global-commands.tsx` was
  // already listed and already resolved everything through the catalog, so ⌘K
  // rendered a translated Navigation group above English Git/Panels rows; these
  // two are what made the same list half-English. Their labels are `label:`
  // object properties rather than JSX, which is why the guard reported them
  // clean the whole time — see the note on `mode: "jsx-only"` above.
  "components/homepage-commands.tsx",
  "components/session-commands.tsx",
  // `use-git-with-feedback.ts` composed its own English around the operation
  // name ("Push failed"), which is why `vcs-split-button.tsx`, `vcs-dialogs.tsx`
  // and the palette all passed it a deliberately English label. It interpolates
  // catalog messages now, so those three no longer have to.
  "hooks/use-git-with-feedback.ts",
  // Toast and feedback copy. None of it is JSX — it is `title:`/`description:`
  // object properties and `toast.error("…")` arguments — so `mode: "jsx-only"`
  // never inspected any of these files and they read as clean for the whole
  // migration. Listing them records that they are done; only the pseudo-locale
  // can prove they stay that way.
  //
  // `changes-panel-hooks.ts` carried the multi-repo half of the same
  // concatenated-English problem as `use-git-with-feedback.ts`
  // (`${operationName} partially succeeded`), including two per-repo counts that
  // now use `count` with `_one`/`_other` instead of a bare "repos".
  //
  // `unarchive-feedback.ts` inflected its own sentence
  // (`${plural ? "Branches" : "Branch"} … no longer ${plural ? "exist" : "exists"}`),
  // which is the inline-plural shape docs/i18n.md rules out; the plural rule
  // lives in the catalog now. `check-inline-plurals.mjs` does not see this
  // form — it is two independent ternaries, not a `+ "s"`.
  "app/tasks/tasks-page-client.tsx",
  "components/azure-devops/azure-devops-task-launcher.tsx",
  "components/github/use-pr-scoped-review-request.ts",
  "components/task/changes-panel-hooks.ts",
  "components/task/task-center-panel-restoration.ts",
  "components/task/use-tunnel-actions.ts",
  "hooks/domains/comments/use-markdown-preview-comments.ts",
  "hooks/domains/session/use-task-environment.ts",
  "hooks/use-file-save-delete.ts",
  "hooks/use-utility-agent-generator.ts",
  "lib/tasks/unarchive-feedback.ts",
  // Dockview panel titles. These are display copy that is ALSO persisted —
  // `toSerializedDockview` writes the title into the stored layout JSON — so
  // they now carry a `titleKey` beside a canonical English `title`, the split
  // the note on the layouts screen in `e2e/tests/i18n/pseudo-coverage.spec.ts`
  // said they needed. `constants.ts` deliberately still holds English `title:`
  // values; they are storage, not copy, and `panel-titles.test.ts` asserts the
  // two stay apart.
  "lib/state/dockview-panel-actions.ts",
  "lib/state/layout-manager/constants.ts",
  "lib/state/layout-manager/serializer.ts",
  // Built-in layout profile names and descriptions, the other half of the same
  // problem: `upsertBuiltInLayoutOverride` copies `name` into a saved override,
  // so `name`/`description` stay canonical English and the settings list renders
  // `nameKey`/`descriptionKey`.
  "lib/layout/layout-profiles.ts",
  // English `??` / `||` fallbacks — "Agent", "Terminal", "Repository",
  // "Untitled task", "An error occurred". They render whenever the real value
  // is absent, which is a normal path rather than an edge case, and none of
  // them is a JSX literal, so the guard never saw them. Values that are
  // PERSISTED or sent to an agent verbatim ("New Repository", the quick-chat
  // session name, `use-plan-actions`' default prompt) are deliberately still
  // English and stay off this list.
  "components/review/review-diff-list-groups.tsx",
  "components/review/review-dialog.tsx",
  "components/task/changes-git-credential-display.ts",
  "components/task/task-session-sidebar-archived-item.ts",
  "hooks/domains/session/use-session-resumption.ts",
  "hooks/domains/session/use-terminals.ts",
  "hooks/domains/session/use-user-shells.ts",
  "hooks/use-editor-keybinds.ts",
  "hooks/use-summarize-session.ts",
  "hooks/use-update-available-toast.ts",
  "lib/agent-runtime-update.ts",
  "lib/api/domains/plan-api.ts",
  "lib/capability-warning.ts",
  "lib/github-auth.ts",
  "lib/recent-tasks.ts",
  "lib/state/slices/comments/format.ts",
  "lib/utils/file-diff.ts",
  // The built-in sidebar view's name, the third persisted-and-displayed string
  // in this change. `SidebarView.name` is user-editable and synced, so the
  // built-in keeps a canonical English `name` and every surface resolves
  // `sidebarViewName()` instead. Found by the pseudo oracle, not by lint: it is
  // a `.ts` constant, which `mode: "jsx-only"` never inspects.
  "lib/state/slices/ui/sidebar-view-builtins.ts",
  // The last English row in the palette. Its `group` was already translated —
  // it has to be, since the palette groups by the resolved value — and a
  // comment deferred the `label`/`keywords` to "the components/task migration".
  // Both are palette copy and belong with the rest of it.
  "components/task/recent-task-switcher-hooks.ts",
  // Second-audit gaps hidden in hook toasts, configuration objects, synthetic
  // display records, and developer QA page chrome rather than JSX text.
  "app/demo/agent-messages/page.tsx",
  "app/demo/messages/page.tsx",
  "components/azure-devops/azure-devops-mode-tabs.tsx",
  "components/azure-devops/azure-devops-scope-bar.tsx",
  "components/azure-devops/azure-devops-task-pull-request-chip.tsx",
  "components/gitlab/my-gitlab/quick-task-launcher.tsx",
  "components/shared/mermaid-error-toast.tsx",
  "components/task/task-pr-shortcut.tsx",
  "components/task-create-dialog-effects.ts",
  "components/task-create-dialog-fresh-branch-consent.ts",
  "components/task-create-dialog-submit.tsx",
  "hooks/domains/kanban/use-implement-fresh.ts",
  "hooks/domains/kanban/use-plan-actions.ts",
  "hooks/domains/review/use-finding-actions.ts",
  "hooks/domains/review/use-send-finding-to-agent.ts",
  "hooks/domains/session/use-commit-diff.ts",
  "hooks/domains/session/use-request-changes-walkthrough.ts",
  "hooks/domains/session/use-session-actions.ts",
  "hooks/domains/session/use-task-plan.ts",
  "hooks/use-file-editors.ts",
  "hooks/use-file-operations.ts",
  "hooks/use-nest-task.ts",
  "hooks/use-open-session-in-editor.ts",
  "hooks/use-sidebar-views-sync.ts",
  "hooks/use-task-workflow-move.ts",
  "lib/keyboard/shortcut-overrides.ts",
  // Third audit: the non-JSX sweep. `check-nonjsx-copy.mjs` used to default its
  // scope to THIS list, so 1141 of 2564 source files were judged by neither gate
  // — and the positions eslint structurally cannot see (config tables, helper
  // returns, parameter defaults, toast arguments) are exactly what lives in the
  // plain `.ts` modules that never appear here. These are the files that sweep
  // externalized or marked `i18n-exempt`. The scanner now defaults to the whole
  // tree and `i18next/no-literal-string` is repo-wide, so this list is the
  // migration record and the `lint:i18n` preview scope, no longer the bound.
  "app/actions/agents.ts",
  "app/demo/messages/demo-messages-data.ts",
  "app/layout.tsx",
  "app/stats/stats-data.tsx",
  "app/stats/stats-utils.ts",
  "app/tasks/rich-task-row-details.ts",
  "components/combobox.tsx",
  "components/debug-overlay.tsx",
  "components/github/my-github/use-github-search.ts",
  "components/gitlab/my-gitlab/presets.ts",
  "components/gitlab/my-gitlab/use-gitlab-search.ts",
  "components/improve-kandev-dialog-model.ts",
  "components/jira/my-jira/filter-model.ts",
  "components/jira/my-jira/use-saved-views.ts",
  "components/kanban-card-edit-submenu.tsx",
  "components/kanban/swimlane-container.tsx",
  "components/kanban/task-search-input.tsx",
  "components/mobile/pull-to-refresh.tsx",
  "components/runs/automation-rows.ts",
  "components/runs/run-feed-item.tsx",
  "components/runs/use-automation-activity.ts",
  "components/runs/use-automation-summaries.ts",
  "components/runs/use-workspace-automations.ts",
  "components/runs/use-workspace-runs.ts",
  "components/settings/key-value-input.tsx",
  "components/settings/lsp-language-cards.tsx",
  "components/settings/settings-breadcrumb-labels.ts",
  "components/settings/settings-save-provider.tsx",
  "components/settings/sleep-inhibition-settings.tsx",
  "components/shared/mermaid-utils.ts",
  "components/task-create-dialog-branch-utils.ts",
  "components/task-create-dialog-computed.ts",
  "components/task-create-dialog-pill.tsx",
  "components/task-create-dialog-setup.ts",
  "components/task/dockview-session-handoff.ts",
  "components/task/dockview-session-tabs.ts",
  "components/task/editor-worktree-options.ts",
  "components/task/executor-environment-status.ts",
  "components/task/file-browser-actions.ts",
  "components/task/file-browser-path.ts",
  "components/task/file-browser-tree-loader.ts",
  "components/task/new-session-form-actions.ts",
  "components/task/recent-task-switcher-model.ts",
  "components/task/review-panel-provider.ts",
  "components/task/session-context-summary.ts",
  "components/task/session-sort.ts",
  "components/task/sidebar-mock-data.ts",
  "components/task/task-cleanup-summary.ts",
  "components/task/task-session-sidebar-link-actions.ts",
  "components/task/use-passthrough-terminal.ts",
  "components/task/use-review-dialog.ts",
  "components/task/use-subtask-submit.ts",
  "components/task/use-terminal-search.ts",
  "components/workspace-source-picker/workspace-source-state.ts",
  "hooks/domains/azure-devops/use-azure-devops-watches.ts",
  "hooks/domains/github/pr-commits-resource.ts",
  "hooks/domains/github/use-github-app-registrations.ts",
  "hooks/domains/github/use-pr-diff.ts",
  "hooks/domains/github/use-pr-feedback.ts",
  "hooks/domains/github/use-task-ci-options.ts",
  "hooks/domains/gitlab/use-gitlab-issue-watches.ts",
  "hooks/domains/gitlab/use-gitlab-review-watches.ts",
  "hooks/domains/gitlab/use-mr-feedback.ts",
  "hooks/domains/gitlab/use-task-mr-automation.ts",
  "hooks/domains/jira/use-jira-issue-watches.ts",
  "hooks/domains/kanban/use-sidebar-archived-tasks.ts",
  "hooks/domains/kanban/use-swimlane-move.ts",
  "hooks/domains/linear/use-linear-issue-watches.ts",
  "hooks/domains/office/use-agent-route.ts",
  "hooks/domains/office/use-provider-health.ts",
  "hooks/domains/office/use-routing-preview.ts",
  "hooks/domains/office/use-run-attempts.ts",
  "hooks/domains/office/use-workspace-routing.ts",
  "hooks/domains/session/use-clarification-group.ts",
  "hooks/domains/session/use-cumulative-diff.ts",
  "hooks/domains/session/use-session-launch.ts",
  "hooks/domains/session/use-session-messages.ts",
  "hooks/domains/session/use-terminals-build.ts",
  "hooks/use-detach-task.ts",
  "hooks/use-drag-and-drop.ts",
  "hooks/use-entity-reference-search.ts",
  "hooks/use-git-operations.ts",
  "hooks/use-inline-mention.ts",
  "hooks/use-message-handler.ts",
  "hooks/use-nav-availability.ts",
  "hooks/use-optimistic-task-mutation.ts",
  "hooks/use-prompt-result-delivery.ts",
  "hooks/use-session-failure-toast.ts",
  "hooks/use-task-deleted-toast.ts",
  "hooks/use-terminal-link-handler.ts",
  "hooks/use-workflow-steps.ts",
  "lib/api/domains/attachment-api.ts",
  "lib/api/domains/automation-api.ts",
  "lib/api/domains/frontend-error-log-api.ts",
  "lib/api/domains/github-api.ts",
  "lib/api/domains/queue-api.ts",
  "lib/api/domains/review-api.ts",
  "lib/api/domains/user-shell-api.ts",
  "lib/api/domains/walkthrough-api.ts",
  "lib/kanban/view-registry.ts",
  "lib/keyboard/constants.ts",
  "lib/logger/intercept.ts",
  "lib/lsp/lsp-progress-view.ts",
  "lib/notifications/sound.ts",
  "lib/plugins/change-request-creation.ts",
  "lib/review/format.ts",
  "lib/routing/client-route-helpers.ts",
  "lib/settings-discovery/catalog/index.ts",
  "lib/settings/external-mcp-snippets.ts",
  "lib/ssr/session-page-state.ts",
  "lib/state/dockview-env-switch.ts",
  "lib/state/dockview-layout-builders.ts",
  "lib/state/layout-manager/session-panels.ts",
  "lib/state/slices/ui/sidebar-task-prefs-actions.ts",
  "lib/state/slices/ui/sidebar-view-actions.ts",
  "lib/tasks/tasks-list-options.ts",
  "lib/terminal/terminal-font.ts",
  "lib/theme/colors.ts",
  "lib/utils/file-change-status.ts",
  "lib/utils.ts",
  "lib/walkthrough-request.ts",
  "lib/ws/client.ts",
  "lib/ws/handlers/agent-session.ts",
  "lib/ws/handlers/empty-turn-notice.ts",
  "lib/ws/handlers/notifications.ts",
  "lib/ws/handlers/quick-chat.ts",
  "lib/ws/use-websocket.tsx",
];
