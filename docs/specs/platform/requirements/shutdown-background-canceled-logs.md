---
status: active
system: platform
created: 2026-09-16
owners:
  - cfl12
---
# Background subsystem context-cancellation log severity Requirements

## Overview

During graceful backend shutdown the root context is cancelled while background
goroutines still have in-flight SQLite writes and agent lookups. Those
operations return `context.Canceled`, and the GitHub PR-watch service, the agent
profile reconciler, and the host-utility profile migration logged the resulting
failures at `ERROR` or `WARN`. A clean shutdown therefore printed a burst of
benign cancellation lines even though the supervisor reported
`error_count: 0`, making it hard to distinguish a real fault from expected
teardown. This requirement scopes those specific background sites; the
session, orchestrator, plugin, and launcher sites are owned by
[Quiet benign teardown log noise on shutdown](shutdown-log-noise.md).

## Terminology

- **Background subsystem:** A long-running backend goroutine, specifically the
  GitHub PR-watch sync loop, the agent profile reconciler, and the one-time
  host-utility profile migration, whose work is bound to the root context.
- **Teardown cancellation:** A `context.Canceled` error observed by a background
  subsystem because the root context was cancelled during graceful shutdown.

## Requirements

### REQ-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001: Quiet background context-cancellation logs on shutdown

**Intent:** Keep clean shutdown logs free of benign cancellation noise from
background subsystems while preserving the original severity of genuine faults.

**User story:** As an operator, I want expected teardown cancellations from
background subsystems recorded at debug level, so that I can read a clean
shutdown and still see real failures at their original severity.

#### Acceptance criteria

- **AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.1:** When a GitHub
  PR-watch sync operation fails with an error that wraps `context.Canceled`
  during shutdown, the backend shall record the failure at `DEBUG`, not
  `ERROR`.
- **AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.2:** When an agent profile
  reconciler store operation fails with an error that wraps `context.Canceled`
  during shutdown, the backend shall record the failure at `DEBUG`, not `WARN`.
- **AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.3:** When the host-utility
  profile migration fails with an error that wraps `context.Canceled` during
  shutdown, the backend shall record the failure at `DEBUG`, not `WARN`.
- **AC-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001.4:** When any of these
  background sites fails with an error that does not wrap `context.Canceled`,
  including `context.DeadlineExceeded`, the backend shall retain the original
  `ERROR` or `WARN` severity so cancellation-aware classification never
  suppresses a real fault.

## Out of scope

- The session-prompt, orchestrator-launch, go-plugin, and launcher-signal sites
  owned by [Quiet benign teardown log noise on shutdown](shutdown-log-noise.md).
- Changing control flow, returned errors, retries, or any task or session state
  during shutdown.
- Reclassifying any non-shutdown log severity.
