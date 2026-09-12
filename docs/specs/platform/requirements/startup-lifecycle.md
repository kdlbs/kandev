---
status: active
system: platform
created: 2026-09-11
owners:
  - kandev
---

# Startup lifecycle requirements

## Overview

Platform owns process startup, liveness, readiness, and shutdown across native launch modes and desktop.
Large databases must complete initialization without a listener-health deadline interrupting valid work.

## Requirements

### REQ-PLATFORM-STARTUP-LIFECYCLE-001: Observable initialization

**Intent:** Separate process liveness from application readiness while preserving upgrade safety.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-LIFECYCLE-001.1:** During database opening, backup, migration, and required recovery, `/health` shall answer successfully with existing version and identity semantics.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.2:** Until required initialization and router installation finish, `/ready` and application endpoints shall remain unsuccessful.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.3:** After liveness succeeds, the launcher shall wait for readiness without an initialization deadline. Explicit listener-timeout overrides shall remain effective.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.4:** Startup shall report phase transitions and elapsed time, with bounded periodic status during long phases and no estimated percentages.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.5:** Failed initialization shall report its phase and underlying error, release resources, and exit unsuccessfully. The launcher shall detect child exit.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.6:** Cancellation shall close startup listeners and stop further initialization admission at safe schema and store boundaries. Signal handling shall remain available throughout startup and shutdown. Restore worker quiescing shall not cancel the process and HTTP lifetime before the restore job can publish its result.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.7:** A required upgrade backup shall complete before application migrations mutate the database. Backup failure shall prevent migrations.
- **AC-PLATFORM-STARTUP-LIFECYCLE-001.8:** A listener bind failure shall stop startup promptly and identify the bind error.

## Exclusions

Backup reuse, retention redesign, migration-registry conversion, and background lifecycle-sweep scheduling are outside this change.

## System design

See [startup lifecycle](../system-design/startup-lifecycle.md).
