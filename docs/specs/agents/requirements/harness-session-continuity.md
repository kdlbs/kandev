---
status: draft
system: agents
created: 2026-09-10
owners:
  - kandev
---

# Harness session continuity requirements

## Overview

The agent system owns continuity across harness processes. Kandev keeps its session identity even when a native harness conversation cannot continue.

This draft defines proposed behavior. It does not claim that the current implementation provides these guarantees.

## Terminology

- Harness: Codex, Claude Code, OpenCode, or another supported agent implementation.
- Native resume: Continue the original harness conversation with its private state.
- Context continuation: Start a new harness conversation with selected Kandev history.
- Incarnation: A durable Kandev session lifetime. Harness generations belong to this lifetime.

## Requirements

### REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001: Restore decisions

**Intent:** A user needs an honest restore result without losing a usable native conversation.

**User story:** As a user, I want a clear restore result, so that I know which conversation continues.

#### Acceptance criteria

- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1:** When native state exists, Kandev must prefer native resume and preserve its identifier after transport, authentication, configuration, or unknown errors.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2:** When native restore is unavailable, Kandev must return a typed reason and an explicit context-continuation action. Opening a session must not authorize replacement.

### REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002: Durable continuation context

**Intent:** Saved Kandev history must support recovery without a harness-specific history file.

**User story:** As a user, I want my saved conversation available after native state loss, so that I can continue the task.

#### Acceptance criteria

- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.1:** When the user selects context continuation, Kandev must prepare a bounded snapshot from the current session's authoritative history before creating a replacement.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.2:** When context delivery has an uncertain outcome, Kandev must preserve its checkpoint and block automatic resend. History must not grant new permissions.

### REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003: Consistent restore configuration

**Intent:** Every restore entry point must preserve effective runtime configuration and the current task session.

**User story:** As a user, I want recovery to preserve my selected configuration, so that it does not change agent behavior.

#### Acceptance criteria

- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1:** When startup, manual recovery, or workspace rebind creates a harness generation, Kandev must use the same restore coordinator and preserve effective configuration.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.2:** When creation or configuration restoration fails, Kandev must retain the previous native identifier and block prompt dispatch until recovery succeeds.

### REQ-AGENTS-HARNESS-SESSION-CONTINUITY-004: Workspace-aware resume

**Intent:** A changed directory must not cause an undocumented native-state replacement.

**User story:** As a user, I want to recover after a workspace move, so that directory restrictions do not erase my conversation.

#### Acceptance criteria

- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.1:** When the workspace changes, Kandev must compare the original agent-visible directory and native-state reference against tested harness capabilities before native resume.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.2:** When a harness cannot resume in the target directory, Kandev must offer explicit context continuation there. Branch replacement must remain a separate native-only action.

### REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005: Visible continuity

**Intent:** Recovery outcomes must remain clear after reload on desktop and mobile.

**User story:** As a user, I want an accurate recovery notice, so that I can distinguish native resume from context continuation.

#### Acceptance criteria

- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1:** When recovery completes or blocks, Kandev must persist its typed outcome and show the source, target directory, and context limitations without exposing secrets.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.2:** Desktop and mobile users must have equivalent recovery actions, accessible status, and localized copy. Recovery must not add horizontal overflow or conflicting scroll containers.

### REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006: Autonomous recovery requires operator action

**Intent:** Unattended work must remain visible and recoverable without unauthorized conversation replacement.

**User story:** As an operator, I want an actionable recovery notice, so that an autonomous run cannot disappear into repeated failures.

#### Acceptance criteria

- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1:** When native recovery blocks an Office or automation run, Kandev must preserve its pending work and persist a recovery requirement. Automatic dispatch must stop.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2:** When an authorized operator resolves recovery, Kandev must retain the original work identity and repeat normal admission checks. Uncertain prompts must not receive automatic resend.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3:** Scheduler ticks, provider retries, configuration reconciliation, status-only agent recovery, and notification dismissal must not clear the recovery requirement or authorize replacement.
- **AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4:** When an unattended run requires recovery, its operator surfaces must show the cause and a session recovery link on desktop and mobile. Repeated observations must not duplicate notices.

## Out of scope

- Implementing a replacement model loop or universal harness state export.
- Automatic continuation after native-state loss without explicit user action.
- Cross-harness migration, machine snapshots, and arbitrary remapping of historical tool paths.
- Changing task queue authority, archive behavior, or existing branch-loss authorization.
