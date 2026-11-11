---
status: current
system: ui
updated: 2026-11-03
requirements:
  - REQ-UI-CLARIFICATION-CONTEXT-001
---

# Clarification context system design

## Purpose and boundaries

UI owns this reusable context and sizing contract across clarification hosts.
Task lifecycle, response delivery, and question barriers retain their existing owners.
The [Markdown design](clarification-markdown.md) owns formatting and link safety.
The context transport and host geometry retain the issue #4810 behavior.
The 2026-11-03 amendment replaces the size controls and adds a Restore target.

## Requirement mapping

- AC-UI-CLARIFICATION-CONTEXT-001.1 through .8: Shared context transport and presentation.
- AC-UI-CLARIFICATION-CONTEXT-001.9 and .10: Host geometry and height state.
- AC-UI-CLARIFICATION-CONTEXT-001.11: Accessible desktop and phone interaction.
- AC-UI-CLARIFICATION-CONTEXT-001.12: Agent guidance.
- AC-UI-CLARIFICATION-CONTEXT-001.13 through .16: Single size control, restoration, and manual resize transitions.

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
`useClarificationResizeState` in `clarification-draft-context.tsx` owns desktop and phone sizing for the current bundle.
`ClarificationDraftProvider` retains the existing task-local, eight-bundle cache across responsive remounts.
Extend that sizing record to retain each presentation's mode and Restore target together.
The hook's standalone fallback uses the same shape when no provider exists.
Collapse keeps the form mounted and preserves the complete sizing record. New bundles start in auto mode.
Task changes and provider disposal retain their existing cache-reset behavior.
No local storage or backend preference is added.

### Sizing transitions

Each desktop or phone record has one of three states:

- `auto`: no explicit height. The existing initial cap and host budget bound the content.
- `manual`: a desired pixel height. Existing host clamping determines the rendered height.
- `expanded`: a saved normal state (`auto` or `manual`). The measured host maximum determines the rendered height.

Store the expanded mode and its saved normal state in one record, through one state update.
Do not derive expanded mode from pixel equality or `canExpand`, because host contraction and manual dragging can produce equal heights.
The two-pixel headroom tolerance in `useResizableClarificationOverlay` remains the expansion eligibility check.

| Input | Current state | Result |
| --- | --- | --- |
| Expand with measured headroom | auto or manual | Save the normal state and enter expanded mode. |
| Expand without measured headroom | auto or manual | Keep state. Disable the visible Expand button. |
| Restore | expanded | Return to the saved normal state and discard that saved target. |
| Host resize | any | Recompute effective geometry without changing the stored mode or normal height. |
| First actual mouse drag movement | any | Enter manual mode at the clamped drag height and discard an earlier Restore target. |
| Grip double-click | any | Return the current presentation to auto mode and clear its Restore target. |
| Collapse and reopen | any | Preserve the state and Restore target. |
| Phone/desktop switch or responsive remount | any | Read that presentation's record from the existing bundle cache. |
| New bundle or complete sizing reset | any | Start auto mode without a Restore target for the affected records. |

Restore uses the saved desired manual height, rather than its temporarily clamped rendered height.
When the host grows again, that saved manual preference becomes effective up to the new host bound.
Expanded mode follows both host growth and contraction. It never replaces the Restore target during remeasurement.
When measurement is unavailable, use the existing bounded initial fallback without discarding expanded mode.
Restore remains enabled in expanded mode, including while measurement is unavailable or the host has no headroom.
An expanded record with an auto Restore target is meaningful and must remain in the bounded cache.
The cache eviction predicate must distinguish that state from an empty pair of auto records.

`resetHeight` remains the grip's auto-sizing action. `resetAllHeights` clears both presentation records at the existing lifecycle boundary.
These resets are distinct from the header's Restore action.
No mouse movement means no drag transition. A grip click alone must preserve the Restore target.

## Accessible desktop and phone interaction

Reuse the inline pending-question surface from `mobile-clarification.spec.ts`.
This is a temporary decision inside an active conversation, so it does not require another route or stacked drawer.
Keep the context before the question and the existing submit/skip flow.
Replace the separate Expand and Reset controls in `ClarificationHeaderActions` with one native icon button.
Use `IconArrowsMaximize` for Expand and `IconArrowsMinimize` for Restore.
Both icons describe panel sizing, distinct from the existing collapse chevron and skip cross.
The fixed control position does not change between modes. Existing conditional Submit stays in its current order.
Use localized `Expand` and `Restore` for both the accessible name and the existing Tooltip primitive.
The tooltip explains the same action as the accessible name and does not contain additional required instructions.
It opens on keyboard focus and pointer hover. Touch activation runs the action directly without another sheet or menu.
Use `aria-controls` for the panel content. Preserve keyboard focus on the same button after a toggle.
Keep collapse's `aria-expanded` independent from the sizing state.

Pass a single toggle handler, explicit expanded mode, and availability through `ClarificationPanelSection` and `ClarificationInputOverlay`.
Use a stable `clarification-panel-size-toggle` test ID across both states.
Migrate every old expand/reset selector, including Coordinator and Threads tests, in the same work order.
Remove size labels from the persistent header. Remove unused translation keys across all catalogs after the caller search.
Add new labels to English and the six translated catalogs, then regenerate Traditional Chinese and pseudo values through repository tooling.
Fine-pointer buttons measure 28px. Phone and coarse-pointer targets measure at least 44px in both dimensions.
Keep the inline phone hierarchy and existing multi-question header wrapping. Do not introduce a separate phone route or drawer.
The existing mouse grip still supports incremental resizing and double-click reset.
Touch users use the explicit controls. This does not claim touch-drag support for the mouse-only grip.

The question's existing scroll region owns vertical scrolling when it fits the host allocation.
In constrained Threads layouts where fixed siblings exhaust the budget, the bounded footer becomes the sole vertical scroll owner.
In that mode, remove the inner vertical scroll trap and preserve the existing footer containment and 80px transcript floor.
Do not introduce a document scroller or two competing active scrollers within the question surface.
Table wrappers own only horizontal overflow. Preserve dynamic viewport and safe-area rules from the host.
Keep custom input and Submit reachable after keyboard viewport contraction.
At desktop-to-phone transitions, derive phone geometry without rewriting desktop mode, Restore target, or answers.

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

Unit tests cover separate caps, observer updates, normal/expanded transitions, Restore, and responsive state preservation.
Cover auto and manual Restore targets, repeated cycles, manual-at-maximum, dragging after expansion, grip reset, and bundle replacement.
Provider tests cover expanded-auto records across remounts, task isolation, and the existing eight-bundle eviction bound.
Browser tests must measure actual bounds after pointer gestures and viewport changes.
Cover task chat, Quick Chat, Needs You, Threads, keyboard expansion, and phone touch completion.
Assert one size button, state-specific accessible names, focus continuity, tooltips, and phone hitbox width and height.
Prove that toggling preserves a draft and never sends or skips an answer.
Measure the control at 767px/768px fine-pointer widths and at a coarse-pointer tablet width.
Use synthetic content only. Physical software-keyboard behavior remains a hardware check beyond viewport emulation.

## Delivery and related contracts

- [Plan](../../../plans/clarification-context-usability/plan.md).
- [Single size control plan](../../../plans/clarification-size-toggle/plan.md).
- [Requirements](../requirements/clarification-context.md).
- [Threads footer design](threads-saved-views.md).
- [Question barrier](../../tasks/requirements/user-question-turn-boundary.md).
- [Current-turn ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md).
- [Answer draft and local sizing ownership](../../../decisions/2026-11-03-clarification-answer-draft-ownership.md).

The existing parser and host ownership remain authoritative. Local policy and geometry changes do not require a new ADR.
