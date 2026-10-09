---
id: "01-preserve-delete-catalogue"
title: "Preserve accepted Settings deletion catalogue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-DELETION-002
acceptance_criteria:
  - AC-WORKSPACES-DELETION-002.1
  - AC-WORKSPACES-DELETION-002.2
  - AC-WORKSPACES-DELETION-002.3
  - AC-WORKSPACES-DELETION-002.4
  - AC-WORKSPACES-DELETION-002.5
system_design:
  - ../../specs/workspaces/system-design/deletion.md
---

# Task 01: Preserve accepted Settings deletion catalogue

## Summary

Independently reproduce stale catalogue publication through the real Settings
workspace deletion flow. Publish target-only removal from the current owning
store after accepted DELETE, retaining existing confirmation, transport,
selection-policy, error and Settings-navigation behavior.

## In scope

- `useWorkspaceDeleteDraft` and immediate form glue in
  `apps/web/app/settings/workspace/workspace-edit-client.tsx`.
- Independently authored real-page/store/API/WS/picker regression and controls
  named in the [plan's test matrix](plan.md#tests).
- Exact affected checks, internal four-file delivery records and normal hooks.

## Out of scope

- Backend or Office deletion, cascades, selection/route redesign, lifecycle,
  global list arbitration, stale saves, unrelated writers and new abstractions.
- Markup, copy, touch, layout, scroll or breakpoint changes; product builds and
  broad E2E/frontend/backend replays. Protected ROOT proof source access/reuse.
- Delegation, recursive tasks, additional sessions or model switches.

## Dependencies and execution gate

No implementation dependency. Same primary executes sequentially only after a
later explicit ROOT reviewed-package implementation INTERRUPT and GLOBALheavy
grant. Design validation does not release either gate. Then mark this order
`in_progress`; read the released source and preserved requirement 001 before
any production change. Install frozen dependencies once before package checks.
Retain all original long-running handles across interrupts and join them.

## Acceptance

1. Independently authored mixed creation/removal and survivor-update tests reach
   the real pre/post-acknowledgement store/picker assertions and fail causally on
   pre-fix production; ordinary accepted and rejected controls pass in that RED
   run. No product component/provider/router/action/handler/primitive is mocked.
2. After the real request succeeds, current-owning-store target filtering
   preserves every survivor and relative order, including new rows, current
   metadata and already-applied selection/revision. Only the deletion hook and
   immediate caller change; existing request/error/guard/navigation remain.
3. All matrix cases and affected verification pass with strict transport and
   settled cleanup. Record actual results and mobile/public-doc classification;
   synchronize this order and plan after local completion without claiming
   hosted success or merge prematurely.

## Implementation sequence

1. Author `workspace-delete-catalogue.integration.test.tsx` independently, with
   optional new `workspace-delete-catalogue.test-helpers.tsx` for fixture size.
   Follow the plan's exact test names and causal matrix. Use real Page,
   StateProvider/createAppStore, SettingsSaveProvider, Toast/Tooltip providers,
   normal router, useRequest/action/fetchJson, registered notifications and
   AppSidebarWorkspacePicker. Isolate only external HTTP/WS delivery and reject
   unexpected transports. Observe a real created choice and unrelated removed
   choice before held DELETE settles. On 204 observe the normal Settings return,
   target absence and survivor choices. Rejection uses actual HTTP 503 handling.
2. Run the new suite on unmodified production and retain expected causal RED
   with passing ordinary/rejected controls. Setup/transport errors do not count.
3. Pass the existing form store API to the deletion hook instead of captured
   array/setter. Read state only after the awaited request succeeds and filter
   current items by submitted ID immediately before the current setter. Remove
   unused catalogue/setter subscriptions. Preserve the normal route and catch.
4. Run GREEN plus the exact verification below serially. Fix only valid local
   findings; checkpoint a required scope or resource change with ROOT.
5. Update documentation/results, use normal active hooks and existing delivery
   authorization, and return the lease after actual local joins/current owned
   process absence. Hosted and separate merge gates follow the plan.

## Files likely touched

- Production: `apps/web/app/settings/workspace/workspace-edit-client.tsx` only.
- New regression: `apps/web/app/settings/workspace/workspace-delete-catalogue.integration.test.tsx`.
- Optional new fixture: `apps/web/app/settings/workspace/workspace-delete-catalogue.test-helpers.tsx`.
- Docs: `docs/specs/workspaces/requirements/deletion.md`,
  `docs/specs/workspaces/system-design/deletion.md`, this work order and `plan.md`.

Read-only integration boundaries: `app/settings/workspace/[id]/page.tsx`,
`components/state-provider.tsx`, `components/settings/settings-save-provider.tsx`,
`app/actions/workspaces.ts` (including its local `fetchJson`), `lib/http/use-request.ts`,
`lib/ws/handlers/workspaces.ts`, `lib/state/slices/workspace/workspace-slice.ts`,
`components/app-sidebar/app-sidebar-workspace-picker.tsx` and phone
`components/navigation/app-nav-sheet.tsx`. No transport edits are admitted.

## Verification

Run from repository root. All package commands require the GLOBALheavy grant
and run serially. Use the repository's already-installed mise environment
(Node 24 and pnpm 9.15.9); this shell initially exposes neither package command
on PATH. Bounds limit resource occupancy; retain/join each original handle and
checkpoint resource failures instead of launching a duplicate.

```bash
mise exec -- bash <<'CHECKS'
set -e
# One-time fresh-worktree dependency setup, after implementation/lease release.
(cd apps && timeout --signal=TERM --kill-after=10s 10m pnpm install --frozen-lockfile)

# RED before production edits; same command GREEN after the minimum correction.
(cd apps/web && timeout --signal=TERM --kill-after=10s 4m pnpm exec vitest run app/settings/workspace/workspace-delete-catalogue.integration.test.tsx --maxWorkers=1)

# Relevant existing request, event, slice and rendered picker controls.
(cd apps/web && timeout --signal=TERM --kill-after=10s 4m pnpm exec vitest run app/actions/workspaces.test.ts lib/ws/handlers/workspaces.test.ts lib/state/slices/workspace/workspace-slice.test.ts components/app-sidebar/app-sidebar-workspace-picker.test.tsx --maxWorkers=1)

# Optional fixture glob is quoted and tolerated only when the file is absent.
(cd apps/web && timeout --signal=TERM --kill-after=10s 4m pnpm exec eslint --max-warnings 0 --no-error-on-unmatched-pattern app/settings/workspace/workspace-edit-client.tsx app/settings/workspace/workspace-delete-catalogue.integration.test.tsx 'app/settings/workspace/workspace-delete-catalogue.test-helpers.tsx')
(cd apps/web && timeout --signal=TERM --kill-after=10s 2m pnpm exec prettier --check app/settings/workspace/workspace-edit-client.tsx app/settings/workspace/workspace-delete-catalogue.integration.test.tsx 'app/settings/workspace/workspace-delete-catalogue.test-helpers.tsx' --no-error-on-unmatched-pattern)
(cd apps/web && timeout --signal=TERM --kill-after=10s 6m pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 4m pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
CHECKS
```

Also run the real `.github/scripts/pr-docs.cjs` `validateCoverage` API on actual
changed paths/content before publication, including this complete work order,
plan, requirement and design. During design, actual doc-only paths must be
exempt; separately labelled prospective `workspace-edit-client.tsx` reference
preflight must be covered with no errors. Never represent a prospective code
path as an actual design-turn edit. No install or product runner is needed for
that dependency-free docs preflight.

```bash
mise exec -- node <<'COVERAGE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/workspaces/requirements/deletion.md',
  'docs/specs/workspaces/system-design/deletion.md',
  'docs/plans/workspace-delete-catalogue-preservation/plan.md',
  'docs/plans/workspace-delete-catalogue-preservation/task-01-preserve-delete-catalogue.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'modified' }));
changedFiles.push({ filename: 'apps/web/app/settings/workspace/workspace-edit-client.tsx', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
assert.equal(result.ok, true, JSON.stringify(result.errors));
assert.equal(result.status, 'covered');
assert.deepEqual(result.errors, []);
COVERAGE
```

This block is a prospective reference check during design. After implementation,
use actual `git diff --name-status` and untracked-path inventory for its
`changedFiles`, including any new tests; preserve the same reference contents.

No broad verification, whole-app build, browser/Playwright or database run is
required. Pure state/data correction preserves phone `AppNavSheet` and shared
picker composition under the mobile-parity exception. Any rendered change
requires ROOT checkpoint and a revised validation contract.

## Risks

- Reads before await or deferred publication restore the original race.
- Active ID retention is existing slice policy; do not silently add a target
  fallback, Office destination, cookie write or selection revision.
- Happy DOM/Radix portals and asynchronous reads need strict owned cleanup.
- Source proof copying defeats independent evidence; author from live code and
  the observable acceptance matrix only.

## Results

Local implementation completed after ROOT's later explicit reviewed-package
release and exclusive resource grant in the same primary. The independent
test/helper files cover every named case. The production change uses the
existing provider API only in the deletion hook/immediate caller. Existing
request, guard, route, selection policy and markup are preserved.

| Command from the Verification section | Actual outcome |
| --- | --- |
| Fresh-worktree `pnpm install --frozen-lockfile` in `apps/` | PASS, pnpm 9.15.9; existing lockfile; no duplicate install |
| New regression command, before correction | Causal RED: 2 failures at accepted current-membership/descriptor assertions, 9 controls PASS |
| New regression command, final code/fixture | PASS: 11 tests |
| Affected four existing test files | PASS: 65 tests in 4 files |
| Scoped ESLint `--max-warnings 0` | PASS after fixture-only callback/repeated-value cleanup |
| Scoped Prettier `--check` | PASS on all 3 changed TypeScript files |
| `pnpm run typecheck` | PASS after fixture-only feature/payload/handler type corrections |
| `pnpm run i18n:check` | PASS; existing orphan catalogue notices are non-failing |
| Design catalogue / spec-linter tests / full spec lint | PASS: 368 decisions, 1,501 specs, 36 tests and all spec files |
| Real PR-docs `validateCoverage` during design | Actual docs exempt; prospective code reference covered; errors empty |

The first test run's empty-executor Radix setup failure is not causal evidence;
the fixture was completed using real executor/profile records. All subsequent
commands ran serially, original sessions were retained and joined, and wrapper
metadata records process exit/empty owned groups. The first install omitted its
native session ID in tool output projection; original wrapper `p.wait`, exit 0,
raw log and empty group are retained, with no duplicate install. Full later
responses and native join chunks are preserved in the own task plan, with logs
under `/tmp/kandev-child107-runs/`.

Actual seven-file documentation coverage and whitespace checks passed. The first
normal commit attempt failed only the new-code i18n guard on synthetic fixture
names and metadata; narrow established reason comments/constants preserve the
same data and test semantics. Normal hooks remain active for the new attempt.
No public procedure, screenshot, API or copy
change is needed. Mobile parity uses the unchanged shared-state exception.
Hosted checks/review, the separate ROOT merge grant and verified cleanup are
pending and are not implied by this work order's local `done` status.
