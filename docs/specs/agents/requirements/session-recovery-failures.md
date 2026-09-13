---
status: active
system: agents
created: 2026-09-11
updated: 2026-09-12
owners:
  - Kandev
---

# Session Recovery Failures Requirements

## Overview

Recover workspace access after startup failure and show one actionable session
recovery explanation. Agents own recovery eligibility and presentation; tasks
retain durable bootstrap error and contribution-admission ownership.

## Requirements

The recovery amendments are implemented in the
[contribution resume recovery package](../../../plans/contribution-resume-recovery/plan.md).
The package records the implementation and browser/regression verification
results, including the shared recovery owner and phone touch-target checks.

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005: Workspace access after failed startup

**Intent:** Recover an eligible retained workspace without restarting the failed agent.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.1:** When an active task retains a valid workspace for a failed session, workspace-only restoration shall not fail solely because the session is `FAILED`.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.2:** Restoration shall preserve terminal session state and provider identity. It shall not start an agent, dispatch a prompt, or change Git history.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.3:** Restoration shall reject archived or deleted tasks, foreign session ownership, active cleanup, and missing or ambiguous workspace ownership. These conditions shall remain checked when registration completes.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.4:** Concurrent restoration requests shall share one workspace runtime. If cleanup wins registration, restoration shall release only its newly created resources and report failure.

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006: One active recovery presentation

**Intent:** Show one actionable explanation of the current session recovery failure.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1:** When the same failure appears in automatic recovery, session state, and the transcript, the selected session shall show one active recovery card. An equivalent top banner, stopped-session warning, or synthetic agent error shall not repeat it.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2:** The card shall show a localized cause summary, valid actions, and one initially collapsed details disclosure. Resume and restore causes shall have separate labels and bounded, sanitized details.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.3:** A failure before agent startup shall be described as startup or recovery failure. It shall not state that the agent encountered an error while working.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4:** Retry shall update the existing card and disable equivalent actions while pending. Successful resume shall clear the active failure; workspace-only success shall show a nonblocking notice that the agent remains stopped.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5:** Reload, reconnect, and reversed event order shall converge on the current failure. A stale attempt or unrelated historical error shall neither replace nor be hidden by that failure.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6:** Desktop and phone shall expose the same recovery choices and details. Phone actions shall have at least 44-pixel touch targets, with no horizontal page overflow or extra details scroller.


## Proposed recovery attempt amendment

The following requirement is draft. The existing requirements remain active.
Implementation belongs to the [resume cancellation package](../../../plans/resume-cancellation/plan.md).

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007: Isolated recovery attempts

**Intent:** Preserve conversation continuity and make cancellation final for the affected attempt.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.1:** If loading a saved conversation times out or returns an inconclusive internal error, recovery shall fail without creating a replacement conversation. A later retry shall use the same saved identity.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.2:** After cancellation completes, the cancelled startup attempt shall never send its prompt, replace conversation identity, or restore a working state. Late startup results shall not affect a later attempt.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.3:** A retry after completed cancellation shall run independently of the cancelled prompt. Each admitted retry shall dispatch at most once. Cancellation shall preserve unrelated queued messages and their Auto-run policy.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.4:** A browser disconnect shall not cancel an accepted recovery attempt. Explicit cancellation shall interrupt startup waits and end with success or a visible bounded failure.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5:** A failed resume before prompt dispatch shall show the existing recovery card with the resume cause. It shall not claim that the agent is busy with a complex task. Reload shall preserve the applicable recovery action and failure details.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.6:** On desktop and phone, users shall be able to cancel startup and retry after cancellation settles. Existing recovery actions, touch targets, keyboard access, and transcript scrolling shall remain available.

## Out of scope

Automatic history replacement, new credential permissions, and changes to
provider conversation reset policy are excluded. Existing archive, navigation,
branch-loss, and provider identity rules remain in
[Agent resume and runtime recovery](agent-resume-runtime-recovery.md).

## System design

[Session recovery failures](../system-design/session-recovery-failures.md).
