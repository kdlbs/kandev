---
id: "01-scope-project-options"
title: "Scope GitLab project choices to the workspace"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.10
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-01.md
---

# Task 01: Scope project choices to the workspace

## Summary

Include workspace identity in the existing GitLab project-option context so
settled MR and issue results cannot retain unrelated projects accumulated in
another workspace. Preserve same-context pagination and intentional selected
project inclusion through real hook/provider and toolbar tests.

## In scope

- Forward normalized workspace identity from `useSearchAndProjects` to
  `useProjectOptions`; add it to the delimiter-safe JSON reset tuple.
- Independently author permanent causal and compatibility tests under the later
  explicit implementation grant. Read ROOT proof only.
- Update direct callers in existing page-state tests for the explicit input.
  Preserve existing guards, filters, sorting and accumulation semantics.

## Out of scope

- Consumer production, accumulator/cache ownership, search/request sequencing,
  cancellation, forced filter resets, saved presets, backend/API/credential or
  unrelated GitHub changes; require causal evidence and ROOT scope release.
- Browser/build/E2E/screenshots or UI changes; the mobile pure-state exception
  applies to unchanged toolbar, rows, copy, touch, navigation and breakpoints.
- Parked profile-toggle candidate, protected fixture copying/replay, new test
  dependencies, setup/harness repair, broad verification or optional polish.

## Acceptance

1. Independently authored real page-state/provider tests fail causally before
   the fix for settled workspace membership and real toolbar choices, while
   the same-workspace accumulation control passes (AC .5, .8, .10).
2. MR and issue workspace changes, both directions/A-B-A and empty replacement
   results, clear all prior accumulated membership. Same-context page/equal-input
   reuse, context changes and explicit selected-filter inclusion remain correct.
3. Focused GREEN and nearby unchanged controls preserve loading/empty/error,
   refresh/disabled search and existing stale-response behavior; no request-order
   redesign or consumer production change is introduced.

## Test matrix

Use `StateProvider`/real store and production `useGitLabPageState`,
`useGitLabSearch`, `useKnownProjects`, `ListToolbar`, Radix Select, native rows
and locale setup. Partial mocks may replace only search MR/issue and settings
transports, preserving unrelated exports. Do not mock hooks/components/store,
copy derivation predicates, inspect source strings or add helper-only mirror tests.

The new suite `GitLab project options workspace scope` contains these exact
baseline names for the anchored RED selector:

- `excludes previous workspace projects after the current MR response settles`
- `offers only current workspace projects in the real toolbar`
- `retains earlier-page projects in the same workspace`

Also cover issue transitions with equal selection/query/milestone; MR and issue
A-B-A and reverse transitions; accumulated A pages followed by settled B; initial
B and empty B. Assert real selectable toolbar choices after B settles and select
a B project to exercise the normal callback/narrowing path. Separately retain
an explicit selected filter absent from current rows, without retaining any
other old project. Cover same-workspace equal-input reuse, query/milestone/kind
resets, loading, same-context empty/error/refresh, disabled search and a deferred
obsolete response. Preserve established search behavior; a new independent
search defect is a checkpoint, not scope for a redesign. Use native promises
and causal waits, with no sleeps or speculative exact request-count contract.

## Verification

ROOT reviewed the four full artifacts after the completed design checkpoint
and authorized implementation with the exclusive global local-heavy lease on
2026-10-06. Retain that lease for installs, tests, ESLint, typecheck, i18n and
commit hooks. No overlapping local command or delegated test authoring is planned.

Run Bash with login disabled from the repository root. Before each command,
record UTC start, absolute cutoff, exact argv, log, outer session/chunks and
PID/PGID. Retain and actually join the original handle and confirm its process
group is gone before proceeding. Timeout/setup/resource/transport/unknown or
out-of-scope outcome ends at ROOT checkpoint, with no automatic replacement or
retry. Correct routine own causal fixture/lint mistakes within scope and rerun
only the affected check.

Use this environment in every command block:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
```

If `apps/node_modules` is absent, perform exactly one conditional install after
the heavy grant; no dependency repair for ROOT's fixture:

```bash
if [ ! -d apps/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

RED, after independent test authoring and before production correction:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales app/gitlab/use-gitlab-page-state.workspace.test.tsx -t '^GitLab project options workspace scope (excludes previous workspace projects after the current MR response settles|offers only current workspace projects in the real toolbar|retains earlier-page projects in the same workspace)$' --maxWorkers=1 --no-file-parallelism)
```

Confirm the two causal failures and passing accumulation control from the joined
original result. Apply only the page-state correction, then focused GREEN:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales app/gitlab/use-gitlab-page-state.workspace.test.tsx app/gitlab/use-gitlab-page-state.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales components/gitlab/my-gitlab/use-known-projects.test.ts components/gitlab/my-gitlab/use-gitlab-search.test.ts components/gitlab/my-gitlab/list-toolbar.test.tsx --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec eslint --max-warnings 0 app/gitlab/use-gitlab-page-state.ts app/gitlab/use-gitlab-page-state.test.ts app/gitlab/use-gitlab-page-state.workspace.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:ratchet)
```

Normal project typecheck includes its existing ignored-JSON pretypecheck
generation. Do not use direct `tsc`, bypass pretypecheck or repair setup. i18n
ratchet must report the actual changed source paths; tests are excluded and
there is no new user-facing copy. Each command is rooted independently; execute
serially and retain every original result rather than launching the whole block
without supervision. No full product suite or browser build.

Documentation checks (also permitted as light design checks, bounded at 60s):

```bash
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/integrations/requirements/gitlab-integration.md docs/specs/integrations/system-design/gitlab-integration-01.md docs/plans/gitlab-project-option-workspace-scope
git status --short -- docs/specs/integrations/requirements/gitlab-integration.md docs/specs/integrations/system-design/gitlab-integration-01.md docs/plans/gitlab-project-option-workspace-scope
```

Run the repository `validateCoverage` API with actual changed paths and complete
four-file contents, then with the declared prospective source/test paths at the
design checkpoint. Report the latter as prospective local preflight, never hosted
evidence. At implementation/publication use actual changed-path coverage:

```bash
timeout --signal=TERM --kill-after=10s 60s node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/integrations/requirements/gitlab-integration.md',
  'docs/specs/integrations/system-design/gitlab-integration-01.md',
  'docs/plans/gitlab-project-option-workspace-scope/plan.md',
  'docs/plans/gitlab-project-option-workspace-scope/task-01-scope-project-options.md',
];
const paths = new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD'], { encoding: 'utf8' }).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
].filter(Boolean));
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles: [...paths], fileContents });
console.log(JSON.stringify({ mode: 'actual working-tree paths', ...result }, null, 2));
if (!result.ok) process.exit(1);
JS
```

Before commit, repeat coverage using the actual branch diff paths relative to
the authoritative PR base, rather than `HEAD` once changes are committed. Keep
frontmatter requirement/AC/design/plan links valid and inventory/hash all four
artifacts, including untracked files; `git diff --check` alone omits untracked
files, so separately check their whitespace. Validate whole changed diff before
publication. Mark local Results truthfully while hosted review and merge remain
pending, avoiding design-wording cleanup after freezing the published SHA.

## Files likely touched

Production ownership:

- `apps/web/app/gitlab/use-gitlab-page-state.ts`

Tests under the reviewed-package implementation grant:

- `apps/web/app/gitlab/use-gitlab-page-state.workspace.test.tsx` (new)
- `apps/web/app/gitlab/use-gitlab-page-state.test.ts` (direct context arguments)

Unchanged nearby controls:

- `apps/web/components/gitlab/my-gitlab/use-known-projects.test.ts`
- `apps/web/components/gitlab/my-gitlab/use-gitlab-search.test.ts`
- `apps/web/components/gitlab/my-gitlab/list-toolbar.test.tsx`

Delivery artifacts are the four files listed in the coverage command. Public
docs remain accurate and unchanged. No dependencies or generated tracked files.

## Dependencies

None. ROOT granted the later reviewed-package implementation interrupt and
exclusive global local-heavy lease on 2026-10-06. Hosted and merge gates remain
separate from this sequential local execution.

## Risks

Reset guards must exclude previous-context rows while preserving same-context
accumulation. Selected-filter inclusion is intentional and must be tested apart
from accidental membership. The module accumulator still has one active key;
this order does not implement a cache or concurrent-instance ownership policy.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/integrations/requirements/gitlab-integration.md),
  AC .5, .8, .10.
- [Owning design](../../specs/integrations/system-design/gitlab-integration-01.md#browse-project-option-context).
- [Manifest evidence and boundaries](plan.md).
- Existing page-state, search, known-project accumulator, toolbar and their tests.
- ROOT's read-only proof/classification receipts retained in the live task plan.

## Results

The design turn ended before ROOT reviewed all four full artifacts and issued
the later implementation grant. Project-options criterion .10 preserves .9 for
the separate pagination package. The only production correction forwards workspace
identity into the existing page-state JSON context tuple.

Local results on 2026-10-06:

- Independently authored anchored RED: two causal failures, one passing
  same-workspace accumulation control, 11 unselected tests. Production was still
  unchanged at this joined result.
- Focused GREEN: two files, 32 tests passed. After routine test lint corrections,
  all 14 new tests passed again.
- Nearby unchanged search, accumulator and toolbar controls: three files,
  31 tests passed.
- Changed-file ESLint passed with zero warnings; normal project typecheck passed
  with the legitimate existing ignored-asset pretypecheck.
- i18n check and ratchet passed; one modified production file was checked, no
  copy was added, and tests are excluded by policy.
- One conditional pinned frozen install completed because worktree dependencies
  were absent; no tracked lockfile or dependency changes.
- Historical design catalog, all-spec lint, 36 linter tests and reference/link/
  whitespace gates passed. Publication checks cover actual changed paths.

Original terminal handles, UTC cutoffs, PID/PGID disappearance, full logs and
receipts are retained in the live task plan. Initial lint failures were corrected
within the new test fixture and the affected checks passed. ROOT's protected
proofs were neither replayed nor copied. Remote main matched the design base at
the prepublication static read, so no incoming owning-document additions required
integration; preserve pagination .9 and project choices .10 on later incoming
main comparisons.

Local implementation is done. Hosted checks, full current-head review, separate
merge authorization, merge verification and joined cleanup remain pending;
status stays in progress until delivery completes.
