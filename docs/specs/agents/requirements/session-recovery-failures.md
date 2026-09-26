---
status: active
system: agents
created: 2026-09-11
updated: 2026-09-20
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
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4:** Retry shall show pending state on the active recovery surface and disable equivalent actions. Successful resume shall retire its actions without removing history. Workspace-only success shall retain the stopped-agent notice.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5:** Reload, reconnect, and reversed event order shall converge on the current failure. A stale attempt or unrelated historical error shall neither replace nor be hidden by that failure.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6:** Desktop and phone shall expose the same recovery choices and details. Phone actions shall have at least 44-pixel touch targets, with no horizontal page overflow or extra details scroller.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.7:** Chat shall show the session error at its chronological position in the transcript. Normal message scrolling shall govern initial placement, pagination, and new entries. Errors shall not force a separate scroll position. Recovery actions and the composer shall remain reachable on desktop and phone.


- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.8:** After manual or automatic recovery, the session error shall remain readable before later messages. It shall retain its original cause and occurrence time after reload.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.9:** Only the current unresolved failure shall offer recovery actions. Pending recovery shall not imply success. A later failure shall not reactivate controls on an older entry.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.10:** A recoverable failure that occurs after agent startup, such as a model provider rejecting a dispatched prompt, shall carry the same bounded, sanitized failure detail in its initially collapsed details disclosure as bootstrap and managed-runtime failures do. The detail shall be sanitized of URLs, credentials, and identifiers, and shall be omitted when sanitization leaves nothing usable, in which case the generic recovery card remains. This lets a user expand a short provider error, such as an invalid tool definition, without the raw detail appearing in the summary line.

#### Presentation amendment (September 19)

The following additions to requirement 006 were implemented in the initial pass.
The September 20 fixture review exposed incomplete ownership across the stopped
composer and specialized failures; the uniform recovery amendment below is implemented.
They extend the existing recovery owner without changing recovery permissions,
error scope, chronological history, or provider identity. Delivery is tracked in
[Session error and recovery UI](../../../plans/session-error-recovery-ui/plan.md).

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.11:** When a send failure, startup failure, stopped notice, and unavailable workspace report the same correlated failure, the task view shall expose one active recovery control surface and one new-failure announcement. A dependent workspace pane shall retain a short unavailable state with a route to the owner. Independent failures and failures without trustworthy correlation shall remain distinct; equal text alone shall not suppress an error.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12:** Each active failure shall show a localized title describing the failed operation, one short cause summary, and at most one recommended eligible recovery action. All eligible recovery actions shall be visible as individual buttons without an overflow menu: the recommended action first and alternatives following, using standard app button styling, wrapping on desktop and stacking on phones. Status-check retry, workspace restoration, session resume, runtime retry, branch recovery, and fresh start shall retain their distinct effects and existing confirmations. A non-retryable refusal shall explain its prerequisite instead of offering a retry that is known to fail.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.13:** Technical details shall be initially collapsed, operation-labelled, bounded, and copyable. Display and clipboard shall use the same sanitized content, excluding credential values, sensitive secret references, local paths, and private identifiers. Redaction shall precede truncation, retain safe cause distinctions, and never expose hidden raw text through accessible labels, tooltips, or clipboard fallbacks. Unusable details shall be omitted. Clipboard failure shall keep selectable sanitized text and announce failure without a raw fallback.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14:** At desktop, resized pane, phone, and coarse-pointer tablet sizes, summaries and expanded diagnostics shall use the available content width, wrap readable words and unbroken tokens, and produce no horizontal document overflow. Text shall not occupy an icon-width column when content width is available. Historical session details shall share the transcript scroll owner. Active recovery details shall use the recovery surface scroll owner without a nested diagnostic scroller; shared task details shall retain their existing dialog or drawer scroll owner. Ordinary fine-pointer actions shall measure 28 pixels at standard font size; phone/coarse-pointer targets shall measure at least 44 pixels.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.15:** A new failure shall announce its short summary once without stealing typing focus or forcing transcript scrolling. Historical entries shall not announce on pagination or reload. Recovery pending and result states shall be accessible; keyboard and touch shall reach all eligible actions and details. Closing a recovery confirmation shall return focus to its initiating action, or a deterministic surviving recovery control if that action disappears. Labels, copy feedback, disabled reasons, and summaries shall update with the selected locale.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16:** While recovery is pending, equivalent actions shall be disabled across all mounted consumers. Success shall retire only matching controls and retain history; workspace-only success shall not imply the agent resumed. A failed new attempt shall preserve its own cause and identity, with no stale completion clearing a successor failure. A missing profile, archive, branch-loss refusal, or unknown session status shall not enable a more permissive recovery path.

#### Uniform recovery amendment (September 20)

The user approved a common recovery layout after inspecting failed startup,
interrupted-session, managed-runtime and provider-quota fixtures. This amendment
changes active control placement while preserving chronological error history.
Delivery is tracked by work order 03 in the existing implementation package.

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17:** When the selected session cannot accept messages because of an unresolved failure, one active recovery card shall replace the composer. Failed startup, interrupted-session recovery, runtime installation, and provider quota shall use the same title, cause, action, and details order and consistent amber warning styling, with spinner/status pending feedback and red failed-attempt text. The transcript shall retain a compact chronological error record without duplicate mutation controls. A session that can accept messages shall retain its usable composer; a historical error alone shall not block messaging.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.18:** Recovery choices shall retain existing capability and refusal checks. Runtime installation shall offer its runtime retry rather than generic resume. Quota failures shall retain provider/model/reset guidance and shall not offer unsupported model switching or a fabricated retry deadline. When no recovery operation is eligible, the card shall explain the prerequisite without an inert primary action. Archive and Delete shall remain available through the task menu, outside the recovery card.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.19:** Correlated dependent workspace panes shall navigate to the active recovery card instead of repeating its diagnostics or actions. Explicit navigation shall activate Chat and focus the card on desktop and phone. Unrelated task, workspace, or session failures shall retain their own scope and explanation. Pending recovery and a failed retry shall update the active card; successful agent recovery shall restore the composer with its draft and attachments intact. Workspace-only recovery shall not enable messaging or claim the agent resumed.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.20:** The active card shall keep title and actions reachable at narrow or short viewport sizes and enlarged text. Expanded details shall be bounded and wrapped within a single recovery-region scroll owner, never a nested preformatted-text scroller. Initial failure shall not steal focus or force transcript scrolling. A user-initiated successful recovery shall return focus to the restored composer only when focus belonged to the disappearing recovery controls; background recovery shall not move focus from unrelated content.

## Session error history amendment

The September 14 amendment changes criteria 006.4 and 006.7 and adds 006.8 and 006.9.
Implementation is complete in the [error scope package](../../../plans/error-scope-and-history/plan.md).
This supersedes the reveal-at-top behavior from the completed startup recovery scrolling package.
The task system owns durable history and shared error scope through
[task error ownership](../../tasks/requirements/task-launch-failure-recovery.md).

## Recovery attempt isolation

Criteria 007.1 through 007.6 are implemented in the
[resume cancellation package](../../../plans/resume-cancellation/plan.md).
The accepted-turn amendment adds criteria 007.7 through 007.9. Its
implementation and verification are recorded in the
[resumed turn cancellation package](../../../plans/resumed-turn-cancellation/plan.md).

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007: Isolated recovery attempts

**Intent:** Preserve conversation continuity and make cancellation final for the affected attempt.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.1:** If loading a saved conversation times out or returns an inconclusive internal error, recovery shall fail without creating a replacement conversation. A later retry shall use the same saved identity.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.2:** After cancellation completes, the cancelled startup attempt shall never send its prompt, replace conversation identity, or restore a working state. Late startup results shall not affect a later attempt.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.3:** A retry after completed cancellation shall run independently of the cancelled prompt. Each admitted retry shall dispatch at most once. Cancellation shall preserve unrelated queued messages and their Auto-run policy.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.4:** A browser disconnect shall not cancel an accepted recovery attempt. Explicit cancellation shall interrupt startup waits and end with success or a visible bounded failure.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5:** A failed resume before prompt dispatch shall show the existing recovery card with the resume cause. It shall not claim that the agent is busy with a complex task. Reload shall preserve the applicable recovery action and failure details.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.6:** On desktop and phone, users shall be able to cancel startup and retry after cancellation settles. Existing recovery actions, touch targets, keyboard access, and transcript scrolling shall remain available.

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7:** After the provider accepts a resumed prompt, successful turn cancellation shall preserve the healthy agent process and saved conversation identity. The next message shall use that process without another startup, resume, or recovery error.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8:** Cancellation concurrent with prompt acceptance shall have one outcome. Before acceptance, cancelled startup shall not dispatch. After acceptance, normal turn cancellation shall own the outcome. An accepted prompt shall not be replayed or reported as cancelled startup because its turn was paused.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.9:** Desktop and phone shall support pause followed by another message during the first resumed turn and later turns. Valid responses and session updates shall continue after each pause. A further resume shall require an independently established loss or stop of the runtime.

## Out of scope

Automatic history replacement, new credential permissions, and changes to
provider conversation reset policy are excluded. Existing archive, navigation,
branch-loss, and provider identity rules remain in
[Agent resume and runtime recovery](agent-resume-runtime-recovery.md).

## System design

[Session recovery failures](../system-design/session-recovery-failures.md).
