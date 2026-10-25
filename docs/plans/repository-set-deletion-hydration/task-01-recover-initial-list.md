---
id: "01-recover-initial-list"
title: "Fence deletion and recover the initial list"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-001.6
  - AC-WORKSPACES-REPOSITORY-SETS-001.7
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 01: Fence Deletion and Recover the Initial List

## Summary

Prevent initial HTTP hydration from undoing a live deletion and obtain the
complete current list for consumers that remain open.

## In scope

- Store revision bump without an existing bucket; actual API/WS/hook regression.
- Shared per-store/workspace request and unloaded-list recovery, lifecycle and
  failure controls; preserve loaded cache and disabled/null-workspace behavior.
- Owning requirement/design clarification, scoped checks, ordinary-hook commit.

## Out of scope

Backend, timestamps, persistence, tombstones, Query migration, UI composition,
plugin preferences, live data mutation, push, PR, merge, and other agents.

## Acceptance

- Deletion by registered event or direct action fences the pending old list
  with either a missing or existing bucket and never resurrects the removed set.
- An unloaded active list recovers all surviving rows from a current request;
  failed recovery settles unloaded and can retry without a request storm.
- Shared consumers and lifecycle/key/provider changes preserve request and
  cache ownership; cached complete data remains usable.
- A hung read or repeated recovery cannot retain shared ownership beyond a
  30-second operation deadline; abort settles loading and permits a fresh retry
  while preserving accepted cache contents.

## Verification

Run from repository root with Node 24 on PATH, retaining the resource guard:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/domains/workspace/use-repository-sets-hydration.test.tsx hooks/domains/workspace/use-repository-sets.test.ts lib/state/slices/workspace/repository-sets-revision.test.ts lib/state/slices/workspace/repository-sets-slice.test.ts lib/ws/handlers/repository-sets.test.ts lib/api/domains/repository-sets-api.test.ts components/task-create-dialog-repository-sets-control.test.tsx app/settings/workspace/workspace-repository-sets-section.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/workspace/use-repository-sets.ts hooks/domains/workspace/use-repository-sets.test.ts hooks/domains/workspace/use-repository-sets-hydration.test.tsx lib/state/slices/workspace/workspace-slice.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --check -- docs/plans/repository-set-deletion-hydration
git status --short -- docs/plans/repository-set-deletion-hydration
```

Also run the existing `.github/scripts/pr-docs.cjs` `validateCoverage` preflight
on actual changed paths plus linked artifact contents, and record active normal
pre-commit/commit-msg results. No full suites or browser builds are required.

## Files likely touched

- `apps/web/lib/state/slices/workspace/workspace-slice.ts`
- `apps/web/hooks/domains/workspace/use-repository-sets.ts`
- `apps/web/hooks/domains/workspace/use-repository-sets.test.ts`
- `apps/web/hooks/domains/workspace/use-repository-sets-hydration.test.tsx`
- `apps/web/components/task-create-dialog.test.tsx` (consistent consumer store fixture)
- `apps/web/components/task-create-dialog-setup.test.ts` (same store fixture contract)
- `docs/specs/workspaces/requirements/repository-sets.md`
- `docs/specs/workspaces/system-design/repository-sets.md`
- This work order and its linked plan.

## Dependencies

None. Fresh worktree dependencies installed before package commands.

## Risks

Incomplete view after rejecting an old snapshot; cross-consumer loading races;
unbounded failure retries; provider/workspace ownership leaks.

## Parallelism

`sequential`

## Inputs

Owning pair's AC 001.6/.7 and client list ownership section; current hook,
store/handler/API tests and `use-repository-branches.ts` store ownership pattern.

## Results

Current-head CI frontend shard 2 reported 13 failures in the existing dialog
lifecycle suite: its selector mock supplied repository-set state but its
`getState()` mock omitted it. Reproduce the exact suite locally, then give both
read paths the same complete empty loaded-list fixture. This is a test harness
correction for the hook's store ownership, not a change to task form behavior.
The owning repository-set behavior and tests remain unchanged. Targeted command:
`(cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task-create-dialog.test.tsx)`.
Shard 1 subsequently reported the same fixture defect in all nine setup-hook
tests. Reproduce and correct that fixture's selector and direct store reads as
well. Its targeted command adds
`components/task-create-dialog-setup.test.ts` to the command above. Original
fixtures reproduced 13 dialog and nine setup failures. Both now derive selector
and direct reads from the same complete fixture; existing assertions are
unchanged. The expanded ten-suite command passed 105 tests, including all 16
hydration integration cases. Post-crash verification repeated that command with
`NODE_ENV=production` (Vitest selects its test environment), zero-warning ESLint
on both fixtures, and the 6 GiB Node 24 typecheck including rich-output
consistency: all passed (owned handle 37175). No runtime source changed in this
fixture remediation.

PR review follow-up: CodeRabbit identified hung-transport ownership retention;
Claude suggested bounding event-driven recovery. Before remediation tests or
production edits, the owning design now specifies one 30-second deadline for
the whole shared operation, preserving the existing failure/retry contract.
Deadline RED: two fake-clock transport-abort cases failed at 30 seconds because
loading remained true (20466, exit 1); 14 prior cases were excluded by the name
filter. GREEN: the complete eight-suite command above passed 82 tests, including
all 16 integration cases (94567, exit 0). The cases prove that a hung loaded
refresh preserves accepted data and can retry, and that unloaded recovery shares
the original deadline rather than resetting it. Both verify cleared timers after
successful retry. The actual fetch transport honors the API-carried abort signal;
no list API, hook, store, or registered handler is mocked.

The same sequential command then passed zero-warning scoped ESLint and the
6 GiB Node 24 typecheck, including rich-output consistency. Catalog, full spec
lint, coverage, and ordinary hook receipts accompany this remediation. The
following earlier receipts remain historical evidence of the initial repair.

- Dependency install: `pnpm install --frozen-lockfile` from `apps`, exit 0
  (owned handle 46519). Node 24.18.0 was added after the guard shims on PATH.
- Original-source RED: focused hydration suite, exit 1 (1865). Twelve tests:
  nine behavioral failures, three passing controls. Both direct action and WS
  entry points resurrected `set-deleted` alongside `set-survivor`; an existing
  empty bucket fenced deletion but never recovered the survivor. No import or
  fixture failure; all held transports settled.
- Initial GREEN: two hook suites, 19 tests passed (68059).
- Full targeted command above: eight suites, 80 tests passed (56948, then
  final behavioral run 37073). The actual menu lost the deleted option and
  selected the recovered survivor. Fourteen new integration cases cover the
  lifecycle and isolation matrix. Test grouping removed an initial ESLint
  function-length warning without suppressing rules.
- Final test-only typing correction: the registered WS handler index requires
  a non-null assertion under its optional map signature. Focused command
  `pnpm exec vitest run --maxWorkers=1 hooks/domains/workspace/use-repository-sets-hydration.test.tsx`
  from `apps/web`: 14 passed, exit 0 (78518).
- Scoped ESLint with `--max-warnings 0`: exit 0, zero warnings (66775 and
  final behavioral run 37073); the ordinary commit hook covers the final
  test-only assertion as well.
- Final typecheck command above: exit 0 (78518), including rich-output catalog
  consistency. First attempt exhausted the default 4 GiB V8 heap (8266,
  exit 134); the 6 GiB retry surfaced the test-only typing issue (37073,
  exit 2), then passed after correction. The 8 GiB resource guard stayed active.
- `python3 scripts/list-docs.py validate`: passed, 396 decisions and 1693
  specifications. `python3 scripts/lint-spec-files.py --all`: passed (36483,
  delivery refresh 83200). Plan and full diff whitespace checks passed.
- Existing `validateCoverage` actual-path preflight: `covered`, no errors;
  accepted the owning requirement/design/work order for both runtime trigger
  paths. The temporary receipt script reads the complete diff against base
  `c4de9a179569527009454cddf2a4f433b7aea06e`.
- Delivery freshness: remote main still equals that base; 70 open PRs inspected.
  Relevant PR 1512, 3656, and 4660 heads match the inspected source patches.
- Mobile-parity state-only exception: the shared hook supplies New Task,
  New Subtask, and workspace repository settings. Actual menu integration and
  existing control/settings suites cover the data outcome. No viewport,
  layout, scrolling, navigation, or touch changes; no browser or live-backend
  execution claimed. Public docs audit found no changed public workflow,
  API, configuration, screenshot, or terminology needing an update.
- Ordinary hook commit receipt and exact final SHA are recorded in the external
  task plan and parent handoff after commit. No push, PR, or native delegation.
