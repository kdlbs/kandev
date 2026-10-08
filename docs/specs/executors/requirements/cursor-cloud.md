---
status: draft
system: executors
created: 2026-09-25
owners:
  - kandev
---

# Cursor Cloud execution requirements

## Overview

Run a Kandev task through Cursor Cloud while preserving conversation identity, workflow guards, and explicit execution limits.
The executor system owns remote execution lifecycle, admission, and recovery. Agent profiles and task transitions retain their existing owners.

## Terminology

- **Cloud conversation:** One Cursor agent bound to one Kandev task session.
- **Cloud run:** One prompt turn within that conversation.
- **Submission unknown:** A request might have started work, but Kandev cannot safely identify its result.

## Scope and planning choices

This draft plans normal interactive tasks with one GitHub repository, text prompts, and Cursor-hosted compute.
The existing local Cursor CLI remains a separate agent family. Each session owns its cloud conversation; sibling sessions do not share it.
A published starting ref is explicit. Cursor creates the output branch. Automatic PR creation defaults off.
These are proposed release boundaries, not claims of shipped behavior.

## Requirements

### REQ-EXECUTORS-CURSOR-CLOUD-001: Configuration and admission

The configured key shares its Cursor account's billing and attribution across authorized profile users.
A profile's visibility does not grant permission to launch work or use its secret.

#### Acceptance criteria

- **AC-EXECUTORS-CURSOR-CLOUD-001.1:** When the feature is enabled, an authorized user shall configure a Cursor Cloud executor profile with a secret reference and a compatible cloud agent profile.
- **AC-EXECUTORS-CURSOR-CLOUD-001.2:** When credentials, callback configuration, model, or repository access are invalid, launch shall fail before remote work starts. Errors shall not reveal secrets.
- **AC-EXECUTORS-CURSOR-CLOUD-001.3:** When the feature is disabled, new configuration, discovery, launches, and follow-ups shall be unavailable through every entry point. Existing records shall remain readable.
- **AC-EXECUTORS-CURSOR-CLOUD-001.4:** When a profile combination is incompatible, Kandev shall reject it without falling back to local execution. Existing executor selections shall retain their behavior.

- **AC-EXECUTORS-CURSOR-CLOUD-001.5:** When a user selects a cloud profile, Kandev shall check profile-use and task/workspace authority before dispatch. Configuration shall disclose shared account billing.

- **AC-EXECUTORS-CURSOR-CLOUD-001.6:** The Agents page shall show the Cursor Cloud agent type only when the user has access to a saved, configured Cursor Cloud executor profile. An unsaved or incomplete profile shall not qualify.
- **AC-EXECUTORS-CURSOR-CLOUD-001.7:** Removing the last qualifying executor shall hide the agent type without deleting saved agent profiles or conversation history. Temporary connection failures shall not hide it.

### REQ-EXECUTORS-CURSOR-CLOUD-002: Conversation and turn lifecycle

#### Acceptance criteria

- **AC-EXECUTORS-CURSOR-CLOUD-002.1:** When an eligible task starts, Kandev shall create one cloud conversation and submit its initial prompt once. Only published repository content shall be included.
- **AC-EXECUTORS-CURSOR-CLOUD-002.2:** When a user submits a follow-up, Kandev shall retain the conversation and serialize turns. Concurrent submissions shall not create concurrent remote runs.
- **AC-EXECUTORS-CURSOR-CLOUD-002.3:** When a user stops work, Kandev shall show cancellation pending until remote termination is confirmed. The conversation shall remain available for later follow-ups.
- **AC-EXECUTORS-CURSOR-CLOUD-002.4:** When remote work finishes, Kandev shall show its outcome without automatically treating it as workflow-step completion. Existing completion and question gates shall apply.
- **AC-EXECUTORS-CURSOR-CLOUD-002.5:** When submission has an unknown outcome, Kandev shall expose that uncertainty and block automatic resubmission. It shall not claim failure or create replacement work.

- **AC-EXECUTORS-CURSOR-CLOUD-002.6:** When a normal task workflow generates a prompt, it shall use the same serialization and duplicate-prevention rules as user prompts. Pending question and completion guards shall apply.
- **AC-EXECUTORS-CURSOR-CLOUD-002.7:** When a task is archived, new cloud prompts shall stop immediately and active runs shall enter termination. Unknown outcomes shall remain visible until resolved.

### REQ-EXECUTORS-CURSOR-CLOUD-003: Observation and recovery

#### Acceptance criteria

- **AC-EXECUTORS-CURSOR-CLOUD-003.1:** When a run emits text or tool activity, Kandev shall show that activity in its existing conversation. Duplicate deliveries shall not duplicate messages or completion effects.
- **AC-EXECUTORS-CURSOR-CLOUD-003.2:** When the backend or stream reconnects, Kandev shall recover only the conversation bound to that session. Recovery shall not submit another prompt.
- **AC-EXECUTORS-CURSOR-CLOUD-003.3:** When history is no longer available, Kandev shall preserve saved activity and show a history-gap notice. It shall retrieve the available terminal result.
- **AC-EXECUTORS-CURSOR-CLOUD-003.4:** When credentials fail, the provider is unavailable, or a remote conversation disappears, Kandev shall show a recoverable or terminal condition as appropriate. It shall not silently replace the conversation.
- **AC-EXECUTORS-CURSOR-CLOUD-003.5:** When a run is live or its status is unknown, reconciliation and watchdogs shall not fail or reclaim it merely because local processes or recent stream activity are absent.

### REQ-EXECUTORS-CURSOR-CLOUD-004: Repository results and workspace limits

#### Acceptance criteria

- **AC-EXECUTORS-CURSOR-CLOUD-004.1:** When launch is requested, Kandev shall accept a GitHub repository with a published branch or commit. Unsupported repository shapes shall fail before submission.
- **AC-EXECUTORS-CURSOR-CLOUD-004.2:** When results arrive, Kandev shall show the final response and validated branch and pull-request links associated with the correct repository and session.
- **AC-EXECUTORS-CURSOR-CLOUD-004.3:** When a cloud session is selected, terminal, workspace editing, LSP, previews, and local Git operations shall be unavailable. Stored desktop preferences shall remain unchanged.
- **AC-EXECUTORS-CURSOR-CLOUD-004.4:** When Cursor produces a branch, Kandev shall not automatically fetch, merge, overwrite, or push the local workspace. Pull-request creation shall require an explicit launch choice.

- **AC-EXECUTORS-CURSOR-CLOUD-004.5:** When a cloud conversation exists, model/profile switching, native permission controls, plan-mode switching, and context reset shall be unavailable. Direct requests shall preserve its frozen settings.

### REQ-EXECUTORS-CURSOR-CLOUD-005: Scoped agent tools

#### Acceptance criteria

- **AC-EXECUTORS-CURSOR-CLOUD-005.1:** When a cloud agent calls Kandev tools, authority shall be limited to its current user, workspace, task, session, and execution. Caller-supplied identifiers shall not expand authority.
- **AC-EXECUTORS-CURSOR-CLOUD-005.2:** When an execution grant is expired, revoked, or stale, tool calls shall fail without side effects. New turns shall receive fresh grants.
- **AC-EXECUTORS-CURSOR-CLOUD-005.3:** When an agent asks a user question or signals completion, existing question barriers, title ownership, and completion guards shall remain authoritative.
- **AC-EXECUTORS-CURSOR-CLOUD-005.4:** When the callback cannot be used, Kandev shall show a configuration error before ordinary task launch. It shall not silently run without required tools.

### REQ-EXECUTORS-CURSOR-CLOUD-006: Desktop and phone operation

#### Acceptance criteria

- **AC-EXECUTORS-CURSOR-CLOUD-006.1:** When using desktop or phone, a user shall configure credentials, choose Cursor Cloud, start work, read activity, send follow-ups, stop work, and open result links.
- **AC-EXECUTORS-CURSOR-CLOUD-006.2:** When loading, disconnected, cancelling, or submission-unknown states occur, both layouts shall show the same state and allowed recovery actions.
- **AC-EXECUTORS-CURSOR-CLOUD-006.3:** When using a phone, controls shall have touch targets of at least 44 pixels. Content shall have one main scroll region without document horizontal overflow.
- **AC-EXECUTORS-CURSOR-CLOUD-006.4:** When using keyboard navigation or a supported locale, new controls shall have accessible names and translated copy. Phone navigation shall not overwrite desktop layout preferences.

## Out of scope

- Office, autopilot, automation runs, utility agents, passthrough mode, and dynamic provider routing.
- Multiple repositories, image/file uploads, arbitrary environment forwarding, and private worker pools.
- Existing PR takeover, writes to the starting branch, local branch synchronization, and provider migration within a conversation.
- Remote filesystem, shell, editor, LSP, preview ports, artifact browsing, detailed billing, and new installation/profile concurrency or spending limits. Existing Kandev admission limits remain applicable.
- Automatic provider deletion or archive when a Kandev task is archived or deleted. Stop outstanding work before local deletion.

## Related contracts

- [Profile editor](profile-editor.md).
- [Session survival and capability gating](agent-survival-session-state.md).
- [System design](../system-design/cursor-cloud.md).

## Implementation Plans

- [Cursor Cloud implementation package](../../../plans/cursor-cloud/plan.md).
