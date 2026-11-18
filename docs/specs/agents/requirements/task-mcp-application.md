---
status: active
system: agents
created: 2026-11-14
owners:
  - kandev
---

# Apply saved task MCP packages to existing sessions

## Intent and ownership

A human can save task packages and receive them in an existing conversation without losing work.
Agents owns package authority and provider continuity. Tasks supplies session identity and prompt admission.
This document owns the application lifecycle. Package definitions remain in [built-in management](builtin-mcp-management.md).

This is an exception to AC-AGENTS-BUILTIN-MCP-003.1 and the automatic-restart restriction in AC-AGENTS-BUILTIN-MCP-003.4.
It also changes next-start wording in AC-AGENTS-BUILTIN-MCP-004.2 and AC-AGENTS-BUILTIN-MCP-005.5.
The safe provider replacement in this document governs human task saves. Other starts keep the existing binding rules.
Workspace policy saves, automation templates, and retained-process adoption retain their existing behavior.

## Terms

- **Saved:** The server committed the human's selection.
- **Pending:** A session has not accepted that selection.
- **Applied:** The verified current provider accepts the saved selection. A replacement also requires confirmation of its fresh catalog.
- **Failed:** An application attempt ended without that evidence. The selection remains saved.
- **Safe boundary:** The session has no active or uncertain turn, unresolved question, permission request, or provider-owned background work.

## Requirements

### REQ-AGENTS-MCP-APPLY-001: Safe automatic application

- **AC-AGENTS-MCP-APPLY-001.1:** A successful human task-package save shall request application for every eligible existing session of that task. Unsaved, denied, conflicting, and equivalent effective selections shall not restart sessions.
- **AC-AGENTS-MCP-APPLY-001.2:** A live idle session with proven resume support shall apply automatically at a safe boundary. Running sessions shall finish their current work first. The change shall not cancel work or answer questions.
- **AC-AGENTS-MCP-APPLY-001.3:** Application shall preserve task and session identities, provider conversation, workspace attachments, files, transcript, pending input, and question barriers. It shall not create a new turn, completion signal, or workflow transition.
- **AC-AGENTS-MCP-APPLY-001.4:** A stopped, archived, completed, cancelled, or never-started session shall not start because settings changed. Its next explicitly authorized start shall use the saved selection.
- **AC-AGENTS-MCP-APPLY-001.5:** Unsupported or uncertain resume behavior shall leave settings pending with a reason. Kandev shall not silently start a fresh conversation.
- **AC-AGENTS-MCP-APPLY-001.6:** Applying task MCP settings shall preserve open terminals, their running commands and output, preview URLs, workspace servers, editor connections, and workspace files. Their existence or activity shall not by itself defer application. These resources shall remain usable during application and after a provider restart failure. Work still owned by the provider shall defer application until it settles.

### REQ-AGENTS-MCP-APPLY-002: Selection and delivery integrity

- **AC-AGENTS-MCP-APPLY-002.1:** Application shall use the committed human selection within current workspace limits. An agent, draft, stale request, or retry shall not expand that selection.
- **AC-AGENTS-MCP-APPLY-002.2:** Concurrent saves shall converge on the latest valid saved selection. An older application result shall not report a newer selection as applied.
- **AC-AGENTS-MCP-APPLY-002.3:** Input received during replacement shall remain ordered and deliver at most once under the existing delivery contract. Uncertain previous delivery shall block replacement and automatic replay.
- **AC-AGENTS-MCP-APPLY-002.4:** Removal shall take effect at the safe replacement boundary. Until then, the UI shall identify the old active selection. The change shall not claim immediate revocation.
- **AC-AGENTS-MCP-APPLY-002.5:** Stale, partial, or unrelated catalog observations shall not prove application. A stale tool call shall fail before effects after the old execution loses authority.

### REQ-AGENTS-MCP-APPLY-003: Failure and visible state

- **AC-AGENTS-MCP-APPLY-003.1:** Settings shall remain saved after application fails. The session shall show the failed stage, retained conversation, and an authorized Retry action.
- **AC-AGENTS-MCP-APPLY-003.2:** One session shall have at most one replacement in progress. Duplicate notifications, reloads, and backend restarts shall not cause duplicate turns or restart loops.
- **AC-AGENTS-MCP-APPLY-003.3:** Retry shall use the latest saved selection and preserve input. It shall not reset the conversation or retry automatically after a terminal failure.
- **AC-AGENTS-MCP-APPLY-003.4:** Desktop and phone shall distinguish unsaved, saving, saved/pending, applying, applied, and failed states for each session. Partial task-wide success shall remain visible after reload.
- **AC-AGENTS-MCP-APPLY-003.5:** Controls shall support keyboard and touch access, localized status text, focus return, and saved drafts across viewport changes. Phone controls shall have at least 44px hit targets and no document horizontal overflow.

## Exclusions and delivery

No homelab automation, new package grants, third-party MCP refresh, changing a live provider binding, forced cancellation, workspace-runtime replacement, or automatic Start fresh.
This work did not restart a user task session.
See [design](../system-design/task-mcp-application.md) and [implementation plan](../../../plans/task-mcp-application/plan.md).
