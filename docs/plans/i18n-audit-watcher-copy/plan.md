---
spec: docs/specs/platform/requirements/i18n-audit-watcher-copy.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Watcher And Task Fallback Localization

## Overview

Separate stable watcher domain values from render-time copy, then update every watcher consumer and
the sender-task fallback. Finish by deleting the 12 audit-confirmed obsolete keys from all catalogs
and running focused tests plus the required i18n audits. The confirmed root cause is that display
labels were exported from plain helper modules as English constants or returned as English strings;
the orphan keys survived UI replacements without source references.

## Frontend

### Watcher profile defaults

- In `apps/web/lib/watcher-profile-default.ts`, retain `STEP_DEFAULT` and `resolveProfileId` as the
  untranslated domain contract and remove `STEP_DEFAULT_LABEL`.
- Add a shared catalog key for the step-default display label and resolve it through each dialog's
  existing `useTranslation()` call in GitHub issue/review, GitLab issue/review, Jira, and Linear
  watcher dialogs.
- Keep the sentinel-to-empty-string payload behavior unchanged and update focused helper/dialog tests
  where they assert the rendered option.

### Watcher repository defaults

- In `apps/web/lib/watcher-repository-default.ts`, retain only sentinel/domain behavior and replace
  `branchPlaceholder`'s English return values with a locale-neutral state/key result.
- In `apps/web/components/watcher-repository-fields.tsx`, resolve no-repository, default-branch,
  repository-first, and loading copy from catalog keys during render. Never translate repository
  names, IDs, or branch names.
- Extend `apps/web/lib/watcher-repository-default.test.ts` to prove locale-neutral state selection and
  unchanged sentinel normalization before changing production behavior.

### Sender task fallback

- In `apps/web/components/task/chat/messages/sender-task-badge.tsx`, resolve the unknown-task fallback
  from `task` catalog copy inside the component.
- Extend the nearest existing chat-message component test to render a sender with no live or snapshot
  title and assert the visible fallback and tooltip text.

### Catalog cleanup

- Remove the following confirmed obsolete entries from English, pseudo, Portuguese, and Simplified
  Chinese catalogs: `agents:profileStartModelSettings`, `agents:selectAModel`,
  `github:togglePrEventPromptAutomation`, `settings:configChatAgentPlaceholder`, `settings:mode`,
  `settings:model`, `settings:utilityDefaultModelAriaLabel`, `settings:utilityDefaultWithModel`,
  `settings:utilityModelNotConfigured`, `settings:utilitySelectAgent`,
  `settings:utilitySelectModel`, and `task:currentPRCommits`.
- The source-history audit ties these keys to superseded profile model controls, profile-based utility
  agent pickers, per-PR automation controls, configuration chat selection, core Voice Mode, and an old
  Changes header. Current replacement surfaces already use different, more specific keys.

### Mobile parity

This is copy-only work inside existing responsive dialogs and badges. It changes no composition,
touch target, scrolling, navigation, or viewport branch, so focused component/rendered tests satisfy
the mobile-parity exception; no new mobile Playwright scenario is required.

## Tests

- **What:** watcher sentinels remain stable while placeholder selection becomes locale-neutral.
  **File:** `apps/web/lib/watcher-profile-default.test.ts` and
  `apps/web/lib/watcher-repository-default.test.ts`. **How:** focused Vitest unit assertions.
- **What:** translated watcher defaults appear through representative dialog/shared-field render
  paths without changing submitted IDs. **File:** existing watcher dialog tests and, if the shared
  fields lack a suitable harness, `apps/web/components/watcher-repository-fields.test.tsx`.
  **How:** focused Vitest component tests using the initialized test i18n instance.
- **What:** an absent sender task title renders the translated fallback. **File:**
  `apps/web/components/task/chat/messages/chat-message.test.tsx` or the closest existing sender badge
  harness. **How:** focused Vitest component assertion.
- **What:** catalogs contain no audit-reported orphan keys and new references are valid. **How:**
  `pnpm run i18n:check` and `pnpm run i18n:sweep -- components lib` from `apps/web`.

## Verification Results

- Focused Vitest: 46 tests passed across 7 files, including repository-first and branch-loading rendered placeholder coverage added during PR review.
- CI remediation: 31 focused component tests and `pnpm run typecheck` passed after allowing the disabled branch select to carry an unset presentation value; live locale switching is asserted with exact English and pseudo fallback copy.
- `pnpm run i18n:check`: passed with 0 orphans, pseudo in sync, and 127 unchanged out-of-scope real-locale parity advisories.
- `pnpm run i18n:sweep -- components lib`: completed across 1,850 files; only the two documented prompt-builder plural findings and 91 unrelated review-by-eye candidates remain.

## Implementation Waves And Parallel Candidates

Wave 1:
- [x] [task-01-localize-watcher-and-task-fallbacks](task-01-localize-watcher-and-task-fallbacks.md)

Wave 2:
- [x] [task-02-remove-obsolete-catalog-entries](task-02-remove-obsolete-catalog-entries.md)

Tasks are sequential because both touch shared locale catalogs and the final orphan audit depends on
the new references from Task 01.
