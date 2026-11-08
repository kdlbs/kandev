---
status: active
system: agents
created: 2026-11-08
updated: 2026-11-08
owners:
  - kandev
---

# Reusable managed npm repairs

## Overview

An incomplete npm execution tree can fail on every cold launch. The existing
[cache recovery contract](managed-npm-cache-recovery.md) repairs each operation
privately. Its successful installation ends with its owner, so later launches
can repeat the same failure and download.

Agents owns reusable repairs because it owns managed runtime identity and
executor-local preparation. This extension preserves the existing selection,
conversation, shared-cache protection, and single replacement budget.

## Terms

- **Cold launch:** A launch that starts a new agent process, including saved-session resume.
- **Installation context:** The selected package and execution environment that determine a compatible npm installation.
- **Published repair:** A validated installation available to later compatible cold launches on the same execution host.

## Requirements

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007: Reusable verified cache repair

**Intent:** A successful repair must prevent repeated recovery from the same
incomplete shared installation while compatible executor storage remains available.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.1:** A verified incomplete-cache failure shall remain eligible for the existing single replacement. Eligibility shall retain requirement 006's evidence and exclusions.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2:** When a repair is published, the next compatible cold launch shall use that installation on its first attempt. It shall require neither another installation nor a user repair action.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3:** Reuse shall cover task start, saved-session resume, startup discovery, profile discovery, and profile model options. Supported protocols shall remain managed ACP and managed Codex app-server.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4:** Reuse shall preserve the selected exact package, registry, authorized environment, workspace, permissions, model, and native conversation. It shall preserve prompt delivery exactly once.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.5:** A published repair shall remain reusable after session teardown and backend or agentctl restart while the same execution-host storage remains available. A new container or lost executor storage shall permit ordinary cold preparation.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.6:** Another user, runtime version, platform, npm configuration, or installation environment shall not inherit an incompatible repair. Ambiguous context shall prevent reuse.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.7:** Publication shall require validated package files and successful protocol initialization. Failed or cancelled initialization shall not publish a repair.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.8:** Concurrent compatible recoveries shall share an available published repair. Publication shall not expose a partial installation or block unrelated installation contexts.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.9:** Cancelling a caller shall stop its owned installation and release its wait. Other eligible callers shall retain independent cancellation and recovery deadlines.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.10:** Automatic recovery shall preserve every shared npm tree and every running agent's installation files. Replacement publication or invalidation shall not remove files used by an earlier process.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.11:** A missing, invalid, or unreadable published repair shall not authorize arbitrary execution. Existing private recovery shall remain available within the original attempt and deadline limits.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.12:** Publication failure after successful initialization shall keep the current agent usable. It shall not create another agent start or claim that later launches can reuse the repair.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13:** All preparation, waiting, and replacement shall retain the existing recovery budget. Task recovery shall allow at most two agent starts and at most 90 seconds, subject to an earlier caller deadline.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.14:** Older peers shall retain their existing private recovery behavior. Compatibility shall not require stopping a healthy retained agent or changing its runtime selection.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.15:** Desktop and phone shall show the existing attempt-2 progress during actual recovery. A later successful first attempt shall use ordinary startup or resume presentation.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.16:** Reuse shall require no new setting, runtime-update click, terminal command, startup-wide scan, or saved profile change. Existing automatic-start restrictions shall remain effective.

## Out of scope

- Removing `--prefer-offline` or changing normal npm freshness policy.
- Automatic removal or reconstruction of the user's shared npm tree.
- Runtime rollback, global binaries, arbitrary missing modules, and requirement 005's pending dependency repair.
- Replaying conversation restoration or prompts after protocol initialization.
- New UI controls, copy, settings, feature flags, or SQL migrations.
- A new executor, storage janitor, or automatic removal of published repair files.

## Delivery

- [System design](../system-design/managed-npm-repair-reuse.md)
- [Implementation plan](../../../plans/managed-npm-repair-reuse/plan.md)
