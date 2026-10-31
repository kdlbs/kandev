---
created: 2026-10-28
status: done
requirements:
  - REQ-UI-AGENT-TODO-LIST-PANEL-002
system_design:
  - ../../specs/ui/system-design/agent-todo-list-panel.md
legacy_specs: []
---

# Implementation Plan: Todo touch disclosure

## Overview

Make the existing session todo chip operable on wide coarse-pointer devices while preserving the phone/tablet popover and fine-pointer desktop hover/focus. One vertical work order adds independent causal regression evidence, the local repair, and native browser checks.

## Scope

In scope: disclosure choice in `TodoIndicator`, focused component tests and real task/session browser coverage. Out of scope: global breakpoint or `useTouchDrawer` semantics, shared primitives, todo data transport, pinned panel semantics, settings, localization and unrelated layout changes.

## Technical approach

In `apps/web/components/task/chat/todo-indicator.tsx`, use the existing `useTouchDrawer` pointer predicate together with `useCompactTaskChrome` to select the existing Popover; leave the HoverCard on fine desktop. Keep `TodoIndicatorContent`, counts, ordering and status resolution shared. The phone status-bar chip is the nearest existing mobile exemplar: its anchored, short popover is retained by explicit repair scope, although the mobile guide generally prefers a drawer for hover disclosures. The existing trigger provides 44px phone/coarse hit area; its list owns vertical scrolling, with no extra page scroll owner or fixed footer. Confirm viewport containment, dismissal and focus return, safe-area clearance and no horizontal overflow in rendered checks; fix geometry only if evidence requires it. State stays shared through `useSessionTodoItems`; no preference is written. Public docs need no change: this restores existing checklist access without new terminology, configuration or workflow.

## Rendered interaction contract

The status chip above the composer opens an anchored checklist with the existing title, completion count, progress and ordered status rows. On wide coarse-pointer desktop the chip opens by tap; fine desktop keeps hover and keyboard focus. Phone and tablet retain the existing tap-operated anchored Popover. The list scrolls inside the overlay; the trigger remains at least 44px on phone/coarse input, the overlay fits the viewport, can be dismissed and returns focus where applicable, and the document does not overflow horizontally. These outcomes map to AC-002.1-.3 and the rendered checks below; there is no new copy or panel layout.

## Tests

`apps/web/components/task/chat/todo-indicator-touch.test.tsx` independently mounts real `TodoIndicator`, responsive hooks, Radix primitives and browser-locales. Simulate viewport/media and a touch sequence for 1280 coarse; assert checklist text after activation, yielding a causal pre-fix RED, with 390 coarse tap and 1280 fine focus controls passing. Also check empty items and unchanged statuses/counts; distinguish setup failures from a behavioral RED. HappyDOM input simulation is not native browser evidence. Existing `todo-indicator.test.tsx` covers pinned/default content sizing.

## E2E tests

`apps/web/e2e/tests/task/todo-indicator-touch.spec.ts` (chromium) creates and settles a real mock-agent task with `e2e:plan`, opens `/t/<id>` and uses real session history and status chip. Enable CDP touch emulation before navigation at 1280px; assert `(pointer: coarse)` and dispatch actual CDP touch at the chip, then check visible checklist, count, dismissal and containment. A separate fine desktop control verifies focus and hover. `apps/web/e2e/tests/task/mobile-todo-indicator-touch.spec.ts` (mobile-chrome) exercises the same seeded state by native tap, verifies content, target geometry, containment and dismissal. Scope locators to the visible disclosure, not transcript copies. Capture desktop/coarse, fine and phone rendered states and preserve captures outside `.pr-assets` before the next managed run clears it. These tests cover AC-002.1-.3; the focused component test covers .4. Capture what actually occurred, including failures.

## Work orders

- [x] [Task 01: Restore touch disclosure](task-01-touch-disclosure.md)

## Verification results

Task 01 checks passed; see its results for browser and focused test evidence. Qualification supplied: real component/hooks/primitives with simulated touch in HappyDOM at baseline `a7479757d8e8f01a21f25591f473efddfabdc0c6` failed wide/coarse and passed phone/coarse and wide/fine controls (1 failed, 2 passed); this is not native browser proof.

## Risks

Pointer mode can change without viewport width changing; the existing responsive subscription must still govern selection. Radix touch emulation and persisted session setup must be proven by browser assertions, not inferred from the viewport or mocked components. Managed runs clear captures; preserve owned evidence before another run.
