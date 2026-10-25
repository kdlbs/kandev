---
spec: docs/specs/platform/requirements/i18n-second-audit-gaps.md
created: 2026-08-13
status: completed
---

# Implementation Plan: Additional I18n Audit Gaps

## Approach

Externalize the second-audit gaps without translating stable contracts. Plain configuration will
store catalog keys or locale-neutral states; React hooks/components will resolve keys through
`useTranslation`, and operation helpers will receive resolved copy or a call-time translator. Existing
server/domain error messages remain detail text, with localized owned fallbacks used only when no
detail exists. English, pseudo, Portuguese, and Simplified Chinese catalogs must retain key parity.

## Implementation

### Stable-value refactors

- Replace `label` strings in `lib/keyboard/shortcut-overrides.ts` with typed catalog keys. Resolve them
  in all Settings consumers without changing shortcut IDs, persistence, or bindings.
- Replace Azure DevOps display strings in `azure-devops-mode-tabs.tsx`, `azure-devops-status.ts`, and
  `azure-devops-workspace-defaults.ts` with typed keys/states and update their consumers. Preserve WIQL,
  IDs, provider/status values, and prompt templates byte-for-byte.

### Operation and toast copy

- Localize review finding actions, send-to-agent results, review-dialog discard copy, task-plan
  fallback errors, walkthrough request states, file-operation fallbacks, task creation/launch results,
  and sidebar synchronization titles.
- Complete the source-audit clusters in task/session/file operations, task movement, task creation,
  GitHub/GitLab feedback, and related hooks/components. At minimum inspect and address
  `use-session-actions.ts`, `use-commit-diff.ts`, `use-task-workflow-move.ts`, `use-implement-fresh.ts`,
  `use-plan-actions.ts`, `use-mr-feedback.ts`, `use-task-mr.ts`, `use-file-editors.ts`,
  `use-open-session-in-editor.ts`, task-create setup/effects/fresh-branch consent, new-session actions,
  file-browser actions, quick task launchers, and task-deleted/removal notifications.
- Use `rg` over toast calls and plain-object `title`, `description`, and `message` fields. Translate only
  text proven to reach users; preserve console diagnostics and raw domain/server detail.

### Isolated surfaces and demos

- Localize the task PR shortcut fallback and Mermaid toast title at call/render time.
- Make the orphan swimlane's stable object locale-neutral and inject the localized title into the
  rendered step without changing filtering/move behavior.
- Resolve the 21 lint violations in `app/demo/agent-messages/page.tsx` and
  `app/demo/messages/page.tsx`; localize page chrome, counts with plural keys, and accessibility copy,
  but retain intentional message fixture payloads.

### Guard and catalogs

- Add keys to the narrowest existing namespaces (`review`, `task`, `common`, `settings`, `azuredevops`,
  `sidebar`, and others matching surface ownership) across every locale.
- Append every newly migrated path not already covered to `i18nGuardFiles`; never remove entries.

## Mobile Parity

This work changes copy and locale-resolution timing only. Existing desktop/mobile composition,
navigation, overlays, scrolling, and touch targets remain unchanged. The nearest exemplars are the
currently shipped versions of each touched dialog, toast, Settings row, kanban lane, and demo page.
Focused rendered tests at the existing responsive surfaces satisfy mobile parity; no new mobile E2E
is required unless implementation changes structure or interaction.

## TDD And Verification

- Add failing focused assertions before changing voice mapping, Azure/shortcut metadata, orphan-step
  derivation, or any helper whose output contract changes.
- Update existing hook/component tests for translated toast payloads and live locale switching where a
  component remains mounted across locale changes.
- Run each task's focused Vitest command, then from `apps/web` run:

```bash
pnpm run i18n:check
pnpm run i18n:sweep -- app components hooks lib
pnpm run lint:i18n -- <changed paths>
```

Record every remaining sweep candidate and its intentional untranslated category. Run focused
rendered/browser verification for the touched UI; if unavailable, record the exact blocker.

## Risks

- Translating an error message too early can freeze locale or accidentally alter a stable domain
  contract. Keep keys/states separate from presentation and resolve at call/render time.
- Helper exports shared by tests and non-React consumers may require typed translation-key metadata
  rather than a hook dependency.
- The broad source audit can include fixture/domain strings that must not be translated; each candidate
  needs reachability review rather than mechanical replacement.

## Tasks

- [x] [Task 02: Azure and shortcut metadata](task-02-azure-shortcut-metadata.md)
- [x] [Task 03: Review and plan operations](task-03-review-plan-operations.md)
- [x] [Task 04: Production operation audit](task-04-production-operation-audit.md)
- [x] [Task 05: Isolated UI leaks](task-05-isolated-ui-leaks.md)
- [x] [Task 06: Demo pages and final audit](task-06-demo-pages-final-audit.md)

Task 01 was removed after rebasing onto main because `#2576` removed core Voice Mode in favor of the
Voice plugin. Remaining tasks are sequential because they share locale catalogs and guard
configuration. No implementation subagents are authorized by this plan.

## Verification Results

- Focused Vitest: 48 tests passed across 8 final regression files; earlier expanded runs passed 51
  tests across 10 files and 43 tests across 11 files.
- `pnpm run typecheck`: passed.
- Targeted `pnpm run lint:i18n -- <changed paths>`: passed with only specialized-config warnings for
  unrelated rule suppressions.
- `pnpm run i18n:check`: passed; 127 pre-existing real-locale advisories and two pre-existing orphan
  keys (`task:downloadingPercent`, `task:failedToLoad`) remain.
- `pnpm run i18n:sweep -- app components hooks lib`: completed across 2,445 files. Remaining hits are
  intentional agent/demo fixture payloads, WIQL/query syntax, prompt/markdown builders, console
  diagnostics, code examples/selectors, provider/domain values, and formatting glyphs. The two plural
  findings are known agent-facing CI prompts and must remain English.
- `node scripts/check-guard-allowlist.mjs`: passed with 508 entries and 26 additions.
