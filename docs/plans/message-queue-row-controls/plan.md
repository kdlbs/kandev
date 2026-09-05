---
created: 2026-09-04
status: complete
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-001
system_design:
  - ../../specs/ui/system-design/message-queue-row-controls.md
legacy_specs: []
---

# Implementation Plan: Message Queue Row Controls

## Overview

Replace the queued-message disclosure's character-count heuristic with rendered overflow measurement, then align the disclosure and destructive action with the existing row-control language. One sequential work order owns the browser-first regression, production change, desktop and phone evidence, and required repository checks because the behavior and files are coupled.

## Scope

### In scope

- Hide the expand/collapse control when the collapsed queued-message preview does not overflow.
- Re-evaluate disclosure availability after content, viewport, `ResizeObserver`-reported panel-width, or browser-zoom layout changes.
- Render the disclosure immediately before Remove when both actions are present.
- Replace Remove's close glyph with the shared trash glyph and apply the destructive color on fine-pointer hover.
- Preserve desktop hover disclosure, coarse-pointer visibility, 44 by 44 CSS-pixel touch targets, action permissions, action handlers, and accessible copy.
- Add focused measurement-lifecycle tests plus desktop and Pixel 5 Playwright evidence for the changed behavior.

### Out of scope

- Production queue persistence, transport, ordering, provenance, authorization, removal semantics, merge rules, or Auto-run behavior. E2E setup may call the existing Auto-run operation through a test API-client wrapper; that adds no production contract.
- Queue panel composition, scroll ownership, safe-area handling, and responsive breakpoints.
- New or changed user-facing copy, public documentation, settings, or backend code.
- Refactoring the separate anchored-last-prompt disclosure beyond reusing its measurement pattern.

## Technical approach

### Adaptive disclosure

Remove `EXPAND_THRESHOLD` and `shouldOfferExpand`. `DisplayView` retains expanded state/rendering and delegates measurement to `useQueuedMessageOverflow`, which returns preview and disclosure-button refs plus `canExpand`. Pass the disclosure ref through `QueuedGhostRowActions` and add `data-testid="queue-entry-actions"`.

Every fit probe measures with the optional disclosure omitted from layout: save its inline `display`, set it to `none`, read preview geometry synchronously, and restore the exact value in `finally` before state publication. No button means direct measurement. This avoids a self-induced fixed point without a state-driven two-pass render or permanent empty slot. Collapsed measurements update cap before publication; expanded measurements use that cap. Remove max-height transition and reset absent content.

Each lifecycle captures generation and preview identity. Cleanup invalidates first. Callbacks guard before reads/publication; observer batches require target identity. Observer support uses a callable-value check. Window and available visual-viewport signals remain additive; fallback is initial plus viewport only.

Keep collapsed/expanded maximum heights. The collapsed height measured at no-disclosure width is the fit source of truth.

### Action order and destructive cue

In `apps/web/components/task/chat/queued-ghost-row-actions.tsx`, replace `IconX` with `IconTrash` and move disclosure immediately before Remove. Set Remove to `variant={null}` because ghost injects competing hover text. Restore `hover:bg-muted dark:hover:bg-muted/50` locally and use only `text-muted-foreground` plus `[@media(pointer:fine)]:hover:text-destructive`. The `:hover` pseudo-class still requires hover, while the media variant additionally requires a fine primary pointer. Retain title, test ID, handler, and coarse dimensions.

### Browser regressions

Extend `apps/web/e2e/helpers/api-client.ts` with `setQueueAutoRun(sessionId, enabled)`, a typed wrapper around `message.queue.auto_run.set` that returns `{ session_id, auto_run, dispatched }`. Place the new desktop test in a `test.describe` suite configured with `test.describe.configure({ timeout: 120_000 })`, so its deadline applies before test-scoped fixtures initialize. Use the existing `seedRunningGeneratingSession` pattern with a 60-second turn and await `waitForActiveSessionForegroundActivity(..., "generating")`. Before target admission, call `setQueueAutoRun(sessionId, false)` and require its authoritative response to report `auto_run === false`; then wait for composer queue mode, admit the target, and poll `message.queue.get` until Auto-run remains false with the expected pending count. Open the panel and confirm the target row before reading geometry. Use the exact target text `"adaptive-width-probe ".repeat(12).trim()` and viewport widths 1280, 800, then 1280 CSS pixels. At each width, assert the geometry precondition before testing disclosure: equality wide, overflow narrow, and equality wide again. Those preconditions must pass in RED; expected failure belongs to disclosure state, adjacency, icon, hover, or removal behavior. Auto-run preserves the row through pre-removal assertions; invoke Remove last and assert disappearance.

Place the new Pixel 5 test in a suite configured with `test.describe.configure({ timeout: 120_000 })` before fixtures initialize. Establish the 60-second generating-turn invariant by calling `sendMessageViaButton("/sleep 60")`, not the keyboard-submit path used by `seedRunningGeneratingSession`, then await `waitForActiveSessionForegroundActivity(..., "generating")`. Follow the same authoritative Auto-run-response, queue-mode, admission, and queue-status sequence before geometry. Queue the exact target text `"mobile-overflow-probe ".repeat(24).trim()`, confirm the row, and assert its `scrollHeight > clientHeight` precondition before checking disclosure. The expected RED failure belongs to disclosure state, adjacency, trash glyph, target sizing, containment, or removal behavior rather than fixture construction. Invoke Remove after the pre-removal checks and assert the row disappears.

## Tests

- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.9:** exact opposing fixtures guarantee RED. A boundary case starts overflow with disclosure present, then uses geometry that overflows only when disclosure participates but fits when its inline display is `none`; after a signal disclosure must disappear and remain absent. Spies require hide-before-read and exact restoration in `finally`, including when geometry throws. Lifecycle, fallback, guard, cleanup-order, and unmount cases remain.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.9:** desktop exact-title E2E proves wide/narrow/wide response at no-control fit semantics.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.10:** both tests assert direct adjacency. Pixel 5 defines the exact locator chain. Document widths are equal; scroll region has `overflowY === "auto"`; evaluating panel plus all descendants, filtering computed `auto|scroll` overflow-Y, and mapping `data-testid` yields exactly `["queue-scroll-region"]`. With `EPSILON = 1`, explicit horizontal inequalities prove panel within viewport, scroll region/row/preview/actions within their named parent, and both Expand and Remove within actions. Preview content width fits and expand/Remove effective width and height each meet 44 pixels. Media, icon, accessibility, color, capture, and removal checks remain.

## E2E tests

- **Desktop Chromium:** exact test proves no-control-width response and direct DOM adjacency: disclosure's next element sibling is terminal Remove.
- **Mobile Chrome / Pixel 5:** exact test proves the same adjacency before retained screenshot, plus touch, geometry, icon, accessibility, and removal contracts.

## Work orders

- [x] [Task 01: Refine queued row controls](task-01-refine-queued-row-controls.md)

## Verification results

- Completed the rendered-overflow hook, row-action ordering/presentation,
  focused lifecycle regressions, desktop Chromium regression, and Pixel 5
  geometry/touch regression.
- Focused GREEN evidence: 14 Vitest tests passed; both exact-title Playwright
  scenarios passed.
- The retained Pixel 5 capture and manifest entry confirm disclosure directly
  before terminal Remove without hierarchy, clipping, overlap, spacing, or
  phone-composition defects.
- Formatting, typecheck, and lint passed. The combined repository command
  reached unrelated host/runtime failures in backend tool discovery, web
  Docker/time-limited tests, and the script suite's unavailable `unzip`;
  affected focused checks passed.

## Risks

- Queue entries can drain as soon as the active turn completes. Browser scenarios must establish a slow running turn, wait for queue-mode readiness, disable Auto-run where needed, and confirm the row before geometry work.
- Optional disclosure can shrink its flex-sibling preview and justify itself. Every probe temporarily removes the button from layout, restores inline display in `finally`, and decides from no-control geometry. A boundary regression makes participation the sole overflow cause.
- The preview's integer geometry can sit at an equality boundary. Use the established strict `scrollHeight > clientHeight` contract rather than a text-length or newline fallback.
- Observer disconnection does not cancel an already queued callback. Generation and identity guards reject callbacks before reads/publication after same-element replacement, element replacement, and unmount. Cleanup invalidates generation before listener removal and disconnect.
- Expanded content uses a different maximum height. Losing the collapsed cap would make every expanded row appear to fit and remove its only collapse path.
- Browser E2E must seed text that demonstrably fits wide and overflows narrow. Assertions should check the measured geometry before checking control state rather than rely on a fixed character count alone.
- Browser zoom is not directly driven by Playwright. Observer-present tests dispatch additive signals. Fallback stubs `ResizeObserver` undefined and asserts construction/observe never occurs while initial and viewport remeasurement work, requiring callable-value detection rather than `"ResizeObserver" in window`; optional visual-viewport absence and cleanup are also covered.
- Fresh worktrees require `pnpm install --frozen-lockfile` from `apps/` before any pnpm or Playwright command.
