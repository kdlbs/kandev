---
status: draft
system: platform
created: 2026-08-08
updated: 2026-09-27
owners:
  - kandev
---

# Agent runtime availability requirements

## Overview

The platform owns availability and replacement of the local standalone agent runtime.
A failed agentctl must not require a healthy backend to restart.
This revision extends the existing outage contract for issue #3962 and PR #3598.
The backend, application data, and independently healthy remote executors remain available during local recovery.
Runtime recovery does not imply that interrupted agent work can safely resume.

## Requirements

### REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001: Runtime availability

**Intent:** Users receive accurate runtime status and a safe recovery action without losing their application session.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.1:** Kandev shall expose a local runtime snapshot with available, recovering, or unavailable status.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.2:** A replacement runtime shall become available only after authentication, ownership checks, and required consumer rebinding succeed.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.3:** An unexpected runtime exit shall publish recovering status and fence affected operations before replacement begins.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.4:** Intentional backend shutdown shall stop recovery and shall not launch another runtime or publish a spurious outage.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.5:** New or reconnected browsers shall receive the latest snapshot and shall reject older updates from the same backend boot.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.6:** Every authenticated route shall preserve last-known data and show persistent recovery or failure feedback while the runtime is not available.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.7:** Recovery feedback shall remain visible with the optional status bar hidden. Phone controls shall fit the viewport and remain touch-accessible.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.8:** After recovery exhaustion, an authorized administrator shall be able to retry runtime recovery without restarting the backend. Unauthorized users shall see status without a mutation control.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.9:** Successful runtime replacement shall preserve the backend boot identity and browser connection. Healthy remote executions shall not be restarted because the local runtime failed.

### REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002: Bounded runtime replacement

**Intent:** Infrastructure can recover automatically without a crash loop, competing owner, or unsafe command retry.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.1:** Concurrent exit, health, and retry signals shall produce at most one replacement attempt at a time.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.2:** Automatic replacement shall stop after three unsuccessful or short-lived starts, or when an outage exceeds sixty seconds. Stable recovery resets the start budget.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.3:** Recovery shall not replace or terminate a process when its ownership or death remains uncertain. It shall expose the blocked reason.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.4:** Requests and callbacks from a retired runtime shall not modify a successor. Mutations interrupted by replacement shall not be automatically retried.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.5:** Recovery shall preserve owned worktrees, native state, and journals. Port numbers alone shall not authorize process cleanup.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.6:** Runtime recovery shall work for a locally launched or previously adopted local control server, with survival enabled or disabled.

### REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003: Session reconciliation after replacement

**Intent:** Restoring infrastructure shall not repeat uncertain work or hide an interrupted session.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.1:** Recovery shall reconcile each affected session using its original identity and retained outcomes before clearing its admission block.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.2:** Retained output shall appear once before a recovered terminal result releases subsequent work.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.3:** Unknown prompt or external-action outcomes shall remain visibly uncertain without automatic resend, tool replay, or replacement conversation.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.4:** A healthy replacement may admit independent new work while unrecoverable prior sessions remain individually blocked.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.5:** Stop shall remain reachable during recovery. If the original process cannot be controlled safely, Stop shall report that limitation without falsely claiming cancellation.
- **AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.6:** Office, automation, queue drain, Send Now, and manual session actions shall retain the same uncertainty and authorization fences.

## Compatibility and exclusions

Existing requirement IDs 001.1-001.8 retain their availability scope with explicitly revised recovery behavior.
The previous restart-required behavior is replaced by bounded in-process recovery.
A full supervised Kandev restart remains an optional operator fallback, never an automatic repair action.
Initial backend startup failure retains its existing behavior.
No new runtime feature flag is introduced. Complete the package before release.
Remote executor restart policies, arbitrary shell interception, and the unproven Git staging explanation in #3962 are excluded.

## Related records

- [Runtime replacement design](../system-design/agent-runtime-availability.md)
- [Runtime replacement decision](../../../decisions/2026-09-27-agentctl-runtime-replacement.md)
- [Durable delivery](durable-agent-delivery.md)
- [Implementation package](../../../plans/agentctl-runtime-replacement/plan.md)
- [Historical containment package](../../../plans/backend-failure-containment/plan.md)
