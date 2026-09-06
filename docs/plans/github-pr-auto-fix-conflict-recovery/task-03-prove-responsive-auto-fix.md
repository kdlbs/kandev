---
id: "03-prove-responsive-auto-fix"
title: "Prove responsive auto-fix"
status: done
wave: 3
depends_on:
  - "02-explain-auto-fix-signals"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-UI-CI-PR-AUTOMATION-001
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.1
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.2
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.3
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.6
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.9
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.9
  - AC-UI-CI-PR-AUTOMATION-001.8
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.2
system_design:
  - ../../specs/integrations/system-design/github-pr-auto-fix-conflicts.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-merge-queue-recovery-controls.md
---
# Task 03: Prove Responsive Auto-Fix

## Summary

Extend the existing desktop and mobile GitHub automation scenarios. Prove one
ordinary conflict repair prompt, stable-state deduplication, and the complete
help text through the shipped responsive surfaces.

## In scope

- Add a desktop conflict auto-fix scenario to the existing PR automation spec.
- Add the same user outcome to the existing mobile automation spec.
- Use the mock GitHub mergeability update to produce `dirty` state.
- Enable auto-fix through the visible popover or drawer.
- Assert the visible conflict snapshot, one round, and no duplicate round.
- Assert help mentions ordinary conflicts and actionable queue removals.
- Re-run the existing merge-queue recovery scenarios with the new regressions.

## Out of scope

- Live GitHub writes.
- New mock-controller endpoints.
- New responsive composition or geometry.
- Product screenshots or video.

## Acceptance

- Desktop and mobile users can enable auto-fix for an already conflicted pull
  request and observe one agent repair turn.
- Repeating the same `dirty` snapshot does not produce another turn.
- Both surfaces show the complete help text. Existing merge-queue recovery
  remains green.

## Verification

Run from `apps/web`:

```bash
pnpm e2e:run tests/pr/ci-automation-options.spec.ts -- --grep 'merge conflict|merge queue recovery'
pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-ci-automation-options.spec.ts -- --grep 'merge conflict|merge queue recovery'
```

## Files likely touched

- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`

## Dependencies

- Task 01 supplies conflict dispatch and corrected queue-removal eligibility.
- Task 02 supplies the final localized help content.

## Risks

- Automation delivery is asynchronous. Tests must wait on the provider or
  session state that causes the visible transcript change.
- Desktop hover and mobile tap can mount separate responsive surfaces. Each
  assertion must scope to the active surface.

## Parallelism

`sequential`

## Inputs

- Existing desktop and mobile CI automation E2E fixtures.
- Existing merge-queue recovery scenarios and causal wait helpers.

## Results

Added desktop popover and mobile drawer conflict scenarios. Each proves one
repair message and round, stable-state deduplication, visible conflict
context, and complete help copy. The final desktop and mobile targeted E2E
commands each passed 2 tests, including the existing merge-queue recovery
scenario.
