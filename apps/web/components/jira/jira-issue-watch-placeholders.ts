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
 * interpolated against, and each `example` is sample Jira data: an issue key, a
 * status name, a project key, a display name. Both are identifiers, so they stay
 * out of the catalog where the pseudo-locale would transliterate them.
 */
export function jiraIssueWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "issue.key",
      description: t("jira:placeholderTicketKey"),
      example: "PROJ-42",
      executor_types: [],
    },
    {
      key: "issue.summary",
      description: t("jira:placeholderTicketSummary"),
      example: "Login fails on mobile",
      executor_types: [],
    },
    {
      key: "issue.url",
      description: t("jira:placeholderTicketUrl"),
      example: "https://acme.atlassian.net/browse/PROJ-42",
      executor_types: [],
    },
    {
      key: "issue.status",
      description: t("jira:placeholderStatusName"),
      example: "In Progress",
      executor_types: [],
    },
    {
      key: "issue.priority",
      description: t("jira:placeholderPriority"),
      example: "High",
      executor_types: [],
    },
    {
      key: "issue.type",
      description: t("jira:placeholderIssueType"),
      example: "Bug",
      executor_types: [],
    },
    {
      key: "issue.assignee",
      description: t("jira:placeholderAssigneeDisplayName"),
      example: "Alice Example",
      executor_types: [],
    },
    {
      key: "issue.reporter",
      description: t("jira:placeholderReporterDisplayName"),
      example: "Bob Example",
      executor_types: [],
    },
    {
      key: "issue.project",
      description: t("jira:placeholderProjectKey"),
      example: "PROJ",
      executor_types: [],
    },
    {
      key: "issue.description",
      description: t("jira:placeholderTicketDescription"),
      example: "When clicking login...",
      executor_types: [],
    },
  ];
}

/**
 * Seeded into the dialog's prompt field, then persisted on the watch and sent to
 * the agent verbatim. Deliberately NOT translated: a watch created under one
 * locale would keep that locale's prompt forever, and the agent reads it as an
 * instruction rather than as UI copy. Same contract as
 * `DEFAULT_ISSUE_WATCH_PROMPT` in components/github/issue-watch-placeholders.ts.
 */
export const DEFAULT_JIRA_ISSUE_WATCH_PROMPT = `You have been assigned a JIRA ticket to work on.

**Ticket:** {{issue.url}}
**Key:** {{issue.key}}
**Summary:** {{issue.summary}}
**Type:** {{issue.type}}
**Status:** {{issue.status}}
**Priority:** {{issue.priority}}
**Assignee:** {{issue.assignee}}
**Reporter:** {{issue.reporter}}
**Project:** {{issue.project}}

## Description

{{issue.description}}

## Instructions

1. Read the ticket carefully and understand the requirements.
2. Explore the codebase to understand the relevant code and architecture.
3. Implement the changes described in the ticket.
4. Write or update tests to cover the changes.
5. Run the test suite to ensure nothing is broken.
6. Commit your changes with a descriptive commit message referencing the ticket key.`;
