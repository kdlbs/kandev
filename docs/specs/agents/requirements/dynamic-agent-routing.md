---
status: draft
system: agents
created: 2026-08-13
updated: 2026-09-28
owners:
  - cfl
---
# Dynamic Agent Routing Requirements

## Overview

Users often name profiles by the capability they want rather than a provider brand. A task can use a profile named Frontier for planning and one named Balanced for execution. The dynamic profile selects Claude, Codex, OpenCode, or another provider from an ordered list of complete agent profiles.

## Requirements

### REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001: Dynamic Agent Routing

**Intent:** Users often name profiles by the capability they want rather than a provider brand. A task can use a profile named Frontier for planning and one named Balanced for execution. The dynamic profile selects Claude, Codex, OpenCode, or another provider from an ordered list of complete agent profiles.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1:** Kandev always registers one built-in virtual agent family with canonical ID `dynamic` and display name Dynamic. It cannot be disabled or uninstalled, does not expose a CLI command, and is not probed as a concrete inference agent. Profiles created under it have kind `dynamic`.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.2:** Agent settings can create a dynamic profile with a user-defined name, description, icon, and availability scope.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3:** A dynamic profile references existing concrete agent profiles. It never copies or merges their credentials, environment, model, ACP options, flags, permissions, passthrough behavior, or MCP configuration.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.4:** A dynamic profile's candidate list can reference only concrete, launchable profiles. It cannot reference itself, another dynamic profile, or a rich Office identity in the first version. An Office identity's separate `execution_agent_profile_id` binding can reference a concrete or dynamic profile.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.5:** The profile name is the only capability label. Users can create profiles such as Frontier, Balanced, Economy, Review, or Security Review. Kandev stores no class or tier field and assigns no semantics to those names.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.6:** Each profile has an ordered candidate list. A candidate identifies one concrete profile and stores separate transient-error and hard-error policies. Each class can wait for a trusted near reset, retry the same candidate with bounded exponential backoff, then either skip the candidate or stop.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.7:** A concrete profile with `AutoFallback=true` is not an eligible dynamic candidate. The conductor is the only owner of cross-candidate fallback. An explicit `FallbackModel` remains part of the concrete profile's start-model policy. It does not advance the dynamic candidate list, and turn attribution records the model that ran.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.8:** Dynamic profiles and their concrete candidates participate in the existing profile-in-use dependency dialog. A dependency lookup failure blocks the change. Otherwise, the user can cancel or explicitly confirm deletion or disabling. Confirmed changes keep durable bindings unchanged: stale selected profiles fail closed, while stale or disabled candidates become ineligible and another configured candidate can be selected.

### REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002: Repeated unclassified failure fallback

An operator can enable a narrow exception to manual recovery for a task using a dynamic profile.
A streak means consecutive qualifying failures from distinct attempts in one session, workflow step, and concrete candidate.
A matching failure has the same trusted origin, phase, semantic code, and complete diagnostic identity.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.1:** An absent, null, or disabled policy shall retain manual recovery for unclassified failures. Known transient and hard policies shall retain their behavior.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.2:** An enabled candidate policy shall accept a consecutive-failure threshold from 2 through 10. Invalid settings shall fail validation without changing saved configuration.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.3:** Each qualifying failure below the threshold shall require manual recovery. The policy shall not schedule retries or reset waits.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.4:** At the threshold, Kandev shall select the next eligible configured candidate in order. The session, conversation, and logical profile shall remain unchanged. Exhaustion shall require manual recovery without wrapping to an earlier candidate.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.5:** Every counted failure shall have current, trusted evidence of no output and no effects. Stale, conflicting, duplicate, incomplete, or ambiguous evidence shall never advance the candidate.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.6:** Only terminal provider failures and proven agent-process startup failures shall qualify. User denials, cancellation, task/repository errors, credential-policy denials, and corrupt resume state shall never qualify.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.7:** A different matching identity shall start a new streak at one. Success, output, effects, classified failure, explicit stop, or a changed step, candidate, logical profile, or profile version shall clear the streak. Repeating the same candidate manually shall preserve the streak. Stale and duplicate events shall leave current state unchanged.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.8:** A restart shall preserve committed counts but shall not schedule work from them. Only a new, independently proven safe failure shall extend a restored streak.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.9:** The optional workflow field `disable_unclassified_fallback` shall default to false. A true value shall veto fallback and clear the streak. Create, update, copy, export, import, synchronization, and step events shall preserve explicit values. An omitted update shall preserve the saved value.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.10:** Policy configuration shall survive settings API and browser read-edit-save round trips. Desktop and phone shall retain the existing recovery actions and route-change presentation.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.11:** Concurrent delivery of one failure shall count once and launch at most one successor. Persistence or workflow-context failures shall stop automatic advancement.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.12:** The exception shall apply only to Kanban/task dynamic sessions. Concrete profiles, utility invocations, and Office routing shall retain existing behavior. An unknown execution scope shall fail closed.

#### Exclusions

This extension adds no automatic retry loop, global candidate health penalty, new provider classification, or new settings controls.
Operators configure the policy through the existing settings API and workflow import/API contracts.
Raw diagnostics, credentials, and prompts shall not appear in streak state or route events.

## System design

The migrated technical source is split into [part 1](../system-design/dynamic-agent-routing-01.md), [part 2](../system-design/dynamic-agent-routing-02.md).

The repeated-failure extension uses [its dedicated design](../system-design/dynamic-unclassified-fallback.md).
