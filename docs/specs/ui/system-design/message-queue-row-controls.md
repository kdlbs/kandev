---
status: current
system: ui
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-001
---

# Message Queue Row Controls System Design

## Purpose and boundaries

The message queue panel renders each pending message as a compact row on desktop and phone surfaces. This design owns the row's adaptive disclosure and destructive-action presentation. Queue ordering, provenance, editing permissions, merge eligibility, persistence, and removal transport remain governed by the existing message queue contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-MESSAGE-QUEUE-MANAGEMENT-001` | [Adaptive disclosure](#adaptive-disclosure), [Action presentation](#action-presentation), [Responsive composition](#responsive-composition), [Verification](#verification) |

## Components and responsibilities

- `DisplayView` retains rendering and expanded-state ownership. It passes visible content and expanded state/setter to the adjacent hook, and passes the returned preview and disclosure-button refs into the preview and action component.
- `useQueuedMessageOverflow` owns both refs, collapsed cap, `canExpand`, lifecycle generation, measurement, signals, and cleanup.
- `QueuedGhostRowActions` forwards the disclosure ref to the optional button, exposes `data-testid="queue-entry-actions"` on the action container, and renders disclosure immediately before Remove.
- `ResizeObserver`, window resize, and visual-viewport resize are additive signals. Callable-value detection selects observer or viewport-only fallback.

## Adaptive disclosure

Fit is always measured at the row width available without the optional disclosure button. Before reading preview geometry, the measurement routine synchronously saves the disclosure button's inline `display`, sets `display: none`, reads collapsed `scrollHeight` and `clientHeight` after layout, and restores the exact prior inline value in `finally` before publishing state. When no disclosure exists, it reads directly. This no-control probe prevents the button from causing or preserving the overflow that justifies itself, without a visible two-pass render or permanently reserved blank action slot.

Collapsed fit remains strict `scrollHeight > clientHeight`; a mutable cap ref updates before state publication. Expanded measurements compare natural height against the collapsed cap while still suppressing the disclosure during the probe. Empty or absent preview resets cap, disclosure, and expanded state.

Each lifecycle owns generation and element identity. Cleanup invalidates generation before teardown. Every callback guards before reads/publication; observer batches require the captured target. Observer support uses `typeof ResizeObserver === "function"`.

Measurement runs in a layout effect for content/state changes and from additive layout signals. The temporary inline style is restored even if a geometry getter throws. If expanded content fits at no-control width, expanded/disclosure clear together. No maximum-height transition remains. Fallback supports initial and viewport-triggered measurement, not element-only panel resizing.

## Action presentation

`QueuedGhostRowActions` keeps Send Now, Merge, and Edit behavior. It exposes an action-container test ID, forwards the measurement ref to disclosure, and renders disclosure as the direct DOM sibling immediately before terminal Remove. Remove uses `IconTrash`, `variant={null}`, local ghost backgrounds, default `text-muted-foreground`, and `[@media(pointer:fine)]:hover:text-destructive`. The pointer-precision variant, rather than hover capability alone, makes destructive color a fine-primary-pointer cue. Existing localized title/name and handlers remain.

No queue mutation, provenance or permission rule, translated copy, or event behavior changes. Disclosure availability intentionally changes to follow rendered overflow.

## Responsive composition

- **Desktop outcome:** the existing compact hover-revealed action row remains. A disclosure appears only for content clipped by the collapsed preview, and the trash action gains destructive hover feedback.
- **Mobile entry point and surface:** the existing inline message queue panel remains the phone composition and keeps the queue list as its single internal scroll owner.
- **Nearest shipped exemplars:** `AnchoredLastPromptBar` supplies the rendered-overflow and `ResizeObserver` pattern; existing trash actions such as the task comment control supply muted-to-destructive hover styling. `mobile-message-queue-management.spec.ts` remains the phone interaction exemplar.
- **Hierarchy and primary action:** message content remains primary. Secondary row actions stay visible on coarse pointers, with the disclosure directly before the terminal Remove action.
- **Surface rationale:** these are frequent, row-local actions, so inline controls are preferable to a drawer or menu. The change adds no overlay, route, scroll owner, or safe-area behavior.
- **Touch geometry:** existing coarse-pointer 44 by 44 CSS-pixel targets remain unchanged. Destructive hover color is a fine-pointer cue; Remove remains discoverable without hover on touch surfaces.
- **Shared logic:** one measurement and action-order path serves desktop and phone widths. Responsive changes affect only whether the rendered content needs disclosure.

## Verification

- Focused Vitest coverage includes a self-induced-overflow boundary. It first establishes genuine overflow and visible disclosure, then changes geometry so the preview overflows only while the disclosure participates in flex layout but fits while that button's inline display is `none`. A layout signal must remove disclosure; subsequent signals must keep it absent. Spies prove the probe hides the button before reads and restores its exact previous inline display in `finally`, including a throwing-geometry case. Remaining cases cover exact opposing fixtures, cap/collapse, additive/fallback signals, and foreign, replacement, generation, cleanup-order, and unmount guards.
- Both browser scenarios sit in suites configured with `test.describe.configure({ timeout: 120_000 })`, so the larger deadline covers test-scoped fixture setup as well as the test body. Desktop uses a 60-second `seedRunningGeneratingSession`-style active turn. Mobile performs the equivalent setup with `sendMessageViaButton("/sleep 60")`, because keyboard submission is unsupported at the Pixel 5 breakpoint. Each awaits `waitForActiveSessionForegroundActivity(..., "generating")`, calls `ApiClient.setQueueAutoRun(sessionId, false)` before target admission, and requires the authoritative `message.queue.auto_run.set` response to report `auto_run === false`. After queue-mode readiness and admission, each polls `message.queue.get` until it reports Auto-run false with the expected pending count, confirms the target row, and only then begins geometry assertions. This invokes existing production Auto-run behavior solely as test setup; no production Auto-run contract changes.
- Desktop uses the exact fixture `"adaptive-width-probe ".repeat(12).trim()` and viewport widths 1280, 800, then 1280 CSS pixels. Before checking the disclosure at each width, it asserts `scrollHeight === clientHeight` wide and `scrollHeight > clientHeight` narrow; these preconditions must pass during RED rather than being mistaken for the expected disclosure failure. The disclosure is then proven absent, present, and absent without reload.
- Desktop exact-title Playwright uses `queue-entry-actions`. When disclosure is present, it evaluates the action container's direct element children and requires the disclosure element's `nextElementSibling` to have `data-testid="queue-entry-remove"`; Remove is also the final actionable child. Opacity, row-scoped role/name, title, color, geometry, and removal assertions remain.
- Pixel 5 exact-title Playwright uses `panel = chat.getByTestId("queued-ghost-list")`, `scrollRegion = panel.getByTestId("queue-scroll-region")`, `row = scrollRegion.getByTestId("queue-entry").filter({ hasText: exactFixture })`, then row-scoped `preview`, `actions`, `expand`, and `remove` test IDs. It requires document `scrollWidth === clientWidth` and `scrollRegion` computed `overflowY === "auto"`. Evaluating `[panel, ...panel.querySelectorAll("*")]` filters elements whose computed `overflowY` is `auto` or `scroll`, maps their `data-testid`, and must yield exactly `["queue-scroll-region"]`. With `EPSILON = 1`, panel lies within viewport width, scroll region within panel, row within scroll region, preview/actions within row, and both Expand and Remove within actions: `child.x >= parent.x - EPSILON` and `child.x + child.width <= parent.x + parent.width + EPSILON`. It requires preview `scrollWidth <= clientWidth + EPSILON`; effective expand/Remove width and height each at least 44; and retains media, icon, accessibility, color, screenshot, and removal assertions.

## Related decisions

None. The change replaces a rendering heuristic with an existing layout-observation pattern and changes no architecture, persistence, or authorization boundary.
