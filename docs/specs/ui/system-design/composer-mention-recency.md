---
status: current
system: ui
created: 2026-08-27
requirements:
  - REQ-UI-COMPOSER-MENTION-RECENCY-001
---

# Composer Mention Recency System Design

## Purpose and boundaries

This design adds device-local selection recency to the chat composer `@` menu.
It does not change the candidate sources or their search limits.

The chat composer receives tasks from the Kanban store, prompts from settings,
and files from `workspace.files.search`. The UI system ranks these candidates
after each source returns them.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-COMPOSER-MENTION-RECENCY-001` | [Ranking](#ranking), [Selection flow](#selection-flow), [Persistence](#persistence), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- A new `apps/web/lib/chat-mention-recency.ts` module owns storage parsing,
  bounded MRU updates, candidate identity, and deterministic ranking.
- `useMentionItems` in `apps/web/components/task/chat/tiptap-input.tsx` keeps
  candidate loading in its current order. It applies the shared rank after task,
  Plan, prompt, and file candidates are available.
- `createMentionSuggestion` in
  `apps/web/components/task/chat/tiptap-suggestion.tsx` records a selection in
  the same command path for pointer, touch, Enter, and Tab.
- `MentionMenu` and `PopupMenu` keep their current rendering, focus, geometry,
  accessibility, and selected-index behavior.

## Data and contracts

The browser key is `kandev.chatMentionRecency.v1`. Its value is a JSON array in
newest-first order. The module limits the normalized array to 50 unique entries.

Each entry has this shape:

```ts
type ChatMentionRecentEntry = {
  kind: "task" | "prompt" | "file";
  id: string;
  workspaceId?: string;
};
```

Task and prompt entries use their stable IDs. File entries require the active
workspace ID and use the returned path as `id`. The module rejects unknown
kinds, empty IDs, and file entries without a workspace ID. It removes duplicate
identities and keeps the newest valid entry.

The stored data contains no task title or prompt content. File entries contain
the path that is necessary to match a later file result.

## Ranking

The rank function accepts the candidates, query, workspace ID, and normalized
MRU entries. It uses this sequence:

1. Apply the existing text filter and score to create the baseline rank.
2. Record the Plan action's index in that baseline rank, then remove the action.
3. Put known MRU candidates before candidates that are not in the MRU list.
4. For known candidates, use the newest-first MRU index.
5. Use the existing text score after recency.
6. Use the baseline index as the final stable tie-breaker.
7. Insert the Plan action at its recorded baseline index.

If the MRU list is empty, this process returns the exact baseline order. A
candidate outside the current source results does not enter the menu.

## Selection flow

1. The user enters an `@` query in a chat composer.
2. `useMentionItems` collects current tasks, Plan, saved prompts, and file search
   results.
3. The rank module reads and normalizes the current device history.
4. The composer shows the ranked candidates and selects index zero.
5. The user selects one candidate with pointer, touch, Enter, or Tab.
6. The TipTap command inserts the mention by using the existing behavior.
7. The selection callback moves the candidate identity to the MRU front.

The callback does not record Plan. It records each successful command path once.
The next query or menu open reads the updated list.

## Failure and recovery

- If browser storage is unavailable, reads return an empty history and writes do
  nothing. Mention insertion continues.
- If JSON parsing or entry validation fails, the module ignores invalid data and
  ranks with the remaining valid entries.
- If a stored candidate no longer exists, it has no rank effect. New selections
  remove stale entries through the fixed limit.
- If the workspace ID is unavailable, file candidates use baseline rank and file
  selections do not enter history. Tasks and prompts continue to use recency.

## Persistence

The browser owns the MRU list in `localStorage`. No backend API, database
migration, Zustand slice, boot payload, or WebSocket event changes.

MRU order defines recency. The design does not use wall-clock timestamps,
frequency counters, or decay. Clearing browser data resets the ranking.

## Responsive behavior

Desktop and phone use the same `TipTapInput`, rank function, and selection
callback. The current contextual popup remains the presentation on both
viewports.

This change does not add a surface, scroll owner, touch control, or breakpoint.
`PopupMenu` remains the nearest shipped mobile exemplar. Existing viewport
containment, 44-pixel rows, touch selection, and focus behavior remain valid.

A desktop E2E proves selection, primary recency rank, and reload persistence. A
phone E2E proves the same rank after touch selection in the existing popup.

## Security

The history remains inside the Kandev browser origin. The module stores only
candidate identities and file paths. It does not store prompt content, task
titles, queries, or message text.

## Observability

No production telemetry is added. Unit tests cover storage recovery, scope,
bounded updates, ranking priority, stable ties, and Plan position. Browser tests
cover the user-visible desktop and phone flows.

## Related decisions

None. The requirement and this design preserve the local ranking choice. The
feature does not create a cross-system architecture boundary.
