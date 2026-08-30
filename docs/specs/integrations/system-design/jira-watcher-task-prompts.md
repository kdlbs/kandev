---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001
---

# Jira Watcher Task Prompt System Design

## Purpose and boundaries

The Jira integration owns the issue data for watcher task prompts. The task system receives the completed prompt and does not fetch Jira data.

This design keeps interactive ticket searches compact. Watcher searches request the description in the same Jira search request that returns each matching issue.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001` | [Provider client contract](#provider-client-contract), [Watcher flow](#watcher-flow), [Description conversion](#description-conversion) |

## Components and responsibilities

- `internal/jira.Client` separates compact ticket searches from watcher searches.
- `internal/jira.CloudClient` selects REST fields for Jira Cloud and Jira Server or Data Center.
- `internal/jira.MCPClient` maps the Atlassian MCP search result to the same `JiraTicket` model.
- `internal/jira.Service.CheckIssueWatch` requests watcher issue data and filters issues that the watcher already processed.
- `internal/orchestrator.interpolateJiraPrompt` replaces supported placeholders before task creation.

## Provider client contract

`Client.SearchTickets` remains the compact search for ticket browsing and work-item references. Its REST field list does not include `description`.

`Client.SearchTicketsForWatch` is the watcher search. The REST implementations include `description` with the existing compact fields in one batched request.

Both methods keep the current JQL and pagination behavior. The MCP implementation uses the same remote search tool because that tool owns its result field selection.

The mock implementation returns its seeded search issues for both methods. This behavior keeps integration tests independent from a remote Jira service.

## Watcher flow

1. `CheckIssueWatch` calls `SearchTicketsForWatch` for one bounded result page.
2. The provider client returns normalized `JiraTicket` values, including `Description`.
3. `CheckIssueWatch` removes issues that exist in the watch deduplication set.
4. The Jira poller publishes each new issue through `NewJiraIssueEvent`.
5. The watcher dispatcher calls `interpolateJiraPrompt` and creates the Kandev task.

The flow does not call `GetTicket` for each issue. This rule prevents extra transition requests and a request fan-out for broad JQL results.

## Description conversion

Jira Cloud REST can return `description` as Atlassian Document Format. Jira Server or Data Center can also return a plain string.

`extractDescription` keeps plain strings unchanged. For Atlassian Document Format, it keeps text order, explicit breaks, and paragraph separation.

The interpolation function replaces a missing description with an empty string. It does not keep an unresolved `{{issue.description}}` token.

## Failure and recovery

A search error stops the current watcher pass before event publication. The next eligible poll can try the query again.

A missing or null description does not stop task creation. Other issue fields remain available to the configured prompt.

## Performance boundary

Watcher polling gets all required fields in one paginated search request. Interactive searches do not receive large descriptions that their rows do not show.

## Test strategy

- REST client tests make sure that watcher searches request and convert descriptions for Cloud and Server or Data Center.
- Service tests make sure that `CheckIssueWatch` uses the watcher search contract and keeps the description on each new issue.
- Orchestrator tests make sure that `{{issue.description}}` appears in the completed task prompt.
