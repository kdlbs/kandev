---
status: draft
system: tasks
created: 2026-09-09
owners:
  - kandev
---

# Resume Prompt Queue Requirements

## Overview

Users can submit a prompt while an agent resumes. Kandev stores the prompt
and runs it after the session becomes ready.

The task system owns this contract because it owns session identity, prompt
admission, and deferred dispatch. The shared composer exposes this behavior.

## Requirements

### REQ-TASKS-RESUME-PROMPT-QUEUE-001: Submit during resume

**Intent:** Users can finish their next instruction without waiting for resume.

#### Acceptance criteria

- **AC-TASKS-RESUME-PROMPT-QUEUE-001.1:** During resume, a session that can
  accept queued prompts shall enable Send and the configured submission shortcut
  for valid composer content.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.2:** After successful admission, the
  composer shall clear the submitted draft and show the accepted prompt through
  the existing queue presentation.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.3:** Before readiness, the accepted prompt
  shall remain pending. After readiness, Auto-run ON shall dispatch eligible
  queued work without another user action.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.4:** A readiness transition concurrent
  with admission shall neither strand the prompt nor dispatch it twice.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.5:** Auto-run OFF shall keep accepted
  prompts pending. Submission shall preserve queue order, merge policy, and
  existing clarification, cancellation, and workflow admission barriers.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.6:** If admission fails or cannot start,
  the composer shall preserve its draft and attachments. It shall show the
  existing applicable error or disabled state.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.7:** If resume fails after admission,
  Kandev shall preserve accepted prompts and show recovery feedback. A later
  successful resume shall retry eligible work under existing queue policy.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.8:** Accepted prompts shall retain their
  session identity and payload across navigation and reload. Switching sessions
  shall not redirect an in-flight submission.
- **AC-TASKS-RESUME-PROMPT-QUEUE-001.9:** Task chat and Quick Chat shall expose
  the same behavior on desktop and mobile. Mobile Send shall remain reachable
  by touch above the existing safe-area inset.

## Compatibility and exclusions

This behavior uses the existing startup queue capability. It also applies to
an existing session during initial startup when that session can accept queued
prompts. Environment preparation alone does not establish that capability.

There is no new scheduler, timer, queue type, resume action, or automatic retry
of a failed resume. Provider terminal input, fresh-session creation without
an existing identity, and changes to Auto-run policy are outside this scope.

Existing movement, failure, recovery, executor, upload, and empty-content gates
still apply. A prompt that dispatches immediately can appear directly in the
conversation without a lasting queue row.

## Related requirements

- [Composer readiness](../../ui/requirements/session-start-composer-readiness.md)
- [Queue automation](../../ui/requirements/message-queue-automation-controls.md)
- [Agent recovery](../../agents/requirements/agent-resume-runtime-recovery.md)
