---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
legacy_specs: []
---

# Implementation Plan: Attachment installation without hard links

## Overview

Restore attachment materialization on filesystems that reject hard links.
This completed record accompanies the user-authorized small-fix workflow,
which explicitly omitted a separate planning handoff.

## Scope

Agentctl attachment installation, causal regression tests, and the existing
materialization design. Other hard-link callers and security boundaries remain
outside this repair.

## Technical approach

`installMaterializedAttachment` keeps its hard-link fast path and collision
naming. A failed link falls back to exclusive creation and a bounded copy with
private permissions. A copy must match the staged size and close successfully.
Failed copies are removed, and installation errors retain their filesystem
cause in logs while the HTTP response stays generic.

## Tests

`attachments_install_test.go` covers unsupported link errors, exclusive
collision handling, symlink preservation, private permissions, source failures,
write/close errors, incomplete copies, cleanup, and safe error logging.
`attachments_test.go` exercises HTTP materialization and concurrent names on
both installation paths, plus descriptor-size and traversal rejection.
These cover AC-TASKS-PROMPT-ATTACHMENTS-001.1, .5, and .7. The HTTP boundary
provides end-to-end evidence for this backend repair; rendered UI is unchanged.

## Work orders

- [x] [Task 01: Restore portable attachment installation](task-01-install-fallback.md)

## Verification results

The work order records exact commands and successful local results. Parent
validation independently reran the focused race tests and passed. Remote CI,
semantic review, and merge remain delivery gates outside this implementation
record.

## Risks

Unsupported-link errors are forced in tests. No real EdenFS/FUSE smoke test
was available. Exclusive-copy destinations exist during copying, but success
is not returned before the byte-count and close checks finish.
