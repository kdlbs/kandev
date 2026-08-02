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
];
