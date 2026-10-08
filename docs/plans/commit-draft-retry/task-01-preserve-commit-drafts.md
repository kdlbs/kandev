---
id: "01-preserve-commit-drafts"
title: "Preserve commit drafts through failed acknowledgements"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-COMMIT-DRAFT-RETRY-001
acceptance_criteria:
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.1
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.2
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.3
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.4
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.5
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.6
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.7
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.8
  - AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.9
system_design:
  - ../../specs/workspaces/system-design/commit-draft-retry.md
---

# Task 01: Preserve commit drafts through failed acknowledgements

## Summary

Add the local feedback acknowledgement and commit-only draft/attempt ownership
described in the paired design. Prove failure retention and unchanged-success
reset through real native dialog controls and Git hooks, then document retry in
the existing public how-to. One coherent sequential implementation pass.

## Release and identity gate

DESIGN ONLY until ROOT reviews this concrete four-file package and sends a
LATER explicit implementation INTERRUPT to task
`d3fa44a4-db3f-4e3d-9d61-82dbfb0f57e5`, session
`ad44d4cc-4a7f-4f81-9f7b-0a405be2d66d`. Keep the readable title **Keep failed
commit messages available for retry** and system marker in the version-safe
platform plan. No automatic implementation, approval/model-switch question,
queued parent-notification gate, delegates, recursive tasks, new sessions/tabs
or model changes. ROOT reads the primary conversation and plan directly.

## In scope

- A `Promise<boolean>` feedback acknowledgement with existing toasts intact.
- Feature-local commit state extraction, captured raw draft/payload and exact
  scope, synchronous duplicate admission, generation/revision settlement guards.
- Native integration, state-hook and feedback-hook tests, public recovery note,
  task-defined affected checks and later authorized normal delivery.
- Keep this work order and manifest results/status synchronized. After all
  checks pass and contracts match, promote the paired requirement to active,
  design to current and plan to implemented.

## Out of scope

Backend/API/permissions, other Git operations, abort/rollback, automatic retry,
durable draft storage, per-repository draft collections, generic mutation
infrastructure, session-lifetime redesign, rendered/copy/layout/touch/navigation
changes, full suites/build/browser work and unrelated repairs. A concrete
blocker needs a separate ROOT scope extension.

## Acceptance

1. Native dialog tests prove both failure paths retain exact title/body,
   Stage all and repo, including same-scope dismissal/reopen and user retry;
   ordinary success closes/resets and payload/blank-title admission is correct
   (`.1` through `.4`). Feedback tests pin actual true/false/exception returns
   and existing loading-to-success/error toast details.
2. Deferred state/native tests prove captured payloads, newer edits (including
   edit/revert), repeated callback admission, visibility choices, exact
   omitted/root/named scope separation, stale outcome isolation, session and
   environment change away/back, and unmount (`.5` through `.7`). Settle every
   admitted deferred request. An old outcome cannot release a newer attempt.
3. Real seeded multi-repository fan-out preserves partial-success/error
   semantics, and shared desktop/phone rendered controls retain state through
   viewport changes (`.8`, `.9`). All exact checks pass; results name actual
   evidence and limitations, and normal hooks run without bypass/amend.

## Mobile parity and rendered evidence

Apply the pure state/data exception described in the [design](../../specs/workspaces/system-design/commit-draft-retry.md#desktop-and-phone-verification).
Rendered structure/copy/classes and touch/navigation/scrolling are unchanged;
no layout preview or new mobile Playwright test is required. Parameterize the
native recovery flow at desktop (1280 by 800) and phone (393 by 851) window sizes
and exercise a viewport change while pending without remount. Assert inputs,
checkbox, scope label, commit disabled/loading, error toast, dismissal/reopen
and successful reset, not only hook state. These checks prove shared state
semantics, not actual phone geometry or live backend Git execution.

## Sequence and verification

After release, read the requirement/design/plan and scoped guidance, mark this
order in_progress, and acquire the global local-heavy lease serial with the
other active child. Use `/tdd`: independently author new permanent tests from
the contract, collect the smallest relevant expected red failure, implement,
and run the selected suites for green. ROOT's immutable candidate is accepted
evidence; never replay/copy/import/edit/delete it. Do not manufacture stronger
claims from the earlier disappearance receipt.

At design time dependencies were absent. After heavy release, ONE pinned pnpm
9.15.9 frozen install completed from apps; never repeat it:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

Use Node 24 and retain native process handles/receipt logs. Run each following
command serially from its rooted directory. Capped Vitest selection is the
same for the final green pass; initial red may select the new suite alone.
The Node path above is the existing installation discovered during design;
use it for the complete verification blocks, without installing another Node.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m corepack pnpm@9.15.9 exec vitest run --project=browser-locales --maxWorkers=1 components/vcs/vcs-dialogs.commit.test.tsx components/vcs/use-commit-dialog-state.test.tsx hooks/use-git-with-feedback.test.tsx components/vcs/vcs-dialogs.test.ts hooks/use-git-operations.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/vcs/vcs-dialogs.tsx components/vcs/use-commit-dialog-state.ts components/vcs/use-commit-dialog-state.test.tsx components/vcs/vcs-dialogs.commit.test.tsx hooks/use-git-with-feedback.ts hooks/use-git-with-feedback.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 6m corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

`typecheck` generates release-note/changelog inputs through its existing
pre-script; inspect status and retain only owned changes. No generated-file,
lockfile or package-config edits are planned. All changed/new test suites are
named above; the existing helper/payload tests are narrow compatibility controls.
No frontend/backend/full Vitest/E2E/build rerun is admitted.

Run actual documentation coverage preflight from repo root. It loads the
four-file package and checks real changed paths, including untracked additions:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const added = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...tracked, ...added])].map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' }));
const paths = ['docs/plans/commit-draft-retry/plan.md', 'docs/plans/commit-draft-retry/task-01-preserve-commit-drafts.md', 'docs/specs/workspaces/requirements/commit-draft-retry.md', 'docs/specs/workspaces/system-design/commit-draft-retry.md'];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

For design validation only, the Python catalog/spec gates, documentation
coverage preflight and scoped git whitespace/status checks may run now; none
executes product code or needs installed dependencies. Public validators run
after the public how-to edit during implementation.

## Files owned

- `apps/web/components/vcs/vcs-dialogs.tsx`
- `apps/web/components/vcs/use-commit-dialog-state.ts` (new, bounded extraction)
- `apps/web/components/vcs/use-commit-dialog-state.test.tsx` (new)
- `apps/web/components/vcs/vcs-dialogs.commit.test.tsx` (new)
- `apps/web/hooks/use-git-with-feedback.ts`
- `apps/web/hooks/use-git-with-feedback.test.tsx` (new)
- `docs/public/sessions-and-review.md` (small existing commit how-to note)
- The paired requirement/system design, manifest and this work order.

Consumers, use-session-git, use-git-operations, existing tests, state store and
backend are read-only inputs/controls. No changes to them are planned.

## Dependencies

No prior work order. Later explicit ROOT release, local-heavy lease and the one
frozen install are execution prerequisites. No material design question remains.

## Delivery and completion gates after release

- Keep ownership narrow. Record every original native handle and actually join
  each to terminal before another heavy command. Retain process-group evidence
  and prove owned groups gone before explicit RETURN of the global heavy lease.
  Never discard a handle or infer its result from a duplicate run.
- Routine causal test/lint repair is affected-only. Any resource, timeout,
  transport, unknown or out-of-scope blocker checkpoints ROOT before retry.
  Use the parent-question barrier only for a critical decision that prevents
  safe continuation; that call ends the turn. Preserve phase/identity/system
  marker/user edits and exact next action in version-safe plan checkpoints.
- After authorized checks, use `/commit`, `/push`, `/pr` with normal hooks and
  new Conventional Commits. No bypass or amend. If formatting hooks fail after
  modifying owned files, re-stage those files and create a new commit normally.
  Never stage unrelated files. Freeze the published head except real corrective
  findings; no main-only rebase or synthetic merge testing.
- One retained observer for the published head:
  `timeout --kill-after=10s 91m scripts/pr-await <PR> --mode all-terminal --deadline-min 90 --interval-sec 60 --format json`.
  Retain/join that original native observer before any ROOT-granted replacement.
  Keep primary commentary updates while it runs. No optional second-review wait.
- Require exact-head six required contexts AND actual Backend/Frontend/E2E
  parent workflows SUCCESS. Required-context rollup alone is insufficient.
  Final evidence must be fresh, complete with `errors: []`, and contain no
  actual actionable, unresolved, hidden or human findings. Inventory configured
  checks from actual repository policy; do not invent context names or trust
  skipped/cancelled parents as success.
- Authenticated configured CodeRabbit App 347564 substantive FULL automatic
  current-head/all-file coverage is acceptable. Only a true coverage gap permits
  one necessary request; no redundant manual review request or optional wait.
- No hosted retry or MERGE authority without a separate ROOT grant. If granted,
  use normal expected-head squash only. Completion requires verified merged SHA,
  owned content and remote state, every original handle joined and only-owned
  cleanup. Retain the clean managed worktree, dependencies and caches for ROOT
  archival and proof release; do not delete ROOT proof artifacts.

## Risks

Late closures, passive scope retirement, StrictMode cleanup, edit/revert revision
checks, root-scope truthiness and partial-success fan-out. Keep the local state
boundary small; a generic mutation owner or abort/rollback path is outside scope.

## Parallelism

`sequential` in the same primary session. No native delegates.

## Inputs

- [Requirement](../../specs/workspaces/requirements/commit-draft-retry.md),
  [system design](../../specs/workspaces/system-design/commit-draft-retry.md),
  [manifest](plan.md), `apps/web/AGENTS.md` and applicable skills.
- `vcs-dialogs.tsx`, feedback hook, real useSessionGit/useGitOperations sources,
  consumer inventory and adjacent rendered dialog/field test patterns.
- Accepted ROOT proof receipts in the manifest. Do not consume candidate source.

## Results

Local implementation completed after the later explicit ROOT release on
2026-10-08, under the sole global local-heavy lease. No permanent test/production
edit or product check occurred in DESIGN. The owning pair is active/current;
the manifest is implemented. This order's local implementation is done;
normal hooks/publication, hosted evidence and separately granted merge remain
external delivery gates, recorded in the direct primary/platform plan.

- RED native 35905, start 9b0497, actual join 34a9fb, exit 1: dialog-loss and
  missing boolean acknowledgement regressions. One fixture wording error was
  corrected; no claim that all 19 failures were product failures.
- Final exact five-suite GREEN native 92091, start c83067, actual join 98bfa3,
  exit 0: 50 tests across five files. Earlier 27/28 green exposed one
  multi-repository fixture dependency assumption, repaired without dispatch edits.
- Changed-file eslint passed; its only initial warning was a test describe
  group over the line limit. Affected state lint/suite repair passed (10 tests).
- Typecheck native 53953, start 4b4f82, actual join 83f29d, exit 0 after three
  integration-fixture typing fixes. Affected integration lint/suite passed
  (16 tests). These are not additional unique tests or broader compatibility runs.
- i18n check native 98262, start 271a0e, actual join b39562, exit 0; ratchet
  native 49582, start b8c4c3, actual join 2c7692, exit 0. Staged normal hooks must
  also check new files, which the earlier unstaged ratchet did not inventory.
- Docs native 48136, start f380f2, actual join 69f13d, exit 0: catalog,
  specification tests/lint, public validator tests/pages, actual coverage
  covered with errors empty and whitespace/status. Status promotion is checked
  again before normal delivery.

All later native results are retained in full in
`/tmp/kandev-child84-execution`, with subprocess exits, UTC cutoffs and absent
owned groups. The one frozen install's subprocess wait/reap exit 0 is recorded,
but its original native handle was omitted and never recovered. ROOT explicitly
qualified that one terminal recovery at
`/tmp/kandev-root-child84-install-terminal-recovery-20261008.json`. Do not claim
a native-level install join or rerun it. No other native handle exception.

Rendered controls at desktop/phone sizes prove shared state semantics with
mocked transports, not browser geometry or live backend Git. Partial successful
Git writes remain completed. No backend/API/other Git operation change,
abort/rollback, persistence, broad suite, browser/build or scope expansion.
