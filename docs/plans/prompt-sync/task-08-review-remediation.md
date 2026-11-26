---
id: "08-review-remediation"
title: "Review remediation"
status: done
wave: 6
depends_on: ["03-promptsync-backend", "05-prompt-sync-ui"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-002
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-001.4
  - AC-INTEGRATIONS-PROMPT-SYNC-002.3
  - AC-INTEGRATIONS-PROMPT-SYNC-003.2
  - AC-INTEGRATIONS-PROMPT-SYNC-002.4
  - AC-INTEGRATIONS-PROMPT-SYNC-002.5
  - AC-INTEGRATIONS-PROMPT-SYNC-003.5
  - AC-INTEGRATIONS-PROMPT-SYNC-003.6
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 08: Review remediation

## Summary

Address PR review findings on the first implementation: authorization race,
oversized files, invalid duplicates, warnings on kept prompts, partial-write
invalidation, and three settings-hook races.

## In scope

- `promptsync`: read and authorize the configuration under the service lock
  and sync that snapshot; record too-large reads as frozen files; group files
  by name before validation; check every remaining synced prompt for
  unresolved references.
- `github`, `gitlab`, `reposync`: `ErrRepoFileTooLarge` sentinels and
  `reposync.IsFileTooLarge`.
- `prompts/service`: publish the change event whenever a write succeeded,
  including on a later error; keep a removed prompt that a surviving saved
  prompt references, following kept prompts to a fixpoint.
- `prompts/store`: run count-limited inserts under a shared PostgreSQL
  advisory lock.
- `use-prompt-sync.ts`: ignore configuration reads that started before a
  mutation, keep edits made while Save is in flight, and re-parse a pasted
  link once the GitHub host is known.

## Out of scope

Workflow Sync's own handling of oversized files.

## Acceptance

Each finding has a regression test that fails without the fix.

## Verification

```bash
cd apps/backend
go test -trimpath -race ./internal/promptsync ./internal/reposync ./internal/workflowsync ./internal/prompts/... -count=1
go test -trimpath ./internal/github ./internal/gitlab -count=1
cd ../web
pnpm exec vitest run hooks/domains/settings/use-prompt-sync components/settings/prompt-sync-section.test.tsx
```

## Results

- RED: each new test failed against the previous head (authorization used the
  replaced workspace; one oversized file failed the run; the invalid twin let
  the valid file apply; kept prompts produced no missing-reference warning;
  the partial write published nothing; nested references were deleted; the
  stale refresh restored a removed
  configuration; Save reset newer edits; the Enterprise link stayed unparsed).
- GREEN: all listed commands pass. The PostgreSQL concurrency test is
  env-gated on `KANDEV_TEST_POSTGRES_DSN` and was not run locally.
