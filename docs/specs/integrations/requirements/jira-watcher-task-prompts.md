---
status: active
system: integrations
created: 2026-08-28
owners:
  - kandev
---

# Jira Watcher Task Prompt Requirements

## Overview

Jira issue watchers create Kandev tasks from matching Jira issues. The integration system owns the Jira data that supplies each configured task prompt.

## Requirements

### REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001: Complete Jira issue context

**Intent:** A watcher-created task must contain the Jira issue context that the user selects in the task prompt template.

**User story:** As a Jira watcher user, I want Jira placeholders to contain issue data, so that each new task has complete work instructions.

#### Acceptance criteria

- **AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.1:** When a watcher finds a matching issue, Kandev shall resolve each supported `{{issue.*}}` placeholder before it creates the task.
- **AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.2:** When Jira returns a description, `{{issue.description}}` shall contain readable description text for each supported Jira connection type.
- **AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3:** When Jira returns Atlassian Document Format, the description shall keep text order and paragraph separation. A plain string shall stay unchanged.
- **AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.4:** When Jira returns no description, Kandev shall replace the placeholder with an empty value and continue task creation.

## Out of scope

- Custom Jira fields that do not have supported placeholders.
- Exact reproduction of Jira rich-text styles, attachments, comments, or embedded media.
- Changes to watcher query, deduplication, task routing, or automatic start behavior.
