---
created: 2026-10-12
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
legacy_specs: []
---

# Fix plan: macOS session recovery identity

## Overview

Repair macOS process ownership verification so retained interrupted submissions
can enter the existing native-conversation recovery path after their owner and
its descendants exit. The user requested immediate implementation and PR
publication. This package records that authorized repair.

## Evidence and root cause

The reported session retained one SQL `dispatching` submission whose journal
record was `interrupted_unknown`. Its journal high-water and acknowledged cursor
both equaled 993. The saved prompt binding included the exact execution,
submission, incarnation, generation, and runtime birth identity. There was no
missing output to invent or prompt to replay.

The saved Darwin birth token ended in `:0`, because the kernel session pointer
is redacted. The parser rejected zero, and live inspection compared a numeric
session ID to the pointer. Separately, `SysctlKinfoProc` returned `EIO` for an
exited PID. Recovery therefore could not prove source termination and left the
old submission unresolved. A later native launch created a generic recovery
block; after explicit Stop, that block also refused cold workspace allocation.

Read-only inspection reproduced both failures. The new macOS subprocess tests
failed before the correction. After the correction, the same saved runtime
identity returned `process_terminated=true` without modifying the database,
journal, recovery block, or running instance.

## Scope and technical approach

Use birth time plus the separately persisted numeric process group/session ID.
Retain compatibility with existing token suffixes, including zero. Require
`ESRCH` from a separate existence check before classifying an `EIO` PID lookup as
exited. Enumerate descendants using `getsid` and birth time, preserve zombie
handling, and fail closed on enumeration errors or changed ownership. Revalidate
each matching process snapshot. Refuse orphan signaling when numeric session
membership cannot prove the dead leader's incarnation.

| Runtime | Evidence | Behavior and validation |
| --- | --- | --- |
| macOS local/worktree | Birth timestamp, numeric process group/session | Real subprocess tests cover live, exited, mismatched, and surviving-child states. |
| Linux / Windows / remote | Existing platform evidence | Platform implementations and admission contracts remain unchanged. |

## Tests

AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23 is primarily covered by
`internal/common/processidentity/processidentity_darwin_test.go`: live capture,
legacy token compatibility, mismatched ownership, exited leader, surviving
child, ambiguous-cleanup refusal, snapshot changes, and unrelated-process survival. Existing lifecycle and
orchestrator recovery tests exercise the consumers and preserved admission
fences. These are OS/runtime changes with no rendered UI change, so browser
screenshots and new browser tests are unnecessary.

## Work orders

- [x] [Task 01: Repair Darwin ownership verification](task-01-darwin-identity.md)

## Verification results

All task-defined race tests, specification/catalog checks, diff checks, and the
PR documentation coverage preflight passed. Review remediation reran the three
race-test commands and full backend lint against the PR base: all passed, with
zero lint issues. See the work order for commands and results.

## Risks and exclusions

Never treat a PID alone, a redacted pointer, or an arbitrary inspection failure
as termination proof. Explicitly stopped sessions remain excluded from automatic
restart; existing authorized Resume remains available. The patch does not
clear durable history, bypass workspace admission, or mutate the user's live
instance. No schema, protocol, UI, or configuration changes.

Public documentation already describes verified automatic restoration and
explicit Stop/Resume in `docs/public/sessions-and-review.md`; this repair makes
macOS conform to that contract. Internal design documentation is updated.
