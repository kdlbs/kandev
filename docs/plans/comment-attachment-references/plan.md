---
created: 2026-11-22
status: implemented
requirements:
  - REQ-TASKS-COMMENT-FILES-001
system_design:
  - ../../specs/tasks/system-design/comment-attachment-references.md
legacy_specs: []
---

# Add attachment references to task comments

## Scope and sequence

Deliver the plan-comment slice first. It establishes durable file ownership before the receiving session is known. Reuse its path rendering and composer controls for session feedback. Clarification responses already use per-question file references. Preserve that route as a regression control. Imported PR feedback and sessionless Office thread extracts are outside this package.

## UI preview

```text
Desktop popover                 Phone sheet
[Selected text]                 [Selected text]
[Comment field]                 [Comment field]
[Attach (2)]  [Save]            [Attach (2)]     [Save]

File list open
screen.png   Ready       Remove
notes.txt    Failed      Retry   Remove
```

## Tests

Extend plan repository and message admission tests for ownership transfer, rejection and rollback. Verify same-name file identity, mixed ordinary and comment attachments, and queued replay at the execution boundary. Extend desktop and mobile task-plan-comments E2E tests for cross-session Send and primary Run. Add desktop and phone comment-attachment-references Playwright cases for every session source. Check final received text and open the referenced bytes in the mock execution. Run the existing clarification MCP result test.

## Work orders

- [x] [Deliver files with task plan comments](task-01-plan-files.md)
- [x] [Deliver files with session comments](task-02-session-files.md)

## Verification results

Nine targeted browser scenarios passed across Chromium desktop and mobile. The tests cover both file editors, Review, plan Send and Run, walkthroughs and agent-output feedback. The same-name file test read the bytes at both paths from the actual mock agent prompt. Focused backend delivery and web composer tests passed. The web production build, TypeScript check, full web lint and localization ratchet passed. The browser runs used rebuilt local binaries with the repository freshness check disabled for ancillary fixtures. SSH terminal delivery was verified with a test agentctl endpoint. See the work orders for package-level results and host limitations.

PR review verification covered replay on SQLite and PostgreSQL 17, legacy file comment IDs containing file paths, retry after lost acknowledgement, and SSH startup with the materialized prompt. The focused web suite passed 85 tests. The phone walkthrough passed in portrait and landscape.
