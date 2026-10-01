---
id: "01-repository-membership"
title: "Complete repository membership for board filtering and search"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-BOARD-REPOSITORY-MATCHING-001
acceptance_criteria:
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.1
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.2
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.3
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.4
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.5
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.6
  - AC-UI-BOARD-REPOSITORY-MATCHING-001.7
system_design:
  - ../../specs/ui/system-design/board-repository-matching.md
---

# Task 01: Complete repository membership

## Summary

Recreate permanent tests for secondary-repository filtering and repository
name/path search, observe the expected red failures, then implement shared
collection-authoritative membership. Prove real hooks and projections retain
multi-repository cards and occupied columns.

## In scope

- Minimal membership shape/helper and selection predicate in `filters.ts`.
- Existing repository-name/path search arm in `use-kanban-data.ts`.
- Regression tests through real helper, hook, workflow and swimlane projections.
- Add a concise multi-repository board-filter sentence to the existing public
  tasks/workflows guide; preserve sidebar distinctions.

## Out of scope

API/schema, task membership writes, sidebar filtering, unrelated search fields,
layout/interaction changes, other fix scopes, broad suites, and live instances.

## Acceptance

1. Real helper and real hook tests fail before the fix specifically because
   secondary membership is ignored; all AC .1-.4 compatibility scenarios pass
   afterward without treating [] as absent or matching attachment IDs.
2. Per-workflow and shared-hook tests establish AC .5-.7, including preserved
   occupancy when search excludes an otherwise repository-matching card.
3. All targeted commands pass, documentation records results, and normal
   exact-head CI/review gates pass before authorized merge.

## Verification

Run from repository root; install was completed during design, repeat only if
workspace dependencies are lost. Respect configured worker budgets.

```bash
(cd apps/web && pnpm exec vitest run lib/kanban/filters.test.ts hooks/domains/kanban/use-kanban-data.test.tsx lib/kanban/task-projections.test.ts hooks/domains/kanban/use-swimlane-render-data.test.tsx components/kanban/swimlane-container.test.ts lib/kanban/workflow-swimlanes.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/kanban/filters.ts hooks/domains/kanban/use-kanban-data.ts lib/kanban/filters.test.ts hooks/domains/kanban/use-kanban-data.test.tsx lib/kanban/task-projections.test.ts hooks/domains/kanban/use-swimlane-render-data.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
(cd apps && pnpm exec node -e 'process.chdir(".."); require("node:child_process").execFileSync(process.execPath, ["--test", "scripts/validate-public-docs.test.mjs"], {stdio: "inherit"})')
(cd apps && pnpm exec node -e 'process.chdir(".."); require("node:child_process").execFileSync(process.execPath, ["scripts/validate-public-docs.mjs"], {stdio: "inherit"})')
git diff --check
git status --short -- docs/plans/multi-repo-board-matching
```

Run this coverage preflight from repository root after implementation. Also
verify the live exact-head PR-documentation status after publication.

```bash
(cd apps && pnpm exec node <<'JS'
process.chdir('..');
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require(process.cwd() + '/.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/ui/requirements/board-repository-matching.md',
  'docs/specs/ui/system-design/board-repository-matching.md',
  'docs/plans/multi-repo-board-matching/plan.md',
  'docs/plans/multi-repo-board-matching/task-01-repository-membership.md',
];
const paths = cp.execFileSync('git', ['diff', '--name-only', 'origin/main', 'HEAD'], { encoding: 'utf8' }).trim().split('\n').filter(Boolean);
const result = validateCoverage({
  changedFiles: paths.map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
JS
)
```

## Files likely touched

- `apps/web/lib/kanban/filters.ts`
- `apps/web/lib/kanban/filters.test.ts`
- `apps/web/hooks/domains/kanban/use-kanban-data.ts`
- `apps/web/hooks/domains/kanban/use-kanban-data.test.tsx`
- `apps/web/lib/kanban/task-projections.test.ts`
- `apps/web/hooks/domains/kanban/use-swimlane-render-data.test.tsx`
- `docs/public/tasks-and-workflows.md`
- This plan/work order and paired requirement/design for accurate results.

## Dependencies

Required separate-turn design handoff; no dependency on PR #1512. Refresh main
and its latest head before implementing and final rebase.

## Risks

A migration can change hook data ownership; adapt to authoritative current
source rather than restoring old store reads. Collection authority must be
shared by filtering and search, including explicitly empty arrays.

## Parallelism

`sequential`; no workers authorized for this subtask.

## Inputs

- [Requirements](../../specs/ui/requirements/board-repository-matching.md)
- [System design](../../specs/ui/system-design/board-repository-matching.md)
- `KanbanState.tasks` repository compatibility comments.
- Existing task-projection and real StateProvider swimlane hook test patterns.

## Results

The parent reviewed the package and explicitly requested implementation in a
later turn. Implementation and targeted local validation are complete.

- Permanent red evidence: helper and real useKanbanData regressions failed on
  secondary membership/search and stale scalar authority before production edits.
  Fixture corrections preceded the accepted hook red run (10 expected failures,
  5 compatibility controls passed).
- Focused Vitest command in Verification: passed, 6 files and 67 tests.
- Focused ESLint command: passed with zero warnings after naming the repeated
  first-task fixture ID.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:ratchet`: passed; zero added and two modified source files clean.
- `python3 scripts/list-docs.py validate`: passed (339 decisions, 1283 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Public-doc validator tests: passed (62 tests); validator passed (47 pages).
  Commands were corrected to launch Node from apps but run the scripts at the
  repository root, where their relative fixture paths resolve.
- `git diff --check`: passed; all four delivery artifacts present.
- Mobile parity: shared state/data projection exception; no layout or
  interaction changes. Hook/projection/swimlane regressions cover both surfaces.
- Public docs updated: `docs/public/tasks-and-workflows.md` (how-to guide), one
  sentence documenting secondary repository selection on Kanban/Pipeline boards.
- Exact-head documentation preflight and PR CI/review/merge remain delivery
  gates, recorded externally after commit/publication. No schema or API change.
