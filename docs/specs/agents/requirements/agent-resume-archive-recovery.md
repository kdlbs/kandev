---
status: active
system: agents
created: 2026-11-17
updated: 2026-11-17
owners:
  - Kandev
---
# Agent Resume Archive Recovery Requirements

## Overview

Recovery eligibility across task archive transitions, split from
[Agent Resume and Runtime Recovery](agent-resume-runtime-recovery.md) without
changing requirement or criterion IDs.

## Requirements

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004: Recovery across task archive transitions

**Intent:** Keep archived history readable and resume eligible work after the user unarchives its task.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.1:** Opening or reconnecting to an archived task shall not start an agent or restore a live workspace. History and the existing Unarchive action shall remain available without a recovery failure banner.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2:** Session status for an archived task shall report that automatic resume and workspace restoration are not needed. Direct resume or restore requests shall reject the archived state without creating a runtime or changing the stored conversation identity.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3:** When a task becomes archived during a recovery attempt, the task view shall discard obsolete recovery feedback and shall not begin a fallback restore. A later unarchive shall not make an earlier attempt current again.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.4:** After successful unarchive in an open task view, recovery eligibility shall be checked again without a page reload. An archive-cancelled session shall follow normal resume policy, including the preference that prevents automatic agent starts on open. Failed unarchive shall leave recovery disabled.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5:** When the task retains recoverable workspace ownership and branch data, recovery after unarchive shall use the existing task session and workspace recovery path. It shall preserve any valid provider conversation identity. Explicitly stopped and completed sessions shall retain their existing automatic-start policy.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6:** If recovery after unarchive cannot resolve the workspace, the view shall preserve history and show the failure with applicable actions. It shall not claim that missing files were restored or silently replace a lost branch or provider conversation.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7:** An archived task with terminal sessions shall close its open recovery blocks under `task_archived` as bookkeeping, without changing outcome, claims, continuation, or native evidence. Unarchive shall reopen them before eligibility; unarchived-task blocks stay open.

Archive resource ownership remains defined by the task runtime cleanup contract.
These criteria define session recovery eligibility and its visible outcomes.

## System design

[Archive transition eligibility](../system-design/agent-resume-runtime-recovery.md#archive-transition-eligibility).
