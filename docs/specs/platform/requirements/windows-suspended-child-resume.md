---
status: active
system: platform
created: 2026-10-20
owners:
  - kandev
---

# Windows Suspended Child Resume Requirements

## Overview

On Windows, Kandev starts managed helper processes with `CREATE_SUSPENDED`,
assigns each one to a kill-on-close Job Object, and then resumes it. This keeps
every descendant inside the Job Object. Resuming the child used to take a
snapshot of every thread on the system and walk it to find the child's threads.
The cost grew with the number of threads on the machine and was paid on every
managed Git command, agent, script, and utility start.

## Terminology

- **Managed helper process:** As defined in
  [Windows background process console](windows-background-process-console.md).
- **Suspended start:** A managed helper process created with `CREATE_SUSPENDED`
  that stays suspended until it is assigned to its Job Object.

## Requirements

### REQ-PLATFORM-WINDOWS-SUSPENDED-RESUME-001: Resume suspended helpers at a cost independent of the system thread count

**Intent:** Starting a managed helper must not cost CPU time in proportion to
every thread on the machine.

**User story:** As a Windows user running several agent sessions, I want
Kandev's process starts to stay cheap, so that agentctl does not take a large
share of my CPU.

#### Acceptance criteria

- **AC-PLATFORM-WINDOWS-SUSPENDED-RESUME-001.1:** When Kandev resumes a
  suspended start, the system shall resume the child through a handle to that
  child process, without enumerating the threads of other processes.
- **AC-PLATFORM-WINDOWS-SUSPENDED-RESUME-001.2:** When Kandev resumes a
  suspended start, the system shall have assigned the child to its Job Object
  first, so that the child runs no code outside the Job Object.
- **AC-PLATFORM-WINDOWS-SUSPENDED-RESUME-001.3:** When Kandev cannot open or
  resume a suspended start, the system shall report an error, and the owner
  shall terminate the suspended child as before.

## Out of scope

- Which processes use a suspended start, and their Job Object lifetimes.
- Unix process groups and sessions.
