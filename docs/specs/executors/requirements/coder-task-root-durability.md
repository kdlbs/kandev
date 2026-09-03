---
status: draft
system: executors
created: 2026-09-03
owners:
  - kandev
---

# Coder Task Root Durability Requirements

## Overview

Executor profiles that target Coder workspaces need a durable mounted task root.
The executor system owns profile validation and health warnings for unsafe
remote storage placement.

## Requirements

### REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001: Admit only safe Coder task roots

**Intent:** Let operators correct profiles that can lose task work when a Coder
workspace is rebuilt.

#### Acceptance criteria

- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.1:** SSH executor profile
  settings shall expose a prominent Coder durability warning before tasks rely
  on an unverified remote root.
- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.2:** The warning shall identify
  `ssh_workdir_root`, recommend a durable mounted root such as `/work/.kandev`,
  and remain visible on all desktop and mobile SSH profile settings surfaces.
- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.3:** Before an agent runs in a
  detected Coder environment, Kandev shall require the selected SSH profile to
  declare a durable workdir policy and shall fail with actionable configuration
  guidance when that declaration is absent or invalid.
- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.4:** When an SSH task root is
  nested beneath another Git checkout, materialization shall treat the task
  root as uninitialized unless Git reports that exact directory as the
  repository top level; it shall not adopt the ancestor checkout or compare
  its origin as the task repository.
- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.5:** Before creating a task
  directory or starting Direction, Kandev shall reject a Coder workdir root
  that is unavailable, non-writable, non-executable, a filesystem root, or a
  known ephemeral home or temporary path.
- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.6:** A profile can explicitly
  allow a known ephemeral path only through a value named and presented as a
  data-loss risk; the escape hatch shall not bypass availability or write
  checks.
- **AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.7:** Workdir admission shall
  use isolated probes and shall not merge task directories, processes, or ports
  for simultaneous Coder tasks.

## Out of scope

- Creating or mounting persistent Coder volumes.
- Claiming that every path below `/work` is durable without remote health
  evidence.
- Blocking non-Coder SSH profiles solely because their durability is unknown.
