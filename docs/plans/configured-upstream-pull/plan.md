---
created: 2026-10-13
status: implemented
requirements:
  - REQ-WORKSPACES-CONFIGURED-PULL-TARGET-001
system_design:
  - ../../specs/workspaces/system-design/configured-pull-target.md
legacy_specs: []
---

# Implementation Plan: Configured Upstream Pull

## Overview

Correct ordinary Pull source selection in one sequential work order. Git's
upstream remote/source-ref fields replace the assumed `origin`/local-branch
target when the checkout reports an upstream. Preserve managed contribution
source selection and the existing no-upstream fallback.

ROOT completed the actual four-artifact review and bounded correction review,
then explicitly released implementation in this same primary session. The sole
work order is implemented and all local checks pass. Hosted review and any
merge authorization remain separate publication gates.

## Confirmed root cause and evidence

At base `371d9c18193af5893fbe70387fee9330f7dbb65b`, `GitOperator.Pull`
initializes `remote = origin` and `pullBranch = current local branch`.
The ordinary upstream-present path never replaces either value. Meanwhile,
`WorkspaceTracker` and shared session derivation publish and enable Pull against
the actual configured upstream.

Supplied ROOT real-Git evidence, not rerun in this design turn:

- `task` tracked `upstream/task`, which was ahead by one commit, while
  `origin/task` stayed at baseline. The real operator returned success and
  `Already up to date`, leaving HEAD old. Native configured `git pull --ff-only`
  reached the expected upstream commit.
- `task-local` tracked `origin/review`, ahead by one commit. The real operator
  failed because remote `task-local` did not exist; native configured Pull worked.
- Three controls passed: same-name `origin`, no-upstream main fallback, and
  existing managed source routing.

ROOT native 89022 actually joined with exit 1 (`d84bcd`); process package 1.229s,
wall 18.25s. Recheck 62514 actually joined with exit 0 (`a1141c`): this HEAD
matched ROOT main, seven relevant blobs matched and related dependency diff was
empty. ROOT private proof fixtures are excluded from access and reuse.

## Scope

### In scope

- The [Workspaces requirement](../../specs/workspaces/requirements/configured-pull-target.md)
  and [paired design](../../specs/workspaces/system-design/configured-pull-target.md).
- Ordinary configured-source resolution in both existing Pull request modes.
- Independent real-Git regressions, fallback and managed-source controls.
- Three bounded routing corrections in the existing public Git guide.

### Out of scope

- Push/destination behavior, upstream/config writes, new target inputs, API/WS
  shapes, UI changes or localization additions.
- Multi-source integration, new absent-upstream policy, retries, status refresh,
  cache/journal/lifecycle work, locks, deadlines, credential or transport changes.
- New tasks, sessions, worktrees, model changes or native delegation.

## Technical approach

Only `apps/backend/internal/agentctl/server/process/git.go` changes existing
production behavior. Add `git_pull_target.go` for a small Pull-specific
`for-each-ref` read and validation through `runGitCommand`. Keep the existing
upstream-presence helper and fallback. Managed binding validation stays first.
Pass the resolved configured name and full source ref through the current Pull
command builder; keep both mode flags, conflicts, errors and result unchanged.

Add `git_pull_target_test.go` with real operators and disposable remotes. Reuse
public `setupTestRepo`, `runGit`, `writeFile`, `newTestLogger` patterns; ensure
test configuration/environment isolates user Git configuration, signing and
hooks in fixtures without changing production environment. Do not read or
copy ROOT's private scratch fixture. No operation or API mocks serve as primary
causal evidence.

Update `docs/public/git-operations.md` only at prerequisite routing wording,
the Pull operation row and wrong-branch troubleshooting. No owner README,
legacy migration, ADR, new page, diagram, API or screenshot change is needed.

### Compatibility matrix

| Checkout/source shape | Intended selection | Evidence |
| --- | --- | --- |
| Ordinary named non-origin remote | Git-reported remote/source ref | Non-origin false-success regression, both modes |
| Differently named upstream branch | Full source ref, independent of local name | Missing-local-name regression, both modes |
| Slash remote/custom fetch storage | Exact configured name/full source ref | Real mapping edge case |
| Local `.` upstream | Local source ref | Real local-source/native Git control |
| Multiple merge config values | Single primary upstream Git reports | Bounded primary-upstream control; no octopus claim |
| Unsafe/unresolvable reported source | Existing failure result; no fallback Pull | Real invalid-operand fixtures with HEAD/config unchanged |
| Same-name origin / no reported upstream | Existing configured source / origin fallback | Same-name, main, master, current-name controls |
| Managed RemoteContribution | Existing explicit source remote/HeadBranch | Existing real test plus conflicting-upstream precedence |
| ContributionDestination / pushRemote | Ordinary Pull upstream | Real destination-isolation control |

All executors reach this shared operator; local disposable transports prove
selection. This package does not claim executor-specific authentication coverage.

## Tests

All new tests are in
`apps/backend/internal/agentctl/server/process/git_pull_target_test.go`.

| Criteria | Planned executable evidence |
| --- | --- |
| `.1`, `.2`, `.6` | `TestGitOperatorPullConfiguredUpstream`: non-origin/same-name, origin/different-name and non-origin/different-name; each with `rebase=false/true`; actual tracker upstream and behind=1, old HEAD, result and final HEAD, independent native configured Git control. Under `.1`, use a clean working tree with no local-ahead commits and assert HEAD equals the source commit. Under `.2`, add a non-conflicting local commit in rebase evidence and assert source ancestry/replayed contents, not HEAD equality to the source. |
| `.3` | `TestGitOperatorPullManagedSourcePrecedence`: real managed source overrides a different ordinary upstream in both modes; existing `TestGitOperatorRemoteContributionPullsFromSourceRemote`. |
| `.3` | `TestGitOperatorPullContributionDestinationKeepsUpstream`: push-only binding and pushRemote cannot displace upstream; config/remotes stay unchanged. Existing destination push test remains a push control, not Pull evidence. |
| `.4` | `TestGitOperatorPullFallbacks`: same-name origin, no-upstream main, master-only, and no main/master but current-name remote branch, both modes where meaningful. |
| `.5` | `TestGitOperatorPullUpstreamOperands`: slash remote plus remapped tracking storage, local dot source, primary upstream with multiple merge values, and unsafe reported remote/ref refusals. Single-source shapes use native configured Pull controls. The multiple-merge-value case compares Git's primary upstream observation, not native octopus integration. Failure cases do not contact an alternate origin or change HEAD/config. |

Keep old process tests intact. Existing API tests
`TestHandleGitPull_RebaseReplaysLocalCommitOntoOrigin` and
`TestHandleGitPull_RebaseConflictAutoAborts` in `git_pull_rebase_test.go`
provide real HTTP-to-operator wiring and unchanged conflict recovery evidence.

## E2E tests

Criteria `.1` and `.6` receive user-operation evidence at the real GitOperator
boundary with actual tracker upstream/behind state. Trace Changes eligibility
and callback through `worktree.pull`, `wsPull`, `Client.GitPull`, the HTTP handler
and operator. Existing HTTP rebase tests validate the unchanged request path.

The desktop/phone surfaces share the data and operation. This package changes no
presentation, touch interaction, navigation or viewport state. Under
mobile-parity's state/data exception, no new Playwright test or frontend/browser
build gate is required. Reopen the design checkpoint if implementation requires
frontend or rendered behavior changes.

## Work orders

- [x] [Task 01: Resolve configured upstream Pull source](task-01-resolve-pull-source.md)

One completed order, wave 1, no task dependency, sequential in this primary session.

## Verification results

Local implementation complete before first publication; public request/result,
managed routing, fallback, runner policy and desktop/phone presentation unchanged.
All commands use the work order's exact working directories and pinned runtimes.

| Command/check | Actual result |
| --- | --- |
| Frozen Corepack pnpm@9.15.9 install from `apps` | Exit 0, once, cached packages; native 78830 joined `6aafe5` (install launch `79d22f`) |
| Initial independently authored real-Git RED | Exit 1, process 2.123s; native 91033 joined `f6bfc3`. Refined the origin/different-name fixture to omit its local-name decoy so the two defect classes remain distinct. |
| Final causal RED, exact named operator test | Exit 1, process 1.911s; native 60675 joined `5e087d`. Both modes exhibit non-origin false success/old HEAD and differently named upstream missing-ref failure; native configured controls pass. Replay lacks source ancestry before the fix. |
| First exact scoped race GREEN, process + API | Exit 0, process 6.383s / API 1.429s; native 78807 joined `c114c8`. |
| First scoped base lint after staging all owned paths | Exit 1, native 78198 joined `e4dbd8`; only a nested-block complexity finding in the operand test. Extracted the refusal assertion and flattened the branch; no production change. |
| Final exact scoped race GREEN after test restructuring | Exit 0, process 6.764s / API 1.421s; native 23373 joined `bc56b5`. Nine selected top-level tests, including existing managed/destination and real HTTP rebase/conflict controls; 22 new parameterized subtests plus destination-isolation evidence. |
| Final scoped `golangci-lint` against base `371d9c18193af5893fbe70387fee9330f7dbb65b` | Exit 0, 0 issues; native 30890 joined `a1ad8a`. New helper/test staged before changed-line lint. |
| `list-docs.py validate` / `lint-spec-files.py --all` | Exit 0; 377 decisions / 1547 specifications, all specs pass. Lifecycle promotion receives a final check before commit. |
| Public docs test / validator | Exit 0; 63 tests pass, 47 published pages validated (`c39e6d` / `8afc41`). |
| Complete actual documentation-coverage heredoc | Exit 0, actual `covered`, errors empty; exactly all eight owned paths, no unexpected paths (`cce006`). No anticipated input is counted as production evidence. |
| `gofmt -l`, working and staged diff checks | Exit 0, no formatting or whitespace findings. |

Requirement/design lifecycle is active/current; sole work order is done. The
three affected public-guide passages now explain the configured source, managed
precedence and unchanged no-upstream fallback. No frontend, i18n, browser/build,
API schema, database, dependency or ownership migration changes were needed.

Normal hooks, commit/push receipts, current-main compatibility, lease return,
hosted CI/review and any later expected-head merge grant are tracked in the
external task plan. These notes are complete before FIRST publish; no optional
post-publication documentation/style push is planned. Every local command is
joined before lease return; only self-owned implementation logs are eligible
for cleanup. ROOT's private source/proofs and all foreign resources are excluded.

## Risks

- Abbreviated tracking names can be ambiguous when remote names include slashes;
  separate Git atoms and custom-fetch fixtures prevent display-name guessing.
- Configuration can change outside Kandev's operation lock. Preserve existing
  external-writer guarantees; do not add transactions, retries or expected-head
  inputs.
- Existing upstream-presence detection can return empty on a failed read or an
  unmaterialized upstream. Its policy is excluded from this source-only fix.
- Native multi-source Pull can make octopus merges. Kandev's existing singular
  source/result surface remains bounded to Git's primary reported upstream.
- A fresh implementation turn requires ROOT's resource lease and the existing
  Go1.26.0 toolchain. No downloads, installs or heavy commands occur during design.
