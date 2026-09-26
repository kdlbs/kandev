---
status: active
system: tasks
created: 2026-09-23
owners:
  - kandev
---

# Conversation fork requirements

## Overview

A user can start related work with recorded conversation context through a selected message.
The user can inspect the context and its estimated size before creating the destination.
The task system owns this contract because it owns conversation records, task relationships, and session admission.

This document defines the current product contract for conversation forks.

## Terminology

- **Fork:** A new destination with an explicit copy of selected historical context.
- **Snapshot:** The fixed context that the user previews and attaches to a destination.
- **Cutoff:** The selected message, included in the snapshot.
- **Destination:** A new task, a child of the source task, or a new agent session on the source task.
- **Tool evidence:** Recorded tool input and output, included through one optional switch as historical text.
- **Execution workspace:** The files and environment used by a task. This differs from the Kandev workspace that groups tasks.

## Requirements

### REQ-TASKS-CONVERSATION-FORK-001: Source selection

**Intent:** The selected message defines a stable and understandable fork boundary.

#### Acceptance criteria

- **AC-TASKS-CONVERSATION-FORK-001.1:** An accepted user message or finalized assistant message in an ordinary task session shall expose Fork from here.
- **AC-TASKS-CONVERSATION-FORK-001.2:** The default snapshot shall include available conversation text from the session start through the cutoff, inclusive, in source order.
- **AC-TASKS-CONVERSATION-FORK-001.3:** Later messages and source edits shall not change an existing snapshot. A changed selection shall require a new preview.
- **AC-TASKS-CONVERSATION-FORK-001.4:** Missing or unauthorized cutoffs shall fail before destination creation. An accepted user message remains eligible while its assistant turn is active. An assistant cutoff requires a completed turn and persisted final content. Partial reads shall never appear as complete history.
- **AC-TASKS-CONVERSATION-FORK-001.5:** Users shall be able to choose a later starting message to reduce context. The preview shall identify the selected range.

### REQ-TASKS-CONVERSATION-FORK-002: Portable context

**Intent:** The destination receives useful history without source runtime authority.

#### Acceptance criteria

- **AC-TASKS-CONVERSATION-FORK-002.1:** The default snapshot shall retain visible user and assistant text, code blocks, and completed clarification questions and answers within the cutoff.
- **AC-TASKS-CONVERSATION-FORK-002.2:** The snapshot shall omit reasoning, hidden system instructions, permissions, runtime events, and executable tool protocol records.
- **AC-TASKS-CONVERSATION-FORK-002.3:** One optional switch shall include tool evidence within the range. It shall default to off. The preview shall disclose unavailable payloads.
- **AC-TASKS-CONVERSATION-FORK-002.4:** Attachments shall be excluded by default. Explicitly selected attachments shall remain available to the destination independently of source attachment retention.
- **AC-TASKS-CONVERSATION-FORK-002.5:** Missing selected content shall require removal or retry. The system shall disclose exclusions and shall not silently shorten included text.
- **AC-TASKS-CONVERSATION-FORK-002.6:** Source saved-prompt references and historical controls shall not acquire new authority. The destination shall receive fresh identity and current instructions.

### REQ-TASKS-CONVERSATION-FORK-003: Preview and size

**Intent:** The user knows what context the destination will receive before launch.

#### Acceptance criteria

- **AC-TASKS-CONVERSATION-FORK-003.1:** Each destination form shall show a removable conversation chip with source, range, message count, and loading or ready state.
- **AC-TASKS-CONVERSATION-FORK-003.2:** Hover or keyboard focus shall show a short preview. An explicit action shall open the complete compiled text and attachment inventory.
- **AC-TASKS-CONVERSATION-FORK-003.3:** The preview shall show an estimated token count and estimation method. A known target limit shall also produce a labeled approximate percentage.
- **AC-TASKS-CONVERSATION-FORK-003.4:** Model changes shall refresh the estimate. Unknown limits and unmeasured attachment costs shall remain explicitly unknown.
- **AC-TASKS-CONVERSATION-FORK-003.5:** Fork size shall remain distinct from new instructions and runtime overhead. The interface shall not claim that its estimate measures total context use.
- **AC-TASKS-CONVERSATION-FORK-003.6:** Context estimates shall not block launch, summarize, or shorten the fork. Provider context errors shall use existing launch or prompt error handling.

### REQ-TASKS-CONVERSATION-FORK-004: Destination creation and delivery

**Intent:** Each destination receives the exact snapshot that the user selected.

#### Acceptance criteria

- **AC-TASKS-CONVERSATION-FORK-004.1:** The picker shall offer a new task, a child task, and a new agent on the source task within the same Kandev workspace.
- **AC-TASKS-CONVERSATION-FORK-004.2:** Creation shall preserve the selected destination, profile, model, executor, workflow, and workspace choices under existing admission rules.
- **AC-TASKS-CONVERSATION-FORK-004.3:** The first prompt shall contain the frozen snapshot before the new request. Explicitly selected attachment copies shall join the first prompt's attachment delivery and use ordinary destination materialization. The destination shall start a fresh provider conversation.
- **AC-TASKS-CONVERSATION-FORK-004.4:** Create-without-start, capacity queues, restarts, and launch retries shall preserve the accepted snapshot and its destination binding. A task-bound snapshot shall bind to exactly one first session; later sessions shall not inherit its delivery metadata or attachments.
- **AC-TASKS-CONVERSATION-FORK-004.5:** Repeated submission of one creation request shall return its original destination. It shall not create another destination or append duplicate context.
- **AC-TASKS-CONVERSATION-FORK-004.6:** The destination shall display its fork provenance and retained snapshot after reload. Source edits or deletion after creation shall not rewrite that context.
- **AC-TASKS-CONVERSATION-FORK-004.7:** The fork shall leave source execution and history unchanged. It shall not restore historical files or copy source runtime handles.

- **AC-TASKS-CONVERSATION-FORK-004.8:** New agents shall share the source execution workspace. New tasks shall use a separate execution workspace. Child tasks shall offer both choices. Separate workspaces shall use existing task defaults for the code baseline and require an executor that can provide isolation. Unsupported executor choices shall fail before creation.

### REQ-TASKS-CONVERSATION-FORK-005: Access and retention

**Intent:** Snapshots remain private, bounded, and reliable throughout creation.

#### Acceptance criteria

- **AC-TASKS-CONVERSATION-FORK-005.1:** Preview and creation shall require current source access and destination creation permission. Knowledge of an identifier shall grant no access.
- **AC-TASKS-CONVERSATION-FORK-005.2:** Unattached snapshots shall expire with a visible expiry error. Expired or discarded drafts shall release their compiled payload and staged attachment copies. Attached snapshots and copies shall follow destination retention and survive ordinary backend restarts.
- **AC-TASKS-CONVERSATION-FORK-005.3:** Source deletion before creation shall prevent attachment. Source deletion after creation shall preserve the authorized destination copy.
- **AC-TASKS-CONVERSATION-FORK-005.4:** Failed creation shall not launch an agent or leave a partially attached snapshot. Task-creation rollback shall restore the fork and copied attachments to a retryable draft before removing a partial destination. Storage failures shall preserve recoverable user input.

### REQ-TASKS-CONVERSATION-FORK-006: Desktop and phone parity

**Intent:** Users can complete the same fork flow with a mouse, keyboard, or touch.

#### Acceptance criteria

- **AC-TASKS-CONVERSATION-FORK-006.1:** Phone users shall reach the action through a visible control and select all three destinations without hover.
- **AC-TASKS-CONVERSATION-FORK-006.2:** Phone creation and full preview shall use a focused full-height surface with one active scroll region and safe-area clearance.
- **AC-TASKS-CONVERSATION-FORK-006.3:** Phone controls shall have at least 44-pixel touch targets. Content shall not cause document-level horizontal overflow.
- **AC-TASKS-CONVERSATION-FORK-006.4:** Loading, expiry, unavailable content, and submission errors shall preserve the draft. Back and Escape shall close the current subview before its parent.
- **AC-TASKS-CONVERSATION-FORK-006.5:** All new interface copy shall use the supported locale catalogs. Keyboard navigation and focus return shall reach the same outcomes.

## Out of scope

- Provider-native conversation cloning, private reasoning, and hidden provider memory.
- Historical filesystem restoration, automatic branch cloning, and new workspace ownership rules.
- Cross-workspace transfers, Office sessions, Quick Chat, and terminal-only source transcripts.
- Automatic AI summarization, arbitrary transcript editing, and selection across multiple sessions.
- New MCP fork tools or automatic forks initiated by agents.

## Related artifacts

- [System design](../system-design/conversation-forks.md)
- [Implementation plan](../../../plans/conversation-forks/plan.md)
- [Additional session workspace reuse](additional-session-workspace-reuse.md)
- [Prompt attachments](prompt-attachments.md)
