# ADR-2026-08-26-quick-chat-tab-order: Store Quick Chat Tab Order as a User Preference

**Status:** accepted
**Date:** 2026-08-26
**Area:** backend, frontend, protocol

## Context

Quick Chat restores conversation tabs from a service list that uses recent task activity. The live
client keeps its existing order, but a page reload can return the same tabs in a different order.
Quick Terminal descriptors have their own creation sequence. Neither source can represent one user
chosen order across both tab types.

The tab strip needs a stable baseline and a portable order after a user moves a tab. The preference
must not become a second owner for tab membership or lifecycle. Existing services already own those
contracts.

## Decision

Store Quick Chat tab order in backend-owned user settings. Use one ordered reference list for each
workspace:

```text
quick_chat_tab_order_by_workspace: map<workspace-id, tab-reference[]>
```

Use `conversation:<sessionId>` for a conversation and `terminal:<tabId>` for a Quick Terminal
descriptor. The type prefix keeps the identity domains separate.

The saved list controls only presentation order. Quick Chat reconciliation continues to own
conversation membership. The Quick Terminal descriptor service continues to own terminal
membership, lifecycle, and stable terminal labels. This keeps the
[server-owned descriptor decision](2026-08-05-server-owned-quick-terminal-descriptors.md) intact.

Use creation time and task ID as the stable conversation baseline. Keep Quick Terminal `sequence`
as the stable terminal baseline. Without a saved list, show conversations first and terminals
second, in their respective baseline order.

Resolve a saved list as a partial preference. Keep known references once and in their saved order.
Ignore unknown, duplicate, stale, and cross-workspace references. Append current references that do
not appear in the saved list. Temporary setup tabs stay at the trailing edge and are not persisted.

The client applies moves optimistically and serializes partial user-settings updates. A completed
move writes the full current list of persisted references for that workspace. The existing boot and
user-settings distribution paths carry the preference to later clients. No new endpoint, database
table, or WebSocket action is required.

## Consequences

- A user-chosen mixed chat and terminal order survives reloads, browser restarts, and later clients.
- New tabs appear in a deterministic position until the user moves them.
- Stale order entries cannot recreate or retain a deleted tab.
- Concurrent clients use last accepted user-settings write semantics. They do not coordinate an
  active drag in real time.
- A failed save can leave the current client on an optimistic order. A reload returns the last
  backend-accepted order.
- Conversation listing stops using recent activity as display order. Activity still updates task
  data and does not move a tab.

## Alternatives Considered

1. **Store the order in browser storage.** Rejected because the preference must follow the user and
   browser storage would compete with backend boot state.
2. **Use conversation activity and terminal sequence.** Rejected because two independent sorts
   cannot represent one mixed user order. Activity also moves tabs without user intent.
3. **Put the mixed order in Quick Terminal descriptors or task records.** Rejected because neither
   model owns both tab types. It would mix presentation preference with membership and lifecycle.
4. **Add a dedicated relational order table.** Rejected because a bounded per-workspace preference
   does not need a new transactional lifecycle. User settings already provide portable partial
   updates and client distribution.
