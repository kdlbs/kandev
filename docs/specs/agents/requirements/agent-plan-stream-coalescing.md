---
status: active
system: agents
created: 2026-09-17
owners:
  - kandev
---

# Agent Plan Stream Coalescing Requirements

## Overview

Agent providers can stream a growing plan through repeated updates for one tool
invocation. The agent system owns the provider-facing event contract and must
present that logical plan once, without exposing transport snapshots as
separate conversation entries.

## Terminology

- **Plan snapshot:** One complete plan value observed on an agent tool update.
- **Plan stream identity:** The session, turn, and source tool-call identity
  that together identify one logical plan-producing invocation.

## Requirements

### REQ-AGENTS-AGENT-PLAN-STREAM-COALESCING-001: One transcript item per plan stream

**Intent:** Users can read and revisit an agent-authored plan without repeated
cards created by incremental provider transport updates.

**User story:** As a task user, I want one current plan card for each plan
invocation, so that streamed drafting does not overwhelm the conversation.

#### Acceptance criteria

- **AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1:** When one plan-producing
  tool invocation emits multiple snapshots, the conversation shall contain one
  plan item for that invocation and shall show its latest accepted snapshot.
- **AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.2:** When separate plan-producing
  tool invocations occur in the same session or turn, the conversation shall
  keep them as separate plan items.
- **AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.3:** After live delivery, page
  reload, session resume, or history pagination, the conversation shall show
  the same one-item-per-invocation result.
- **AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.4:** When historical messages
  lack a plan stream identity, a contiguous same-turn sequence whose content
  grows strictly by prefix shall render only its final snapshot; non-prefix
  plans and plans separated by another conversation item shall remain distinct.
- **AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.5:** Desktop and phone task
  conversations shall apply the same plan-item cardinality without changing
  the existing plan card controls, scrolling, or touch behavior.

## Out of scope

- Deleting or rewriting historical conversation rows.
- Changing task-plan document storage, revision history, or agent plan-write
  safety.
- Changing plan-card layout, controls, copy, or responsive composition.
