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

Both `CloudClient.GetTicket` and `CloudClient.SearchTicketsForWatch` use
`issueToTicket`, which supplies `JiraTicket.Description` through this converter.
The fetched ticket also supplies Jira launch-preset descriptions. Neither
consumer adds paragraph boundaries after conversion.

`walkADF` traverses `content` recursively in source order. Paragraph boundaries
apply at every container depth, including sibling list items, several paragraphs
inside one item, nested lists, blockquotes, and other containers. The traversal
does not depend on a fixed list of enclosing node types. Adjacent inline text
nodes within one paragraph remain adjacent; paragraph boundaries produce two
newline characters between successive nonempty paragraph bodies.

Structural separators are emitted once at a boundary: an existing document
sibling separator satisfies the paragraph boundary there. Ancestor containers
must not add duplicate separators for the same boundary. Explicit hard and soft
breaks and newline characters already present in text retain their existing
contribution. In particular, the converter must not globally collapse runs of
newlines, trim individual text nodes, or insert a separator after each text node.
The existing final ADF `TrimSpace` and top-level document sibling separation
remain compatible, including top-level empty blocks.

Empty nested paragraphs contribute no text or additional blank rows; surrounding
nonempty paragraphs still remain separated. Missing or incorrectly typed text
and content values, and non-object children, keep the current tolerant traversal
behavior. Unknown containers with readable descendant paragraphs use the same
paragraph rule; unsupported leaf nodes do not acquire a new text representation.
This conversion does not reproduce list markers, table layout, rich-text marks,
mentions, media, or link targets.

The provider model follows Atlassian's [listItem schema](https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/listItem/),
which permits paragraphs and nested lists, and [paragraph schema](https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/paragraph/),
which separates block structure from inline content.

The interpolation function replaces a missing description with an empty string. It does not keep an unresolved `{{issue.description}}` token.

## Failure and recovery

A search error stops the current watcher pass before event publication. The next eligible poll can try the query again.

A missing or null description does not stop task creation. Other issue fields remain available to the configured prompt.

## Performance boundary

Watcher polling gets all required fields in one paginated search request. Interactive searches do not receive large descriptions that their rows do not show.

## Test strategy

- REST client tests make sure that watcher searches request and convert descriptions for Cloud and Server or Data Center.
- Real HTTP client regressions exercise nested paragraph descriptions through
  both ticket lookup and watcher search, asserting the complete normalized value.
- Converter tests cover paragraph boundaries under multiple container depths,
  inline adjacency, explicit breaks, empty and malformed nodes, and existing
  plain-string and top-level whitespace behavior.
- Service tests make sure that `CheckIssueWatch` uses the watcher search contract and keeps the description on each new issue.
- Orchestrator tests make sure that `{{issue.description}}` appears in the completed task prompt.
