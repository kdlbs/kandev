import type { TFunction } from "i18next";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

/**
 * Placeholder tokens available in the issue-watch prompt. See
 * `review-watch-placeholders.ts` for why this is a function, not a const.
 */
export function issueWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "issue.link",
      description: t("github:placeholderIssueUrl"),
      example: "https://github.com/org/repo/issues/42",
      executor_types: [],
    },
    {
      key: "issue.number",
      description: t("github:placeholderIssueNumber"),
      example: "42",
      executor_types: [],
    },
    {
      key: "issue.title",
      description: t("github:placeholderIssueTitle"),
      example: "Fix login page crash",
      executor_types: [],
    },
    {
      key: "issue.author",
      description: t("github:placeholderIssueAuthor"),
      example: "octocat",
      executor_types: [],
    },
    {
      key: "issue.repo",
      description: t("github:placeholderRepository"),
      example: "org/repo",
      executor_types: [],
    },
    {
      key: "issue.labels",
      description: t("github:placeholderCommaSeparatedLabels"),
      example: "bug, priority:high",
      executor_types: [],
    },
    {
      key: "issue.body",
      description: t("github:placeholderIssueBody"),
      example: "When clicking login...",
      executor_types: [],
    },
  ];
}

/**
 * Seeded into the prompt field, then persisted and sent to the agent verbatim.
 * Deliberately NOT translated — see `DEFAULT_REVIEW_WATCH_PROMPT`.
 */
export const DEFAULT_ISSUE_WATCH_PROMPT = `You have been assigned a GitHub issue to work on.

**Issue:** {{issue.link}}
**Title:** {{issue.title}} (#{{issue.number}})
**Repository:** {{issue.repo}}
**Author:** {{issue.author}}
**Labels:** {{issue.labels}}

## Instructions

1. Read the issue description carefully and understand the requirements.
2. Explore the codebase to understand the relevant code and architecture.
3. Implement the changes described in the issue.
4. Write or update tests to cover the changes.
5. Run the test suite to ensure nothing is broken.
6. Commit your changes with a descriptive commit message referencing the issue.`;
