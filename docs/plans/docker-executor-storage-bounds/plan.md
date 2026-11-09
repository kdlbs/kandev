---
created: 2026-11-07
status: in_progress
requirements:
  - REQ-EXECUTORS-STORAGE-001
system_design:
  - ../../specs/executors/system-design/docker-storage-bounds.md
legacy_specs: []
---

# Docker executor storage bounds delivery plan

Implement [REQ-EXECUTORS-STORAGE-001](../../specs/executors/requirements/docker-storage-bounds.md)
through the shared Docker create boundary described in the
[system design](../../specs/executors/system-design/docker-storage-bounds.md).

[Task 01](task-01-create-bounds.md) owns the single implementation and
verification boundary. There is no UI or schema change.

Verification covers resulting HostConfig values, explicit-mount preservation,
package race tests, and an opt-in real local-daemon canary. Scratch memory/swap
use and unbounded persistent/writable-layer paths are the material limits.
Activation follows ordinary review and deployment; existing containers retain
their storage configuration until their normal lifecycle replaces them.

## Review follow-up

Task 01 includes mount-path normalization, Docker create-request regressions,
and a canary that waits through the same client that creates its container.
Public documentation states the logging override, memory accounting, and
scratch loss at container stop. The redundant operator note was removed.

Verification of the review changes remains pending. The review request
explicitly excludes builds, tests, linters, and other validation commands.
