import type { TFunction } from "i18next";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

/**
 * Placeholder tokens available in the issue-watch prompt.
 *
 * A function rather than a const because `description` is copy — it is rendered
 * in the prompt field's tooltip and in the editor's completion list — and a
 * module-scope `t()` would freeze it at the boot locale (see docs/i18n.md).
 *
 * `key` and `example` are NOT copy. `key` is the token the prompt template is
 * interpolated against, and each `example` is sample Sentry data: a short ID, an
 * issue permalink, a project slug, a level and status drawn from the
 * `SentryLevel`/`SentryStatus` unions, a culprit frame, an event count, an ISO
 * timestamp. Both are identifiers, so they stay out of the catalog where the
 * pseudo-locale would transliterate them into dead pointers. Same contract as
 * `linearIssueWatchPlaceholders` in components/linear.
 */
export function sentryIssueWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "issue.short_id",
      description: t("sentry:placeholderShortId"),
      example: "PROJ-123",
      executor_types: [],
    },
    {
      key: "issue.title",
      description: t("sentry:placeholderIssueTitle"),
      example: "TypeError: cannot read property of undefined",
      executor_types: [],
    },
    {
      key: "issue.url",
      description: t("sentry:placeholderIssueUrl"),
      example: "https://sentry.io/organizations/acme/issues/123/",
      executor_types: [],
    },
    {
      key: "issue.project",
      description: t("sentry:placeholderProjectSlug"),
      example: "frontend",
      executor_types: [],
    },
    {
      key: "issue.level",
      description: t("sentry:placeholderIssueLevel"),
      example: "error",
      executor_types: [],
    },
    {
      key: "issue.status",
      description: t("sentry:placeholderIssueStatus"),
      example: "unresolved",
      executor_types: [],
    },
    {
      key: "issue.culprit",
      description: t("sentry:placeholderCulprit"),
      example: "app/routes/checkout.tsx in handleSubmit",
      executor_types: [],
    },
    {
      key: "issue.assignee",
      description: t("sentry:placeholderAssigneeDisplayName"),
      example: "Alice",
      executor_types: [],
    },
    {
      key: "issue.count",
      description: t("sentry:placeholderEventCount"),
      example: "42",
      executor_types: [],
    },
    {
      key: "issue.user_count",
      description: t("sentry:placeholderUserCount"),
      example: "7",
      executor_types: [],
    },
    {
      key: "issue.first_seen",
      description: t("sentry:placeholderFirstSeen"),
      example: "2026-05-20T11:30:00Z",
      executor_types: [],
    },
    {
      key: "issue.last_seen",
      description: t("sentry:placeholderLastSeen"),
      example: "2026-05-23T08:14:00Z",
      executor_types: [],
    },
  ];
}

// DEFAULT_SENTRY_ISSUE_WATCH_PROMPT mirrors apps/backend/config/prompts/sentry-issue-watch-default.md.
// Kept in sync by hand: the UI shows this when the user clears the field, and
// the backend reads the .md when the saved prompt is empty.
//
// Deliberately NOT translated, for the same reason as
// DEFAULT_LINEAR_ISSUE_WATCH_PROMPT: it is seeded into the field, then persisted
// on the watch and sent to the agent verbatim. A watch created under one locale
// would keep that locale's prompt forever, and the agent reads it as an
// instruction rather than as UI copy — it also has to keep matching the backend
// .md, which has no locale at all.
export const DEFAULT_SENTRY_ISSUE_WATCH_PROMPT = `You have been assigned a Sentry issue to triage and fix.

**Issue:** {{issue.url}}
**Short ID:** {{issue.short_id}}
**Title:** {{issue.title}}
**Project:** {{issue.project}}
**Level:** {{issue.level}}
**Status:** {{issue.status}}
**Assignee:** {{issue.assignee}}
**Culprit:** {{issue.culprit}}
**Events:** {{issue.count}} ({{issue.user_count}} users affected)
**First seen:** {{issue.first_seen}}
**Last seen:** {{issue.last_seen}}

## Instructions

1. Open the issue in Sentry and read the full stack trace.
2. Reproduce the bug locally if possible.
3. Implement the fix and add a regression test.
4. Run the test suite to ensure nothing else broke.
5. Commit your changes with a descriptive message referencing {{issue.short_id}}.`;
