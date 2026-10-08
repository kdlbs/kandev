---
id: "01-current-dialog-validation"
title: "Keep validation and confirmation in the current dialog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-001
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-001.1
  - AC-WORKSPACES-LOCAL-REPOSITORIES-001.3
  - AC-WORKSPACES-LOCAL-REPOSITORIES-001.9
  - AC-WORKSPACES-LOCAL-REPOSITORIES-001.10
  - AC-WORKSPACES-LOCAL-REPOSITORIES-001.11
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 01: Keep Validation and Confirmation in the Current Dialog

## Summary

Implement local dialog validation authority and guard actual draft confirmation.
Prove the fix through the actual exported page hook and form with only fetch
transport mocked. Start only after a later explicit ROOT release of this same
primary with exclusive GLOBAL LOCAL-HEAVY; DESIGN does not authorize execution.

## In scope

- `useDiscoverDialog` lifetime/visit/workspace/trimmed-input/newest-attempt
  authority, synchronous retirement, current render projection, owned busy state
  and stale async/captured callback rejection.
- Causally necessary close/confirm/build glue in the same client file; existing
  action/API contracts, canonical paths, discovered selection and draft defaults.
- The complete [test matrix](plan.md#tests), fixture cleanup, and package status/results.

## Out of scope

- Generic useRequest, backend/action transport/API/store/discovery coordinator,
  schema/config/cache/global owner changes, broad consumer audits.
- Layout/copy/routes/touch/scroll/navigation, disabling manual input, browser,
  build/E2E, runtime/harness edits, delegates or new tasks/tabs/sessions.
- ROOT proof replay/modification; unapproved resource recovery or scope expansion.

## Acceptance

1. Current render and retained callbacks cannot publish or confirm retired
   validation; latest same-input attempt owns every settlement and busy branch,
   including synchronous reentry, unmount and A-to-B-to-A transitions.
2. Real input/dialog/action tests prove all matrix rows, including canonical path,
   current success/invalid/rejection, discovered selection, blank/no-workspace,
   editable pending input, current pending controls and async caller settlement.
   Use Repository admits only a valid local draft and sends no creation POST.
3. Exact scoped checks pass with meaningful RED/GREEN and actual terminal joins
   recorded; owned files stay within source/spec limits and all delivery links
   resolve. No unrelated framework or visual behavior changes.

## Implementation sequence

Read the current source and ownership design before changing tests. Mark this
order in_progress only after the later explicit ROOT release. Audit source blobs
against the recorded baseline without replaying ROOT's temporary proof. Write
permanent causal form regressions and join RED before changing production.
Resolve only own-scope fixture issues if RED has setup errors; expected assertion
failures must establish the race. Implement the smallest local owner, preserving
the transport action and editable controls. Join GREEN, then run the affected
checks below once. Routine causal fixture/lint corrections may rerun affected
checks; unknown/out-of-scope/transport/resource/timeouts checkpoint ROOT with
saved evidence and no automatic recovery. Keep actual pending/terminal handles.

## Verification

Run from repo root, commands independently rooted. Prefix command-local PATH
with existing `/home/jcfs/.nvm/versions/node/v24.18.0/bin` on this host; do not
edit runtime or harness files. Confirm pnpm is 9.15.9 from `apps/package.json`.
If `apps/node_modules` is absent, perform exactly one frozen install under the
ROOT heavy lease. If present, reuse it; no speculative reinstall/cache changes.

```bash
(cd apps && pnpm --version)
# Only if apps/node_modules is absent, after the later ROOT heavy lease:
(cd apps && pnpm install --frozen-lockfile)
```

Join each command before the next. For RED and GREEN run this exact suite
selection (new tests use one flat describe prefix; no empty-selection success).
The new file belongs to the existing browser-locales project; do not alter
project-selection config.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 VITEST_MAX_WORKERS=1 pnpm exec vitest run --project browser-locales app/settings/workspace/workspace-repositories-client.test.tsx -t '^manual repository validation ' --maxWorkers=1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec eslint --max-warnings 0 app/settings/workspace/workspace-repositories-client.tsx app/settings/workspace/workspace-repositories-client.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the real PR documentation coverage evaluator against actual changed paths
and the four package files. In DESIGN use the client as a prospective trigger
without modifying it. This command is a local evaluator, not a GitHub mutation:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const files = [
  'docs/specs/workspaces/requirements/local-repositories.md',
  'docs/specs/workspaces/system-design/local-repositories.md',
  'docs/plans/manual-repository-validation-ownership/plan.md',
  'docs/plans/manual-repository-validation-ownership/task-01-current-dialog-validation.md',
];
const changedFiles = [...files, 'apps/web/app/settings/workspace/workspace-repositories-client.tsx'];
const fileContents = Object.fromEntries(files.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({status: result.status, errors: result.errors}));
if (!result.ok) process.exitCode = 1;
NODE
```

If immediate existing glue outside the client proves causally necessary, checkpoint
ROOT before expanding ownership; amend the bounded order/check paths only after
its release. Use normal active commit hooks after checks and later publication
authorization; no hook bypass. Heavy hooks also require the global lease.

## Files likely touched

- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/app/settings/workspace/workspace-repositories-client.test.tsx` (new)
- `docs/specs/workspaces/requirements/local-repositories.md`
- `docs/specs/workspaces/system-design/local-repositories.md`
- This work order and its sibling manifest.

Read-only form inputs: `workspace-add-local-repository-dialog.tsx`,
`workspace-repositories-dialog.tsx`, `workspace-repositories-validation.ts`,
`components/repository-discovery-controls.tsx`, real StateProvider/store/router
and workspace actions. No additional production changes are expected.

## Dependencies

None. One later explicit ROOT implementation release and exclusive global heavy
lease are execution prerequisites; serial ROOT MERGE lease is separate.

## Risks

See the [manifest risks](plan.md#risks). Do not let canonical spelling differences,
commit timing, captured handlers, StrictMode lifetime cleanup, or an older finally
clear current authority. Transport fixtures must fully settle even on assertion
failure. Preserve managed dependencies/worktree and all ROOT proof resources.

## Parallelism

`sequential`; no delegates, tasks, tabs or sessions.

## Inputs

- [Requirement](../../specs/workspaces/requirements/local-repositories.md), 001.1,
  001.3, 001.9 through 001.11.
- [Design](../../specs/workspaces/system-design/local-repositories.md#settings-manual-validation-ownership).
- [Entry-point and closure inventory](plan.md#current-entry-points-and-closure-inventory),
  accepted evidence, full matrix and mobile/public-doc assessment.
- Existing real StateProvider/form pattern in
  `apps/web/components/settings/repository-card.test.tsx`; deferred-public-state
  ownership examples in `hooks/domains/workspace/use-repository-branches.test.tsx`.
  Reuse fixture structure only; their API mocks do not satisfy this order's
  fetch-only requirement.

## Results

Implemented after ROOT's explicit later release of the same primary. One frozen
pnpm 9.15.9 install completed; no lockfile changes. The only production change is
the owning client, with private same-file selection authority and guarded draft
confirmation. Actual dialog, actions, store/provider, router and discovery are
used by 36 transport-only regressions.

RED on the original source failed 28 of 34 cases (19 assertions and nine fixture
waits). The fixture incorrectly awaited stale transport before asserting that no
transport should be admitted. Its causal correction moved that assertion first;
the affected RED then failed ten assertions without timeouts or setup errors:
workspace replacement/roundtrip/null, unmounted success/rejection and retained
handlers after edit/selection/close/workspace/attempt. Other original RED
assertions demonstrated stale editable-input feedback, reopened blank validity,
older-result loading reversal and wrong canonical drafts. Both logs are retained.
Initial GREEN passed 33/34; the remaining fixture expected a freshly replaced
workspace's dialog to be open. Reopening the actual fresh dialog corrected that
assumption; affected cases passed. Added explicit empty/no-workspace and reopened
visit/root-control assertions bring final coverage to 36.

Final exact anchored browser-locales run: 36/36 passed, exit 0, 11.10s. Changed
ESLint, typecheck, i18n:check and i18n:ratchet each exited 0. The i18n catalog
reported 430 existing orphan keys, with no missing/invalid translations or copy
violations. Public documentation tests passed 62/62 and the validator accepted
47 pages. No browser, E2E, build, backend or broad frontend test was run; the
state/data-only mobile exception above applies.

All command handles were actually joined. PID/start/terminal receipts and full
logs are under `/tmp/kandev-child41-0ns7zf95`: install, red, red-fixture, green,
green-fixture, green-final, format, eslint, typecheck, i18n-check, i18n-ratchet,
public-doc-tests and public-doc-validation. RED's first fixture waits and the
single GREEN fixture correction are retained rather than hidden by passing replay.
ROOT's independent proof was not replayed or modified.

Catalog validated 351 decisions and 1,369 specs; all spec lint passed. The real
PR documentation evaluator reported covered with no errors for all six changed
paths, and tracked/untracked whitespace passed. Normal active hooks and
publication receipts are recorded in the durable task plan. Hosted review/check
and separate ROOT merge gates remain pending; done here means the bounded
implementation and local test matrix.
