---
status: active
system: ui
created: 2026-08-02
owners:
  - kandev
---
# Context Compaction Count Requirements

## Overview

Users can see how much of an agent session's context window is currently occupied, but they cannot tell how often the conversation has already been compacted. That history helps them understand when an old session has repeatedly shed context and when starting a fresh session may be preferable.

## Requirements

### REQ-UI-CONTEXT-COMPACTION-COUNT-001: Context Compaction Count

**Intent:** Users can see how much of an agent session's context window is currently occupied, but they cannot tell how often the conversation has already been compacted. That history helps them understand when an old session has repeatedly shed context and when starting a fresh session may be preferable.

#### Acceptance criteria

- **AC-UI-CONTEXT-COMPACTION-COUNT-001.1:** The context-window hover shows a non-negative compaction count for the active agent session.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.2:** Kandev infers an automatic compaction when a valid context-window sample reports fewer used tokens than the previous persisted sample for the same session.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.3:** The first valid sample establishes a comparison baseline and does not increment the count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.4:** Equal or increasing usage does not increment the count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.5:** A successful explicit context reset or model change clears the usage comparison baseline without incrementing or erasing the session's lifetime count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.6:** The hover includes an info control explaining that Kandev derives the count from observed usage drops rather than an ACP compaction event, so missing samples and provider-side resets can make it approximate.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.7:** The count and its explanatory control are available through the existing desktop hover and mobile tap-pinned context-window surface.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.8:** **GIVEN** a session has no persisted context-window sample, **WHEN** its first valid sample arrives, **THEN** Kandev stores the sample and shows `0` compactions without incrementing the count.

## Migrated source detail

## Why

Users can see how much of an agent session's context window is currently occupied, but they cannot tell how often the conversation has already been compacted. That history helps them understand when an old session has repeatedly shed context and when starting a fresh session may be preferable.

## What

- The context-window hover shows a non-negative compaction count for the active agent session.
- Kandev infers an automatic compaction when a valid context-window sample reports fewer used tokens than the previous persisted sample for the same session.
- The first valid sample establishes a comparison baseline and does not increment the count.
- Equal or increasing usage does not increment the count.
- A successful explicit context reset or model change clears the usage comparison baseline without incrementing or erasing the session's lifetime count.
- The hover includes an info control explaining that Kandev derives the count from observed usage drops rather than an ACP compaction event, so missing samples and provider-side resets can make it approximate.
- The count and its explanatory control are available through the existing desktop hover and mobile tap-pinned context-window surface.

## Data model

`task_sessions.metadata` owns both the current sample and the lifetime count:

- `context_window`: existing nullable object containing `size`, `used`, `remaining`, `efficiency`, and `source`; its `used` value is the comparison baseline.
- `context_compaction_count`: non-negative integer. Absence is equivalent to `0` for sessions created before this feature.

The count belongs to one `task_sessions` row. It is not shared across sessions for the same task and is not reset when `context_window` is cleared.

## API surface

No new HTTP route or ACP extension is introduced. Existing session metadata and `session.state_changed` payloads expose `context_compaction_count` alongside `context_window`. Context-window frontend state represents the count as `compactionCount` for the active session hover.

## Failure modes

- If a context-window update cannot be persisted, neither its baseline nor a derived increment becomes visible; Kandev logs the failure and a later valid sample compares against the last successfully persisted sample.
- ACP does not identify compaction events. A provider-side usage reset that appears as a decrease can be counted as a compaction, and compactions between missing usage samples can be missed. The UI disclosure states that the value is approximate.
- Invalid or unusable context-window reports remain hidden under the existing reliability rules and do not create user-visible count-only UI.

## Persistence guarantees

- `context_compaction_count` survives backend restarts, agent execution recovery, page refreshes, and explicit context resets for as long as the owning task session exists.
- Restart recovery uses the persisted `context_window.used` value as the next comparison baseline when it is present.
- Legacy sessions without `context_compaction_count` begin at zero without a schema backfill.
- Deleting the owning task session deletes its count with the session metadata.

## Scenarios

- **GIVEN** a session has no persisted context-window sample, **WHEN** its first valid sample arrives, **THEN** Kandev stores the sample and shows `0` compactions without incrementing the count.
- **GIVEN** a session's persisted usage is 120,000 tokens with zero compactions, **WHEN** a valid sample reports 80,000 used tokens, **THEN** Kandev persists and shows a count of `1`.
- **GIVEN** a session already has a compaction count, **WHEN** equal or increasing usage samples arrive, **THEN** the count remains unchanged.
- **GIVEN** a session has a persisted baseline and count, **WHEN** the backend restarts and the next valid sample reports lower usage, **THEN** the persisted count increments exactly once.
- **GIVEN** the same decreased-usage update is delivered more than once, **WHEN** Kandev persists each delivery, **THEN** the count increments only for the first delivery because subsequent deliveries equal the persisted baseline.
- **GIVEN** a session has a non-zero count, **WHEN** the user successfully resets context or changes models, **THEN** the current context-window sample is cleared and the lifetime count remains unchanged.
- **GIVEN** a counted session has a reliable current context-window sample, **WHEN** the user opens its context hover with a pointer or touch, **THEN** the hover shows the count and an accessible explanation that Kandev infers it from usage drops and that it may be approximate.

## Out of scope

- Adding a compaction event to ACP or agent adapters.
- Distinguishing an automatic compaction from every provider-side reset when both produce the same observed usage decrease.
- Counting explicit user context resets or model changes as compactions.
- Aggregating compactions across sessions, tasks, agents, or workspaces.
- Showing a count outside the existing context-window hover.
