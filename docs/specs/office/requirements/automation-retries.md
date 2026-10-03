---
status: draft
system: office
created: 2026-09-29
owners:
  - kandev
---

# Automation Retry Requirements

## Overview

Automation retries let an operator recover transient failures without creating a new trigger. Retry policy, attempt ownership, scheduling, and visible history form one Office-owned lifecycle across the automation editor, scheduler, persistence, and run-history views.

## Terminology

- **Retry group:** The durable identity for one original automation firing and all of its attempts.
- **Attempt:** One run within a retry group, identified by its ordinal and exact scheduled time.

## Requirements

### REQ-OFFICE-AUTOMATION-RETRIES-001: Durable automation retry lifecycle

**Intent:** Operators need transient automation failures to recover predictably, while each logical firing retains its policy, input, ownership, and complete history.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-RETRIES-001.1:** When an operator saves retry policy, the system shall persist `disabled`, `finite`, or `infinite` mode, fixed or exponential backoff, and `attempts` or `timeline` history presentation. Disabled mode shall use the canonical disabled values, and finite mode shall require at least one retry.
- **AC-OFFICE-AUTOMATION-RETRIES-001.2:** When a firing becomes eligible for retry, the system shall create the next attempt under the original retry-group identity, preserve the firing's policy and launch-input snapshots, and store its exact due time. A retry count limit shall bound finite mode; infinite mode shall not impose that limit.
- **AC-OFFICE-AUTOMATION-RETRIES-001.3:** When multiple Kandev instances schedule or recover retries, durable claims and operation leases shall prevent concurrent dispatch of one attempt. Startup recovery shall preserve unexpired ownership and reclaim only expired leases; an interrupted operation shall be safely resumable without creating a duplicate task.
- **AC-OFFICE-AUTOMATION-RETRIES-001.4:** When an operator disables an automation or stops a retry group, future attempts shall be fenced from dispatch. If stopping a live run fails, its durable ownership and retry records shall remain available for later reconciliation.
- **AC-OFFICE-AUTOMATION-RETRIES-001.5:** When an operator opens retry history, the system shall expose bounded, cursor-paged attempt history with a stable high-water mark, and timeline mode shall also page through all standalone runs. Timeline presentation shall project retry groups before applying status filters or counts, while attempt presentation shall retain individual runs. Deleting a visible retry group shall remove its complete group history without deleting unrelated runs.
- **AC-OFFICE-AUTOMATION-RETRIES-001.6:** When a webhook-triggered attempt is retried, the system shall retain trigger values needed by existing interpolation, dedup-key, and repository-selector paths. Configured safe JSON pointers shall bound retained payload fields; omitted pointers shall preserve legacy full-payload path resolution within the 1 MiB webhook body limit.

## Out of scope

- Retrying failures from workflow-step automation, provider webhooks, or task creation APIs that do not use the workspace automation run lifecycle.
- Automatically changing an operator's retry policy after a failure.
