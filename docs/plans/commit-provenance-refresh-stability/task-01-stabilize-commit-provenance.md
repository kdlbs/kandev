---
id: "01-stabilize-commit-provenance"
title: "Stabilize commit provenance across refreshes"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.7
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
---

# Task 01: Stabilize Commit Provenance Across Refreshes

## Summary

Retain the last successful provider commit list for display while a newer evidence version for the same
pull request is pending or failed. Keep relation classification and all remote actions bound to current,
complete evidence, then prove the task-switch behavior through focused unit and browser regressions.

## In scope

- Add an explicit display-commit projection to the provider commit resource and hook result.
- Carry retained display commits only across source versions with the same stable contribution identity.
- Keep provider head, completeness, errors, and authoritative commits scoped to the current version.
- Route authoritative evidence to relation classification and display evidence to Changes reconciliation.
- Cover pending, failure, success replacement, identity switch, action safety, and task-switch rendering.

## Out of scope

- Provider snapshot persistence.
- Local Git status or backend commit-history changes.
- Commit marker design, translations, or responsive layout changes.
- General Git subscription deduplication.

## Acceptance

- A same-PR version refresh never replaces previous confirmed pushed markers with unpushed arrows while
  the current request is pending or failed. A successful response replaces the retained display
  snapshot.
- A different workspace, repository, or pull request cannot receive retained commits, and late responses
  cannot replace the selected identity or version.
- Retained display commits never establish alignment or divergence and never enable Push, Pull, or
  contribution replacement without complete current evidence.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-pr-commits.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx components/task/changes-panel-remote.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run tests/pr/pr-switcher-changes.spec.ts)
(git diff --check)
```

## Files likely touched

- `apps/web/hooks/domains/github/pr-commits-resource.ts`
- `apps/web/hooks/domains/github/use-pr-commits.ts`
- `apps/web/hooks/domains/github/use-pr-commits.test.ts`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.ts`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.test.tsx`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-remote.test.ts`
- `apps/web/e2e/tests/pr/pr-switcher-changes.spec.ts`

If the existing mock API cannot hold the provider response pending, these test-support files can also
change:

- `apps/web/e2e/helpers/api-client.ts`
- The corresponding backend mock GitHub handler and test

## Dependencies

None.

## Risks

- A combined authoritative/display state can accidentally make stale provider evidence actionable.
- Cache pruning and rapid task switches can drop retained evidence or expose it under the wrong identity.
- The existing E2E mock may need a narrow deterministic latency control to observe the pending interval.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001`, especially
  `AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4` and `.7`.
- `docs/specs/tasks/system-design/remote-contribution-tasks.md` provider-history and failure sections.
- `docs/decisions/2026-08-13-provider-history-changes-enrichment.md`.
- Existing stale-while-revalidate behavior in
  `apps/web/hooks/domains/github/use-active-task-pr-files.ts` and its refresh-retention tests.

## Results

- Added separate display and authoritative commit lists to the provider resource.
- Retained display commits now follow the latest selected source version for each contribution identity.
  A late response cannot become the retained source for a newer version.
- Pending and failed refreshes preserve pushed markers. A successful refresh replaces the retained list.
- Relation classification uses only complete current evidence. All remote actions stay disabled when
  that evidence is pending or failed.
- Added focused coverage for pending, failed, successful, identity-switch, and late-response cases.
- Extended the Chromium task-switch scenario with a failed provider refresh and a pushed-marker check.
- No public docs change is needed because this correction changes no command, setting, label, or user
  workflow.
