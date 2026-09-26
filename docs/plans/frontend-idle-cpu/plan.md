---
specs:
  - docs/specs/ui/requirements/persistent-status-motion.md
  - docs/specs/platform/requirements/browser-console-retention.md
created: 2026-08-23
status: completed
---

# Implementation Plan: Frontend Idle CPU

## Overview

Reduce idle CPU on a focused task page without removing working-state motion or
weakening diagnostic retention. The first change moves long-lived status
rotation to a compositor-prepared HTML wrapper. The second change makes browser
log retention incremental and serializes each tab's write drain.

The supplied 8.34-second Chromium trace showed seven persistent animations on
most frames. It recorded 1,155 `Layerize` events and 921 layout-tree updates
with seven affected elements. The same trace showed about 50,000 IndexedDB
success callbacks from repeated walks of a store near its 10,000-entry limit.

## Requirements and design

- [Persistent status motion requirements](../../specs/ui/requirements/persistent-status-motion.md)
- [Persistent status motion design](../../specs/ui/system-design/persistent-status-motion.md)
- [Browser console retention requirements](../../specs/platform/requirements/browser-console-retention.md)
- [Browser console retention design](../../specs/platform/system-design/browser-console-retention.md)
- [Amended diagnostic-bundle decision](../../decisions/2026-07-30-file-backed-diagnostic-bundles.md)

## Delivery strategy

### Compositor-backed persistent status motion

Add one shared UI primitive for a static SVG inside an animated HTML wrapper.
Audit task, session, agent, and run status surfaces that can remain mounted for
the lifetime of domain state. Migrate those call sites and preserve their
existing state selection, motion, visual style, semantics, and selectors.

Use focused component tests for wrapper ownership, state precedence, and the
live Web Animations API path. Extend the existing desktop settled-spinner and
mobile task-status flows. Capture a new Chromium trace after implementation
and attribute its steady-state rendering events to individual animation
targets. The acceptance trace keeps the production target running while
disabling unrelated grid and status animations, then compares it with a CSS
animation control and the Web Animations API path so page-wide frame events are
not misattributed to the shared compositor primitive.

### Incremental browser-log retention

Increase the existing IndexedDB schema version and add one retention metadata
record. Initialize it from existing rows once during the upgrade. Each later
append updates entries and totals in one transaction, scans only the expired
prefix, and visits oldest rows only while a cap is exceeded.

Make `runtime.ts` own one in-flight drain promise. Scheduled drains and
snapshots join the same loop. Use `fake-indexeddb` only in development tests to
exercise the upgrade and transaction rules.

## Backend

No backend or protocol change is required.

## Desktop and mobile contract

The spinner wrapper is shared by desktop and mobile. It preserves dimensions
and adds no route, overlay, touch target, or scroll owner. The mobile task
switcher remains the native compact status surface and keeps touch navigation.

Browser-log retention has no user-interface composition change.

## Verification strategy

Work Order 01 owns component, desktop Playwright, mobile Playwright, and trace
evidence for status motion. Work Order 02 owns IndexedDB integration tests,
runtime concurrency tests, and frontend type checking.

No public documentation changes are required. The behavior and user-facing
copy remain unchanged.

## Implementation waves and parallel candidates

Wave 1:

- [x] [Task 01: Move persistent status motion to the compositor](task-01-compositor-status-motion.md) (completed)
- [x] [Task 02: Make browser-log retention incremental](task-02-incremental-browser-log-retention.md) (completed)

The work orders have no code dependency. The primary session implements them
sequentially so each performance result has one clear cause.

## Risks

- Moving classes from an SVG to a wrapper can change size, margins, inherited
  color, accessible names, or test selectors if attributes are split
  incorrectly.
- `will-change` is a promotion hint. The repeated trace is required because
  browser compositor decisions cannot be proven by class inspection alone.
- Retention totals become incorrect if any entry mutation commits without the
  metadata store in the same transaction.
- The version-2 upgrade must preserve existing browser history and must not run
  a rebuild on every page load.
