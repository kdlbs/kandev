---
id: "02-retry-managed-runtime-launch"
title: "Retry managed runtime launch"
status: done
wave: 2
depends_on: ["01-classify-and-build-recovery-command"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 02: Retry managed runtime launch

Add one race-safe online retry to host-local managed runtime startup.

- **Acceptance:** A strict npm version-resolution failure during ACP
  initialization stops the first child, invalidates the exact execution key,
  and retries the same trusted runtime once with current npm metadata.
- **Acceptance:** The retry preserves the active exact version, command prefix,
  model, permissions, session identity, and ACP arguments.
- **Acceptance:** A startup attempt generation prevents delayed completion and
  stream events from the first child from publishing a terminal state for the
  retry.
- **Acceptance:** Cancellation and shutdown win. Native, remote, passthrough,
  unrelated, and repeated online failures do not start another recovery.
- **Acceptance:** Success continues the original launch without a failure
  event. Repeated failure publishes one stable failure code and bounded
  sanitized details.
- **Verification:** Add failing lifecycle tests first, then run:

  ```bash
  cd apps/backend
  go test ./internal/agent/runtime/lifecycle -run 'Test.*ManagedRuntime.*Npm|Test.*StartupRecovery'
  go test ./internal/agent/runtime/lifecycle
  ```

- **Files likely touched:**
  `apps/backend/internal/agent/runtime/lifecycle/types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`,
  `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`,
  `apps/backend/internal/agent/runtime/lifecycle/event_types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/events.go`, and focused new or
  existing lifecycle test files.
- **Dependencies:** Task 01.
- **Parallelism:** sequential because this consumes the classifier, command,
  and cache contracts.
- **Inputs:** The Task 01 classifier and trusted helpers, plus lifecycle process
  identity and shutdown rules.
- **Output contract:** Report files changed, RED and GREEN commands and results,
  attempt-generation race evidence, exact retry-count evidence, and
  synchronized task and plan status.
