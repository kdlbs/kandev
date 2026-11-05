---
status: current
system: ui
requirements:
  - REQ-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001
created: 2026-10-28
owners:
  - kandev
---

# Quick Chat Active Tab Visibility System Design

## Purpose and boundaries

This strip-local presentation contract applies to all shared Quick Chat tab kinds. [Quick terminal](quick-terminal.md) owns membership, ordering and focus; [Quick Chat selection](quick-chat-selection.md) owns remembered selection. No selection, persistence, process or backend ownership moves here.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001` | Identity and geometry; responsive behavior and verification |

## Identity and geometry

`QuickChatTabStrip` in `apps/web/components/quick-chat/quick-chat-tab-strip.tsx` receives `activeKind`, `activeSessionId` and `activeTerminalTabId`. Derive a kind-qualified active identity from the effective selection, including setup IDs, so equal IDs of different kinds cannot collide. Give the existing overflow element a ref and associate each rendered tab, including the separate setup path, with its exact identity. Persisted conversation and terminal tabs are children of `SortableQuickChatTab`; preserve its DnD node ref, attributes, listeners, transforms and order. A separate wrapper or ref association must not break that ownership.

On initial mounted selection and each actual identity change, after the selected DOM is rendered, compare the strip's inner horizontal viewport (excluding borders) with the selected tab's bounds. If the full tab fits, use the smallest signed delta that brings both edges inside; if already inside, use zero. If the tab is wider than the viewport, use the label-bearing activation control's bounds instead: reveal that entire control with the smallest delta if it fits, otherwise align its leading edge to the strip's leading edge. Remaining tab actions stay reachable by ordinary manual scrolling. Clamp naturally to the strip's available scroll range; allow fractional CSS pixel tolerance in assertions. Do not rely on `scrollIntoView`, focus, document scrolling, animation, global CSS, observers or polling. A missing selection or DOM target produces no scroll and no change to selection. The trigger is identity, not changes in arrays, names, content, order or activity; manual browsing is respected on unchanged-identity rerenders. No promise is made to re-reveal after later resizing or reordering.

## Responsive behavior and verification

Keep the existing desktop floating dialog and full-height phone dialog, existing visible phone action menu and touch-sized targets. The selected content retains vertical scrolling; the tab row alone scrolls horizontally. The existing Quick Chat phone tab surface is the immediate exemplar; the curated mobile task switcher supplies the current-context and touch-accessibility principles. This bounded change retains the established tab surface because it preserves direct interaction, reorder and frequent switching; a navigator redesign is outside this contract. Existing dynamic viewport and safe-area treatment remain unchanged.

A mounted test must establish causal geometry and selection-triggered deltas, including oversized tabs, both kinds and setup, unchanged selection, focus and vertical scroll. Before production edits, an isolated real browser flow must demonstrate that an ordinary launch, setup or adjacent-close activation leaves an active item hidden on the unchanged implementation; a test-only store mutation alone does not establish that integration path. If browser focus already reveals it or no ordinary path reproduces, stop and report evidence. Focused desktop and configured `mobile-chrome` tests use genuine overflow, keyboard and touch paths, check document containment and preserve ordering and drag behavior. Capture only isolated synthetic data.

## Implementation Plans

- [Quick Chat active tab visibility](../../../plans/quick-chat-active-tab-visibility/plan.md).
