---
id: "04-operator-documentation"
title: "Document the DB history retention policy"
status: done
wave: 3
depends_on: ["01-message-metadata-target", "02-cleanup-snapshot-target"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-006
acceptance_criteria:
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.1
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.2
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.5
system_design:
  - ../../specs/system-page/system-design/db-history-retention.md
---

# Task 04: Document the DB history retention policy

## Summary

Document the periodic policy for operators: what each target reduces, what is
preserved, and how the stored-byte result differs from compaction.

## Scope

- One public operator page describing the retention card: the 14-day
  message-detail window for finished or archived tasks, the completed
  cleanup-snapshot target that runs on the same pass, the preserved data
  (messages, selection, task identity, archive evidence), and that the result is
  a stored-byte reduction with compaction as the separate operator action.
- State that the policy ships disabled, that no automatic `VACUUM` runs, and that
  the sibling tool-payload and archived-data policies keep their own windows.
- Keep engineering and public text English and free of personal paths, internal
  operating data, and credential material.

## Exclusions

- No code change.
- No new documentation site, generator, or verification tool.
- No documentation of the deferred REQ-003 and REQ-004 designs as shipped
  behavior.

## Acceptance conditions

1. The public docs describe each target's eligibility and protections and the
   run flow, without exposing internal identifiers or identifiers of reduced
   rows.
2. The docs match the shipped behavior: disabled by default, a 14-day initial
   message window for finished or archived tasks, no automatic `VACUUM`, and a
   stored-byte reduction distinct from file size.

## Likely files

- `docs/public/**` settings/storage page where the retention cards are described
- `docs/public/**` operator guidance or CLI page, only if the periodic policy
  needs to be described there

## Verification

```bash
git diff --check
```

## Results

Implemented. Added a "Conversation history retention" section to
`docs/public/operations.md` describing both targets, the 14-day and 7-day initial
windows, the run flow, what is preserved, and the stored-byte versus file-size
distinction.
