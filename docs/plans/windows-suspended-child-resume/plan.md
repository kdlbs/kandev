---
status: done
created: 2026-10-23
requirements:
  - REQ-PLATFORM-WINDOWS-SUSPENDED-RESUME-001
system_design:
  - ../../specs/platform/system-design/windows-suspended-child-resume.md
---

# fix(winproc): resume suspended children through their process handle

Resume the exact suspended Windows child through its process handle with work independent of the system thread count.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Job ownership changes, force termination, and changes to non-Windows launch behavior. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.
