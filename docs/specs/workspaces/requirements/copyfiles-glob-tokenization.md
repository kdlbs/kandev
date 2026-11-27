---
status: active
system: workspaces
created: 2026-10-01
owners:
  - kandev
---

# Repository Copy-file Pattern Requirements

## Overview

Workspaces owns repository configuration and worktree file seeding. Users need
valid copy-file patterns to select the same source files on host and remote
executors. This contract makes the existing grammar from
[ADR 0010](../../../decisions/0010-worktree-copy-files.md) testable.

## Requirements

### REQ-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001: Pattern boundaries

**Intent:** Preserve supported glob syntax when a repository copy-file setting
contains multiple entries.

#### Acceptance criteria

- **AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.1:** Commas inside character
  classes, including negated classes, and nested brace alternation shall remain
  part of their pattern. Braces inside a character class shall not group entries.
- **AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.2:** On POSIX hosts, backslash
  escapes shall preserve escaped delimiters and literal glob characters without
  grouping later entries. On Windows hosts, backslashes shall remain path
  separators and shall not escape entry delimiters or glob syntax.
- **AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.3:** Supported patterns shall
  select the same files and source bytes for host copying and remote byte
  materialization, including adjacent entries with the terminal `:symlink` mode.
- **AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.4:** Pattern boundaries shall
  preserve first-entry precedence, duplicate normalization, literal colons,
  doubled-colon suffix escaping, and rejection of a reserved suffix without a path.

## Out of scope

- New settings fields or additional materialization modes.
- Broadening glob validation at save time.
- Changes to containment, overwriting, file size limits, or worktree lifetime.
- UI changes or remote links to host repositories.

## System design

- [Repository copy-file tokenization](../system-design/copyfiles-glob-tokenization.md)
