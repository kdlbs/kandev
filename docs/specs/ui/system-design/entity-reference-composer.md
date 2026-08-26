---
status: draft
system: ui
requirements:
  - REQ-UI-ENTITY-REFERENCE-COMPOSER-001
created: 2026-07-21
updated: 2026-08-24
owners:
  - kandev
---
# Entity Reference Composer System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-ENTITY-REFERENCE-COMPOSER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-ENTITY-REFERENCE-COMPOSER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Users discuss external work items in agent chat, but plain ticket keys and pull-request numbers are ambiguous and easy to mistype. They need one fast way to search the active workspace's connected systems and insert a durable reference that both people and agents can resolve. Kandev tasks already have the established `@` mention path.

## What

- Task chat and Quick Chat support a `#` entity-reference trigger in their shared TipTap composer. Passthrough, task creation, comments, plans, Office text inputs, and other editors remain unchanged.
- Typing `#` at the start of a text block or after whitespace opens a search menu directly above the composer. Its shared geometry and visible-viewport containment follow the [composer suggestion overlay requirements](../requirements/composer-suggestion-overlays.md). Its rendered bottom edge stays anchored to a composer anchor inside the visible viewport even when only a short result set is visible; an anchor below that viewport is clamped to its padded bottom edge. A `#` inside another token or a code block remains literal text.
- A trigger starts only when the user enters a new `#` character. Pasted, dropped, restored, or programmatically inserted text remains literal and does not open the menu.
- After the user types at least one query character, search covers the active workspace's connected, searchable sources:
  - Jira tickets;
  - Linear issues;
  - GitHub issues and pull requests;
  - GitLab issues and merge requests;
  - Azure DevOps work items and pull requests; and
  - Sentry issues.
- Results respect the provider configuration and scope owned by the active workspace. Kandev tasks are excluded even if a stale or misbehaving search response contains a `kandev_tasks` group.
- Results are grouped by provider and work-item kind. Provider order and ordering within each group are deterministic; Kandev does not present unrelated provider scores as globally comparable relevance. Sources that are not configured or cannot search the active workspace are omitted from the menu rather than rendered as empty unavailable sections.
- Search groups carry provider-neutral display descriptors. Composer, message, and queue code use normalized fields and a generic fallback rather than exhaustive switches over native integrations, so a future plugin bridge can supply the same contract.
- Search is debounced, cancels or ignores stale requests, and caps work per provider and configured workspace/project scope. Each connected provider continues examining available in-scope results until it finds the requested number of matches, has no more results, reaches a search bound, or the request is canceled. One disconnected, slow, rate-limited, or failing provider never hides successful results from other providers.
- Cancellation of a superseded search is a normal client outcome. The backend
  does not report it as an internal server failure.
- Arrow Up and Arrow Down move the active result. Tab or Enter selects it. Pointer or touch selection has the same behavior.
- Escape closes the menu without changing or sending the draft. Continued input in the dismissed range cannot reopen it. A new valid `#` entry can open it again.
- A space immediately after a bare `#` closes the menu and leaves the text literal. Spaces after a query character remain part of a multi-word search.
- If no result is selectable, Enter follows the configured composer behavior. The menu closes when the cursor leaves the trigger range.
- Selection replaces only the active `#query` range with an inline reference chip, appends a trailing space, keeps focus in the composer, and never submits or queues the message.
- A chip displays a stable key when one exists (for example `#ENG-123` or `#123`) and otherwise a concise title. It retains a title snapshot and source label for disambiguation.
- Submitted references remain clickable in sent user messages through a canonical HTTP(S) URL whose origin matches the workspace's validated provider configuration.
- Visible message content serializes each chip to a portable Markdown link. Structured reference metadata is persisted with direct and queued user messages so rendering and agent context never depend on parsing a mutable title.
- Draft storage preserves the structured chip. Recalling a sent message rehydrates links produced by this feature into chips; ordinary Markdown links remain ordinary links.
- If a referenced item is later renamed, deleted, disconnected, or inaccessible, the sent message keeps its label snapshot and Markdown fallback. Opening the target may then show the destination's normal unavailable behavior.
- `@` remains the context-attachment trigger for Kandev tasks, files, saved prompts, and the current plan. `#` is reserved for external work items.
- Provider titles, URLs, keys, and error text are untrusted. Kandev validates reference shapes and destinations, escapes provider query languages server-side, and sanitizes agent-facing context.
- Each registered `(provider, kind)` owns its destination/scope authorizer. The same provider-owned rule filters search hits and authorizes direct or queued submission against the workspace derived from the persisted conversation; unknown or ambiguous providers fail closed.

## Data model

`EntityReference` is a versioned value stored in `messages.metadata.entity_references` and, while queued, `queued_messages.metadata_json.entity_references`. No new table is introduced.

| Field      | Type              | Contract                                                                                                                                                                                                                                                                                                           |
| ---------- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `version`  | integer           | `1` for this contract. Unknown versions render through visible Markdown only.                                                                                                                                                                                                                                      |
| `ref`      | string            | Canonical opaque identity, unique within one provider connection and kind.                                                                                                                                                                                                                                         |
| `provider` | namespaced string | Stable registry-owned provider identity. Built-ins use `jira`, `linear`, `github`, `gitlab`, `azure_devops`, or `sentry`. A built-in migrated to a plugin keeps its existing ID (or an explicit compatibility alias); a brand-new plugin contribution uses the reserved `plugin:<plugin-id>:<provider>` namespace. |
| `kind`     | string            | Additive work-item kind. Built-ins use `issue`, `pull_request`, `merge_request`, or `work_item`; unknown future values use generic presentation.                                                                                                                                                                   |
| `id`       | string            | Provider-native immutable ID, or documented provider-native composite identity where no single immutable ID exists.                                                                                                                                                                                                |
| `key`      | string, optional  | Human-facing identifier such as `ENG-123` or `123`.                                                                                                                                                                                                                                                                |
| `title`    | string            | Sanitized display snapshot captured at selection time.                                                                                                                                                                                                                                                             |
| `url`      | string            | Validated canonical provider URL.                                                                                                                                                                                                                                                                                  |
| `scope`    | string            | Non-secret provider connection/site/instance scope needed to prevent identity collisions.                                                                                                                                                                                                                          |

Metadata is deduplicated by `ref` in first-appearance order. It never contains credentials, provider query syntax, or raw upstream error bodies.

The registry, not an adapter, assigns `provider` and constructs `ref` from validated provider registration, connection scope, kind, and immutable resource identity. Native and future plugin candidates are untrusted input: Kandev bounds counts and field lengths and validates UTF-8, kind, identity, and URL before exposing or persisting a result.

The editor's `entityReference` atom stores the same presentation and identity fields in local draft JSON. Visible serialization is `[#<key-or-title>](<url>)` with Markdown-safe text and URL encoding.

## API surface

### Search

`GET /api/v1/workspaces/:workspaceId/mentions/search?q=<plain text>&limit=<per-source limit>`

- `workspaceId` is required and must identify the active task or Quick Chat workspace.
- `q` is plain user text, 1-200 Unicode characters after trimming. Provider query languages are not accepted.
- `limit` defaults to 5 and is clamped to 1-10 per source.
- A successful aggregate request returns HTTP 200 even when individual sources fail:

```json
{
  "query": "auth",
  "groups": [
    {
      "source": "linear_issues",
      "provider": "linear",
      "kind": "issue",
      "display_name": "Linear",
      "kind_label": "Issue",
      "status": "ok",
      "results": [
        {
          "version": 1,
          "ref": "mention:v1:linear:issue:acme:issue-uuid",
          "provider": "linear",
          "kind": "issue",
          "id": "issue-uuid",
          "key": "ENG-123",
          "title": "Fix authentication",
          "url": "https://linear.app/acme/issue/ENG-123",
          "scope": "acme"
        }
      ]
    }
  ]
}
```

- Group status is one of `ok`, `not_configured`, `unauthorized`, `rate_limited`, `timeout`, `upstream_error`, or `unsupported_scope`.
- `source` is an opaque stable source ID. `display_name` and `kind_label` let the UI render a provider without importing its native model; unknown providers use a generic work-item icon.
- Sources with `not_configured` or `unsupported_scope` status are omitted from the menu, but their safe status remains available in the response. Transient failures from configured sources may be shown non-disruptively. Raw provider errors are never returned.
- Invalid query/limit returns 400. An unknown workspace returns 404. A request
  canceled by the client returns 499. Aggregate infrastructure failure returns
  500, and every 500 has one corresponding structured backend error record.
- Search diagnostics can include the workspace ID, stable failure stage, source,
  provider, kind, and safe status. They never include the query, prompt,
  credentials, tokens, or raw provider payloads. Provider-level failures remain
  represented as safe groups in an HTTP 200 aggregate response.

### Message submission

The existing `message.add`, `message.queue.add`, and `message.queue.update` payloads accept optional `entity_references: EntityReference[]`. For `message.queue.update`, omitting `entity_references` preserves existing reference metadata, an explicit empty array clears it, and a non-empty array replaces it after validation. Deleting a link from edited queue text cannot leave stale reference metadata behind when the update supplies the replacement array.

Message responses and queue entries expose the validated array through `metadata.entity_references`. Queue drain copies it to the created user message unchanged apart from canonical validation and deduplication.

## State machine

1. **Closed** — no active valid `#` trigger.
2. **Primed** — the menu is open for bare `#` and prompts the user to type; no external search runs.
3. **Searching** — a debounced request for the current query is in flight.
4. **Results** — one or more provider groups contain selectable results; partial provider failures may also be shown non-disruptively.
5. **Empty** — all successful providers returned no hits and no request is in flight.

Only direct entry of a valid `#` moves Closed to Primed. Paste, drop, draft restore, history recall, and programmatic content changes keep the menu Closed.

Typing updates Primed/Searching/Results/Empty. Escape dismisses the active trigger and returns to Closed. Later input in that range cannot reopen it.

Moving outside the trigger range, entering space after a bare `#`, selection, session change, or composer unmount returns to Closed. A new valid `#` can start a new trigger.

A response whose query generation is no longer current has no state transition.

## Permissions

- Search is workspace-scoped. A provider adapter may use only credentials, instances, repository scope, projects, and operational settings associated with `workspaceId`, following [ADR 0030](../../../decisions/0030-workspace-scoped-integration-settings.md).
- The backend validates that the target task or Quick Chat session belongs to the same workspace as submitted provider scopes.
- Search results and message metadata expose no credentials or secret identifiers.
- Opening a reference uses the destination's existing authorization behavior; selection does not grant new provider access.

## Failure modes

- A provider timeout, auth failure, rate limit, malformed upstream response, or unsupported workspace scope produces a safe group status while other groups remain selectable.
- A canceled request stops provider work and returns 499. It is not logged as an
  unexpected HTTP 500.
- A workspace repository failure or another unexpected aggregate search failure
  returns 500 only after the handler writes one structured error with the safe
  workspace and failure-stage context.
- If the aggregate endpoint is unreachable, the menu preserves the draft and shows a retryable error state. It never submits the draft.
- Stale responses after continued typing, session changes, or menu dismissal are ignored.
- Invalid or unsafe returned URLs are discarded. URLs must use HTTP(S) and match the configured provider origin; internal routes are rejected.
- Provider query text is escaped inside provider adapters. User text never becomes raw JQL, WIQL, GitHub/GitLab qualifiers, or Sentry query syntax.
- Malformed submitted reference metadata is rejected or omitted from structured metadata without executing or resolving its URL. The visible Markdown text remains ordinary user content.

## Persistence guarantees

- Composer drafts preserve entity-reference atoms through the existing per-session local draft JSON.
- Direct messages and queued messages preserve normalized reference metadata in their existing JSON metadata columns across backend restarts.
- Sent visible content preserves a portable Markdown-link fallback even if metadata is missing or a future client does not understand its version.
- Search results, in-flight requests, provider health snapshots, and typeahead caches are transient and need not survive restart.

## Scenarios

- **GIVEN** task chat in workspace A with connected Jira and GitHub providers, **WHEN** the user types `#auth`, **THEN** the menu above the composer shows grouped workspace-A Jira and GitHub hits without Kandev tasks or workspace-B results.
- **GIVEN** a draft that contains `#2730 explicit profile overridden`, **WHEN** the user pastes the draft into task chat, **THEN** the text remains literal and no entity-reference menu or search request starts.
- **GIVEN** a closed menu after pasted, restored, or programmatically inserted hash text, **WHEN** the user types a new valid `#auth` trigger, **THEN** the menu opens for the new trigger.
- **GIVEN** an open entity-reference menu, **WHEN** the user presses Escape and continues to type in the same range, **THEN** the draft stays unchanged and the menu stays closed.
- **GIVEN** a primed menu for a bare `#`, **WHEN** the user types a space, **THEN** the menu closes and the literal `# ` remains in the draft.
- **GIVEN** an active multi-word query and no selectable result, **WHEN** the user presses Enter, **THEN** the composer follows its configured Enter behavior and the menu does not stay active outside the trigger range.
- **GIVEN** a search returns only one short result group, **WHEN** the menu renders, **THEN** its bottom edge remains directly anchored to the composer instead of reserving unused menu height.
- **GIVEN** GitLab is not configured or cannot search the active workspace, **WHEN** another source returns results, **THEN** the menu omits GitLab rather than showing an unavailable GitLab section.
- **GIVEN** Quick Chat and a matching Linear issue, **WHEN** the user presses Arrow Down and Tab, **THEN** the active range becomes one inline reference chip, focus stays in the composer, and no message is sent.
- **GIVEN** the menu is open on a phone viewport, **WHEN** the user taps a result, **THEN** the same chip is inserted and every required row is touch reachable without horizontal document overflow.
- **GIVEN** GitHub times out while Jira returns hits, **WHEN** search finishes, **THEN** Jira results remain selectable and GitHub exposes only a safe timeout state.
- **GIVEN** the user types another character before an earlier request returns, **WHEN** the older response arrives last, **THEN** only results for the newest query are displayed.
- **GIVEN** the browser cancels a superseded search request, **WHEN** the backend
  observes cancellation, **THEN** provider work stops and the request completes
  as 499 without an internal-server-error record.
- **GIVEN** workspace validation or aggregate search fails unexpectedly, **WHEN**
  the endpoint returns 500, **THEN** one structured backend error identifies the
  workspace and stable failure stage without recording the query or provider
  secrets.
- **GIVEN** one provider search fails while another succeeds, **WHEN** aggregation
  completes, **THEN** HTTP 200 contains a safe status for the failed provider and
  retains the successful results.
- **GIVEN** a connected provider's first available results do not match the query but later in-scope results do, **WHEN** the user searches, **THEN** matching results are returned up to the requested per-source limit unless the provider has no more results, a workspace/project search bound is reached, or the request is canceled.
- **GIVEN** a selected external reference followed by ordinary text, **WHEN** the user explicitly sends, **THEN** the visible message contains a clickable chip/Markdown fallback and its stable identity is stored in message metadata and supplied to the agent as sanitized reference context.
- **GIVEN** the agent is busy, **WHEN** the user queues a message containing two references, **THEN** both references survive queue display, backend restart, drain, and sent-message rendering.
- **GIVEN** the user edits a queued message and removes one generated reference link, **WHEN** the edit is saved, **THEN** metadata no longer contains the removed reference.
- **GIVEN** a queued message already has reference metadata, **WHEN** an update omits `entity_references`, **THEN** the existing metadata is preserved.
- **GIVEN** a queued message already has reference metadata, **WHEN** an update supplies an empty `entity_references` array, **THEN** the existing metadata is cleared.
- **GIVEN** a queued message already has reference metadata, **WHEN** an update supplies a valid non-empty `entity_references` array, **THEN** the validated array replaces the existing metadata.
- **GIVEN** a sent reference, **WHEN** the user recalls the message into an empty composer, **THEN** generated reference Markdown becomes an editable reference chip with the same identity.
- **GIVEN** task chat with a sibling Kandev task, **WHEN** the user types `@` and searches its title, **THEN** the existing task suggestion remains available and sends through the established task-context behavior.
- **GIVEN** passthrough mode, task creation, or a comment textarea, **WHEN** the user types `#auth`, **THEN** the text remains literal and no entity-reference search opens.
- **GIVEN** a provider title containing newlines, angle brackets, or a fake system tag, **WHEN** the reference is sent, **THEN** agent context remains within one sanitized Kandev system block.

## Out of scope

- Kandev tasks, people, agents, files, prompts, plans, branches, commits, or arbitrary web-page references under `#`.
- Slack message search; Slack has no issue-like reference contract in this release.
- Task creation, comments, plan editing, Office text inputs, passthrough, and terminal inputs.
- Creating, importing, transitioning, closing, or otherwise mutating a selected work item.
- Raw provider query-language syntax, advanced filters, user-controlled result pagination, or a full-screen global search page.
- Implementing the plugin manifest contribution, workspace permission/grant, Kandev-to-plugin search RPC, or loading plugin-provided sources in this release. The normalized registry and DTO MUST remain transport-neutral so a later plugin bridge can implement the same provider contract without changing composer/message formats. Search-provider contribution is distinct from the plugin-to-Kandev Host data API and its `api_read` capabilities.
- Cross-workspace search.
- Changing the Enter or Tab selection behavior when a result is selectable.
- Removing spaces from valid multi-word work-item searches.

## Implementation plans

- [Entity reference composer](../../../plans/entity-reference-composer/plan.md)
- [Backend failure containment](../../../plans/backend-failure-containment/plan.md)
- [Entity reference trigger lifecycle](../../../plans/entity-reference-trigger-lifecycle/plan.md)
