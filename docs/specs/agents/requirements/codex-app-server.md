---
status: draft
system: agents
created: 2026-09-24
owners:
  - kandev
---

# Codex app-server requirements

## Overview

Users can choose native Codex sessions without replacing their Codex ACP profiles.
The agent system owns provider identity, protocol behavior, and protocol diagnostics.
The [costs system](../../costs/requirements/conversation-usage.md) owns usage attribution and built-in cost displays.

## Terms

- **Native child:** A Codex subagent thread, distinct from a Kandev task or session.
- **Turn:** Work started by one prompt, including multiple model responses and tool calls.
- **Response:** One upstream model completion. A rendered message is not necessarily one response.

## Requirements

### REQ-AGENTS-CODEX-NATIVE-001: Separate gated agent profiles

#### Acceptance criteria

- **AC-AGENTS-CODEX-NATIVE-001.1:** An enabled installation shall offer `codex-app-server`, displayed as `Codex app server`, with independent agent profiles.
- **AC-AGENTS-CODEX-NATIVE-001.2:** The feature shall default to disabled in every shipped runtime profile. Disabled installations shall reject native profile selection and execution.
- **AC-AGENTS-CODEX-NATIVE-001.3:** Existing Codex ACP profiles and conversations shall retain their identity and behavior. Disabling native support shall preserve native history and profile records.
- **AC-AGENTS-CODEX-NATIVE-001.4:** Flag changes shall require a restart. The interface shall distinguish the requested setting from the effective setting before restart.

### REQ-AGENTS-CODEX-NATIVE-002: Normalized native conversations

#### Acceptance criteria

- **AC-AGENTS-CODEX-NATIVE-002.1:** Native sessions shall support prompts, streamed replies, reasoning summaries, tools, plans, model selection, interruption, and resume through existing Kandev conversation surfaces.
- **AC-AGENTS-CODEX-NATIVE-002.2:** Native approvals and questions shall use existing permission and clarification controls. Cancellation or provider resolution shall close the corresponding request.
- **AC-AGENTS-CODEX-NATIVE-002.3:** Repeated or restored events shall not duplicate messages, tool actions, or completion transitions. Unknown optional events shall not terminate the conversation.
- **AC-AGENTS-CODEX-NATIVE-002.4:** Unsupported capabilities shall be unavailable with a reason. Native failure shall not silently switch the session to ACP.

### REQ-AGENTS-CODEX-NATIVE-003: Native child and background activity

#### Acceptance criteria

- **AC-AGENTS-CODEX-NATIVE-003.1:** Users shall see native child identity, nesting, available activity, and execution status without creation of persistent Kandev child tasks.
- **AC-AGENTS-CODEX-NATIVE-003.2:** Parent-turn completion shall not complete active children or background commands. Their later events shall remain visible and correctly attributed.
- **AC-AGENTS-CODEX-NATIVE-003.3:** After reconnect, Kandev shall reconcile provider state before it claims that background work is complete. Unrecoverable state shall appear as unknown.
- **AC-AGENTS-CODEX-NATIVE-003.4:** Interrupting the foreground turn shall not claim that every child stopped. Full session termination shall stop the owned process tree.

### REQ-AGENTS-CODEX-NATIVE-004: Conversation forks

#### Acceptance criteria

- **AC-AGENTS-CODEX-NATIVE-004.1:** A user shall be able to fork through a completed native turn into a new session of the same task.
- **AC-AGENTS-CODEX-NATIVE-004.2:** The fork shall preserve conversation lineage and leave its source unchanged. Inherited history shall not count as new usage.
- **AC-AGENTS-CODEX-NATIVE-004.3:** The fork action shall state that files remain shared in the existing workspace. It shall not create a Git branch or copy files.
- **AC-AGENTS-CODEX-NATIVE-004.4:** Fork creation shall fail without a partial selectable session when the source is active, inaccessible, or unavailable on the selected executor.

### REQ-AGENTS-CODEX-NATIVE-005: Protocol inspection

#### Acceptance criteria

- **AC-AGENTS-CODEX-NATIVE-005.1:** A developer CLI shall probe initialization, models, and capabilities without a model prompt, and record the exact protocol frames locally.
- **AC-AGENTS-CODEX-NATIVE-005.2:** Explicit CLI operations shall exercise prompts, read/resume, fork, interrupt, background observation, and MCP attachment with bounded execution time.
- **AC-AGENTS-CODEX-NATIVE-005.3:** Each capture shall identify the executable version, protocol direction, chronological sequence, request identity, and close reason. Captures shall distinguish absent events from proven failures.
- **AC-AGENTS-CODEX-NATIVE-005.4:** An agent skill shall select the operation from the user's debugging request, inspect the capture, and report evidence and limitations.
- **AC-AGENTS-CODEX-NATIVE-005.5:** Inspection shall not attach to or mutate a live user session by default. Raw captures and child stderr shall remain explicit local diagnostic artifacts.

### REQ-AGENTS-CODEX-NATIVE-006: Desktop and phone access

#### Acceptance criteria

- **AC-AGENTS-CODEX-NATIVE-006.1:** Profile selection, approvals, questions, background inspection, and fork actions shall have equivalent desktop and phone outcomes.
- **AC-AGENTS-CODEX-NATIVE-006.2:** Phone actions shall use visible touch controls. Detailed child history shall have one scroll owner, safe-area clearance, and no document horizontal overflow.
- **AC-AGENTS-CODEX-NATIVE-006.3:** Missing authentication, incompatible protocol versions, and disabled support shall have actionable localized states.

## Out of scope

- Automatic migration or replacement of Codex ACP.
- Import of arbitrary external Codex histories or external live process attachment.
- New worktrees for conversation forks, account switching, and account billing administration.
- Guaranteed availability of internal-only upstream events on every Codex version.

## Implementation plans

- [Native Codex support](../../../plans/codex-app-server/plan.md)
