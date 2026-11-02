---
status: current
system: ui
requirements:
  - REQ-UI-CLARIFICATION-CONTEXT-001
---

# Clarification context system design

## Purpose and boundaries

UI owns this reusable context and sizing contract across clarification hosts.
Task lifecycle, response delivery, and question barriers retain their existing owners.
The [Markdown design](clarification-markdown.md) owns formatting and link safety.
This design records the issue #4810 behavior implemented by the linked plan.

## Requirement mapping

- AC-UI-CLARIFICATION-CONTEXT-001.1 through .8: Shared context transport and presentation.
- AC-UI-CLARIFICATION-CONTEXT-001.9 and .10: Host geometry and height state.
- AC-UI-CLARIFICATION-CONTEXT-001.11: Accessible desktop and phone interaction.
- AC-UI-CLARIFICATION-CONTEXT-001.12: Agent guidance.

## Shared context transport and presentation

`readQuestionContext` in `apps/backend/internal/mcp/server/handlers.go` remains authoritative.
Non-empty `context_paragraphs` takes precedence and joins paragraphs with blank lines.
Legacy `context` remains verbatim. No word-count validator, truncation, or escape inference is added.
`ClarificationInputOverlay` reads the first sorted message and shows context once above the active question.
Whitespace-only context remains absent. Carousel navigation does not repeat context.

## Host geometry and height state

`ClarificationPanelSection` separates the initial ratio from the expanded pixel maximum.
Rename `maxHeightVh` to `initialMaxHeightVh` at all three callers.
`useResizableClarificationOverlay` consumes measured host bounds, not a second viewport ratio.
Auto mode omits explicit height and uses the smaller of the initial cap and the host budget.
Manual mode uses the host budget for both CSS max-height and drag clamping.
The same computed value must govern both, so CSS cannot silently stop a valid drag.

Hosts supply explicit allocation refs and sibling reservations. Do not infer the host from arbitrary parent traversal.
Measure the allocation's intersection with the visual viewport, then subtract visible sibling controls and borders.
Task chat reserves its measured footer and existing transcript minimum. Quick Chat reserves its input and activity slot.
Needs You uses the inbox viewport and row chrome, not the growing row's own content height.
Threads uses its tile body, retains the 80px transcript floor, and subtracts the non-question footer content.
Never measure the growing clarification as its own available maximum.

Observe allocation and reserved siblings with `ResizeObserver`.
Recompute on window resize, visual viewport resize/scroll, and host scrolling that changes the visible allocation.
Remove observers and listeners on unmount. Coalesce measurements to avoid resize feedback loops.
If a host has no spare space, expansion stops at the measured bound.
Use the 120px minimum only when the host can contain it. A smaller host takes precedence over that minimum.
If the budget is temporarily unavailable, retain the bounded initial mode until measurement succeeds.

Keep desired height separate from effective clamped height. Host shrinkage must not erase user intent.
Track desktop and phone desired heights separately in component memory, keyed by the current bundle.
Collapse keeps the form mounted and preserves height. New bundles reset both height preferences.
Reset clears the current presentation's manual height. No local storage or backend preference is added.

## Accessible desktop and phone interaction

Reuse the inline pending-question surface from `mobile-clarification.spec.ts`.
This is a temporary decision inside an active conversation, so it does not require another route or stacked drawer.
Keep the context before the question and the existing submit/skip flow.
Add labelled Expand and Reset controls to the existing question header, alongside the collapse control.
These are native buttons with keyboard activation and localized accessible names.
Expand selects the available maximum. Reset restores auto sizing. Disable expansion when there is no headroom.
Fine-pointer controls retain normal desktop density. Phone and coarse-pointer targets measure at least 44px.
The existing mouse grip still supports incremental resizing and double-click reset.
Touch users use the explicit controls. This does not claim touch-drag support for the mouse-only grip.

The question's existing scroll region owns vertical scrolling when it fits the host allocation.
In constrained Threads layouts where fixed siblings exhaust the budget, the bounded footer becomes the sole vertical scroll owner.
In that mode, remove the inner vertical scroll trap and preserve the existing footer containment and 80px transcript floor.
Do not introduce a document scroller or two competing active scrollers within the question surface.
Table wrappers own only horizontal overflow. Preserve dynamic viewport and safe-area rules from the host.
Keep custom input and Submit reachable after keyboard viewport contraction.
At desktop-to-phone transitions, derive phone geometry without rewriting desktop desired height or answers.

## Agent guidance

Update `registerInteractionTools` in `apps/backend/internal/mcp/server/server.go`.
The tool description and both context-field descriptions carry the concise-context recommendation.
Retain the paragraph precedence and verbatim-storage descriptions.
Update `userQuestionSection` in `apps/backend/internal/sysprompt/sysprompt.go` with the same guidance.
Detailed analysis belongs before the tool call. The hard user-input barrier still applies after that call.
Parent-question tools, no-question profiles, and external permissions retain their current guidance.
Tests cover the conditional prompt profiles as well as the published tool schema.

## Failure, security, and persistence

Long context remains accepted and fully readable through scrolling.
Unsafe links remain passive. Reference activation must not trigger parent panel handlers or answer actions.
No schema migration, API shape, telemetry, or clarification lifecycle change is required.
Do not log synthetic or real context to diagnose geometry.

## Verification

Unit tests cover separate caps, observer updates, collapse/reset, and responsive state preservation.
Browser tests must measure actual bounds after pointer gestures and viewport changes.
Cover task chat, Quick Chat, Needs You, Threads, keyboard expansion, and phone touch completion.
Use synthetic content only. Physical software-keyboard behavior remains a hardware check beyond viewport emulation.

## Delivery and related contracts

- [Plan](../../../plans/clarification-context-usability/plan.md).
- [Requirements](../requirements/clarification-context.md).
- [Threads footer design](threads-saved-views.md).
- [Question barrier](../../tasks/requirements/user-question-turn-boundary.md).
- [Current-turn ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md).

The existing parser and host ownership remain authoritative. Local policy and geometry changes do not require a new ADR.
