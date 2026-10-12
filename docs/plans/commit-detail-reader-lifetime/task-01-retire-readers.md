---
id: "01-retire-readers"
title: "Retire commit-detail readers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-ONLY-COMMIT-DETAILS-001
acceptance_criteria:
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.1
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.2
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.3
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.6
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.8
  - AC-UI-PR-ONLY-COMMIT-DETAILS-001.9
system_design:
  - ../../specs/ui/system-design/commit-detail-target-types.md
---

# Task 01: Retire commit-detail readers

## Summary

After the later explicit implementation release, add a meaningful real-provider
RED regression, then retire closed/replaced reader requests in the existing
live hook. Preserve current success, localized errors, retry, source routing,
local readiness retry, latest-request ordering, and returned shape.

## In scope

- Layout-bound per-instance owner admission/retirement, captured-owner retry
  denial, and owner/sequence guards in success, catch/toast, and finally.
- Real-provider DOM lifecycle suite with only request/report transport mocks.
- Exact checks and synchronized requirement/design/plan/order results.

## Out of scope

Exclude unused `useCommitDiff`, transport cancellation, reporter/store changes,
ToastProvider timer repair, context/identity redesign, backend/parser/preview,
UI composition/copy/touch/navigation/breakpoint changes, browser/build/E2E,
broad suites, optional polish, and all delegation.

## Acceptance

1. The permanent closed-reader deferred-rejection test fails before the repair
   for the parent's proven DOM-toast defect, then passes. Retired success/error/
   finally cannot publish; retained retired callbacks dispatch no request,
   including at layout commit before passive cleanup and after A -> B -> A.
2. The real-provider suite proves independent readers, close/reopen, committed
   target replacement, StrictMode cleanup/setup, latest overlapping success/
   failure and loading, current ordinary/protocol error toast/report, successful
   retry, local payload/context/readiness retry, and GitHub isolation from local
   readiness/fallback. Fixtures retain the real protocol error class.
3. Exact checks pass with terminal receipts; owned timers/deferred work are
   cleaned and docs accurately record results. The repair stays within the
   existing hook and follows the reviewed design with no public contract change.

## Mobile and rendered evidence

Use the state-only assessment in [the plan](plan.md#mobile-and-rendering-assessment).
Live `CommitRowFiles`, `CommitDetailPanel`, and phone `CommitDiffView` share this
hook. Real `StateProvider`/`createAppStore` and `ToastProvider` remain mounted
while a real Reader is closed/replaced. Assert returned data/loading/error,
toast DOM, frontend report sink, and request admissions. No geometry, copy,
composition, or touch behavior changes warrant a viewport or browser test.

## Verification

Run each command separately from repo root, with ONE heavy process at a time.
Use the existing Node 24 directory, not a new runtime install. If
`apps/node_modules` is absent, run the single setup command once and join it:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

First create and run the permanent regression to record expected RED. After
the minimal fix and full affected controls, run the same targeted suite for
GREEN, then each remaining check sequentially:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run hooks/domains/session/use-commit-detail.lifecycle.test.tsx --maxWorkers=1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/session/use-commit-detail.ts hooks/domains/session/use-commit-detail.lifecycle.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/commit-detail-reader-lifetime
```

If an additional test file must change for a concrete dependency, record the
reason and add its exact path to the targeted Vitest/eslint selection before
marking done. Do not replay unchanged suites or parent proof. A returned
`session_id` means RUNNING: join via `write_stdin` to terminal before another
install/check/hook. Reconcile a lost handle before starting any duplicate.

Run actual documentation coverage preflight from repo root with existing Node:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const added = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...tracked, ...added])].map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' }));
const paths = ['docs/plans/commit-detail-reader-lifetime/plan.md', 'docs/plans/commit-detail-reader-lifetime/task-01-retire-readers.md', 'docs/specs/ui/requirements/pr-only-commit-details.md', 'docs/specs/ui/system-design/commit-detail-target-types.md'];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/web/hooks/domains/session/use-commit-detail.ts`
- `apps/web/hooks/domains/session/use-commit-detail.lifecycle.test.tsx` (new)
- `docs/specs/ui/requirements/pr-only-commit-details.md`
- `docs/specs/ui/system-design/commit-detail-target-types.md`
- `docs/plans/commit-detail-reader-lifetime/plan.md`
- This work order.

The production consumers and transport are inspection inputs, not planned edits.
Existing source tests are inspection controls; current behavior must be proved
within the new real-provider suite instead of rerunning unchanged broad suites.

## Dependencies

None. Later explicit reviewed-root implementation interrupt is mandatory.
Standing delivery authority applies after that release; no new approval or
model/profile question is required.

## Risks

StrictMode flag revival, passive-cleanup races, stale callback admission before
sequence increment, and accidental changes to local readiness/GitHub routing.
Use captured owner plus request sequence; keep state projection semantics.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/ui/requirements/pr-only-commit-details.md), .1-.3/.6/.8/.9.
- [Existing design](../../specs/ui/system-design/commit-detail-target-types.md#committed-reader-lifetime).
- Existing hook, real providers, `commit-detail-request.ts`, actual consumers,
  and `use-file-upload.ts` layout-bound lifetime pattern.
- Parent-owned `/tmp/kandev-commit-detail-lifetime-repro.test.tsx`, read-only
  accepted proof; do not mutate or replay it.

## Docs impact

Internal docs updated. Public commit navigation/source/error instructions in
`docs/public/sessions-and-review.md` and `git-operations.md` stay accurate: no
new user action, terminology, API, setting, composition, or recovery procedure.
No public docs or screenshot update is needed for suppressing retired feedback.

## Results

Implemented on 2026-10-03 after the reviewed-root release. Only the existing
hook, its new real-provider lifecycle suite, and four design-package documents
changed. The paired requirement is active and its design is current.

- ONE frozen `corepack pnpm@9.15.9 install --frozen-lockfile` from `apps`:
  joined handle 8821, exit 0 (923 reused packages, no lockfile change).
- Permanent RED: targeted Vitest joined handle 48039, exit 1; expected
  closed-reader real DOM toast defect failed, current error/toast/report
  control passed (1 failed / 1 passed, 56 ms tests, 2.78 s package).
- Minimal GREEN: joined 90897, exit 0, both initial controls passed.
- Final targeted Vitest: joined 17373, exit 0; 18 passed, 111 ms tests,
  2.82 s package. A sibling-preservation fixture was corrected before final
  validation; no product change resulted from that intermediate fixture failure.
- Changed-file eslint `--max-warnings 0`: joined 19599, exit 0.
  A shared local current-request predicate keeps complexity within the existing
  limit; no lint policy was weakened.
- `pnpm run typecheck`: joined 32718, exit 0. Initial fixture ID brand errors
  were corrected with the existing test-fixture ID constructors.
- `pnpm run i18n:check`: joined 41826, exit 0; existing 419 orphan warnings
  are non-failing. No copy or locale changes.
- `pnpm run i18n:ratchet`: joined 47825, exit 0.
- `list-docs.py validate`, specification lint, actual documentation coverage
  preflight, and `git diff --check`: exit 0; exact final documentation receipts
  are retained in the platform plan.

All process handles joined; owned timers and deferred test work cleaned.
No runtime/browser/E2E/build/broad suites or unchanged-test replay. No public
instructions, screenshots, global policy, transport, or unused-hook changes.
Normal hook/commit/push, exact-head hosted CI/full semantic review, and actual
merge/cleanup evidence remain delivery gates; local GREEN is not completion.
