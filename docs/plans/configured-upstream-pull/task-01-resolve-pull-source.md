---
id: "01-resolve-pull-source"
title: "Resolve configured upstream Pull source"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CONFIGURED-PULL-TARGET-001
acceptance_criteria:
  - AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.1
  - AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.2
  - AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.3
  - AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.4
  - AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.5
  - AC-WORKSPACES-CONFIGURED-PULL-TARGET-001.6
system_design:
  - ../../specs/workspaces/system-design/configured-pull-target.md
---

# Task 01: Resolve Configured Upstream Pull Source

## Summary

Resolve the ordinary Pull source from Git's separate upstream remote and source
ref fields. Independently prove the two causal regressions before implementation,
preserve compatibility controls and correct the public routing explanation.

## In scope

- Read the four reviewed artifacts and scoped guidance. Mark this order
  `in_progress` only after FULL ROOT actual review and a later explicit
  implementation interrupt to this same primary.
- Write real-Git regressions and observe meaningful RED before production edits.
  Follow the plan's named test matrix and acceptance mapping.
- Add a small Pull-specific helper using `runGitCommand` and safe operands;
  integrate it only in ordinary upstream-present Pull selection.
- Update the three identified public Git guide passages alongside the fix.
- Run exact scoped checks; record every actual result and native join. Promote
  the requirement/design to active/current and this order/plan to done/implemented
  only after all mapped behavior and checks pass.

## Out of scope

The manifest exclusions apply. No push mutation, new source selector, changed
upstream-presence policy, generic runner/security refactor, frontend helper,
browser test, UI copy, additional order, task, session, worktree or delegate.
No staging, commit, push or hosted checks during the design turn.

## Acceptance

1. The independent real-operator fixtures fail on the reviewed base for the
   wrong remote/false-success and missing local branch-name causes, then pass
   for both existing modes using the configured source. Tracker upstream,
   behind=1, result, HEAD and native controls support the conclusion. For `.1`,
   a clean working tree with no local-ahead commits reaches exact source HEAD;
   for `.2` with replayed local work, assert source ancestry and replayed contents
   rather than HEAD equality to the source.
2. All fallback, managed precedence, destination isolation and safe-operand
   controls in the manifest pass without a changed public request/result shape,
   runner policy or Git configuration mutation.
3. The public guide matches implemented routing. Targeted Go and document checks
   pass, all four artifact statuses/results are synchronized, and the Changes
   desktop/phone-to-operator trace remains intact.

## Verification

Run commands from the repository root. Select the existing toolchains explicitly
for every later implementation command: Go1.26.0 from mise, Node24 from nvm,
`GOTOOLCHAIN=local`, and Corepack pnpm@9.15.9. The shell's Go1.22 and absent bare
pnpm must not trigger a toolchain download or an assumed package-manager install.
Implementation only: obtain the fresh ROOT resource lease first. Install frozen
workspace dependencies once, even for Go-only commit hooks:

```bash
(cd apps && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local corepack pnpm@9.15.9 install --frozen-lockfile)
```

Author permanent tests from public patterns only. RED before production edits:

```bash
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s go test -race -trimpath -tags=fts5 -p=1 ./internal/agentctl/server/process -run '^TestGitOperatorPullConfiguredUpstream$' -count=1 -timeout=240s)
```

Inspect failures: the non-origin case must actually stay at the old HEAD after
reported success, and the different-name case must fail against the absent
local-name remote ref. Native configured controls must reach the intended source.
RED for any other cause is not evidence. Do not access ROOT private proofs.

GREEN and scoped compatibility (one bounded native command, both packages):

```bash
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 280s go test -race -trimpath -tags=fts5 -p=1 ./internal/agentctl/server/process ./internal/agentctl/server/api -run '^(TestGitOperatorPull(ConfiguredUpstream|Fallbacks|UpstreamOperands|ManagedSourcePrecedence|ContributionDestinationKeepsUpstream)|TestGitOperatorRemoteContributionPullsFromSourceRemote|TestGitOperatorContributionDestinationPushesWithoutChangingPullRemote|TestHandleGitPull_Rebase(ReplaysLocalCommitOntoOrigin|ConflictAutoAborts))$' -count=1 -timeout=240s)
```

Before FIRST publish, run scoped Go lint against the reviewed base, with ordinary
required hooks active:

```bash
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 330s golangci-lint run ./internal/agentctl/server/process/... --new-from-rev=371d9c18193af5893fbe70387fee9330f7dbb65b --timeout=5m)
```

If a later actual Go fixup is needed, the backend fixup rule requires the full
package lint command before its push, still reporting only new issues against
the base:

```bash
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 330s golangci-lint run ./... --new-from-rev=371d9c18193af5893fbe70387fee9330f7dbb65b --timeout=5m)
```

Spec gates and public-document gates, with no package install needed for these:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node --test scripts/validate-public-docs.test.mjs
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node scripts/validate-public-docs.mjs
```

Use the repository's documentation-coverage validator with the COMPLETE actual
base diff, staged diff and untracked inventory. Report every actual path and
unexpected path; do not filter the inventory to obtain coverage. Load the linked
four documents and any actual changed Markdown contents. During design the
actual four added documents are expected `exempt`; the separately labelled
anticipated production preflight includes ALL eight planned owned paths and is
expected `covered`. After implementation, the actual inventory must itself be
`covered`; anticipated input is not evidence of implementation coverage.
This is local validation, not a GitHub mutation or hosted status:

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = '371d9c18193af5893fbe70387fee9330f7dbb65b';
const documents = [
  'docs/specs/workspaces/requirements/configured-pull-target.md',
  'docs/specs/workspaces/system-design/configured-pull-target.md',
  'docs/plans/configured-upstream-pull/plan.md',
  'docs/plans/configured-upstream-pull/task-01-resolve-pull-source.md',
];
const planned = [
  ...documents.map(filename => ({ filename, status: 'added' })),
  { filename: 'apps/backend/internal/agentctl/server/process/git.go', status: 'modified' },
  { filename: 'apps/backend/internal/agentctl/server/process/git_pull_target.go', status: 'added' },
  { filename: 'apps/backend/internal/agentctl/server/process/git_pull_target_test.go', status: 'added' },
  { filename: 'docs/public/git-operations.md', status: 'modified' },
];
const git = args => execFileSync('git', args, { encoding: 'utf8' });
const inventory = new Map();
for (const args of [
  ['diff', '--cached', '--name-status', '-z', base],
  ['diff', '--name-status', '-z', base],
]) {
  const fields = git(args).split('\0');
  for (let i = 0; i < fields.length && fields[i];) {
    const code = fields[i++];
    const original = fields[i++];
    const renamed = /^[RC]/.test(code);
    const filename = renamed ? fields[i++] : original;
    const status = code.startsWith('A') ? 'added' : code.startsWith('D') ? 'removed' : renamed ? 'renamed' : 'modified';
    inventory.set(filename, { filename, status, ...(renamed ? { previous_filename: original } : {}) });
  }
}
for (const filename of git(['ls-files', '--others', '--exclude-standard', '-z']).split('\0').filter(Boolean)) {
  inventory.set(filename, { filename, status: 'added' });
}
const changedFiles = [...inventory.values()];
const owned = new Set(planned.map(change => change.filename));
const unexpected = changedFiles.filter(change => !owned.has(change.filename));
console.log(JSON.stringify({ label: 'actual complete path inventory', changedFiles, unexpected }));
const contentPaths = new Set([...documents, ...changedFiles.map(change => change.filename).filter(p => p.endsWith('.md'))]);
const fileContents = Object.fromEntries([...contentPaths].filter(p => fs.existsSync(p)).map(p => [p, fs.readFileSync(p, 'utf8')]));
const actual = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ label: 'actual coverage', ok: actual.ok, status: actual.status, errors: actual.errors }));
if (!actual.ok || unexpected.length) process.exitCode = 1;
if (actual.status === 'exempt') {
  const anticipated = validateCoverage({ changedFiles: planned, fileContents });
  console.log(JSON.stringify({ label: 'anticipated production preflight ONLY', changedFiles: planned, ok: anticipated.ok, status: anticipated.status, errors: anticipated.errors }));
  if (!anticipated.ok || anticipated.status !== 'covered') process.exitCode = 1;
}
JS
```

After implementation, formatting output must be empty and diff/status paths must
match ownership:

```bash
/home/jcfs/.local/share/mise/installs/go/1.26.0/bin/gofmt -l apps/backend/internal/agentctl/server/process/git.go apps/backend/internal/agentctl/server/process/git_pull_target.go apps/backend/internal/agentctl/server/process/git_pull_target_test.go
git diff --check
git status --short
git diff --check -- docs/plans/configured-upstream-pull
git status --short -- docs/plans/configured-upstream-pull
```

Design-only validation may run the two Python spec gates, the coverage heredoc,
and diff/status checks now. Run public-doc checks after the actual guide edit;
Go formatting/tests/lint and pnpm installation remain deferred. No broad audit gate
is added. Reconcile any actual required CI or review correction later with its
scoped checks under the parent publication protocol.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_pull_target.go` (new helper)
- `apps/backend/internal/agentctl/server/process/git_pull_target_test.go` (new tests)
- `docs/public/git-operations.md` (three routing passages)
- `docs/specs/workspaces/requirements/configured-pull-target.md` (lifecycle status)
- `docs/specs/workspaces/system-design/configured-pull-target.md` (lifecycle status)
- `docs/plans/configured-upstream-pull/plan.md` (status/results)
- `docs/plans/configured-upstream-pull/task-01-resolve-pull-source.md` (status/results)

## Dependencies

No preceding work orders. Base:
`371d9c18193af5893fbe70387fee9330f7dbb65b`. Recheck owner/dependency/consumer
compatibility against current main at implementation and publication boundaries;
do not rebase merely for unrelated drift. ROOT review and later explicit
implementation authorization are required checkpoints, not callback gates.

## Risks

Use full source refs and separate atoms; never guess from display upstream.
Assert selected operands remain within existing security validation. A material
policy ambiguity or need to expand scope checkpoints ROOT before expansion.
Preserve external-writer semantics and no-upstream detection rather than adding
configuration concurrency hardening.

## Parallelism

`sequential`

## Inputs

- The paired Workspaces requirement/design and manifest test matrix.
- `git.go` Pull, runner validation, upstream and default-branch helpers.
- `workspace_tracker_test.go` public real-repository helpers;
  `git_test.go` real managed source/destination patterns;
  `git_pull_rebase_test.go` real HTTP/rebase/conflict controls. Use
  `localGitRemotePath` for disposable remote paths on supported hosts.
- The unchanged source path in `changes-panel-hooks.ts`,
  `use-session-git-derived.ts`, `use-session-git.ts`, `use-git-operations.ts`,
  `git_handlers.go`, agentctl client `git.go`, and API `git.go`.
- Scoped backend/API guidance and `/tdd` including `backend-tests.md` before
  permanent tests. Mobile-parity state/data exception and docs-maintainer audit.

## Results

Done. ROOT's actual four-artifact review and bounded correction review were
followed by a later explicit implementation release to this same primary.

- Frozen install ran once through existing Node24/Corepack pnpm@9.15.9 and local
  Go1.26.0 selection; exit 0. Normal hooks remain active.
- Independent real-Git RED joined with exit 1 before production edits. The final
  RED distinguishes non-origin false success/old HEAD from the missing local-name
  remote ref, both modes; native configured Git controls pass.
- Minimal Git upstream-atom resolution and six-line ordinary Pull integration
  implement all six acceptance criteria. Managed source precedence and push-only
  destination isolation are independently exercised. Main/master/current-name
  fallbacks, slash remote/custom fetch storage, local dot, primary merge value,
  unsafe remote/ref refusal and configuration preservation pass.
- Final exact scoped race command: exit 0, process 6.764s / API 1.421s (native
  23373 actually joined `bc56b5`), including nine selected top-level tests and
  22 new parameterized subtests. Equality and replay assertions follow `.1`/`.2`.
- Scoped base371 Go lint: initial test-only nesting finding corrected; final
  exit 0, 0 issues (native 30890 actually joined `a1ad8a`). New Go paths were staged.
- Public docs: only three routing passages corrected; 63 validator tests pass
  and 47 published pages validate. Catalog/spec gates pass; complete actual
  eight-path inventory is `covered`, errors empty, unexpected paths empty.
- Formatting and diff checks pass. Paired requirement/design are active/current;
  manifest is implemented. Final lifecycle/diff/coverage receipts, normal hook
  receipt, commit/push/lease return and hosted gates stay in the external task
  plan; no optional tracked closing-note update after FIRST publish.

Exact commands are retained in Verification; original handles, joins and all
local command results are recorded in the manifest. No delegates, foreign
resource changes, ROOT-private fixture use, or UI/browser/i18n expansion.
