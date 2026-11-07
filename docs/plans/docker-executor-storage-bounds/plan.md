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
