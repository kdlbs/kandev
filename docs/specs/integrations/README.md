---
status: draft
system: integrations
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Integration system

## Purpose

The integration system owns connections to external services and the
provider-specific contracts that synchronize or act on external work.

## Ownership

This system owns provider credentials, provider identity, external issue and
pull-request synchronization, integration settings, provider-aware review
automation, external question or answer flows, and provider-specific UI
outcomes that expose those contracts.

## Exclusions

- Generic authentication belongs to the [auth system](../auth/README.md).
- Durable Kandev tasks belong to the [task system](../tasks/README.md).
- Plugin-owned services belong to the [plugin system](../plugins/README.md).
- Reusable presentation contracts without provider state belong to the
  [UI system](../ui/README.md).

## Specification map

### Requirements



- [Azure DevOps Integration](requirements/azure-devops-integration.md)
- [Bitbucket Connector Plugin](requirements/bitbucket-plugin.md)
- [Claude Fork Review Allowlist](requirements/claude-fork-review-allowlist.md)
- [Clickable integration cards](requirements/clickable-integration-cards.md)
- [Integration Enable/Disable Toggle & Nav Visibility](requirements/enable-disable-toggle.md)
- [External MCP Endpoint](requirements/external-mcp.md)
- [External Question Answering — authorized, discoverable, idempotent clarification resolution](requirements/external-question-answering.md)
- [Workspace GitHub Authentication](requirements/github-authentication.md)
- [GitHub PR Merge Queue](requirements/github-pr-merge-queue.md)
- [GitLab Integration](requirements/gitlab-integration.md)
- [GitLab MR Status Chip](requirements/gitlab-mr-status-chip.md)
- [GitLab MR Badge on the Sidebar and Tasks-List Rows](requirements/gitlab-mr-task-list-badges.md)
- [GitLab Workflow Sync](requirements/gitlab-workflow-sync.md)
- [Jira Ticket Status Filter](requirements/jira-status-filter.md)
- [MCP Tool Argument Validation](requirements/mcp-tool-argument-validation.md)
- [Pull request outcome attribution](requirements/pr-outcome-attribution.md)
- [Provider-Aware Review Automation Runtime](requirements/provider-aware-review-automation.md)
- [Slack Integration](requirements/slack.md)

### System design



- [Azure DevOps Integration System Design Part 1](system-design/azure-devops-integration-01.md)
- [Azure DevOps Integration System Design Part 2](system-design/azure-devops-integration-02.md)
- [Bitbucket Connector Plugin System Design Part 1](system-design/bitbucket-plugin-01.md)
- [Bitbucket Connector Plugin System Design Part 2](system-design/bitbucket-plugin-02.md)
- [External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 1](system-design/external-question-answering-01.md)
- [External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 2](system-design/external-question-answering-02.md)
- [External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 3](system-design/external-question-answering-03.md)
- [External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 4](system-design/external-question-answering-04.md)
- [Workspace GitHub Authentication System Design Part 1](system-design/github-authentication-01.md)
- [Workspace GitHub Authentication System Design Part 2](system-design/github-authentication-02.md)
- [Workspace GitHub Authentication System Design Part 3](system-design/github-authentication-03.md)
- [GitHub PR Merge Queue](system-design/github-pr-merge-queue.md)
- [GitLab Integration System Design Part 1](system-design/gitlab-integration-01.md)
- [GitLab Integration System Design Part 2](system-design/gitlab-integration-02.md)
- [GitLab MR Status Chip System Design Part 1](system-design/gitlab-mr-status-chip-01.md)
- [GitLab MR Status Chip System Design Part 2](system-design/gitlab-mr-status-chip-02.md)
- [GitLab MR Status Chip System Design Part 3](system-design/gitlab-mr-status-chip-03.md)
- [GitLab MR Status Chip System Design Part 4](system-design/gitlab-mr-status-chip-04.md)
- [GitLab MR Status Chip System Design Part 5](system-design/gitlab-mr-status-chip-05.md)
- [GitLab MR Badge on the Sidebar and Tasks-List Rows System Design Part 1](system-design/gitlab-mr-task-list-badges-01.md)
- [GitLab MR Badge on the Sidebar and Tasks-List Rows System Design Part 2](system-design/gitlab-mr-task-list-badges-02.md)
- [GitLab MR Badge on the Sidebar and Tasks-List Rows System Design Part 3](system-design/gitlab-mr-task-list-badges-03.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Auth](../auth/README.md): authenticates users and service requests.
- [Tasks](../tasks/README.md): owns the Kandev task receiving external work.
- [Plugins](../plugins/README.md): owns plugin-host integration contracts.
