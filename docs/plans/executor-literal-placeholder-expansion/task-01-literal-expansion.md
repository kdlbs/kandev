---
id: "01-literal-expansion"
title: "Restore literal template expansion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-001
acceptance_criteria:
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.2
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.4
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.5
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
---

# Task 01: Restore literal template expansion

## Summary

Prove the resolver can rewrite inserted literal branch data into the wrong Git
command, then replace its evolving-output substitution loop with one expansion
of the original template. Keep provider values, quoting, and collision
precedence unchanged.

## In scope

- Add independent resolver and native Git regression tests in the owning package.
- Change only production `Resolver.Resolve` to one literal-pattern replacement
  operation over the original input. Retain existing empty-input/map fast paths.
- Keep the owning design and this package's statuses/results accurate.

## Out of scope

Provider/branch-normalizer changes, new interpolation syntax, recursive/fixed-point
expansion, lifecycle/caller validation changes, persistence, migrations, API,
UI, live executors, toolchain/dependency upgrades, frontend validation, broad
backend suites, and companion fixes. The conditional frozen install for normal
commit-hook prerequisites below is included; it does not change tracked
dependency or lock files.

## Acceptance

1. A new test on the unchanged resolver records independent causal RED: actual
   repository/workspace providers produce a mutated rendered command, native
   Git execution fails to materialize the selected branch, and an independent
   literal-command control succeeds with exact selected sentinel bytes.
2. Every post-fix render of the original template matches the independent
   expected command; native current fetch-style and saved clone-style templates
   select the exact branch/HEAD/sentinel, while an unavailable literal branch
   fails without default-branch fallback.
3. Compatibility controls, the complete owning race package, affected-package
   lint, and light documentation checks pass with recorded results.

## TDD sequence and test design

Author `resolver_test.go` and `resolver_git_test.go` without importing lifecycle
or copying any external reproduction source. Use existing test helpers only
where they fit; keep the new Git fixture local to this package.

1. `TestResolverOriginalTemplateOnly`: register actual `RepositoryProvider`
   metadata with plain branch `topic/{{workspace.path}}` and actual
   `WorkspaceProvider` with a temporary checkout path. Include both original
   branch and workspace tokens. Construct expected command text independently
   from the known literal branch and fixture path; do not derive the expectation
   by calling the resolver or iterating its merged map. Check all 128 renders,
   collecting mismatch count and first mismatching command for diagnostics.
   Use the same bounded sampling in the native checkout regression. Never
   retry a test invocation to obtain the desired RED.
2. `TestResolverLiteralBranchCheckout`: require native Git and a POSIX shell for
   the recorded proof. Create a temporary bare origin and seed repository with
   different commits and exact sentinel file bytes on `main` and
   `topic/{{workspace.path}}`. Isolate Git system/global config and credentials;
   use only local fixture paths and context-bounded subprocesses. Render two
   independent templates: current-style init/add-origin/fetch exact refspec/
   checkout, and saved-style `git clone --branch {{repository.branch}}`.
   Both must contain original workspace and repository tokens.
   For each form render 128 times; execute the first mismatching command once,
   or the first correct render when none mismatch. Assert successful execution,
   exact branch, selected HEAD, and byte-exact sentinel. On the old resolver,
   a mutation must expose the actual missing/wrong-branch Git error before
   reporting the render assertion. Execute an independently literal-constructed
   command in a separate checkout as a fixture control first; it must succeed
   and select the same sentinel. Do not execute every sample or construct the
   control by resolving the template.
3. `TestResolverLiteralBranchMissing`: use an absent branch that contains a
   known-token spelling, for both script forms. Assert nonzero Git exit and no
   fallback `HEAD` at `main` or its sentinel. A correct literal miss remains a
   failure after the fix.
4. `TestResolverCompatibility`: table controls cover ordinary plain branches,
   repeated registered tokens, literal unknown tokens, empty script, no vars,
   empty replacement, provider-over-static precedence regardless of registration
   position, later provider over earlier provider, and later static registration.
   Check provider call count/order, including no provider call for empty input.
5. `TestResolverScriptFragmentsVerbatim`: compare inserted bytes with real
   repository setup, agent-install, and generated controller/auth/identity
   provider outputs. Include a repository/agent fragment containing known and
   unknown tokens, newlines, quotes, and `${...}` shell syntax. Compare with
   provider-returned text so install-provider trimming/deduplication stays owned
   by that provider. Verify ordinary generated fragments still insert unchanged.
6. Run the focused RED command once before production edits. A skipped fixture,
   unrelated failure, or no observed causal mutation does not establish RED;
   record the limitation and leave the correction pending. After qualified RED,
   use one `strings.NewReplacer(...).Replace(script)` operation built from the
   existing merged map. Run the complete owning package GREEN and lint.

## Verification

Run from repository root. The first command is the pre-change RED selection;
the second is the post-change complete owning-package race check. The third
lints the complete affected package through the repository helper.

```bash
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -p=1 -tags fts5 -race -count=1 -timeout=2m ./internal/scriptengine -run '^(TestResolverOriginalTemplateOnly|TestResolverLiteralBranchCheckout|TestResolverLiteralBranchMissing|TestResolverCompatibility|TestResolverScriptFragmentsVerbatim)$' -v)
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -p=1 -tags fts5 -race -count=1 -timeout=2m ./internal/scriptengine)
timeout --kill-after=10s 360s env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local bash scripts/lint-go-changed apps/backend/internal/scriptengine/resolver.go apps/backend/internal/scriptengine/resolver_test.go apps/backend/internal/scriptengine/resolver_git_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/executor-literal-placeholder-expansion
```

Use GNU `timeout`: each owning Go command has a 280-second outer limit and a
10-second kill grace, covering compilation as well as its two-minute internal
test timeout. The canonical affected-package lint helper has a 360-second outer
limit and the same kill grace. Keep its current resource defaults: 3 GiB memory
target, two workers, and five-minute internal lint timeout. Do not apply the Go
test's lower memory target to lint. Do not retry with changed resources or fall
back to full-backend lint; CI owns that scan. No toolchain/dependency upgrades
or cache changes are part of verification.

Only if new concrete implementation evidence creates a caller-specific
uncertainty, use this exact selection and record the trigger and result.
It is conditional evidence, not a mandatory gate:

```bash
(cd apps/backend && timeout --kill-after=10s 280s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -p=1 -tags fts5 -race -count=1 -timeout=2m ./internal/agent/runtime/lifecycle -run '^(TestSpritesPrepareScript_OriginBranch|TestSpritesPrepareScript_MissingBranch|TestBranchNameCommandInjection_Regression)$')
```

Also run the repository PR-documentation `validateCoverage` preflight for this
work order, plan, owning design, unchanged owning requirement, and actual
changed paths. Record full returned errors rather than inferring coverage
from specification lint. No validator unit tests are required for prose edits.

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/executor-literal-placeholder-expansion/plan.md',
  'docs/plans/executor-literal-placeholder-expansion/task-01-literal-expansion.md',
  'docs/specs/executors/system-design/repository-branch-resolution.md',
  'docs/specs/executors/requirements/repository-branch-resolution.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD'], { encoding: 'utf8' }).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
])].filter(Boolean).map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
process.stdout.write(JSON.stringify(result, null, 2) + '\n');
if (!result.ok) process.exitCode = 1;
NODE
```

During design, separately preflight the six prospective owned paths from the
file list below against the same four document contents. Label this as
prospective runtime-path coverage, not as production changes or test evidence.

## Normal commit-hook prerequisites

After implementation and verification, before a normal commit, inspect this
worktree's workspace dependencies. If commitlint or Prettier is missing, run
one frozen install from `apps/` using existing Node 24 and Corepack. The closest
`apps/package.json` selects `pnpm@9.15.9`; do not use an absent old mise pnpm
path, upgrade tooling, change tracked dependency/lock files, or retry the install.
Use the existing Node 24.18/Corepack/pnpm shims on PATH for the normal hooks.

```bash
(export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"; cd apps && if [ ! -x node_modules/.bin/commitlint ] || [ ! -x node_modules/.bin/prettier ]; then timeout --kill-after=10s 600s corepack pnpm install --frozen-lockfile; fi)
git diff --exit-code -- apps/package.json apps/pnpm-lock.yaml
```

The active Git pre-commit and commit-msg hooks use `.pre-commit-config.yaml`.
Commitlint is `always_run` at commit-msg, and the public-copy Node guard is
`always_run` at pre-commit, so a Go/docs-only commit still needs Node and
commitlint. Go formatting and changed-package lint apply to the Go files;
catalog/specification hooks apply to the owning design. Harness/architecture
hooks retain their normal scope. Prettier and frontend lint/i18n/E2E hooks are
file-scoped and do not match this package's six owned Go/docs paths.
Do not skip normal hooks. The install is solely a hook prerequisite and does
not add frontend tests, typechecking, builds, browser work, or screenshots.
Use a 600-second GNU outer bound with a 10-second kill grace for the normal
hook-bearing commit and a 300-second bound with the same grace for the ordinary
push. Join the original commit command before pushing.

## Files likely touched

- `apps/backend/internal/scriptengine/resolver.go`
- `apps/backend/internal/scriptengine/resolver_test.go` (new)
- `apps/backend/internal/scriptengine/resolver_git_test.go` (new)
- `docs/specs/executors/system-design/repository-branch-resolution.md`
- `docs/plans/executor-literal-placeholder-expansion/plan.md`
- `docs/plans/executor-literal-placeholder-expansion/task-01-literal-expansion.md`

## Dependencies and inputs

None. Read the active owning requirement, design's Template expansion and
Validation sections, `resolver.go`, `providers.go`, `providers_branch_test.go`,
and `providers_test.go`. Current template reference:
`defaultSpritesPrepareScript` in `default_scripts.go`; saved clone-style
reference: `branchPrepareFixture.resolve` in `repository_branch_prepare_test.go`.
The historical `remote-clone-branch` and `remote-pr-review-checkout` packages are
implemented context, not tasks to reopen.

## Risks

Map-order sampling is bounded rather than deterministic enumeration. Failure to
observe a causal RED blocks implementation evidence and must be reported.
SSH's existing unresolved-sequence rejection remains outside scope; do not
claim successful live executor behavior from this native fixture.

## Parallelism

`sequential`

## Results

Completed on 2026-10-14 with the verification commands above:

- Focused RED selection: one invocation on the unchanged production resolver,
  exit 1, package runtime 0.300 seconds. No test was skipped. Current-fetch
  rendered 83/128 mutated commands; the selected command failed with native
  Git exit 128 and an invalid refspec containing the workspace path.
  Saved-clone rendered 84/128 mutations; its selected command failed with
  native Git exit 128 because the rewritten branch was absent from origin.
- Both independent literal-command controls exited 0, selected exact branch
  `topic/{{workspace.path}}`, verified HEAD
  `119f647df715d96b5d499ac5941ca9f7d3f85905`, and matched the selected binary
  sentinel bytes, which differ from the default branch's sentinel.
- The original-template unit assertion observed 92/128 mutations. Repository
  setup and agent-install fragment assertions observed 113/128 and 114/128
  mutations. Ordinary branch, repeated/unknown tokens, empty input/map/value,
  static/provider precedence and evaluation controls all passed. Generated
  controller/identity/authentication fragment controls passed.
- Complete `internal/scriptengine` race package: exit 0, 1.403 seconds.
  Every bounded render assertion now has zero mismatches; current/saved native
  checkouts satisfy branch/HEAD/sentinel assertions. Missing literal branches
  fail without a fallback HEAD or sentinel. Existing provider tests remain green.
- Canonical `bash scripts/lint-go-changed` with all three changed Go files:
  exit 0, `0 issues.` The complete affected package was analyzed using current
  helper defaults and the documented GNU outer bound.
- `python3 scripts/list-docs.py validate`: exit 0.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- `git diff --check` and package status inspection: passed.
- Repository `validateCoverage` using actual six changed paths and the four
  referenced documents: `covered`, `ok: true`, `errors: []`.

The requirement remains active and unchanged; the owning design remains current.
The manifest is completed. No conditional caller check, frontend validation,
live executor operation, or public documentation edit was needed. Normal hook
dependency installation and commit receipts are operational delivery evidence
kept outside tracked artifacts.
