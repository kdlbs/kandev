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
 * interpolated against, and each `example` is sample Linear data: an issue
 * identifier, a team key, a workflow state name, a priority label, a display
 * name. Both are identifiers, so they stay out of the catalog where the
 * pseudo-locale would transliterate them. Same contract as
 * `jiraIssueWatchPlaceholders` in components/jira.
 */
export function linearIssueWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "issue.url",
      description: t("linear:placeholderIssueUrl"),
      example: "https://linear.app/acme/issue/ENG-7",
      executor_types: [],
    },
    {
      key: "issue.identifier",
      description: t("linear:placeholderIssueIdentifier"),
      example: "ENG-7",
      executor_types: [],
    },
    {
      key: "issue.title",
      description: t("linear:placeholderIssueTitle"),
      example: "Login fails on mobile",
      executor_types: [],
    },
    {
      key: "issue.team",
      description: t("linear:placeholderTeamKey"),
      example: "ENG",
      executor_types: [],
    },
    {
      key: "issue.state",
      description: t("linear:placeholderWorkflowStateName"),
      example: "In Progress",
      executor_types: [],
    },
    {
      key: "issue.priority",
      description: t("linear:placeholderPriorityLabel"),
      example: "High",
      executor_types: [],
    },
    {
      key: "issue.assignee",
      description: t("linear:placeholderAssigneeDisplayName"),
      example: "Alice",
      executor_types: [],
    },
    {
      key: "issue.creator",
      description: t("linear:placeholderCreatorDisplayName"),
      example: "Bob",
      executor_types: [],
    },
    {
      key: "issue.description",
      description: t("linear:placeholderIssueDescription"),
      example: "Tap submit, nothing happens.",
      executor_types: [],
    },
  ];
}

// DEFAULT_LINEAR_ISSUE_WATCH_PROMPT mirrors apps/backend/config/prompts/linear-issue-watch-default.md.
// Kept in sync by hand: the UI shows this when the user clears the field, and
// the backend reads the .md when the saved prompt is empty. Diverging would
// surprise the user — they'd see one default in the dialog and another get
// sent to the agent.
//
// Deliberately NOT translated, for the same reason as
// DEFAULT_JIRA_ISSUE_WATCH_PROMPT: it is seeded into the field, then persisted
// on the watch and sent to the agent verbatim. A watch created under one locale
// would keep that locale's prompt forever, and the agent reads it as an
// instruction rather than as UI copy — it also has to keep matching the backend
// .md, which has no locale at all.
export const DEFAULT_LINEAR_ISSUE_WATCH_PROMPT = `You have been assigned a Linear issue to work on.

**Issue:** {{issue.url}}
**Identifier:** {{issue.identifier}}
**Title:** {{issue.title}}
**Team:** {{issue.team}}
**State:** {{issue.state}}
**Priority:** {{issue.priority}}
**Assignee:** {{issue.assignee}}

## Description

{{issue.description}}

## Instructions

1. Read the issue description carefully and understand the requirements.
2. Explore the codebase to understand the relevant code and architecture.
3. Implement the changes described in the issue.
4. Write or update tests to cover the changes.
5. Run the test suite to ensure nothing is broken.
6. Commit your changes with a descriptive commit message referencing {{issue.identifier}}.`;
