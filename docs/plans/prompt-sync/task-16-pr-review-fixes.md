---
id: "16-pr-review-fixes"
title: "PR review fixes for references, missing repository versions, and conflict choices"
status: done
wave: 14
depends_on: ["15-independent-rows-and-drafts"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-003.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.2
  - AC-INTEGRATIONS-PROMPT-SYNC-009.1
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 16: PR review fixes for references, missing repository versions, and conflict choices

## Summary

Automated and independent review of PR #5394 found these defects:

1. The vanished-file pass counted references from the prompt list read before
   the run's writes. A file updated or created in the same run to reference a
   removed prompt did not protect it, so the prompt was deleted.
2. Choosing Repository for an override whose file had left the source saved
   it as synced without a kept local version; the sync that follows deleted
   the only copy.
3. The conflict review read earlier choices with a plain index, so prompt
   names such as `constructor` or `__proto__` picked up inherited values
   instead of the default choice.
4. Adoption checked only ID and manual ownership, so a prompt renamed or
   edited after the sync read it could be adopted.
5. A GitHub contents listing stops at 1,000 entries, so files beyond it were
   treated as removed.
6. The removal checkbox deleted every repository-only prompt, including one
   synced after the review, and release left `updated_at` unchanged so a
   stale save could overwrite the kept version.
7. A pending Repository choice stayed in effect after the override's file
   left the source.
8. An unsaved edit of a prompt that left the list was dropped without a
   warning.
9. A file with a NUL character passed validation and aborted the apply on
   PostgreSQL.

## In scope

- `removeVanishedSyncedPrompts` re-reads surviving prompts after the run's
  writes before the reference check.
- `ErrNoRepositoryVersion` in `SavePromptVersions`, mapped to HTTP 409; the
  web helper `hasRepositoryVersion` replaces the row switch with "Removed
  from repository" and the override badge tooltip explains it, in seven
  locales.
- An own-property check in `usePromptConflictReview`.
- Adoption guarded by the name and content the sync read; a truncated GitHub
  listing fails the run (`ErrListingIncomplete`); NUL bytes make a file
  invalid.
- Removal deletes only the reviewed `delete_repository_only` names and
  advances `updated_at` on released prompts.
- `choiceFor` and `isSwitched` ignore a pending Repository choice when there
  is no repository version; `orphanedDrafts`, a notice, and a recreate save
  keep the text of a prompt removed while edited, in seven locales.

## Out of scope

- Schema changes; no new columns or routes.

## Verification

- `go test -trimpath ./internal/prompts/... ./internal/promptsync/...`
- `pnpm exec vitest run lib/settings components/settings hooks/domains/settings`
- E2E: `prompt-sync.spec.ts`, `prompt-sync-conflicts.spec.ts` (chromium);
  `mobile-prompt-sync.spec.ts` (mobile-chrome)
- `make fmt`, `make typecheck lint`; spec lint and docs validation

## Results

- Regression tests first failed with the reported behavior, then passed:
  `TestApplySyncedPromptsKeepsPromptsReferencedByThisRunsWrites`,
  `TestSavePromptVersionsRejectsRepositoryWhenTheFileIsGone`,
  `TestSaveVersionsRouteRejectsRepositoryWhenTheFileIsGone`, the
  Object-property names case in `use-prompt-conflict-review.test.ts`, and the
  removed-file row and helper cases.
- Follow-up regressions: `TestAdoptionSkipsAPromptRenamedOrEditedAfterTheSyncReadIt`,
  `TestReleaseDeletesOnlyTheReviewedRepositoryOnlyPrompts`,
  `TestReleaseAdvancesTheRevisionOfReleasedPrompts`,
  `TestDeleteConfigKeepsRepositoryOnlyPromptsTheUserDidNotReview`,
  `TestParsePromptFilesFreezesAFileWithANulCharacter`,
  `TestCompleteListingRejectsACappedGitHubDirectory`, and the detached-switch,
  orphaned-draft, and draft-meta web tests.
