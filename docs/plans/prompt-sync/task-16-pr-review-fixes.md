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

Automated review of PR #5394 found three defects:

1. The vanished-file pass counted references from the prompt list read before
   the run's writes. A file updated or created in the same run to reference a
   removed prompt did not protect it, so the prompt was deleted.
2. Choosing Repository for an override whose file had left the source saved
   it as synced without a kept local version; the sync that follows deleted
   the only copy.
3. The conflict review read earlier choices with a plain index, so prompt
   names such as `constructor` or `__proto__` picked up inherited values
   instead of the default choice.

## In scope

- `removeVanishedSyncedPrompts` re-reads surviving prompts after the run's
  writes before the reference check.
- `ErrNoRepositoryVersion` in `SavePromptVersions`, mapped to HTTP 409; the
  web helper `hasRepositoryVersion` replaces the row switch with "Removed
  from repository" and the override badge tooltip explains it, in seven
  locales.
- An own-property check in `usePromptConflictReview`.

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
