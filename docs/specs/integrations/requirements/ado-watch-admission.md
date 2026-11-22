---
status: draft
system: integrations
created: 2026-11-06
owners:
  - kandev
---

# Azure DevOps Watch Admission

## Overview

An operator limiting an Azure DevOps watch must be able to admit later eligible
work after existing tasks finish. A capacity refusal must not permanently mark
an uncreated work item as already handled. Integrations owns the external-item
reservation; task creation and workflow limits remain owned by Tasks.

## Requirements

### REQ-INTEGRATIONS-ADO-WATCH-ADMISSION-001: Retryable bounded admission

#### Acceptance criteria

- **AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.1:** When a work-item watch has a maximum in-flight value of two and returns several new matches, the system shall create no more than two open watcher tasks, including tasks being created concurrently. Each remaining uncreated match shall remain eligible for a later poll.
- **AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.2:** When a watch dispatch is refused before task creation because the maximum in-flight gate refuses it, that dispatch shall not leave the uncreated item permanently deduplicated. A later eligible poll shall be able to retry it.
- **AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.3:** Capacity refusal shall release only a reservation explicitly claimed by that event. An unclaimed Azure event or another integration shall not release a reservation it does not own. Existing task-backed deduplication shall remain unchanged.
- **AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.4:** With no configured cap, existing unbounded watch behavior shall remain available. Disabled watches shall create no tasks. Existing task completion, archive, cleanup, watch generation, and reset semantics shall remain unchanged.

## Exclusions

No backlog buffer, ordering policy, new user control, provider write, production
installation, automatic watch reset, or cross-watch duplicate rule is added.
Owner/dependency early-exit cleanup and historical reservation repair are separate concerns.
The change does not infer whether an ambiguous historical reservation created a
task. The same shared admission path also carries pull-request watches; its
existing reservation ownership must remain safe when that path changes.
