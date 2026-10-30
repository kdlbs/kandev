---
id: "02-title-rename-browser-coverage"
title: "Prove transcript and historical review behavior"
status: done
wave: 2
depends_on:
  - "01-preserve-branch-binding"
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.14
  - AC-TASKS-TURN-CHANGES-002.15
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-005.6
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.13
  - AC-TASKS-TURN-CHANGES-006.1
  - AC-TASKS-TURN-CHANGES-006.3
  - AC-TASKS-TURN-CHANGES-006.5
  - AC-TASKS-TURN-CHANGES-006.7
system_design:
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 02: Prove transcript and historical review behavior

## Summary

Exercise the real title tool during a captured prompt-first turn on a generated worktree branch.
Prove the successful zero-file outcome and the one-file historical review flow on desktop and phone.
Keep genuine branch-switch history visibly unavailable.

## In scope

- Create prompt-first tasks with `auto_title: true` through `apiClient.rawRequest` and existing seeded workflow/worktree profiles.
  Use the existing HTTP contract; no new production endpoint or metadata shortcut is needed.
  Enable turn capture before creation and restore preferences afterward.
- Add focused mock-agent scenarios for title rename alone, rename with one file edit, and rename followed by a branch switch.
  The branch-switch scenario must keep the real HEAD commit unchanged.
  Call `e.callMCPToolCtx` with `set_task_title_kandev` from the active task prompt.
  Require an accepted title and successful per-repository rename before optional writes or the negative branch switch.
  Keep each scenario inside one admitted turn and emit a unique completion marker.
- Add focused scenario tests for action order, MCP failure, and the real filesystem mutation.
  Reuse `script.go` and existing history scenario patterns; do not emit fake tool results as comparison proof.
- Seed through HTTP, then open `/t/<task-id>` and assert the rendered transcript.
  Correlate the exact session, terminal turn, and change-set ID before testing an absent card.
  Wait for a finalized ready summary with a direct basis and zero files before asserting no card.
- For the edited case, require one file, correct counts, an enabled Open diff action, and the exact historical patch.
  Reload and change the live file after completion; the retained turn must show the original patch.
- On phone, open `mobile-diff-sheet`, verify direct historical file selection, dismissal, focus return, and no horizontal document overflow.
  Reuse the current drawer's scroll owner and safe-area behavior.
- On desktop, require unavailable history and its existing card for the same-commit branch-switch scenario.
- Add a short continuity statement beside generated-branch renaming in `docs/public/git-operations.md`.
  Preserve its reference format and existing manual-rename instructions.
- Record focused browser results in this work order and the plan.

## Out of scope

Production web components, projection changes, new user-facing strings, screenshots, and unrelated executor qualification.
Do not suppress history failures or synthesize summary rows in SQL.

## Acceptance

1. Desktop and phone prove that a terminal rename-only turn has a ready direct zero-file comparison and no transcript card.
2. Both viewports show the exact historical one-file change after rename, reload, and later workspace edits through existing diff surfaces.
3. A genuine same-commit branch switch retains unavailable history; public documentation describes the implemented title-rename behavior.

## ASCII UI preview

Reuse [UI-01 and UI-02 in the full plan](plan.md#ascii-ui-preview).

```text
UI-01, desktop and phone:
[Final assistant reply]
[No changes card for the ready zero-file result]

UI-02, desktop:
[1 changed file +1 -0                 Open diff]
[turn.txt                            Added +1]

UI-02, phone:
[1 changed file +1 -0]
[Open diff]
[turn.txt       Added +1]
        +--> [Existing full-height diff drawer]
```

The bracketed absence note is an annotation, not product copy.
Counts are illustrative; tests assert the actual fixture totals.
Keep the existing inline card, desktop dock, and full-height `MobileDiffSheet` composition.
The shared projection and historical selection own data; phone wrappers retain touch targets, internal scrolling, safe areas, and focus return.
Map: AC-002.14-.15, AC-005.6-.7 and .13, AC-006.1, .3, .5 and .7.

## Verification

Run from the repository root after task 01.
Install workspace dependencies once before the first pnpm command in a fresh worktree.
The managed E2E runner builds fresh production artifacts and tears down its disposable runtime.
Run desktop and phone commands sequentially; each accepts one project.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./cmd/mock-agent -run '^TestTurnTitleRenameScenario' -count=1 -v)
(cd apps/web && pnpm e2e:run --project chromium tests/git/turn-changes-title-rename.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-turn-changes-title-rename.spec.ts)
git ls-files --modified --others --exclude-standard -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/turn-change-title-branch-continuity
```

Expect at least three desktop and two phone cases. Record actual discovery and pass counts.
Task 01 owns behavioral RED for the backend defect.
These browser tests validate integration after the correction; they do not replace that RED evidence.
Run this PR-documentation preflight from the repository root before delivery.
It includes current unstaged files, staged files, and untracked work orders.

```bash
node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const list = (args) => cp.execFileSync('git', args, { encoding: 'utf8' }).split('\0').filter(Boolean);
const tracked = list(['diff', 'HEAD', '--name-only', '-z']);
const added = list(['ls-files', '--others', '--exclude-standard', '-z']);
const changedFiles = [...new Set([...tracked, ...added])].map(filename => ({
  filename, status: added.includes(filename) ? 'added' : 'modified',
}));
const documents = [
  'docs/plans/turn-change-title-branch-continuity/plan.md',
  'docs/plans/turn-change-title-branch-continuity/task-01-preserve-branch-binding.md',
  'docs/plans/turn-change-title-branch-continuity/task-02-title-rename-browser-coverage.md',
  'docs/specs/tasks/requirements/turn-changed-files.md',
  'docs/specs/tasks/requirements/turn-changed-files-presentation.md',
  'docs/specs/tasks/system-design/turn-change-history-integrity.md',
];
const fileContents = Object.fromEntries(documents.map(file => [file, fs.readFileSync(file, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/backend/cmd/mock-agent/turn_title_rename_scenario.go` (new)
- `apps/backend/cmd/mock-agent/turn_title_rename_scenario_test.go` (new)
- `apps/web/e2e/tests/git/turn-changes-title-rename.spec.ts` (new)
- `apps/web/e2e/tests/git/mobile-turn-changes-title-rename.spec.ts` (new)
- `apps/web/e2e/tests/git/turn-changes-title-rename-helpers.ts` (new)
- `docs/public/git-operations.md`
- This plan and work-order Results sections.

## Dependencies

Task 01. The browser fixture must exercise the completed guarded rename path.

## Risks

- A Local executor or explicit remote checkout is ineligible for generated renaming and cannot prove this repair.
- An absent card before finalization is a false positive. Assert the durable terminal summary first.
- The worker checkout and preferences need cleanup so the edit and negative scenarios remain independent.
- Test fixtures must use real MCP invocation and Git writes within the same turn, without arbitrary sleeps or production summary fabrication.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially UI-01, UI-02, the test matrix, and compatibility scope.
- [History design](../../specs/tasks/system-design/turn-change-history-integrity.md#generated-branch-continuity).
- [Presentation requirements](../../specs/tasks/requirements/turn-changed-files-presentation.md).
- Existing desktop and phone `turn-changes-history-integrity` specs and turn-change helpers.
- `apps/backend/cmd/mock-agent/script.go` for real MCP calls and `turn_history_integrity_scenario.go` for Git fixture patterns.
- `.agents/skills/e2e/SKILL.md` and `.agents/skills/mobile-parity/SKILL.md`.

## Results

Completed on 2026-10-30.

- Added three mock-agent scenarios: rename-only, rename with one file edit, and rename followed
  by a same-commit branch switch. Each calls the real `set_task_title_kandev` MCP tool and refuses
  optional writes or the switch unless the title was accepted and the per-repository rename was
  verified on the real checkout.
- Scenario tests cover action order, MCP failure, rejected/preserved/failed rename, wrong branch,
  malformed results, exact file content, and the real branch switch with unchanged HEAD.
- Desktop `chromium`: 3 passed (`rename-only turn has no changes card`,
  `renamed turn retains its one-file historical diff`, `same-commit branch switch remains
  unavailable`) in 27.5 seconds after the review correction.
- Phone `mobile-chrome`: 2 passed (`rename-only turn has no changes card on the phone`,
  `renamed turn keeps its one-file historical diff in the phone drawer`) in 24.1 seconds after the
  review correction.
- Both viewports assert the finalized ready comparison before checking an absent card, and the
  edited case retains the exact historical patch after reload and a later live-file edit.
- Added the generated-branch continuity statement to `docs/public/git-operations.md`.
- Documentation checks passed: public-docs validation (63 tests, 54 pages), 421 decisions and 1848
  specifications, spec lint (36 tests), and the full specification scan.
- No explicit skips.
