---
status: active
system: executors
created: 2026-08-17
owners:
  - tbd
---
# Executor-Profile Environment Precedence Requirements

## Overview

Environment values split into two kinds that today share one home:

## Requirements

### REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001: Executor-Profile Environment Precedence

**Intent:** Environment values split into two kinds that today share one home:

#### Acceptance criteria

- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.1:** global-secret references (`validateGlobalProfileEnvRefs`, `service_resources.go:1665`), and
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.2:** the Sprites token requirement.
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.3:** "the same key bound to different secret IDs blocks launch";
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.4:** "an executor literal and repository secret using the same key block launch, even if their current plaintext happens to match";
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.5:** "a repository key colliding with a managed runtime value blocks launch rather than replacing it";
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.6:** "repository order and task-repository position never choose a winner";
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.7:** alternative 6, "Compare decrypted values and deduplicate equal plaintext", was **rejected**.
- **AC-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001.8:** **AC-1** WHEN a key has one literal definition from origin `executor profile` and one literal definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key to the `executor profile` value and SHALL NOT return an error.

## System design

The migrated technical source is split into [part 1](../system-design/executor-profile-env-precedence-01.md), [part 2](../system-design/executor-profile-env-precedence-02.md), [part 3](../system-design/executor-profile-env-precedence-03.md), [part 4](../system-design/executor-profile-env-precedence-04.md), [part 5](../system-design/executor-profile-env-precedence-05.md).
