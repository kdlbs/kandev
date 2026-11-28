---
id: "03-packaged-monitor-verification"
title: "Prove the packaged monitor"
status: in_progress
wave: 3
depends_on:
  - "01-ambient-summary-sampling"
  - "02-configurable-monitor-ui"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-001
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-002
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-003
acceptance_criteria:
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.4
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.5
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.2
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.5
  - AC-PLUGINS-TASK-MANAGER-MONITOR-002.7
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.2
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.3
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.4
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.5
system_design:
  - ../../specs/plugins/system-design/task-manager-host-monitor.md
---

# Task 03: Prove the Packaged Monitor

## Summary

Make the complete feature a repeatable repository and artifact gate. Run the
UI checks in CI, update public plugin documentation, inspect the host-platform
package, and exercise the installed plugin in a disposable Kandev instance.

## In scope

- CI coverage for pure UI tests and the portable browser harness.
- README updates for semantics, settings ownership, platform availability,
  refresh cost, and development commands.
- Five-platform build and host-platform archive inspection.
- Disposable Kandev install, config-save restart, personal persistence,
  top-bar/dialog, failure, and disable/re-enable smoke evidence.

## Out of scope

- Tagging, publishing, changelog release metadata, or marketplace mutation.
- Removing the built-in monitor.
- Treating unavailable temperature/load values as packaging failures on
  platforms that do not expose them.

## Acceptance

- CI runs Go formatting/vet/tests plus deterministic UI and browser checks from
  a clean checkout with pinned dependencies and no developer-specific paths.
- The README accurately distinguishes host readings, task readings, personal
  display settings, administrator sampling settings, and panel-versus-ambient
  cost.
- The installed host package preserves settings and cleanup across restart and
  re-enable, contains every referenced UI asset and generated checksum, and
  passes the documented desktop/mobile smoke flow.

## Verification

```bash
rtk make fmt
rtk make vet
rtk make test
rtk make test-ui
rtk make test-harness
rtk make package
rtk tar -tzf kandev-plugin-task-manager-*.tar.gz
```

Run from `kdlbs-kandev-plugin-task-manager`, followed by the documented
`.harness/real-app.mjs` command against a disposable Kandev instance.

## Files likely touched

- `kdlbs-kandev-plugin-task-manager/.github/workflows/ci.yml`
- `kdlbs-kandev-plugin-task-manager/Makefile`
- `kdlbs-kandev-plugin-task-manager/README.md`
- `kdlbs-kandev-plugin-task-manager/.harness/real-app.mjs`
- Pinned UI-test package metadata and lockfile introduced by Task 02.

## Dependencies

- Task 01: complete backend summary and cross-platform build.
- Task 02: complete settings, rendering, and local harness flows.

## Risks

- A real-app smoke test needs a disposable instance and cannot uninstall from a
  developer's primary Kandev data directory.
- Same-version UI assets can be browser-cached during local iteration; the smoke
  procedure must use a fresh document or a deliberately bumped development
  version.

## Parallelism

`sequential`

## Inputs

- System-design section **Verification strategy**.
- Existing package/release workflow and `.harness/real-app.mjs` patterns.
- Canonical Kandev plugin authoring artifact-verification procedure.

## Results

Local verification completed on 2026-09-05. CI now installs pinned harness
dependencies and Chromium, runs Go/UI/browser verification, and builds the
package. The README documents host versus task readings, operator versus
personal settings, and panel versus ambient cost. The package archive contains
all five platform executables, the manifest, the UI bundle, and generated
checksums.

The real-app smoke command remains pending and must be run with `KANDEV_URL` set
to a disposable Kandev instance. This work order remains in progress until that
smoke result is recorded.
