---
id: "03-upgrade-prompt-defaults-safely"
title: "Upgrade prompt defaults safely"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.15
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-automation-02.md
---

# Task 03: Upgrade Prompt Defaults Safely

## Summary

Refresh exact untouched `ci-auto-fix` prompt revisions on startup while
preserving every edited or unknown stored prompt.

## In scope

- Enumerate the shipped legacy prompt bodies from repository history and pin
  their SHA-256 hashes in the prompt store.
- Add a conditional refresher following the existing
  `changes-walkthrough` pattern.
- Require built-in identity, a known content hash, equal creation/update
  timestamps, and an exact compare in the update statement.
- Verify idempotent startup, missing rows, edited rows, unknown legacy content,
  and concurrent-edit protection.

## Out of scope

- Resetting arbitrary user edits.
- A new reset-to-default UI action.
- Storing the immutable outcome protocol in the editable prompt body.
- Migrating `mr-auto-fix`.

## Acceptance

- The June 19 untouched prompt and each later known shipped revision upgrade
  to the current embedded content exactly once.
- Any row with changed content or `updated_at != created_at` remains byte-for-
  byte unchanged.
- Reseeding remains safe on SQLite and Postgres-backed installations.

## Verification

```bash
make -C apps/backend test ARGS='./internal/prompts/...'
make -C apps/backend lint
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/prompts/store/sqlite.go`
- `apps/backend/internal/prompts/store/sqlite_test.go`
- `apps/backend/config/prompts/ci-auto-fix.md`

## Dependencies

None.

## Risks

- Hash lists can miss a shipped revision. Derive them from Git history and add
  a fixture for each accepted hash.
- Timestamp equality is only one preservation guard; the SQL compare must also
  require the exact stored content.

## Parallelism

`sequential`

## Results

Implemented exact-hash, unchanged-timestamp refreshes for the known legacy
`ci-auto-fix` revisions. Added tests proving idempotency and preservation of
edited, unknown, deleted, and concurrently changed prompt rows.
