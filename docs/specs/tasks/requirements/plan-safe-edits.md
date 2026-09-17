---
status: active
system: tasks
created: 2026-09-16
owners:
  - kandev
---

# Safe agent plan edits Requirements

## Overview

Agents must correct routine plan-edit mistakes without losing task context or requiring a user to restore history.
The tasks system owns this contract because it owns the shared plan and its history.
The contract applies to agent plan tools. Browser editing retains its current interaction contract.

## Terminology

- **Edit version:** an opaque value that identifies one committed plan state. It changes even when history combines consecutive writes.
- **Suspicious reduction:** replacement of at least 2,000 Unicode code points with fewer than half that number.
- **Exact edit:** replacement of one uniquely matching text fragment within the current plan.
- **Restore:** a new history entry that copies a selected revision. It does not erase history.

## Requirements

### REQ-TASKS-PLAN-SAFE-001: Reject accidental replacement before storage

#### Acceptance criteria

- **AC-TASKS-PLAN-SAFE-001.1:** An agent replacement shall require the current edit version when a plan already exists, including replacement through the create tool.
- **AC-TASKS-PLAN-SAFE-001.2:** A missing or stale edit version shall reject the replacement without changing content, title, history, or implementation state.
- **AC-TASKS-PLAN-SAFE-001.3:** A suspicious reduction shall require explicit truncation acknowledgement and a matching edit version. Otherwise, the plan shall remain unchanged.
- **AC-TASKS-PLAN-SAFE-001.4:** An acknowledged reduction shall preserve the preceding content in history and create a separate revision. Unknown history shall reject the write.
- **AC-TASKS-PLAN-SAFE-001.5:** An unreadable current plan shall reject an agent replacement. The result shall distinguish this condition from an absent plan.
- **AC-TASKS-PLAN-SAFE-001.6:** Initial creation shall work without an edit version. A concurrent creation shall not turn that request into an unchecked replacement.

### REQ-TASKS-PLAN-SAFE-002: Detect intervening edits

#### Acceptance criteria

- **AC-TASKS-PLAN-SAFE-002.1:** A plan read shall return exact content and its edit version from the same committed state.
- **AC-TASKS-PLAN-SAFE-002.2:** Every successful content or title write shall produce a new edit version, including identical writes, history coalescing, and browser writes.
- **AC-TASKS-PLAN-SAFE-002.3:** Versions shall survive restarts and remain distinct after plan deletion and recreation. A rejected write shall not change the version.
- **AC-TASKS-PLAN-SAFE-002.4:** Concurrent agent edits with one expected version shall admit at most one write. Other callers shall receive a conflict with no mutation.
- **AC-TASKS-PLAN-SAFE-002.5:** Plan comments and implementation-start markers shall remain intact after edits and restores. Marker-only changes shall not invalidate an edit version.

### REQ-TASKS-PLAN-SAFE-003: Edit one fragment without resending the plan

#### Acceptance criteria

- **AC-TASKS-PLAN-SAFE-003.1:** An exact edit shall require the current edit version and a nonempty fragment that occurs exactly once.
- **AC-TASKS-PLAN-SAFE-003.2:** A successful exact edit shall change only that fragment. It shall preserve every other byte and the title.
- **AC-TASKS-PLAN-SAFE-003.3:** Missing, ambiguous, or stale matches shall reject the edit without mutation. No fuzzy matching or whole-document fallback shall occur.
- **AC-TASKS-PLAN-SAFE-003.4:** Exact edits shall obey the composed-content size limit and suspicious-reduction protection. An empty replacement fragment shall permit deletion.
- **AC-TASKS-PLAN-SAFE-003.5:** Append shall retain its existing composition and retry rules. An optional edit version shall reject a stale append when supplied.

### REQ-TASKS-PLAN-SAFE-004: Read and restore exact revisions through agent tools

#### Acceptance criteria

- **AC-TASKS-PLAN-SAFE-004.1:** An authorized agent shall list revision metadata in bounded pages and fetch a selected revision's exact title and content.
- **AC-TASKS-PLAN-SAFE-004.2:** A restore shall require the current edit version and the selected revision's snapshot version. A changed source or destination shall reject the restore.
- **AC-TASKS-PLAN-SAFE-004.3:** Restore shall accept only a revision belonging to the addressed task. It shall create a separate, agent-attributed revision with a source reference.
- **AC-TASKS-PLAN-SAFE-004.4:** A restore shall preserve the previous HEAD and source revision. Missing or unverifiable history shall reject the request without mutation.
- **AC-TASKS-PLAN-SAFE-004.5:** Agent restore shall require an existing plan. Missing plans and revisions shall return distinct outcomes without creating replacement content.
- **AC-TASKS-PLAN-SAFE-004.6:** Unauthorized requests shall reveal no plan content, revision metadata, or version tokens. Tools shall retain existing task reach restrictions.
- **AC-TASKS-PLAN-SAFE-004.7:** Stored oversized revisions shall remain readable and restorable under the existing historical-content exemption.

### REQ-TASKS-PLAN-SAFE-005: Guide correction without a forced task stop

#### Acceptance criteria

- **AC-TASKS-PLAN-SAFE-005.1:** Rejections shall carry a stable reason, an explicit no-write result, and a correction action. They shall not report lost content.
- **AC-TASKS-PLAN-SAFE-005.2:** Tool guidance shall direct routine correction and continuation. It shall forbid reconstruction from memory and automatic truncation overrides merely to clear an error.
- **AC-TASKS-PLAN-SAFE-005.3:** Recovery guidance shall require an identified source and the absence of intervening edits before automatic repair of the agent's own overwrite.
- **AC-TASKS-PLAN-SAFE-005.4:** A conflict shall direct a fresh read and reconciliation. Unresolved intent or conflicting user changes shall require user input, not a blind restore.
- **AC-TASKS-PLAN-SAFE-005.5:** Successful write acknowledgements shall report the new edit version without echoing the full plan. Explicit reads shall preserve exact content.
- **AC-TASKS-PLAN-SAFE-005.6:** A lost response shall not justify a blind append or a refreshed-token retry. The agent shall inspect current state before another mutation.
- **AC-TASKS-PLAN-SAFE-005.7:** Agent recovery tools shall be available on each existing surface that permits plan writes. Read-only or excluded surfaces shall gain no write authority.

## Relationship to existing contracts

This is the successor to the agent safety policy in
[plan write consistency](plan-write-consistency.md) and
[append mode](plan-write-append-mode.md). The affected agent clauses now use
this contract; the older documents remain the traceability source.
The [design](../system-design/plan-safe-edits.md#contract-transition) identifies each replaced clause.

The implementation is complete. Agent plan tools use this contract; browser
editing keeps its existing interaction and payload behavior.
Append composition, authorization, history serialization, and the
[size limit](plan-content-size-limit.md) retain their owners.

## Out of scope

- Browser conflict controls, layout changes, and browser truncation policy.
- Automatic task restart, model changes, or guarantees that an LLM always continues.
- Markdown heading parsers, fuzzy patches, multi-edit batches, or automatic plan summarization.
- Append deduplication, revision retention changes, and multi-document migration.
- Restoration of the incident task during this planning work.

## Implementation Plans

- [Safe agent plan edits](../../../plans/plan-safe-edits/plan.md)
