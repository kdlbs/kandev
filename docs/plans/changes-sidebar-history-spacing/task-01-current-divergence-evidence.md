---
id: "01-current-divergence-evidence"
title: "Require current divergence evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.7
system_design:
  - ../../specs/tasks/system-design/remote-contribution-relation.md
---

# Task 01: Require Current Divergence Evidence

## Summary and scope

Correct the unconditional divergence fallback without changing Git state.
Own classifier/hook regressions and desktop/phone history presentation proof.
Backend transport, new provider support, and spacing belong outside this task.

## Acceptance

1. Write failing tests named `returns unknown for a stale upstream snapshot`
   and `returns unknown for unequal heads with zero upstream counts`. Correct
   the existing unrelated-upstream test; do not equate commit metadata or patches.
2. Unknown produces unified presentation and unavailable-evidence policy,
   preventing replacement/restoration and generic remote mutation. Aligned,
   proven provider-ahead/local-ahead, and matching two-sided divergence retain
   their behavior. Preserve retained published provenance through refresh.
3. Desktop and phone render unified history for stale snapshots and retain two
   comparison groups for confirmed divergence. Existing branch/repository
   selection remains authoritative. A mixed-repository hook case must prove a
   stale sibling cannot provide evidence for the selected repository.

## ASCII UI preview

UI-01/03, [full preview](plan.md#ascii-ui-preview), existing criteria 001.4/.7:

```text
Desktop                 Phone Changes scroll body
o PR CHANGES (11) >     | o PR CHANGES (11) > |
o COMMITS (2) >         | o COMMITS (2) >    |
```

Confirmed divergence retains UI-02. Spacing is illustrative until task 02.

## Verification

Bootstrap once if workspace dependencies are absent. Ensure Node 24 and pnpm
are available in the shell; this session located Node under mise but its default
PATH did not expose node or pnpm.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx components/task/changes-panel-timeline-history.test.tsx hooks/domains/github/use-pr-commits.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-history-regression.spec.ts -- --grep 'stale upstream|confirmed divergence')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-changes-history-regression.spec.ts -- --grep 'stale upstream|confirmed divergence')
```

Managed E2E builds current code and uses isolated instances. Run the RED
regressions before the production edit and record final results below.

## Files likely touched

- `apps/web/hooks/domains/session/remote-contribution-relation.ts`
- `apps/web/hooks/domains/session/remote-contribution-relation.test.ts`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.test.tsx`
- `apps/web/e2e/tests/git/changes-history-regression.spec.ts` (new)
- `apps/web/e2e/tests/git/mobile-changes-history-regression.spec.ts` (new)
- `docs/public/git-operations.md` only if its comparison explanation requires
  clarification; apply docs-maintainer before finishing implementation.

## Dependencies and inputs

None. Read the referenced requirements/design and use the existing
`git-changes-panel.spec.ts` / `mobile-pr-checkout-drift.spec.ts` provider/status
fixtures. Do not interact with a developer's live instance.

## Risks and parallelism

Unknown can defer comparison until ordinary refresh confirms the graph.
Re-run true divergence controls; never use a stale provider list for actions.
Execution is `sequential`.

## Results

Implemented the matching-head ancestry gate and regression coverage. RED:
six unit assertions failed on incorrect separate/destructive classification;
desktop stale-history regression failed because unified history was absent.
GREEN: the exact four-suite Vitest command passed 53 tests. Managed desktop
and phone stale/confirmed-divergence subsets each passed two tests. The final
browser runs reused the freshly rebuilt classifier bundle via `--host --no-build`.
No remote mutation was performed. Public Git guidance now distinguishes unknown
snapshots from confirmed divergence. Final package static checks follow task 02.
