---
id: "01-preserve-current-catalogue"
title: "Publish normal profile mutations over the current catalogue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.5
  - AC-EXECUTORS-PROFILE-EDITOR-001.13
  - AC-EXECUTORS-PROFILE-EDITOR-001.14
  - AC-EXECUTORS-PROFILE-EDITOR-001.15
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Task 01: Publish normal profile mutations over the current catalogue

## Summary

Make successful normal profile save publication use the current owning store
catalogue. Independently prove and then correct the same hook's sibling delete
publication; retain existing target behavior and settings interactions.

## Release and ownership

ROOT released implementation and the exclusive local-heavy lease after actual
DESIGN END in this same primary on 2026-10-09. Merge remains unauthorized.

Read the [manifest](plan.md#release-resource-and-delivery-gates) and live task
plan before resuming. The DESIGN prerequisite was satisfied by ROOT's later
explicit same-primary implementation interrupt and exclusive local-heavy lease.
No agents, additional sessions/tabs, model changes, or operator approval.
After release mark this sole work order `in_progress`; use `/tdd`.
Own only the normal `ProfileEditForm/useProfilePersistence` publication in
`apps/web/app/settings/executors/[profileId]/page.tsx`, the independent regression
file below and its narrowly needed test helpers, plus these four artifacts.
Immediate `upsertExecutorProfile` helper is permitted only if causal evidence
requires it; currently no helper change is expected. Never read/copy/import/
replay/edit/chmod/remove ROOT's protected diagnostic source.

## In scope

- Current-catalogue save acknowledgement with unrelated mixed-state preservation.
- Independently authored same-hook delete causal regression before its correction.
- Actual page, store/providers, coordinator, transport and production option hook;
  unchanged-success, rejected-save/draft/dirty, delete/navigation and failure controls.
- Exact targeted checks, actual-diff coverage and standing bounded delivery.

## Out of scope

All manifest exclusions, including server arbitration, target-resurrection
guarantees, lifecycle/request ownership, revisions, migration/writer audit,
backend/API/schema, plugin/Kubernetes connection edits, layout/copy/navigation
changes, new browser/build/full suites, foreign resources and delegation.

## Acceptance

1. The independent real-page save regressions reach causal catalogue and actual
   picker assertions in RED, then GREEN with current additions/updates/removals,
   sibling profiles and executor metadata preserved; accepted target and
   rejection/draft/dirty/notification controls retain existing behavior.
2. The independent real-dialog delete interleaving reaches its own causal RED
   before remove changes. GREEN removes only target and preserves current mixed
   unrelated state/options with successful real navigation; unchanged-success
   and rejection controls preserve the normal dialog and route behavior.
3. Every required scoped test/check and actual-diff documentation coverage passes
   with recorded original results and owned joins/gone evidence. No implementation
   or passing evidence is inferred from ROOT's private diagnostic or planned checks.

## Implementation sequence

1. Check local dependency/toolchain presence read-only. If absent, use the one
   frozen install below only after ROOT release/lease; no lockfile changes.
2. Author the plan's nine named cases in new
   `profile-edit-catalogue-publication.test.tsx`, independently of the protected
   test. Mount real `ProfileEditPage({profileId})`, `StateProvider` (actual
   `createAppStore`), `ToastProvider`, `SettingsSaveProvider` and existing UI
   providers. Seed loaded normal Local executor fixtures with target and
   unrelated/sibling profiles; preserve typed payloads and actual baseline readiness.
3. Stub external `fetchJson` by endpoint/method. Supply faithful ancillary
   transport responses; hold only target PATCH/DELETE using deferred promises
   and an explicit admitted-request signal. If necessary stub only external
   `@monaco-editor/react` visual rendering, preserving the internal script
   wrappers. Do not stub route/store/domain API/page/contributor/picker logic.
4. Change Name in the actual form. Wait for dirty/baseline readiness; invoke the
   actual shared save coordinator or rendered Save action. While admitted PATCH
   is held, publish later catalogue changes via real `setExecutors`. Observe
   those rows/options before releasing acknowledgement. Then assert every
   current unrelated row/value and absent removed row, accepted target value,
   actual hook option value/label/executor metadata and unchanged eligibility.
5. The subscribed consumer must apply the exact production flatten projection
   and `executor_type`/`executor_name` fallback from task-create and subtask code,
   and call actual `useExecutorProfileOptions`. Assert resulting choices in
   addition to store rows. This is end-to-end component evidence, not a live
   backend/remote execution claim. Cover separate additions/updates/removals
   and one mixed fixture so universal retention is not an existential assertion.
6. For delete use actual Delete Profile and confirm controls; hold admitted
   DELETE, publish mixed later state, then resolve. Assert only target gone,
   metadata/siblings/unrelated options retained and real client routing to
   `/settings/executors`. Reject another DELETE and verify unchanged current
   catalogue/route plus existing dialog recovery. Do not call private `remove`.
7. Execute original RED command below before any production edit. Record actual
   failed test names, reached catalogue/option assertions and passing controls.
   A Monaco fixture failure or collection/transport/resource error is not causal
   RED: checkpoint ROOT before alternatives. Independently proved delete loss
   is required before its correction; otherwise checkpoint ROOT.
8. Change `useProfilePersistence` to obtain `useAppStoreApi`; after transport
   success read current `executors.items` immediately before publication. Reuse
   existing `upsertExecutorProfile` for save; map/filter current state for delete.
   Update dependencies and remove the persistence-only captured catalogue
   subscription. Preserve all existing target fallback, payload, success/error,
   contributors, permission, deletion ordering, navigation bypass and notifications.
9. Run original GREEN plus the scoped controls, changed ESLint, typecheck, i18n
   and doc/reference checks below under the same resource discipline. Mark work
   order `done` and manifest `implemented` only with actual results. Existing
   owning requirement/design remain `active`/`current`; no historical package
   statuses are rewritten. If helper or test-helper edits become causal, add
   their exact paths/tests to the same checks before marking done.

## Verification

All shell blocks are rooted independently from the repository root. These are
future implementation commands, not authorization to run during design.
Use the existing Node 24.21.0 and pinned pnpm 9.15.9; prepend their installed
bin paths for each resumed shell, without repurposing HOME or CODEX_HOME:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
```

Only if workspace dependencies are absent, after ROOT lease, once:

```bash
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

RED: one worker, files sequential, Node heap 4 GiB, only new causal suite:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run components/settings/profile-edit/profile-edit-catalogue-publication.test.tsx --maxWorkers=1 --no-file-parallelism)
```

GREEN and narrowly relevant existing contributor/dialog controls, one original
command at a time (do not replay a passing command without a new change/reason):

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run components/settings/profile-edit/profile-edit-catalogue-publication.test.tsx components/settings/profile-edit/profile-edit-page-chrome.test.tsx components/settings/profile-edit/use-executor-profile-save-contributor.test.tsx components/settings/profile-edit/use-kubernetes-profile-page-save-contributor.test.tsx --maxWorkers=1 --no-file-parallelism)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 'app/settings/executors/[profileId]/page.tsx' components/settings/profile-edit/profile-edit-catalogue-publication.test.tsx components/settings/profile-edit/profile-edit-catalogue-publication.test-helpers.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use `.github/scripts/pr-docs.cjs` `validateCoverage` for the **actual** changed
paths, including untracked files, and their referenced contents. Design-only
classification is `exempt`; implementation must return `ok=true`,
`status=covered`, one changed work order and `errors: []`. Do not mistake the
planned-trigger design preflight for actual production coverage. The exact
local preflight (Node needs the PATH above) is:

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = 'ca69af0bd062d2fcff5bb155b6d37b41b9e10e7d';
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', base], {encoding: 'utf8'}).split('\0');
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding: 'utf8'}).split('\0');
const paths = [...new Set([...tracked, ...untracked].filter(Boolean))];
const docs = [
  'docs/specs/executors/requirements/profile-editor.md',
  'docs/specs/executors/system-design/profile-editor.md',
  'docs/plans/executor-profile-catalogue-preservation/plan.md',
  'docs/plans/executor-profile-catalogue-preservation/task-01-preserve-current-catalogue.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({filename, status: fs.existsSync(filename) ? 'modified' : 'removed'}));
const result = validateCoverage({changedFiles, fileContents});
process.stdout.write(JSON.stringify(result, null, 2) + '\n');
if (!result.ok || result.status !== 'covered' || result.errors.length || result.workOrders.length !== 1) process.exitCode = 1;
NODE
```

Before marking done verify the source diff remains inside ownership and inspect
changed paths for any extra ESLint/test inputs. Pure state/data mobile exception
means no browser/build/heavy full suite. Normal pre-commit/commit-msg hooks stay
enabled, without bypass. Persist every native handle and actual terminal result;
fresh owned process groups must be absent before releasing the heavy lease.
Any timeout/resource/transport/unknown/out-of-scope outcome checkpoints ROOT
before alternatives, never a cache wipe, foreign kill or weaker test.

## Files likely touched

- `apps/web/app/settings/executors/[profileId]/page.tsx` (bounded persistence only).
- New `apps/web/components/settings/profile-edit/profile-edit-catalogue-publication.test.tsx`.
- New narrowly needed adjacent `.test-helpers.tsx` only if the fixture would
  otherwise exceed local size/function limits; list and lint it if created.
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx` and
  existing test only if independently causal; expected unchanged.
- Existing owner requirement/design and this plan/work order (four artifacts).

## Dependencies

None. ROOT's later implementation interrupt plus local-heavy lease is the
execution prerequisite. No other work order, worker or moving-main dependency.

## Risks

See the manifest. Same-profile server ordering, target fallback and unrelated
writers remain existing contracts. Cleanup must settle test promises/timers,
unmount providers and restore history/navigation blockers on every exit.

## Parallelism

`sequential`

## Inputs

- [Owner requirement](../../specs/executors/requirements/profile-editor.md),
  AC .5 and .13 through .15.
- [Owner design](../../specs/executors/system-design/profile-editor.md#current-catalogue-publication).
- [Evidence, consumer audit and historical companion plans](plan.md#evidence-and-requirement-history).
- Scoped `apps/web/AGENTS.md`, `/tdd`, mobile-parity state/data exception,
  docs-maintainer audit and manifest delivery/merge barriers.
- Existing `dynamic-agent-profile-editor-save-concurrency.test.tsx` for real
  providers/deferred external transport and fixture cleanup patterns only.

## Results

Implementation in progress after ROOT's later explicit release. All originals
are archived under `/tmp/kandev-executor-catalogue-71d65c60-20261009`.

- Frozen pinned pnpm 9.15.9 install: `6a1e5a/session59609/cac741`, exit 0,
  actual join and owned group absent. Lockfile unchanged.
- Independent RED original `022760/session81137/01d7ee`, exit 1: five causal
  cases/four controls reached, but one missing external Monaco `loader` export
  produced an unhandled fixture error. This is not clean RED evidence.
- Corrected independent RED `fd42cf/session89928/ee9cce`, exit 1: five causal
  failures, four passing controls, zero unhandled errors. Both catalogue and
  actual picker assertions reached in four save cases and real-dialog delete;
  actual routing checked. Only external default renderer substituted, actual
  Monaco named exports preserved. Production was unchanged until this verdict.
- Minimal normal-page current-store correction, existing helper unchanged.
- Scoped formatting `3b113c`, exit 0. GREEN `599b46/session94862/35101f`, exit 0:
  four prescribed suites, all 20 tests passed. Node 4 GiB, one worker, sequential.
- Initial changed ESLint `149396/session44500/d47899`, exit 1: fixture duplicate
  string. An incorrect executor-ID extraction was followed by affected new-suite
  GREEN `a19c43/session50642/9f1a38`, exit 0, all nine tests passed. Unchanged
  existing passing suites were not replayed.
- Changed ESLint `e49d62/session60224/fbdc27`, exit 1; a second incorrect
  profile-ID extraction followed. Changed ESLint `f70637/session77164/54f1d9`,
  exit 1. Exact final diagnostic is `added-executor` four occurrences at line 70
  and line 106 of the fixture helper. Earlier live-plan guesses about the
  duplicate identity were incorrect. There is no lint suppression.

The `/fix` three-check stop rule requires a ROOT checkpoint before another
attempt. All nine original invocation groups are freshly absent and every
native handle actually joined; `lease-return-checkpoint.json` records the lease
returned to ROOT. No product processes, observer, publication or PR exists.
No foreign resources or protected proof were touched.

Next correction: extract the exact `added-executor` fixture constant and remove
unnecessary earlier constants if appropriate. After ROOT continuation and lease,
run only the affected nine-case suite and changed ESLint, then pending typecheck,
i18n checks, docs/actual coverage and normal active-hook commit/push/ready PR.
The final profile-ID constant edit has no passing suite receipt yet. No change
to production since the 20-test GREEN. Merge remains unauthorized.

ROOT subsequently regranted the exclusive lease for one precise constant repair.
The exact four `added-executor` uses now share `ADDED_EXECUTOR`; values and
assertions are unchanged. Affected nine-case suite `194a20/session75062/118c30`
passed all nine; changed ESLint `e1490b/session62791/10593b` passed with zero
warnings. Both original invocations actually joined with owned groups absent.
Earlier unrelated identity constants remain valid and were retained as allowed.
The earlier stop and lease-return record above remains historical. Pending
typecheck, i18n, docs/actual coverage and normal delivery continue under release.

Initial typecheck `c6cd92/session88195/b531d1` exited 2 on two test-only typed
inputs: unsupported executor-state `loaded`/`loading` fields and unsupported
role-query `exact` option. Used the actual `ExecutorsState` items shape and
anchored `/^Delete$/` accessible name, preserving catalogue and selection.
Affected suite `4fba91/session50359/e9e331` passed all nine; changed ESLint
`e0ec87/session41540/09b929` passed. Typecheck
`a97351/session4553/71cf75` passed. i18n check
`6bcb44/session18155/da0b66` passed (existing orphan-key warnings retained).
All originals actually joined, owned groups absent, no production expansion.

i18n ratchet `46fea4/session40468/ad111d` passed. Documentation checks
`a6c925/session66552/3b41fd` passed: 365 decisions/1,477 specs, 36 validator
tests, specification lint and diff whitespace. Actual changed-path coverage
`3449b6` passed: `ok=true`, `status=covered`, one owning work order, `errors=[]`,
including the page and adjacent helper triggers. All original calls joined
and fresh owned groups were absent. Implementation acceptance is complete;
normal active-hook delivery, hosted gates and ROOT serial merge remain separate
task-completion gates. No browser/full-suite/public-guide expansion.

Normal commit `f17532/session26989/d3a10d` exited 1; no commit was created.
Active harness/docs/architecture/spec/web-lint/public-copy hooks passed; Go
and E2E sleep hooks skipped. Prettier formatted one fixture line. Staged
i18n-new-code rejected the two fixture profile-name constants: its exact test
filename exclusion omits `.test-helpers.tsx`, which ESLint explicitly treats
as test-only. No suppression, hook bypass, rename or new production copy was
introduced. ROOT checkpoint required before a remedy; local-heavy lease
returned with every original actually joined and a fresh owned PID/group scan
empty (`hook-blocker-lease-return.json`). Local acceptance remains complete;
publication and hosted gates remain blocked. Latest hook formatting is
uncommitted. Persistent task remains incomplete and merge unauthorized.

ROOT released the bounded hook remedy and exclusive lease. Original parent
question failed without a question ID; no duplicate question was sent. The exact
profile-name literals remain in the recognized test file and are passed into
adjacent fixture setup/builders; no rename, exemption, suppression or production
copy change. The original formatter linewrap was restaged without amend.
Affected nine-case suite `53e56f/session57514/a4b541`, changed lint
`fad78d/session69894/201f8f`, typecheck `09b6e5/session97591/5e72c3`,
and staged i18n ratchet `b768d1/session69732/95f76f` all passed. The staged
ratchet included the new helper (one added and one modified guarded file).
All originals actually joined and fresh owned groups were absent. Earlier
passing compatibility/RED/install/full-i18n/doc checks were not replayed.
