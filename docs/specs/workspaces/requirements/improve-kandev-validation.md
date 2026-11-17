---
status: draft
system: workspaces
created: 2026-11-16
owners:
  - kandev
---

# Improve Kandev validation requirements

## Overview

Improve Kandev must turn relevant lifecycle obligations into explicit validation evidence.
General test commands cannot prove a scenario that the tests do not exercise.
Workspace ownership follows the persisted Improve workflow and its supplied phase instructions.
The [existing Improve contract](improve-kandev.md) retains ownership of report creation and workspace isolation.

## Requirements

### REQ-WORKSPACES-IMPROVE-VALIDATION-001: Explicit lifecycle evidence

**Intent:** Contributors can identify the proof required before an improvement is complete.

#### Acceptance criteria

- **AC-WORKSPACES-IMPROVE-VALIDATION-001.1:** For a change to scheduling, background delivery, or task/session lifecycle, the workflow shall identify applicable lifecycle scenarios before implementation.
- **AC-WORKSPACES-IMPROVE-VALIDATION-001.2:** The assessment shall cover pause/resume, restart/recovery, duplicate or stale delivery, ownership changes, and concurrent or merged work.
- **AC-WORKSPACES-IMPROVE-VALIDATION-001.3:** Each applicable scenario shall name its observable invariant, acceptance criterion, test, exact command, and result. Each exclusion shall give a concrete reason.
- **AC-WORKSPACES-IMPROVE-VALIDATION-001.4:** A broad passing suite, coverage annotation, or declared work-order status shall not substitute for the scenario's asserted outcome.
- **AC-WORKSPACES-IMPROVE-VALIDATION-001.5:** When required proof is missing or blocked, the completion report shall identify the gap and shall not claim complete verification.
- **AC-WORKSPACES-IMPROVE-VALIDATION-001.6:** The Test phase shall give the user relevant lifecycle reproduction steps in an isolated instance. It shall preserve the existing manual phase transitions.
- **AC-WORKSPACES-IMPROVE-VALIDATION-001.7:** Changes without relevant lifecycle behavior shall retain targeted validation without unrelated lifecycle tests.

### REQ-WORKSPACES-IMPROVE-VALIDATION-002: Safe instruction upgrades

**Intent:** Existing installations receive known revisions of supplied Improve instructions without losing user changes.

#### Acceptance criteria

- **AC-WORKSPACES-IMPROVE-VALIDATION-002.1:** On upgrade, known historical supplied prompts shall advance to the current revision before subsequent Improve task admission or step entry.
- **AC-WORKSPACES-IMPROVE-VALIDATION-002.2:** Both new workflow creation and reuse of an existing workflow shall receive the updated instructions.
- **AC-WORKSPACES-IMPROVE-VALIDATION-002.3:** A prompt with unknown or customized content, or a row marked edited by differing creation and update timestamps, shall remain unchanged and produce a bounded diagnostic.
- **AC-WORKSPACES-IMPROVE-VALIDATION-002.4:** An upgrade shall preserve workspace, workflow, step, task, and session identities and all unrelated workflow properties.
- **AC-WORKSPACES-IMPROVE-VALIDATION-002.5:** Repeated startup shall be idempotent. A concurrent prompt edit shall not be overwritten by an upgrade based on an older read.
- **AC-WORKSPACES-IMPROVE-VALIDATION-002.6:** The upgrade shall not restart agents, advance tasks, rewrite transcripts, or inject instructions into an accepted turn.
- **AC-WORKSPACES-IMPROVE-VALIDATION-002.7:** A failed write or an unresolved second template compare conflict shall prevent admission with falsely reported current instructions. Unknown content shall remain distinguishable from a failed upgrade.

## Exclusions

- Automatic proof of agent reasoning or semantic test completeness.
- Changing Office delivery guarantees, workflow transitions, or user model selection.
- Requiring screenshots for code without a rendered UI outcome.
- Replacing unknown prompts or adding a general workflow migration framework.

## Implementation plan

- [Lifecycle validation and prompt upgrades](../../../plans/improve-kandev-lifecycle-validation/plan.md)
