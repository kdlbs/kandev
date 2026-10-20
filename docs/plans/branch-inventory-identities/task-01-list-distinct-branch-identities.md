---
id: "01-list-distinct-branch-identities"
title: "List distinct branch identities"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-INVENTORY-001
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-INVENTORY-001.1
  - AC-WORKSPACES-BRANCH-INVENTORY-001.2
  - AC-WORKSPACES-BRANCH-INVENTORY-001.3
  - AC-WORKSPACES-BRANCH-INVENTORY-001.4
system_design:
  - ../../specs/workspaces/system-design/repository-branch-inventory.md
---

# Task 01: List Distinct Branch Identities

## Summary

Preserve every distinct local and remote-tracking branch identity through real
service enumeration, regardless of loose or packed storage. Independently author
native-Git regressions first, then make the smallest internal map-key correction.

## In scope

- All ACs in `REQ-WORKSPACES-BRANCH-INVENTORY-001` and the paired design's
  internal identity, flow, compatibility, and verification sections.
- Change keys in `collectLocalBranches`, `collectRemoteBranches`, and
  `parsePackedRefs` together to full qualified ref names. Keep returned values,
  map type, sort, loose-before-packed order, and packed absent-only insertion.
- Own the new regression file and the production discovery file only, plus
  delivery results and spec lifecycle fields in this package.

## Out of scope

All exclusions in the plan apply, particularly frontend production, option-value
ambiguity, request/save ordering, ref validation/normalization, authorization,
providers, network Git operations, public shape, schema, copy, and layout.
No production or permanent test edits until ROOT review and a later explicit
implementation interrupt to this same primary. No delegation or new sessions.

## Acceptance

1. The independently authored `TestListBranchesWithCurrent_DistinctBranchIdentities`
   exercises actual service reads against genuine `t.TempDir()` Git repositories.
   Loose and packed collision cases fail before the correction for the expected
   missing local or remote entry; both disjoint controls pass. Both saved-local
   repository ID and explicit-path reads return complete ordered expected
   identities, unchanged metadata, and the checked-out local name after the fix.
2. `TestListBranchesWithCurrent_MixedBranchStorage` and
   `TestListBranchesWithCurrent_LinkedInventory` prove exact-identity cardinality,
   shared refs, worktree-specific HEAD, existing ordering and remote HEAD
   exclusion. Fixture oracles verify distinct tips and real loose overrides;
   service results assert fields it actually exposes. Listing leaves refs,
   commits, and checkout unchanged.
3. All exact bounded checks below pass with recorded command/exit evidence;
   actual owned tracked plus untracked code/test coverage reports `covered`
   and `errors: []`; results and lifecycle states reflect completed behavior.

## Implementation sequence

After the later interrupt, read this package and scoped backend guidance, then
mark only this work order `in_progress`. Reuse existing native-Git helpers such
as `initRealGitRepo`, `isolatedGitEnv`, and service setup patterns where suitable;
do not extend oversized existing test files or change global Git policy.
Create independent fixtures and expectations, never private ROOT fixture code.

Create local `remotes/origin/topic` and remote-tracking `origin/topic` at distinct
real commits with native Git. Check out the local branch. Run loose and
`pack-refs --all --prune` cases with disjoint `feature/topic` controls in both
forms. Exercise both local entry points. Native `show-ref`/`rev-parse` checks
establish fixture identities and tips; expected `Branch` values are stated
independently of production key construction. Compare the complete ordered list,
not only containment of one entry. Capture before/after refs and HEAD for read
non-mutation. Keep all writes inside owned temporary repositories.

For mixed storage, pack then update both exact identities to newer tips so
native Git creates loose overrides over stale packed entries. Assert each
identity once and unchanged unrelated inventory. A real linked worktree tests
the common-directory path with its own current branch. Include a second remote,
nested branch names, and remote HEAD exclusions. Preserve existing provider,
grant, detached-HEAD, and other compatibility tests via the full service package.

Run the narrow red command and record expected failures plus passing controls.
Change all identity sites, rerun that same command green, then run the full
service race package and changed-Go lint sequentially. No full backend gate,
frontend install/build, screenshot, Playwright, or HTTP/browser coverage claim.

## Verification

Run from repository root with Go 1.26. Verify the toolchain before execution;
do not download or switch toolchains speculatively. GNU timeouts bound only
these task-owned commands. No resource escalation or cache deletion.

```bash
# Existing toolchains; the default shell Go is older and omits Node from PATH.
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
env GOTOOLCHAIN=local go version
node --version

# RED, then GREEN after the production correction.
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 180s go test -v -race -trimpath -tags=fts5 -p=1 -count=1 -run '^TestListBranchesWithCurrent_(DistinctBranchIdentities|MixedBranchStorage|LinkedInventory)$' ./internal/task/service)

# After GREEN, one canonical complete-package race run.
(cd apps/backend && env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 180s go test -race -trimpath -tags=fts5 -p=1 -count=1 ./internal/task/service)

# Complete changed Go package, internal lint timeout 5m, outer timeout 360s.
env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS='-p=1 -trimpath' timeout --kill-after=10s 360s bash scripts/lint-go-changed apps/backend/internal/task/service/repository_discovery.go apps/backend/internal/task/service/repository_branch_inventory_test.go

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Run actual documentation-coverage preflight from repository root, including
owned untracked files and the four documents. No speculative filenames may
stand in for actual changed code/test inventory:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
node <<'NODE'
const fs = require('node:fs');
const {execFileSync} = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/workspaces/requirements/repository-branch-inventory.md',
  'docs/specs/workspaces/system-design/repository-branch-inventory.md',
  'docs/plans/branch-inventory-identities/plan.md',
  'docs/plans/branch-inventory-identities/task-01-list-distinct-branch-identities.md',
];
const owned = new Set([...docs,
  'apps/backend/internal/task/service/repository_discovery.go',
  'apps/backend/internal/task/service/repository_branch_inventory_test.go',
]);
const readPaths = args => execFileSync('git', args, {encoding: 'utf8'})
  .split('\0').filter(Boolean);
const actual = [...new Set([
  ...readPaths(['diff', '--name-only', '-z', 'HEAD']),
  ...readPaths(['ls-files', '--others', '--exclude-standard', '-z']),
])];
const changedFiles = actual.filter(p => owned.has(p)).map(filename => ({
  filename, status: fs.existsSync(filename) ? 'modified' : 'removed',
}));
for (const filename of [...owned].filter(p => p.endsWith('.go'))) {
  if (!changedFiles.some(p => p.filename === filename)) {
    throw new Error('Expected actual owned Go change missing: ' + filename);
  }
}
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify({actualOwnedPaths: changedFiles.map(p => p.filename), ...result}, null, 2));
if (!result.ok || result.status !== 'covered' || result.errors.length) process.exit(1);
NODE
```

The design turn runs only light document checks. Its actual docs-only preflight
is correctly `exempt`; that is not implementation coverage. Later implementation
must include both owned Go files in the actual changed inventory and pass the
`covered` condition above. Keep normal staged i18n selectors and active hooks
when subsequently delivering; install frozen hook dependencies once only if
missing, using the toolchain versions specified in the plan.

## Files likely touched

- `apps/backend/internal/task/service/repository_discovery.go`
- `apps/backend/internal/task/service/repository_branch_inventory_test.go` (new)
- The paired requirement/design and this plan/work order for lifecycle/results.

## Dependencies

No work-order dependency. ROOT review and the later same-primary implementation
interrupt are the execution checkpoint. No callback or WFI prerequisite.

## Risks

- Updating only one collector can hide branches or duplicate exact identities.
- Commit tips are fixture evidence, not returned service metadata.
- Never read/copy/replay/delete private `candidate_test.go` or `run.py`.
- Preserve protected foreign resources and all existing source/evidence receipts.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/repository-branch-inventory.md)
- [System design](../../specs/workspaces/system-design/repository-branch-inventory.md)
- [Plan](plan.md), including admission and later delivery constraints.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- `repository_discovery.go`, `repository_discovery_test.go`, and
  `fresh_branch_test.go` native-Git helpers in the service package.

## Results

Completed after ROOT's later explicit implementation release to the same primary.
All four ACs pass through actual service reads. Production changed only the
private map keys in the three collectors; loose insertion order and absent-only
packed merging are preserved. No public metadata, sorting, path/provider policy,
or frontend production changed.

| Command in Verification / setup | Native handle and actual join | Result |
| --- | --- | --- |
| One `pnpm install --frozen-lockfile` from `apps/` | 36412 / 341417 | exit 0; 2.072s; existing caches reused |
| Narrow RED, verbose race command above | 40376 / 4dac44 | expected exit 1; command 130.504s, package 4.210s |
| Same narrow command GREEN after key correction | 44439 / 1ac426 | exit 0; command 116.747s, package 3.925s |
| One complete service race command above | 38279 / 39ee05 | exit 0; command 95.964s, package 90.069s |
| Canonical changed-Go package lint command above | 97043 / d0787a | exit 0; command 113.785s |

RED showed the opposite omitted identities in loose and packed collision cases
for both path and saved-local-repository reads. Both disjoint controls passed in
both storage forms and through both entry points. GREEN passed all three test
methods, including real loose overrides over packed refs and primary/linked
worktree reads. Complete expected lists prove exact cardinality and metadata;
native Git proves distinct/overridden tips. Refs, commit reachability, and HEAD
snapshots match before and after each service read. No returned-tip claim is made.

Final `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, cross-reference checks, actual six-path
coverage preflight (`covered`, `errors: []`), and whitespace/status checks passed.
No unchanged linter self-tests were replayed. Requirement is `active`, design is
`current`, plan is `implemented`, and this single work order is `done`.

Receipts directory:
`/tmp/kandev-child136-branch-inventory-implementation-5c4AGR/`.
Each command's `<label>-startup.json`, `<label>.stdout`, `<label>.stderr`, and
`<label>-join.json` retain the original invocation and actual terminal result;
labels are `install`, `red`, `green`, `service-race`, and `go-lint`.
Final light-check/coverage evidence is `implementation-preflight.json` there.
