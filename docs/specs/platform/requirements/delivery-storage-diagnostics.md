---
status: draft
system: platform
created: 2026-10-27
updated: 2026-10-28
owners:
  - kandev
---

# Delivery storage diagnostics requirements

## Overview

Users need the actual storage cause when agent output pauses or a session fails.
The message must identify the affected runtime and a useful recovery action.

Platform owns this contract because it owns delivery evidence across executors.
The existing [delivery contract](durable-agent-delivery.md) retains storage and admission authority.
The [agent recovery contract](../../agents/requirements/session-recovery-failures.md) retains session recovery behavior.

This proposal covers storage diagnostics, including their desktop and phone presentation.
It does not change cleanup policy or infer why disk usage grew.

## Terminology

- **Current observation:** A recent storage report from the currently owned runtime.
- **Failure snapshot:** Storage evidence captured when the reported operation failed.
- **Unknown measurement:** A value that the source cannot establish. It is different from zero.

## Requirements

### REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001: Accurate storage causes

**Intent:** Distinguish host capacity problems from delivery limits and other storage errors.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.1:** Reports shall distinguish low disk space, filesystem allocation exhaustion, filesystem quota exhaustion, journal limits, stream limits, oversized events, and unclassified storage errors.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.2:** The owning agentctl shall report capacity from its executor's monitored filesystem. Reports shall identify journal or workspace storage and shall not substitute backend-host capacity.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.3:** Known measurements shall include their observation time and units. Reports shall distinguish filesystem availability, required write capacity, retained bytes, and configured limits.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.4:** Unknown measurements shall remain unknown. Malformed values, unsupported fields, and old error text shall not establish low disk capacity or a specific limit.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.5:** A terminal storage cause shall retain its failure snapshot across reload and reconnect. A later capacity sample shall not rewrite that historical cause.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.6:** Diagnostics shall exclude prompts, tool output, credentials, raw provider errors, and arbitrary paths. Existing ownership and permission restrictions shall apply.

### REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002: Storage pressure visibility

**Intent:** Expose a blocked output path even when that path cannot save another event.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.1:** When owner-verified status reports a storage warning or storage pressure, connected chat shall show that observation without another journal event or prompt.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.2:** Current pressure, recovered capacity, and unavailable observations shall remain distinct. After 90 seconds without a fresh observation, the UI shall label measurements as last reported.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.3:** A pressure observation shall not establish process termination, prompt failure, turn completion, or permission to repeat an instruction.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.4:** Fresh recovery shall clear only the corresponding live condition, subject to its clearance threshold. It shall not resolve historical failures, delivery uncertainty, or other recovery restrictions.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.5:** Delayed observations from a previous execution or native generation shall not replace current diagnostics. Unrelated sessions shall retain independent state.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.6:** When SQL persistence fails but control traffic remains available, the live warning shall remain available to connected users. It shall not claim durable persistence.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.7:** Observation work shall remain bounded and independent of browser count. Unsupported peers and failed status requests shall preserve ordinary recovery behavior without a false healthy result.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.8:** At 90% filesystem usage or above, users shall receive an advisory warning. After usage falls below 85%, that warning shall clear.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.9:** Warning sampling shall use a 30-second interval and share valid samples across tasks on the same proven filesystem. Available fresh samples shall be reusable.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.10:** The advisory shall leave running tasks and prompt admission unchanged. Existing write-capacity protection shall remain independent of the warning threshold and sample cache.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.11:** Local, SSH, Sprites, Docker, and Kubernetes environments shall retain independent observations. A shared profile, hostname, path string, or filesystem identifier alone shall not establish shared storage.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.12:** Concurrent tasks shall reuse one capacity sample within a proven shared agentctl environment and filesystem. Separate executor filesystems shall receive separate samples.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.13:** Capable agentctl instances shall support the advisory independently of durable delivery. Without a configured journal, workspace capacity shall remain explicitly identified as workspace storage.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.14:** Background warning checks shall reuse existing executor connections. They shall not start, restore, or wake an inactive remote environment solely to collect capacity.

### REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003: Actionable recovery presentation

**Intent:** Help users recover the affected session without unnecessary loss of conversation context.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.1:** Chat shall show the specific storage cause before Technical details. High usage, immediate write pressure, and historical interruptions shall use different wording.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.2:** Local runtime capacity failures shall offer the existing Storage settings entry when the viewer has access. Remote failures shall identify executor storage instead.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.3:** Physical capacity guidance shall explain how to free space and continue the same session. It shall not recommend journal deletion or a fresh conversation as a capacity remedy.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.4:** Existing recovery admission, pending state, Stop, and uncertainty rules shall remain authoritative. The UI shall not start cleanup, change policy, or resume a session automatically.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.5:** Technical details and copied diagnostics shall retain only validated evidence. Users shall see the failure time separately from the latest observation time.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.6:** Desktop and phone shall expose the same cause, measurements, details, and eligible actions. Phone and coarse-pointer targets shall measure at least 44 pixels.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.7:** The phone advisory shall remain a compact row above the composer. Historical recovery actions shall stack, and expanded details shall retain one bounded scroll owner.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.8:** All new copy and measurement formatting shall use the selected locale. Every shipped real locale shall include the new copy.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.9:** The 90% advisory shall show usage and available capacity when known. It shall not claim that work paused or offer Resume without a separate recovery cause.
- **AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.10:** Each visible chat composer shall show at most one advisory row with vertical padding. Extra guidance and measurements shall remain available through explicit disclosure.

## Out of scope

- Changes to journal quotas, disk reserves, acknowledgment, retention, or cancellation policy.
- Automatic cleanup, new cleanup permissions, or changes to cleanup defaults.
- Disk growth forecasting, cross-task incident grouping, or disk usage attribution.
- Recovery from erased native state, or replacement of uncertain instructions.

## Design and delivery

- [System design](../system-design/delivery-storage-diagnostics.md).
- [Implementation package](../../../plans/delivery-storage-diagnostics/plan.md).
