---
id: "02-configurable-monitor-ui"
title: "Build configurable monitor UI"
status: completed
wave: 2
depends_on:
  - "01-ambient-summary-sampling"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-001
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-002
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-003
acceptance_criteria:
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.1
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.2
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.3
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.4
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.5
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.1
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.2
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.3
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.4
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.5
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.6
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.7
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.3
system_design:
  - ../../specs/plugins/system-design/task-manager-host-monitor.md
---

# Task 02: Build Configurable Monitor UI

## Summary

Implement the personal preference model and owner-scoped settings card, then
drive a responsive, ordered top-bar monitor from the summary contract. Keep one
shared browser-generation controller so the settings page, top bar, and other
signed-in clients converge without timer or stale-write leaks.

## In scope

- Versioned defaults, normalization, value derivation, and formatting helpers.
- `host.storage` load/save/conflict/subscription behavior and shared
  Save/Discard integration.
- Metric enablement, CPU mode, memory unit, disk threshold, independent bars,
  drag order, and keyboard move controls.
- A keyboard-focusable Disk information icon whose localized tooltip explains
  filesystem-capacity metadata, the absence of file/directory scanning, and
  continued sampling while threshold visibility hides the reading.
- Translation catalogs for all new settings, values, tooltips, errors, and
  accessible names.
- Completion-scheduled summary polling, stale/unavailable behavior, progress
  fill styling, desktop/mobile containment, and dialog activation.
- Deterministic unit and browser-harness coverage for the above behavior.

## Out of scope

- Schema-driven operator-config persistence, which Task 01 owns.
- Changes to Kandev host components or built-in monitor state.
- Changing detailed dialog sorting, task metrics, or process controls.

## Acceptance

- A new user sees the backward-compatible CPU-only layout; saved personal
  settings round-trip, synchronize, discard, and conflict without cross-user or
  stale-read overwrites.
- Pointer and keyboard ordering, all CPU modes, memory units, threshold edges,
  and per-metric bars render correctly in desktop and mobile top bars.
- The Disk information tooltip opens on hover and keyboard focus, uses the same
  explanatory copy in both cases, and exposes an accessible icon name and
  tooltip relationship.
- No setting or initial snapshot appears as a false zero; all-disabled state
  creates no poller; unmount/destroy leaves no timers, requests, subscriptions,
  modal, or injected style.

## Verification

```bash
rtk go test ./server/...
rtk make test-ui
rtk make test-harness
```

Run from `kdlbs-kandev-plugin-task-manager`. This work order adds the
`test-ui` and portable `test-harness` targets before using them.

## Files likely touched

- `kdlbs-kandev-plugin-task-manager/ui/bundle.js`
- New `kdlbs-kandev-plugin-task-manager/ui/monitor-model.mjs`
- New `kdlbs-kandev-plugin-task-manager/ui/monitor-model.test.mjs`
- `kdlbs-kandev-plugin-task-manager/.harness/harness.js`
- `kdlbs-kandev-plugin-task-manager/.harness/shoot.mjs`
- `kdlbs-kandev-plugin-task-manager/.harness/index.html`
- `kdlbs-kandev-plugin-task-manager/Makefile`
- Optional pinned browser-test package metadata if the harness cannot consume
  the sibling Kandev Playwright installation portably.

## Dependencies

- Task 01: the stable summary request/response and effective interval.

## Risks

- HTML drag events do not provide complete keyboard or touch support by
  themselves. Explicit move controls are required even when pointer drag is
  present.
- Same-tab storage notifications are intentionally suppressed by the host, so
  the controller must publish successful local saves directly.
- The current bundle is a large hand-written module; extraction must preserve
  relative asset loading in an installed package.

## Parallelism

`sequential`

## Inputs

- All requirement acceptance criteria listed in frontmatter.
- System-design sections **Personal preference model**, **Settings surface**,
  **Top-bar rendering**, **Synchronization and lifecycle**, and **Failure and
  recovery**.
- Kandev's documented `plugin-settings`, `main-top-bar`, `host.storage`, and
  `host.useSettingsSaveContributor` contracts.

## Results

Completed on 2026-09-05. The plugin now stores normalized per-user metric
settings, contributes ordered CPU/memory/disk/temperature/load segments, uses
the shared Save/Discard contract, supports pointer and keyboard ordering, and
provides localized accessible disk help. Ambient polling is completion
scheduled, stale-safe, and disabled when no reading is enabled.

Verification: `make test-ui` and `make test-harness` passed, including desktop
and mobile containment, settings persistence, disk-help focus, threshold
behavior, and the all-disabled no-poller state.
