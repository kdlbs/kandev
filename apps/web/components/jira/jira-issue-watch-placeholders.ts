import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const JIRA_ISSUE_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "issue.key",
    description: "Ticket key",
    example: "PROJ-42",
    executor_types: [],
  },
  {
    key: "issue.summary",
    description: "Ticket summary",
    example: "Login fails on mobile",
    executor_types: [],
  },
  {
    key: "issue.url",
    description: "Ticket URL",
    example: "https://acme.atlassian.net/browse/PROJ-42",
    executor_types: [],
  },
  {
    key: "issue.status",
    description: "Status name",
    example: "In Progress",
    executor_types: [],
  },
  {
    key: "issue.priority",
    description: "Priority",
    example: "High",
    executor_types: [],
  },
  {
    key: "issue.type",
    description: "Issue type",
    example: "Bug",
    executor_types: [],
  },
  {
    key: "issue.assignee",
    description: "Assignee display name",
    example: "Alice Example",
    executor_types: [],
  },
  {
    key: "issue.reporter",
    description: "Reporter display name",
    example: "Bob Example",
    executor_types: [],
  },
  {
    key: "issue.project",
    description: "Project key",
    example: "PROJ",
    executor_types: [],
  },
  {
    key: "issue.description",
    description: "Ticket description",
    example: "When clicking login...",
    executor_types: [],
  },
];

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
