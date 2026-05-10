import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const LINEAR_ISSUE_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "issue.url",
    description: "Linear issue URL",
    example: "https://linear.app/acme/issue/ENG-7",
    executor_types: [],
  },
  {
    key: "issue.identifier",
    description: "Issue identifier",
    example: "ENG-7",
    executor_types: [],
  },
  {
    key: "issue.title",
    description: "Issue title",
    example: "Login fails on mobile",
    executor_types: [],
  },
  {
    key: "issue.team",
    description: "Team key",
    example: "ENG",
    executor_types: [],
  },
  {
    key: "issue.state",
    description: "Workflow state name",
    example: "In Progress",
    executor_types: [],
  },
  {
    key: "issue.priority",
    description: "Priority label",
    example: "High",
    executor_types: [],
  },
  {
    key: "issue.assignee",
    description: "Assignee display name",
    example: "Alice",
    executor_types: [],
  },
  {
    key: "issue.creator",
    description: "Issue creator display name",
    example: "Bob",
    executor_types: [],
  },
  {
    key: "issue.description",
    description: "Issue description body",
    example: "Tap submit, nothing happens.",
    executor_types: [],
  },
];

// DEFAULT_LINEAR_ISSUE_WATCH_PROMPT mirrors apps/backend/config/prompts/linear-issue-watch-default.md.
// Kept in sync by hand: the UI shows this when the user clears the field, and
// the backend reads the .md when the saved prompt is empty. Diverging would
// surprise the user — they'd see one default in the dialog and another get
// sent to the agent.
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
