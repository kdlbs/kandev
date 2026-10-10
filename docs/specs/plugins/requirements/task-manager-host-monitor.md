---
status: draft
system: plugins
created: 2026-09-05
owners:
  - kandev
---

# Task Manager Host Monitor Requirements

## Overview

The official Task Manager plugin already attributes CPU and memory to live
Kandev task process trees. Its top-bar contribution shows only task-attributed
CPU in the `top`/`htop` convention, where 100% is one logical core. Users need
a configurable ambient host monitor without losing that task breakdown. The
plugin system owns this contract because the independently released plugin
owns the measurements, personal display state, and contributed UI; Kandev only
supplies its existing plugin settings, storage, and top-bar contracts.

## Terminology

- **Host CPU:** CPU consumed across the machine, including processes that are
  not attributed to Kandev tasks.
- **Task CPU:** The sum of CPU consumed by processes attributed to Kandev tasks.
- **Per-core CPU:** The `top`/`htop` scale where one fully occupied logical core
  is 100% and a multi-core workload can exceed 100%.
- **Relative CPU:** CPU as a share of total logical CPU capacity, clamped to the
  inclusive range 0% through 100%.
- **Ambient monitor:** The compact plugin contribution in the Home, Kanban, and
  Tasks top bar. The Task Manager dialog remains the detailed task breakdown.

## Requirements

### REQ-PLUGINS-TASK-MANAGER-MONITOR-001: Configurable resource readings

**Intent:** Let a user choose the resource values that answer their monitoring
question while retaining the existing task-oriented CPU view.

**User story:** As a Kandev user, I want to choose the CPU source and scale and
add host memory and disk values, so that the top bar shows the resource pressure
that matters to me.

#### Acceptance criteria

- **AC-PLUGINS-TASK-MANAGER-MONITOR-001.1:** When CPU is enabled, the user shall
  be able to select host-relative CPU, task-relative CPU, or task per-core CPU.
  Both relative modes shall remain between 0% and 100% inclusive, while task
  per-core CPU may exceed 100% and shall retain the current task attribution
  semantics.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-001.2:** When memory is enabled, the user
  shall be able to show host memory used as either a percentage of available
  host capacity or an absolute value in GB. The detail text shall expose used
  and total bytes even when the compact value is absolute.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-001.3:** When disk is enabled, the ambient
  monitor shall show usage for the operator-configured filesystem as a
  percentage and identify that filesystem in inspectable detail.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-001.4:** The plugin shall also offer the
  CPU-temperature and one-minute system-load readings available in Kandev's
  built-in host monitor. A reading unsupported by the current platform shall be
  shown as unavailable with inspectable error detail and shall not suppress
  supported readings.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-001.5:** Top-bar display preferences shall
  not change the Task Manager dialog's per-task and per-process CPU, memory,
  sorting, filtering, history, or keyboard-shortcut behavior. Activating the
  ambient monitor shall continue to open that dialog.

### REQ-PLUGINS-TASK-MANAGER-MONITOR-002: Personal top-bar composition

**Intent:** Give each user control over a compact monitor without making one
user's layout the install-wide default for everyone else.

**User story:** As a Kandev user, I want to choose, arrange, and simplify the
top-bar readings, so that the monitor fits how I scan resource pressure.

#### Acceptance criteria

- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.1:** Each supported reading shall have
  an independent enabled state. Disabling every reading shall remove the
  ambient contribution and stop its polling without disabling the plugin or
  its keyboard shortcut.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.2:** The plugin settings page shall let
  the user drag enabled and disabled readings into a preferred order. It shall
  also provide keyboard-operable move controls, preserve focus after a move,
  and render enabled readings in that order on desktop and mobile.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.3:** CPU, memory, and disk shall each
  have an independent progress-bar preference. Turning off one bar shall keep
  that reading's label or icon, formatted value, accessible name, and detail
  available without changing another reading.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.4:** Disk shall support an optional
  visibility threshold from 1% through 100%. When threshold visibility is on,
  an available disk reading below the threshold shall be omitted; a reading at
  or above the threshold, or an unavailable reading, shall remain visible.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.5:** The Disk settings row shall place
  a keyboard-focusable information icon beside its label. Its hover and focus
  tooltip shall explain that the plugin reads capacity metadata for the
  filesystem containing the configured path, does not scan files or
  directories, and continues sampling when a visibility threshold hides the
  top-bar reading.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.6:** Personal display preferences shall
  participate in Kandev's shared settings Save and Discard flow, survive page
  reloads and plugin restarts, propagate to another signed-in client for the
  same user, and remain isolated from other users.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.7:** A user without saved preferences
  shall get the backward-compatible ambient layout: task per-core CPU enabled
  first with its progress bar, and every newly added reading disabled. A newer
  plugin version shall append new known readings without losing an existing
  user's order.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-002.8:** Before personal settings or the
  first resource snapshot load, the ambient monitor shall not present a zero as
  a measured value. A failed settings read shall remain retryable and shall not
  overwrite stored preferences; after a transient sampling failure, the last
  successful reading may remain visible with stale or error detail.

### REQ-PLUGINS-TASK-MANAGER-MONITOR-003: Install-wide sampling policy

**Intent:** Let an operator bound background collection cost and choose the
filesystem once for all users of the Kandev installation.

**User story:** As a Kandev operator, I want one refresh and disk policy, so
that users cannot create conflicting host collection settings and I can trade
freshness for lower overhead.

#### Acceptance criteria

- **AC-PLUGINS-TASK-MANAGER-MONITOR-003.1:** Plugin configuration shall offer
  one install-wide ambient refresh interval from 1 through 300 seconds and one
  install-wide disk path. Defaults shall be five seconds and `/`, matching the
  built-in monitor's current collection defaults.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-003.2:** After an administrator saves valid
  sampling configuration, the active plugin shall restart through the existing
  plugin lifecycle and subsequent ambient snapshots shall report and use the
  new interval and disk path without a Kandev restart.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-003.3:** Ambient requests shall collect only
  enabled resource families and shall not perform proportional-set-size reads
  or construct the detailed per-process response. Opening the Task Manager
  dialog may use its existing faster interactive cadence and full task scan;
  closing it shall release that additional polling.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-003.4:** An invalid or inaccessible disk
  path, an unsupported platform reading, or one failed collector shall degrade
  only the affected reading. The endpoint and remaining readings shall stay
  usable, and errors shall be bounded and safe to display.
- **AC-PLUGINS-TASK-MANAGER-MONITOR-003.5:** The feature shall use the existing
  authenticated plugin webhook, `plugin-settings`, `main-top-bar`, and
  `host.storage` contracts. It shall require no Kandev host API, database,
  WebSocket protocol, or first-party monitor change.

## Out of scope

- Removing or changing Kandev's built-in resource monitor.
- Showing execution-environment or remote-host metrics in the Task Manager
  plugin.
- Per-user refresh intervals or per-user disk paths.
- Killing processes, setting resource limits, retaining metric history across
  reloads, or generating alerts outside the top bar.
- Reordering the Task Manager contribution relative to other Kandev or plugin
  top-bar items; the preference orders only readings inside this plugin's
  contribution.
