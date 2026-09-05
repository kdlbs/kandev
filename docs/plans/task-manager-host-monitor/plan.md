---
created: 2026-09-05
status: completed
requirements:
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-001
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-002
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-003
system_design:
  - ../../specs/plugins/system-design/task-manager-host-monitor.md
legacy_specs: []
---

# Implementation Plan: Task Manager Host Monitor

## Overview

Extend the independently released Task Manager plugin in three sequential
increments. First add a lightweight, platform-aware summary contract and
install-wide sampling configuration. Then add versioned personal preferences
and the configurable top-bar composition. Finally make the browser and package
checks self-contained, document the behavior, and prove the artifact against a
disposable Kandev instance. This order keeps each UI state backed by a tested
runtime contract before it is exposed.

## Scope

### In scope

- Host and Kandev-task CPU sources with relative and backward-compatible
  per-core presentation.
- Host memory percentage/GB, disk percentage/threshold, CPU temperature, and
  one-minute load.
- Per-user metric enablement, order, unit, threshold, and progress-bar choices.
- Install-wide ambient refresh interval and disk path.
- Lightweight ambient polling separated from the expensive detailed task scan.
- Desktop/mobile, accessibility, synchronization, lifecycle, documentation,
  and packaged-artifact evidence.

### Out of scope

- Removing or modifying Kandev's built-in resource monitor.
- A Kandev host/SDK/protocol change.
- Remote executor metrics, alerts, retained time series, process control, or
  resource limits.
- Reordering the plugin contribution relative to other top-bar items.
- Publishing a release or marketplace update.

## Technical approach

### Plugin backend and manifest

- In `kdlbs/kandev-plugin-task-manager/manifest.yaml`, add the authenticated
  `summary` webhook, `capabilities.user_state: true`, and required
  `refresh_interval_seconds`/`disk_path` config fields.
- Add a small configuration loader around `pluginsdk.Host.GetConfig`. Cache only
  a successful normalized value; rely on the host's plugin restart after config
  save to refresh it.
- Add platform-specific host readers for aggregate CPU counters, physical or
  cgroup memory capacity, filesystem capacity, CPU temperature, and one-minute
  load. Keep raw parsing/calculation in common testable helpers and return
  independent availability per metric.
- Extend the current sampler with a CPU-only attributed summary that uses the
  existing stable process keys, attribution cache, baseline rules, and mutex but
  never calls PSS/RSS aggregation or title APIs.
- Route `summary` separately from the existing `usage` response. Validate metric
  IDs and CPU source, omit unrequested collectors, return partial metric errors,
  and include the effective ambient interval and core count.
- Implement the collector from operating-system interfaces rather than copying
  the AGPL Kandev collector into the MIT plugin.

### Personal settings and shared frontend state

- Add a versioned monitor-preference model and deterministic normalization for
  missing, malformed, duplicate, older, and newly introduced metric entries.
- Persist the confirmed object with `host.storage` at instance/profile scope.
  Use conditional writes, cross-client invalidation, a same-tab shared
  controller, and generation fencing.
- Register an owner-scoped `plugin-settings` card. Use host UI primitives and
  the native Save/Discard contributor for enabled state, CPU mode, memory unit,
  disk threshold, per-metric bars, and ordering.
- Put a focusable information icon beside the Disk row. Its localized tooltip
  explains the filesystem-capacity query, absence of file/directory scanning,
  and that threshold visibility does not suspend sampling.
- Implement pointer drag with explicit insertion feedback plus keyboard Move up
  and Move down controls. Keep disabled metrics in the ordered list and keep
  focus on the moved row.
- Register translated new UI copy through the plugin translation contract.

### Ambient top-bar monitor

- Replace the fixed CPU-only chip with ordered metric segments derived from the
  confirmed preference. Preserve one click target for the existing dialog and
  the current desktop/mobile placement.
- Use one completion-scheduled timeout rather than overlapping intervals. The
  first request starts immediately, and later delays use the effective interval
  returned by the backend.
- Render CPU source/scale, memory unit, disk threshold, independent bar choices,
  unavailable/stale detail, and sampled time. Define the missing progress-fill
  CSS and test its computed width.
- Do not mount a poller until preferences load; stop it when all metrics are
  disabled. Keep detailed dialog polling independent and cancel it on unmount.

### Verification and documentation

- Extract pure UI model functions into an importable ES module and cover them
  with Node tests. Make the existing Playwright harness resolve dependencies
  from the plugin repository instead of developer-specific absolute paths.
- Extend the harness host with `host.storage`, settings-save coordination,
  translations, summary responses, and both top-bar presentations. Exercise
  pointer/keyboard reorder, Save/Discard, threshold boundaries, CPU modes,
  units, bars, errors, polling cadence, and containment.
- Add UI checks to CI alongside existing Go tests and retain five-platform
  packaging. Update README behavior, settings ownership, cost claims, platform
  availability, and development commands.
- Build `package-host`, inspect its entries/checksums, install it into a
  disposable Kandev instance, and smoke-test settings persistence,
  disable/re-enable cleanup, the top bar, and the dialog.

## Tests

- `server/hostmetrics_test.go` tests host/task CPU normalization, memory and
  disk capacity math, requested-family selection, unsupported metrics, bounded
  errors, invalid config, and cancellation. Platform-specific parser tests use
  injected fixture files or pure syscall-result conversion helpers.
- `server/sampler_test.go::TestSummarySkipsProcessMemoryAndTitles` proves the
  ambient task CPU path never invokes PSS/RSS enrichment or the Task API while
  retaining stable-key and stale-baseline behavior.
- `server/plugin_test.go::TestSummaryWebhookReturnsPartialMetrics` and
  `TestSummaryWebhookRejectsInvalidSelectors` cover the wire boundary and leave
  the existing `usage` method/response tests unchanged.
- `ui/monitor-model.test.mjs` covers versioned defaults, upgrade normalization,
  three CPU modes, memory units, disk threshold equality, metric order,
  per-metric bars, all-disabled behavior, and stale/unavailable view models.
- `.harness/shoot.mjs` covers shared Save/Discard coordination, conditional
  storage writes and conflict recovery, same-tab and cross-client updates,
  timer rescheduling, cleanup, accessible reordering, disk-help hover/focus and
  accessible naming, rendering, and dialog activation.

## E2E tests

- `.harness/shoot.mjs` mounts the packaged UI contract at wide, narrow, and
  mobile widths. It saves personal settings, reorders metrics by pointer and
  keyboard, reloads from storage, crosses the disk threshold, switches every
  CPU mode, toggles bars and memory units, and confirms the dialog still opens.
- `.harness/real-app.mjs` installs or targets the host-platform package in a
  disposable Kandev instance. It verifies the owner-scoped settings card,
  administrator config fields, restart recovery, top-bar containment, sampled
  values, and disable/re-enable cleanup.

## Work orders

- [x] [Task 01: Add ambient summary sampling](task-01-ambient-summary-sampling.md)
- [x] [Task 02: Build configurable monitor UI](task-02-configurable-monitor-ui.md)
- [x] [Task 03: Prove the packaged monitor](task-03-packaged-monitor-verification.md)

Dependency order: Task 01, then Task 02, then Task 03. The work orders share
the manifest, UI bundle, harness, and package contract, so none is
parallel-safe.

## Verification results

Completed on 2026-09-05. The plugin implementation, deterministic UI model
tests, portable browser harness, cross-platform builds, and packaged archive
checks passed in the plugin repository:

- `make fmt`
- `make vet`
- `make test` (89 tests)
- `make test-ui` (8 tests)
- `make test-harness`
- `make package`
- `tar -tzf kandev-plugin-task-manager-0.1.1.tar.gz`
- Darwin, Windows, and FreeBSD `go test -c` compilation checks

## Risks

- The Kandev monorepo is AGPL-3.0 and the plugin is MIT. Host collector behavior
  can be used as an oracle, but implementation text must not be copied without
  an explicit license decision.
- Task CPU summaries still require a process-table scan and attribution. The
  CPU-only path removes PSS/title/process-payload cost but is heavier than a
  whole-host CPU counter read.
- macOS aggregate host sampling can require cgo/Mach calls, while Windows and
  macOS do not expose every temperature/load reading used on Linux. Partial
  availability is a normal result, not an all-or-nothing failure.
- `main-top-bar` treats one plugin component as opaque. Internal metric ordering
  belongs to the plugin and does not replace Kandev's separate status-item
  ordering behavior.
- Multi-user browsers can still issue concurrent summary requests. The
  install-wide interval bounds each client, and backend serialization prevents
  corrupt deltas, but the plugin does not become a global push broadcaster.
