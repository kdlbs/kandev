---
id: "01-reset-workspace-page"
title: "Reset the GitLab workspace page"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-01.md
---

# Task 01: Reset the GitLab workspace page

## Summary

Deliver the hook's workspace reset and independent regressions for both existing
transport routes. Prove a smaller workspace's first-page row through the real
MR consumer and pagination while retaining same-workspace navigation and the
existing response, visibility and lifecycle controls.

## In scope

- Add workspaceId to the existing reset dependencies in `use-gitlab-search.ts`.
- Author permanent behavioral tests under the same directory, using the real
  hook, StateProvider/store, Tooltip/locales, MRList/shared rows/native links,
  and ResultsPagination. Only search API transport is partially mocked.
- Run the exact checks below under ROOT's implementation/heavy grants;
  update these four package files with actual local outcomes before publication.

## Out of scope

Consumer production edits, issue consumer expansion, milestone/project reset
policy changes, extra coordinator/generation/cache/cancellation architecture,
request-count promises on workspace transitions, backend/permissions/other
providers, copy/layout/mobile interactions, browser/build/E2E, original proof
replay/copy/mutation, delegation/tasks/sessions/model switching.

## Acceptance

1. Independent permanent RED observes MR and issue workspace page failures and
   the real MR consumer's hidden first-page row with unchanged production;
   same-workspace and initial small-workspace positive controls pass.
2. The narrow hook correction passes the [planned matrix](plan.md#tests),
   including A -> B -> A, existing filter/kind resets, pending visibility, late
   old-workspace/old-page success and failure, current empty/failure, refresh
   and disabled controls. Real result links and pagination controls are asserted.
3. Affected GREEN and static/documentation gates pass with actual original
   process joins and group-gone evidence; local status/results remain separate
   from pending hosted review, CI and ROOT-only merge authorization.

## TDD and fixture discipline

The later authoring grant permitted the independent creation of
`use-gitlab-search.workspace.test.tsx`; do not read or copy the
protected original candidate. Use workspace A with at least three pages and
B with a known first-page row and server total 1. Derive transport results
from independent paged datasets rather than mirror the reset logic. Navigate
using actual ResultsPagination controls for the consumer test. Assert the
native link's href, current page and actual navigation presence/absence.
Same-workspace equal-input rerender must preserve page 3 without extra transport.

Hold old-workspace and B old-page requests explicitly, accept B page 1, then
resolve/reject superseded calls. Assert items/total/loading/error and accepted
fetch timestamp remain intact. Cover both transport functions without adding
issue consumer production changes. Existing hook and page-state tests retain
milestone/project and committed-filter behavior. Do not infer request count
from effect ordering; no transition exactly-once assertions.

Unmount and settle held promises with React act/causal waits; no arbitrary
new sleeps, production-hook/store/row mocks, copied predicates, source assertions,
or generic fixture abstractions. Differentiate expected rejected fixtures from
environment/resource/transport failures. Routine owned causal fixture/lint
corrections need no extra permission; unknown, timeout, resource, transport or
out-of-scope findings checkpoint ROOT without automatic retry.

## Mobile and docs

Use the [pure-state assessment](plan.md#mobile-and-public-documentation-assessment).
No browser/build/Playwright execution: shared layout/copy/navigation/touch
behavior is unchanged. Real-consumer tests supply visible-outcome evidence.
Public guide audited; only internal documentation changes are planned.

## Verification

Run from repo root in `/bin/bash`, `login:false`. Every package command uses
the existing explicit Node24.21.0 PATH and pinned Corepack pnpm9.15.9. Before
each command publish start/cutoff/argv/log and native shell PID/PGID. Retain
every original outer session/chunk; actually join and prove the group gone
before the next command. ONE global local-heavy lease is required for install,
tests, ESLint, typecheck and hooks. No lease exists during design. No overlap
or automatic retry. The commands below are the implementation verification recipe.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
# After ROOT's explicit global local-heavy lease only; one conditional install.
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 install --frozen-lockfile)
fi

# RED with unchanged production and newly authorized permanent tests.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales components/gitlab/my-gitlab/use-gitlab-search.workspace.test.tsx -t 'resets MR page for a smaller workspace|resets issue page for a smaller workspace|shows the new workspace row through real list and pagination|keeps same-workspace page navigation on equal inputs|shows a first-page row on initial small workspace' --maxWorkers=1 --no-file-parallelism)

# GREEN after the sole-hook correction; all affected suites and page-state controls.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales components/gitlab/my-gitlab/use-gitlab-search.test.ts components/gitlab/my-gitlab/use-gitlab-search.workspace.test.tsx app/gitlab/use-gitlab-page-state.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/gitlab/my-gitlab/use-gitlab-search.ts components/gitlab/my-gitlab/use-gitlab-search.test.ts components/gitlab/my-gitlab/use-gitlab-search.workspace.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:ratchet)

timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 60s git diff --check
git status --short
```

Use normal PROJECT `run typecheck`, including the existing `pretypecheck`
generation of ignored release-note/changelog JSON; no direct tsc, setup repair,
script/harness/global-setting change. If another owned test file changes for a
necessary fixture correction, include it in affected GREEN and ESLint before
claiming complete changed-path coverage. Record i18n scope against actual
changed production lines; tests contain fixture data, no UI copy is planned.

Run the following repository-reference preflight (60s TERM/kill10) at design
and after implementation. Design checks actual four documentation files first,
then separately labels the planned hook trigger; it must never report a planned
path as an actual changed production file or claim executable-test coverage.

```bash
timeout --signal=TERM --kill-after=10s 60s /home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/gitlab-integration.md',
  'docs/specs/integrations/system-design/gitlab-integration-01.md',
  'docs/plans/gitlab-workspace-pagination/plan.md',
  'docs/plans/gitlab-workspace-pagination/task-01-reset-workspace-page.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [
  ...tracked.map(filename => ({ filename, status: 'modified' })),
  ...untracked.map(filename => ({ filename, status: 'added' })),
];
const hook = 'apps/web/components/gitlab/my-gitlab/use-gitlab-search.ts';
const actual = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ scope: 'actual changed checkout', changedFiles, result: actual }, null, 2));
assert.equal(actual.ok, true);
assert.deepEqual(actual.errors, []);
if (!tracked.includes(hook)) {
  assert.deepEqual([...tracked, ...untracked].sort(), [...paths].sort());
  const planned = validateCoverage({ changedFiles: [...changedFiles, { filename: hook, status: 'modified' }], fileContents });
  console.log(JSON.stringify({ scope: 'design only: planned hook trigger', result: planned }, null, 2));
  assert.equal(planned.ok, true);
  assert.deepEqual(planned.errors, []);
}
NODE
```

At the design checkpoint read FULL four files and publish SHA256 values and
exact whitespace/status/spec36/catalog/reference results. All light commands
are <=60s TERM/kill10. Implementation results must map the ACs to actual
executed tests, include expected RED failures and passing controls, and retain
every command's original handle/log and join/group receipt. No generic QA,
review, simplify, full verification or coverage-percentage gate is added.

## Files likely touched

- `apps/web/components/gitlab/my-gitlab/use-gitlab-search.ts` (sole production file).
- `apps/web/components/gitlab/my-gitlab/use-gitlab-search.test.ts` (meaningful controls if missing).
- `apps/web/components/gitlab/my-gitlab/use-gitlab-search.workspace.test.tsx` (new independent regressions).
- Exactly this work order, sibling plan, owning GitLab requirement and part-1
  design for local status/results updates.

## Dependencies

No work-order dependency. ROOT accepted all four full files and hashes in
`/tmp/kandev-root-child54-reviewed-package-20261006.json` at 16:10:29Z, then
released same-primary authoring. ROOT later accepted CHILD53's explicit heavy
return and granted CHILD54 the exclusive lease and normal delivery. Production
was changed only after meaningful permanent causal RED was joined. Preserve
managed worktree/branch/deps/logs/foreign processes/shared caches/FETCH_HEAD,
protected proof and paused/unproved volumes. The live task plan owns the later
explicit heavy RETURN, one original terminal collector and separate serial
ROOT MERGE grant. Local checks do not complete the platform task or grant merge.

## Risks

Fixtures must tolerate old-page/page-1 effect ordering and hold superseded
responses without coupling to invocation index. Display clamping alone is
insufficient; the real native first-page link is the consumer oracle.
Environment/unknown failures require ROOT checkpoint, never a fabricated RED.

## Parallelism

`sequential`. Same primary; no delegation/tasks/sessions/model switch.

## Inputs

- [Requirement .5, .8, .9](../../specs/integrations/requirements/gitlab-integration.md).
- [Workspace browse pagination design](../../specs/integrations/system-design/gitlab-integration-01.md#workspace-browse-pagination).
- [Accepted proof and limits](plan.md#baseline-and-accepted-evidence).
- Scoped `apps/web/AGENTS.md`; `/tdd` and `/mobile-parity` skills after grant.

## Results

Local implementation complete. One independent permanent suite,
`use-gitlab-search.workspace.test.tsx`, now covers MR and issue workspace resets,
same-workspace controls, roundtrips, real MRList/native link/pagination outcomes,
both transports' superseded success/failure, pending visibility, existing
preset/query/kind resets, empty/failure/refresh, and disabled/no-workspace gates.
Only the two search transports are partially mocked; real providers, store,
rows, native links, Tooltip and locales remain in use. The existing hook and
page-state suites provide the unchanged milestone/project controls.

The one conditional frozen install used existing Node24.21.0 and Corepack
pnpm9.15.9, followed by formatting of the owned test. Permanent RED against
hook blob `8cf1678edf6eb6de210f31fd57db112fe18e64e5` failed exactly three
regressions: MR and issue pages stayed at 3, and the real MR list hid B's native
first-page link. Both same-workspace route controls and the initial small-B
consumer control passed (3 failed, 3 passed, 19 deselected). The original RED
handle was joined and its process group gone before adding workspaceId to the
existing reset dependencies.

GREEN passed 61 tests across the new workspace suite, existing hook suite, and
GitLab page-state suite; all 25 new regressions passed. Changed-file ESLint,
normal PROJECT typecheck (including ignored pretypecheck release-note/changelog
generation), i18n:check, and i18n:ratchet passed. The ratchet covered the one
modified production file; tests contain fixtures and add no product copy.
Native command receipts, final documentation gates, and file hashes are retained
in the live task plan and `/tmp/kandev-gitlab-pages-exec-5a73bdd1`.

Status `done` records local implementation only. Hosted review, CI, and merge
remain pending before publication. Normal active hooks and ready-PR delivery
are authorized; merging requires a later separate ROOT serial grant. No browser,
build, E2E, production consumer expansion, or original-proof replay was performed.
