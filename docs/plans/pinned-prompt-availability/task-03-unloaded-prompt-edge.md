---
id: "03-unloaded-prompt-edge"
title: "Classify the pinned prompt edge without a rendered row"
status: done
wave: 3
depends_on:
  - "01-resolve-last-prompt-outside-window"
  - "02-load-unloaded-last-prompt"
plan: "plan.md"
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
acceptance_criteria:
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.3
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.4
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.9
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
---

# Task 03: Classify the pinned prompt edge without a rendered row

## Summary

Give the transcript's edge tracker a classification for a resolved last prompt
that has no rendered row, so the anchored bar opens and the control points
upward when the prompt precedes every rendered row, stays closed and points
downward when the prompt follows every rendered row, and keeps the control
available in the residual case.

## In scope

- Pure (and exported, so `message-list-shared.test.tsx` can assert it directly)
  `resolveUnloadedPromptEdge` and `findPromptNeighbors` in `message-list-shared.tsx`
  beside `resolveLastPromptEdge`, with the same two-pixel tolerance; the walk lives here because it resolves row keys through
  `getItemKey`, which this module owns, so the lib module stays free of component
  imports.
- `useTranscriptEdgeTracking` in `message-list-native.tsx` taking the resolved
  last-prompt message, the panel's unloaded flag, and the rendered transcript
  items (`RenderItem[]`), and classifying only when the prompt has no row element
  and the flag is set. The hook is exported for its suite, following the
  `useScrollToDividerOrBottom` precedent already in that module, because
  `message-list-native.test.tsx` mounts hooks rather than the list component.
- Exporting and reusing `findMessageRow` from `message-list-native-scroll.ts` for
  the prompt row and the neighbor keys the walk returns, so both lookups are
  container-scoped and a second mounted transcript's copy of the prompt row cannot
  supply this container's geometry.
- `MessageList` prop plumbing from `task-chat-panel.tsx` for the resolved prompt
  and the unloaded flag.

## Out of scope

- Resolving the last prompt (Task 01) and loading an unloaded prompt (Task 02).
- The loaded prompt's measurement path, the anchored bar's rendering, and the
  directional contract itself.
- Any change to `firstMessageId` tracking for the scroll-to-start affordance.

## Acceptance

- With the prompt row absent and the panel's unloaded flag set, classification
  follows the design's order: no rendered item holds a message on either side
  yields `visible`; the prompt following every rendered row (no newer bound, since
  a newer bound is the first item holding any newer message) yields `below`, so
  the bar stays closed and the control points down; the prompt preceding every
  rendered row (no older bound) yields `above`, so the bar presents the prompt and
  the control points up.
- Bounds are by containment, not strict ordering: an item holding any message on a
  side bounds that side, so a `turn_group` whose messages straddle the prompt
  supplies both bounds and one row can be both. An item is unordered only when it
  holds at least one non-synthetic message without a parseable `created_at`, and
  the synthetic task-description row is excluded by identity first, so the
  fallback row never makes the walk inconclusive. The walk's prompt is always
  orderable by contract: it only runs for an unloaded prompt, and an unloaded
  prompt is a projection entry, which the prompt cache admits only with a parseable
  `created_at`; an unparseable window prompt is a rendered prompt whose edge comes
  from `resolveLastPromptEdge`. The walk therefore returns the two bound keys plus an
  `unordered` term, because a bound key names a row and cannot carry that, and the tracker
  maps `unordered` with either bound missing to `visible` before it classifies; a missing
  bound with no unordered item present keeps the `above`/`below` outcomes above.
- With rendered rows on both sides, an older bound fully above the container and a
  newer bound fully below it classify as `visible` (the prompt's slot is inside
  the viewport range), a newer bound fully above the container classifies as
  `above`, an older bound fully below it classifies as `below`, and any other
  combination classifies as `visible`.
- Neighbors are rendered rows, not messages: `findPromptNeighbors` walks the
  render items, bounds a `turn_group` item by its messages, and returns the row
  keys (`getItemKey`) the tracker measures through the exported container-scoped
  `findMessageRow` (`message-list-native-scroll.ts`), so a grouped-only window
  still classifies from its group row and a duplicate `msg-<id>` row in a second
  mounted transcript cannot supply this container's rects. The prompt row itself is
  resolved the same way, so a second transcript that renders the prompt row while
  this container does not (the prompt is unloaded here) cannot supply its geometry
  and cannot bypass `resolveUnloadedPromptEdge`. Both lookups run per
  measurement with the rendered item list as an effect input, so a prompt id that
  resolves before the rows render is re-classified once they do. Without the
  unloaded flag a missing row reports `visible` as today, an unresolved prompt
  reports `visible`, and the loaded-prompt classification of a row this container
  holds is unchanged.

## ASCII UI preview

`UI-01: Desktop transcript, last prompt outside the loaded window` (planned
view; full preview in [plan.md](plan.md#ui-01-desktop-transcript-last-prompt-outside-the-loaded-window)):

```text
+---------------------+--------------------------------------------+----------+
| sidebar             | [chat] [Changes]                           | right    |
|                     +--------------------------------------------+          |
|                     | [^] LAST-PROMPT-MARKER-9F2Q please handle  | <- added by
|                     |     this scrolled-past regression ... [v]  |    this task
|                     +--------------------------------------------+          |
|                     |  tool call: read file ...                  |          |
|                     |  filler message 120 ...                    |          |
+---------------------+--------------------------------------------+----------+
```

Fixed region: the anchored bar, pinned below the view-tab row, overlaying the top
of the scrolling region and reserving `--anchored-bar-h`. Scrolling: the
transcript body (`.chat-message-list`); the bar never scrolls with it. The bar
shown here is this task's classification plus Task 01's gate.

Phone (`UI-02`; the phone keeps one inline composition): the anchored bar is
desktop-only, so this task changes nothing visible on the phone, and the
`above`/`below` classification is not rendered there. The phone check is the
bar's absence with `show_anchored_prompt_bar` enabled, which keeps the phone AC
from passing vacuously.

```text
+----------------------------------+
| < Task                           |  fixed: task chrome
+----------------------------------+
| tool call: read file ...         |  scrolling: .chat-message-list
| filler message 120 ...           |
| (no user prompt row is loaded)   |
+----------------------------------+
| (no anchored bar on phone)       |  fixed: composer row
| [ composer ]   [^ last prompt]   |
+----------------------------------+
```

ACs: `AC-UI-PINNED-PROMPT-AVAILABILITY-001.3` (bar present with the upward
direction for a prompt that precedes every rendered row) and `.9` (absent while
the window is empty).
Targeted rendered check: Task 04's desktop pass asserts the bar open with the
prompt's stored text and the upward arrow (the seeded prompt is older than every
rendered row), and the phone pass asserts no bar with the setting enabled.
`AC-UI-PINNED-PROMPT-AVAILABILITY-001.4` (the prompt following every rendered
row: bar closed, control pointing down) has no browser state in this package, so
it is evidenced by unit tests exactly as the plan's Tests table maps it — the
`below` truth-table row in
`apps/web/components/task/chat/message-list-shared.test.tsx` and the tracker case
in `apps/web/components/task/chat/message-list-native.test.tsx`.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-native-scroll.test.ts components/task/task-chat-panel.last-prompt.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/message-list-shared.tsx components/task/chat/message-list-native.tsx components/task/chat/message-list-native-scroll.ts components/task/task-chat-panel.tsx components/task/chat/message-list-native.test.tsx)
```

## Files likely touched

- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-shared.test.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/task-chat-panel.tsx`

## Dependencies

- Task 01 supplies the resolved prompt message the tracker classifies, and Task 02
  supplies the jump the classification describes: without Task 02 the bar can open
  for an unloaded prompt that the control cannot reach, so this work order depends
  on both.

## Risks

- The tracker effect re-runs on its id dependencies; an unloaded prompt must
  still re-classify on scroll and container resize, and must re-observe nothing
  for the missing row.
- Neighbors must come from rendered rows (`RenderItem`s), not from messages with
  individual rows: a message collapsed into a `turn_group` has no `msg-<id>`
  element, so a message-keyed walk would find no neighbor in a tool-heavy window
  and leave the bar closed where the prompt precedes every rendered row. Bounds
  must be by containment so a straddling group still bounds the prompt on both
  sides; the tracker test must include a window that is a single turn group, and
  `message-list-shared.test.tsx` (which owns `findPromptNeighbors`) a straddling group plus an
  item holding a non-synthetic message
  without a parseable `created_at` (mixed and all-unparseable both count as
  unordered → inconclusive → `visible`), which is the term the walk returns and the
  tracker consumes before classifying.
- The rendered item list must be an effect input, not only the prompt id: in the
  target scenario the prompt id resolves from the projection read while the rows
  arrive from the session fetch, so a single capture per effect run would report
  `visible` forever and the bar would never open. A tracker test renders the
  prompt with no rows, then rerenders with rows, and asserts the new
  classification.
- Both lookups must be container-scoped: the prompt row through
  `findMessageRow(scrollRef.current, lastPromptMessageId)` and the neighbor keys
  through `findMessageRow(scrollRef.current, key)`. Today the prompt row uses a
  document-global `getElementById`, which a second mounted transcript can satisfy
  with its own copy of that row (a different panel, dockview split, or thread
  view), so the first panel would classify from the foreign row's geometry and skip
  `resolveUnloadedPromptEdge` entirely. The two-transcript tests therefore place a
  foreign prompt row whose geometry would force a different edge, one with the
  prompt row alone in the foreign transcript and one duplicating a **neighbor's**
  id, each positioned where a document-global lookup classifies this container
  differently, or neither test can fail against the old lookup. Existing
  loaded-prompt edge tests must stay green.
- The classification must stay behind the panel's unloaded flag: classifying a
  cached-but-unrendered prompt by geometry would re-introduce a control whose
  activation cannot land.
- `message-list-native.test.tsx` must control container and row geometry explicitly,
  because the test environment reports zero-sized rects by default, and it must install
  a local controllable `ResizeObserver` stub: the tracker constructs a real observer and
  happy-dom's is a no-op that never fires, so only a controllable stub can drive a
  resize. The named cases re-classify from the rendered-item effect input and need
  stubbed rects for their geometry assertions.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/system-design/pinned-prompt-availability.md` section
  "Edge classification".
- `docs/specs/ui/requirements/last-prompt-pinning-regressions.md` for the
  directional contract the classification must satisfy.
- Existing patterns: `useTranscriptEdgeTracking`,
  `resolveLastPromptEdge`, `resolveLastPromptControls`.

## Results

Verified: prompt-edge, rendered tracker, and panel tests passed in the 23-file
focused suite (360 tests total); web typecheck passed.
