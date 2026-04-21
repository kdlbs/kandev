import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const ISSUE_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "issue.link",
    description: "Issue URL",
    example: "https://github.com/org/repo/issues/42",
    executor_types: [],
  },
  {
    key: "issue.number",
    description: "Issue number",
    example: "42",
    executor_types: [],
  },
  {
    key: "issue.title",
    description: "Issue title",
    example: "Fix login page crash",
    executor_types: [],
  },
  {
    key: "issue.author",
    description: "Issue author username",
    example: "octocat",
    executor_types: [],
  },
  {
    key: "issue.repo",
    description: "Repository (owner/name)",
    example: "org/repo",
    executor_types: [],
  },
  {
    key: "issue.labels",
    description: "Comma-separated labels",
    example: "bug, priority:high",
    executor_types: [],
  },
  {
    key: "issue.body",
    description: "Issue body text",
    example: "When clicking login...",
    executor_types: [],
  },
];

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
