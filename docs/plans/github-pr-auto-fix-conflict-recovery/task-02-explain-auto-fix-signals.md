---
id: "02-explain-auto-fix-signals"
title: "Explain auto-fix signals"
status: done
wave: 2
depends_on:
  - "01-correct-auto-fix-signals"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-UI-CI-PR-AUTOMATION-001
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.6
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.9
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.7
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.9
  - AC-UI-CI-PR-AUTOMATION-001.8
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.2
system_design:
  - ../../specs/integrations/system-design/github-pr-auto-fix-conflicts.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-merge-queue-recovery-controls.md
---
# Task 02: Explain Auto-Fix Signals

## Summary

Update shared automation help and public GitHub documentation. Explain ordinary
conflicts, actionable queue removals, settled checks, deduplication, and round
limits without changing the switch label.

## In scope

- Update `autoFixRoundExplanation` and `autoFixPromptDescription`.
- Update English, Portuguese, Simplified Chinese, Hong Kong Chinese, Taiwan
  Chinese, and pseudo-locale content through the repository workflow.
- Update the public integration and session-review explanation sections.
- Keep the desktop popover and mobile drawer on the same copy keys.

## Out of scope

- New controls, layouts, routes, or translation keys.
- Backend behavior and E2E tests.
- GitLab auto-fix copy.

## Acceptance

- Shared help names ordinary merge conflicts and actionable failed-check queue
  removals.
- The prompt description states that `{{pr.feedback}}` can include ordinary
  conflict context.
- Public documentation matches the implemented trigger, checkpoint, and round
  behavior.

## Verification

Run the first commands from `apps/web`:

```bash
pnpm run i18n:zh-hant
pnpm run i18n:check
```

Run the remaining commands from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/pt-pt/github.json`
- `apps/web/src/locales/zh-cn/github.json`
- `apps/web/src/locales/zh-hk/github.json`
- `apps/web/src/locales/zh-tw/github.json`
- `apps/web/src/locales/pseudo/github.json`
- `docs/public/integrations.md`
- `docs/public/sessions-and-review.md`

## Dependencies

- Task 01 defines the final shipped trigger behavior.

## Risks

- Longer copy must remain clear in the existing phone drawer.
- Every locale must preserve `{{placeholder}}`, `<0>`, and interpolation tokens.

## Parallelism

`sequential`

## Inputs

- The conflict design's control-help section.
- Existing GitHub automation help and public explanation sections.

## Results

Updated the shared English, Portuguese, Simplified Chinese, Hong Kong Chinese,
Taiwan Chinese, and pseudo-locale GitHub copy, plus both public integration
explanation pages. `pnpm run i18n:check`, public-doc tests and validation, and
specification lint all passed.
