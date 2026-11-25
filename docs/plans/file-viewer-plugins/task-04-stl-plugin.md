---
id: "04-stl-plugin"
title: "Standalone STL viewer"
status: in_progress
wave: 4
depends_on: ["03-marketplace-discovery"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-FILE-VIEWERS-002
  - REQ-PLUGINS-FILE-VIEWERS-003
  - REQ-PLUGINS-FILE-VIEWERS-005
acceptance_criteria:
  - AC-PLUGINS-FILE-VIEWERS-002.2
  - AC-PLUGINS-FILE-VIEWERS-003.1
  - AC-PLUGINS-FILE-VIEWERS-003.2
  - AC-PLUGINS-FILE-VIEWERS-003.3
  - AC-PLUGINS-FILE-VIEWERS-003.4
  - AC-PLUGINS-FILE-VIEWERS-003.5
  - AC-PLUGINS-FILE-VIEWERS-005.1
  - AC-PLUGINS-FILE-VIEWERS-005.2
  - AC-PLUGINS-FILE-VIEWERS-005.3
  - AC-PLUGINS-FILE-VIEWERS-005.4
system_design:
  - ../../specs/plugins/system-design/file-viewers.md
---

# Task 04: Standalone STL viewer

## Summary

Build the first production viewer in a dedicated plugin repository from the official
template. Use host React and local Three.js assets; package and smoke-test it against
a disposable Kandev instance containing tasks 01-03.

## In scope

Own STL parsing/preflight, worker cleanup, orbit/zoom/pan, dimension/grid UI,
accessibility, dependency pins, tests, packaging, and operator help. Resolve the
repository through create-kandev-plugin before writing production files. Define the
following npm scripts in that repository as reproducible test entry points.

## Out of scope

Other work orders, publication, and changes to unrelated editor behavior.

## Acceptance

- ASCII/binary, malicious inputs, dimension fixtures, and cancellations pass without external fetches.
- Desktop/mobile interaction and permission lifecycle pass against the installed package.
- Measured allocations meet the budget at accepted ceilings; otherwise report the unmet scope instead of silently lowering limits.

## UI composition preview

UI-04: STL ready. Model viewport, XYZ model-space dimensions, labelled grid,
Fit/Reset, zoom, and model-unit dimensions. Desktop supports pointer and keyboard;
phone uses full-height viewing, touch orbit/pinch/pan, and bottom controls.
Malformed input, resource exhaustion, and GPU failure show distinct recovery text.

See [combined previews](plan.md#ui-composition-previews). Apply the AC IDs above.

## Verification

Run from the Kandev repository root unless specified otherwise. New suite names
are implementation deliverables, not existing test evidence.

```bash
# Run from the dedicated plugin repository after defining these test scripts.
go test ./...
npm ci
npm test
npm run check
npm run build
npm run test:resource-limits
make verify-package-host KANDEV_TOOLING=/path/to/kandev/apps/backend
KANDEV_HOST_SOURCE=/path/to/kandev npm run test:package-smoke
KANDEV_HOST_SOURCE=/path/to/kandev npm run test:e2e
```

## Files likely touched

Dedicated plugin repository: manifest.yaml, Go entry point, UI source, worker,
package.json/lockfile, test fixtures, e2e/, packaging and release files. No production
plugin source in the Kandev monorepo.

## Dependencies

03-marketplace-discovery

## Risks

Exact SDK release floor, artifact provenance, renderer memory overhead, and parser allocation bounds need implementation evidence.

## Parallelism

Sequential. No delegation authorized.

## Inputs

- [Requirements](../../specs/plugins/requirements/file-viewers.md).
- [Design](../../specs/plugins/system-design/file-viewers.md).
- [Security matrix](../../reviews/file-viewers-security.md).
- Existing registration, approval, file-reader, and marketplace tests beside the scoped source.

## Results

Implementation is present. See the [implementation record](../../reviews/file-viewers-implementation.md)
for completed checks and remaining validation gates. No publication is authorized.
