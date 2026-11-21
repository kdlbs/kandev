---
status: draft
system: agents
created: 2026-11-15
owners:
  - cfl
---

# Dynamic Agent Routing Unclassified Failure Fallback Requirements

## Overview

This spec holds the repeated unclassified failure fallback of
[Dynamic Agent Routing](dynamic-agent-routing.md), kept apart so the main
requirements file stays within its size limit.

## Requirements

### REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002: Repeated unclassified failure fallback

An operator can enable a narrow exception to manual recovery for a task using a dynamic profile.
A streak means consecutive qualifying failures from distinct attempts in one session, workflow step, and concrete candidate.
A matching failure has the same trusted origin, phase, semantic code, and complete diagnostic identity.
An empty normal turn end has no diagnostic, so its identity is a fixed, text-free origin tuple.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.1:** An absent, null, or disabled policy shall retain manual recovery for unclassified failures. Known transient and hard policies shall retain their behavior.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.2:** An enabled candidate policy shall accept a consecutive-failure threshold from 2 through 10. Invalid settings shall fail validation without changing saved configuration.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.3:** Each qualifying failure below the threshold shall require manual recovery. The policy shall not schedule retries or reset waits.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.4:** At the threshold, Kandev shall select the next eligible configured candidate in order. The session, conversation, and logical profile shall remain unchanged. Exhaustion shall require manual recovery without wrapping to an earlier candidate.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.5:** Every counted failure shall have current, trusted evidence of no output under its origin's definition and no effects. Stale, conflicting, duplicate, incomplete, or ambiguous evidence shall never advance the candidate.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.6:** Only terminal provider failures, proven agent-process startup failures, and empty normal turn ends (AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.13) shall qualify. User denials, cancellation, task/repository errors, credential-policy denials, and corrupt resume state shall never qualify.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.7:** A different matching identity shall start a new streak at one. Success, output, effects, classified failure, explicit stop, or a changed step, candidate, logical profile, or profile version shall clear the streak. Repeating the same candidate manually shall preserve the streak. Stale and duplicate events shall leave current state unchanged.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.8:** A restart shall preserve committed counts but shall not schedule work from them. Only a new, independently proven safe failure shall extend a restored streak.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.9:** The optional workflow field `disable_unclassified_fallback` shall default to false. A true value shall veto fallback and clear the streak. Create, update, copy, export, import, synchronization, and step events shall preserve explicit values. An omitted update shall preserve the saved value.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.10:** Policy configuration shall survive settings API and browser read-edit-save round trips. Desktop and phone shall retain the existing recovery actions and route-change presentation.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.11:** Concurrent delivery of one failure shall count once and launch at most one successor. Persistence or workflow-context failures shall stop automatic advancement.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.12:** The exception shall apply only to Kanban/task dynamic sessions. Concrete profiles, utility invocations, and Office routing shall retain existing behavior. An unknown execution scope shall fail closed.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.13:** An enabled, non-vetoed candidate shall count a normal `end_turn` (no error) with current trusted evidence of no answer text, plan content or tool activity/effects, including reasoning-only turns. Reasoning alone shall not clear the streak or authorize replay after other failures. Cancelled, errored, stale, duplicate and terminal-replay completions shall not count. Absent/disabled/vetoed policy or unknown step shall preserve completion behavior without manual recovery. A counted end shall survive its own completion or process exit.

#### Exclusions

This extension adds no automatic retry loop, global candidate health penalty, new provider classification, or new settings controls.
Operators configure the policy through the existing settings API and workflow import/API contracts.
Raw diagnostics, credentials, and prompts shall not appear in streak state or route events.

## System design

The extension uses [its dedicated design](../system-design/dynamic-unclassified-fallback.md).
