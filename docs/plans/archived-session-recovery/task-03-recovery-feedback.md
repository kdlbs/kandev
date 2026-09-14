---
id: "03-recovery-feedback"
title: "Present compact recovery feedback"
status: done
wave: 3
depends_on:
  - "02-client-archive-lifecycle"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 03: Present compact recovery feedback

## Summary

Show a short automatic recovery summary with applicable actions. Keep each
underlying failure in accessible, initially collapsed details.

## In scope

- Add component regression `keeps both recovery causes in collapsed details`
  to `ensure-session-error.test.tsx`. Cover summary, operation labels,
  expansion/collapse, unknown errors, and unchanged initial-creation errors.
- Retain structured outcome and separate typed resume/restore causes through
  the automatic hook. Do not parse a translated combined error string. Cover
  dual failure, successful read-only fallback, successful retry clearing both
  causes, and archive transitions clearing stale structured feedback.
- Update `SessionRecoveryFeedback` to use short localized copy. Preserve
  nonblocking read-only notices, relevant recovery actions, and actual causes.
  Generic creation errors keep their existing title and detail behavior.
- Disable equivalent Retry controls while the request is pending. Retain
  existing manual recovery behavior, including explicit branch replacement and
  Start fresh confirmation. Preserve typed errors through shared helpers.
- Use an inline semantic disclosure. On mobile, stack actions, provide at least
  44px coarse-pointer hit areas, and wrap long details. On fine-pointer desktop,
  retain compact button sizes. Reuse task layout scroll/safe-area behavior.
- Add failure fixtures and disclosure/retry cases to Task 02's E2E specs.
  Include a missing-workspace failure after unarchive. Assert the transcript
  survives, the view does not claim file recovery, and both error causes can be
  inspected. A successful retry clears stale feedback.
- Add copy to all supported catalogs. Generate the Traditional Chinese pair
  with the repo script and keep pseudo-locale coverage complete.
- Use `/docs-maintainer` to update the archive/recovery section of
  `docs/public/tasks-and-workflows.md`. Explain Unarchive, the automatic-start
  preference, compact details, and the limits of missing-workspace recovery.

## Out of scope

- A generic error framework, redesigning every alert, changing provider
  recovery choices, or silently rebuilding missing workspace ownership.

## Acceptance

1. The disclosure regression fails before presentation changes, then passes
   with both original causes accessible and no raw nested error in the summary.
2. Desktop/mobile tests prove disclosure and retry outcomes, busy disablement,
   keyboard/touch reachability, and absence of horizontal page overflow.
3. Locale checks pass; public docs describe the resulting behavior and limits.

## Verification

Run from `apps/web`, after Task 02's dependency installation:

```bash
rtk pnpm test components/task/ensure-session-error.test.tsx hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts components/task/preview-session-tabs.test.tsx components/quick-chat/quick-chat-session-view.test.tsx components/task/chat/messages/action-message-recovery.test.tsx
rtk pnpm run typecheck
rtk pnpm run i18n:zh-hant
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet
rtk pnpm e2e:run --project chromium tests/task/archived-session-recovery.spec.ts -- --retries=0
rtk pnpm e2e:run --project mobile-chrome tests/task/mobile-archived-session-recovery.spec.ts -- --retries=0
```

Run from repository root after documentation edits:

```bash
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Run the disclosure regression for RED before implementation. Run the full named
files for GREEN, and save collapsed/expanded mobile screenshots. Do not count
planned commands as executed results.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-resumption.ts` and its tests
- `apps/web/hooks/domains/session/use-session-recovery-feedback.ts`
- `apps/web/components/task/ensure-session-error.tsx` and its tests
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/preview-session-tabs.tsx` and its tests
- `apps/web/components/quick-chat/quick-chat-session-view.tsx` and its tests
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
- `apps/web/e2e/tests/task/archived-session-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-archived-session-recovery.spec.ts`
- `apps/web/e2e/helpers/archived-session-recovery.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 02 supplies archive-safe lifecycle state and shared browser fixtures.

## Risks

- A raw combined string loses typed causes and branch-recovery details.
- Changing the shared creation banner globally could hide actionable creation
  errors. Keep automatic recovery rendering explicitly scoped.
- Long identifiers can overflow phone layouts even when the summary fits.
- Missing environment metadata cannot prove that a branch is gone. Offer
  replacement only through the existing typed branch-loss contract.

## Parallelism

`sequential`

## Inputs

- Requirements `002` and `004.6` in the linked plan.
- Design sections: Visible recovery errors; Responsive and accessible behavior.
- Existing `EnsureSessionErrorBanner`, `SessionRecoveryFeedback`,
  `use-session-recovery-feedback.ts`, and recovery component tests.
- Task 02 browser fixtures and existing branch-recovery E2E patterns.
- Mobile-parity inline feedback design in the owning system design.

## Results

RED evidence:

- The new disclosure regression initially failed because automatic resume and workspace-restore errors were combined into the generic recovery presentation and no labeled details surface existed.

GREEN evidence:

- `rtk pnpm test hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts components/task/ensure-session-error.test.tsx components/task/preview-session-tabs.test.tsx components/quick-chat/quick-chat-session-view.test.tsx components/task/chat/messages/action-message-recovery.test.tsx components/task/mobile/session-mobile-top-bar-repository.test.tsx components/task/task-layout-repository.test.tsx`: 9 files, 105 passed.
- `rtk pnpm run i18n:check`: passed for all five supported catalogs and the pseudo locale.
- `rtk pnpm run i18n:ratchet`: passed with zero new-copy violations.
- `rtk pnpm run typecheck`: passed.
- `rtk pnpm run lint`: passed with zero warnings.
- `rtk node --test scripts/validate-public-docs.test.mjs`: 61 passed. `rtk node scripts/validate-public-docs.mjs`: 46 public pages validated.
- `rtk pnpm e2e:run --project=chromium tests/task/archived-session-recovery.spec.ts`: 3 passed. The disclosure case verifies collapsed state, keyboard expansion, both labeled causes, retry, and no horizontal overflow.
- `rtk pnpm e2e:run --project=mobile-chrome tests/task/mobile-archived-session-recovery.spec.ts -- --retries=0`: 3 passed. The disclosure case verifies touch expansion, 44px summary hit area, both causes, retry, and no horizontal overflow; the other cases verify same-session recovery and the prevent-auto-start preference.

Docs:

- Updated `docs/public/tasks-and-workflows.md` with the archived read-only behavior, in-place Unarchive recovery, prevent-auto-start behavior, compact recovery details, retry path, and archive-during-recovery limit.
