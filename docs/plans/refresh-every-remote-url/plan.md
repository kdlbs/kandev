---
created: 2026-10-03
status: implemented
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
legacy_specs: []
---

# Implementation Plan: Refresh every remote URL after provider changes

## Overview

Restore branch and structured inspection resolution for every cached remote
URL after provider availability changes. One sequential work order owns both
proven hook defects and the native consumer regression. It was implemented
after the explicit parent release in the same primary session.

## Ownership and confirmed evidence

Plugins owns provider availability and structured repository inspection used
by native task creation. The existing owning pair is extended, rather than
creating a repair or UI specification. Adjacent Tasks multi-branch contracts
own input/retry UX; Workspaces remote resolution owns registration and clone
materialization. Neither boundary changes.

Both `useBranchesByURL` and `usePRInfoByURL` compare one hook-wide registry
version, but delete only the URL passed to the first `ensure`. That first call
consumes the version transition while other cached URLs remain loaded.
`RemoteRepoChipsRow` ensures every row through both callbacks, so a late plugin
boot recovers the first pasted repository while the second remains unresolved.

Parent proof accepted without replay, using production hooks and real registry
with only API/provider transports mocked:

- Branch handle 51885 joined, exit 1: one URL passed; two URLs failed because
  beta expected `beta-main` but received `[]`. Test 1077 ms, package 3.57 s.
- Inspection handle 6170 joined, exit 1: one URL passed; two URLs failed because
  beta expected repository name `beta` but received `undefined`. Test 1076 ms,
  package 3.46 s.
- Read-only archives: `/tmp/kandev-branch-provider-registry-repro.test.tsx`
  (SHA256 `a5cc4d408132a0e3c9e56448b4a776f8028a6eddf00b1e9e443c78e7038375f2`)
  and `/tmp/kandev-provider-inspection-registry-repro.test.tsx`
  (SHA256 `dfe175c78383cbb4173ca30f3d8a1fae9a31ba6bcfa2da8d548a9c5b91b7a54d`).
  Parent owns their cleanup after merge.
- Proof base `0ec8e2c6db36c1b75a075637549f183ca87beb2b`; design checkout
  `546bef5183c4086c089b362c36c3a0287747bf59` has identical owned hook/chips
  blobs. No proof replay, production/test edits, dependency install, or heavy
  verification during design.

## Scope

### In scope

- Hook-local invalidation of every cached URL on the existing registry signal.
- Pending request fencing using existing sequences and abort controllers.
- Real-hook multi-URL regressions and rendered native chips outcome evidence.
- Same-version sharing, explicit retry, trimming, workspace isolation, and
  existing provider routing compatibility.

### Out of scope

- Registry, SDK, backend, transport schema, cache result shape, or lifecycle
  framework changes; provider identity architecture or passive-effect repair.
- Global tab/workspace freshness, provider-specific version signals, committed
  row selection rewriting, output policy, ToastProvider, or poller changes.
- Layout, copy, touch, scroll, navigation, breakpoint changes, browser/build/E2E,
  broad suites, optional polish, or additional work orders/delegation.

## Technical approach

Apply whole-instance invalidation before each hook's per-URL deduplication gate
on the first valid `ensure` for a new registry version. Advance the existing
sequence values, abort pending controllers, clear in-flight/loaded/visible
state, then record the version and proceed through existing routing. Preserve
workspace epoch checks and explicit clear. Later ensures for other URLs become
eligible under the same version. No eager registry effect or shared abstraction
is required.

Direct consumers audited: `task-create-dialog-state.ts` and
`task/new-subtask-form-state.ts` each own both multi-URL hooks. The native
`RemoteRepoChipsRow` has the all-row effect and structured inspection hydration.
`RemoteRepositoryRow` in the attach-sources dialog owns one hook instance per
row and ensures branches; its metadata behavior is outside this correction.
No consumer production change is planned.

| Provider / transport | Intended result and fallback | Evidence |
| --- | --- | --- |
| Plugin `inspectURL` / `listBranches`, real registry | Every affected URL uses current structured identity and branch transport; null inspection preserves built-in/unsupported fallback | Both new real-hook registry suites; native rendered chips test |
| GitHub API | Current branch and PR/issue results; existing workspace credential scope | Existing hook suites plus registry-removal fallback regression |
| GitLab / Azure branch API | Existing parsing, scope, and routing | Existing branch suite |
| Unsupported URL | Empty branch/no metadata behavior; eligible again after registration | Multi-URL late-registration and removal regressions |

## Tests

New files and scenario groups (test names may be phrased more precisely during
TDD without weakening the outcomes):

- `apps/web/hooks/domains/github/use-branches-by-url.registry.test.ts`:
  all cached URLs recover after registration in either ensure order; success,
  error and empty entries refresh on replacement; removal uses fallback; old
  pending settlements cannot replace current results or release current slots.
- `apps/web/hooks/domains/github/use-pr-info-by-url.registry.test.ts`: the same
  registry boundaries for distinct repository/PR inspection sentinels, including
  unsupported and null-inspection results.
- `apps/web/components/task-create-dialog-remote-repo-chips.provider-refresh.test.tsx`:
  render real chips row, real chips, both real hooks, and real registry; initialize
  two pasted unsupported URLs, register provider later, assert both rows hydrate
  with their own descriptor/default branch and both native branch pickers offer
  distinct selectable branch sentinels. Use a stateful form harness, not mocked
  hooks or row coordinator. Mock provider/API transports only.
- Existing hook suites preserve per-URL concurrent/settled sharing, clear retry,
  normalization, workspace pending races, and built-in/structured routing.
  Add only missing behavioral coverage, with no helper-only tests or permutation
  explosion.

These map to AC-PLUGINS-REPOSITORY-TASK-CREATION-001.10 through .12 and preserve
.9. All changed test paths must appear in the final focused command.

## E2E and mobile parity

The mobile-parity pure state/data exception applies within the unchanged native
dialog. The real rendered component integration supplies consumer-level outcome
evidence; focused hook tests prove shared viewport-independent behavior. No
ASCII preview, new mobile/browser test, or production build is needed. Existing
first-use desktop/phone E2E in `plugin-repository-task-resolution` remains its
completed server-resolution evidence and is not rerun by this package.

## Documentation audit

Reviewed the current provider-registration and first-use task-creation sections
in `docs/public/plugins-authoring.md`, and searched root README/screenshots.
Existing guide already requires revocation, cancellation and workspace-scoped
structured inspection. Recovery fixes that behavior; no API, operator step,
label, screenshot, or public guide change is required. Preserve the completed
server-resolution companion package; its scopes, statuses and evidence remain
valid. No new ADR is needed for this local implementation correction.

## Work orders

- [x] [Task 01: Refresh all cached URLs in both native loaders](task-01-refresh-all-cached-urls.md)

Dependency order: one sequential task; no native agents, new persistent tasks,
tabs, or model switch.

## Verification results

Design validation, all terminal exit 0:

- `python3 scripts/list-docs.py validate`: 346 decisions and 1327 specifications
  validated (receipt `a4af80`).
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed
  (receipt `d85968`).
- `python3 scripts/lint-spec-files.py --all`: all specification files passed
  (receipt `52c59b`).
- `git diff --check`, status and catalog inspection: tracked docs clean;
  requirement/design catalogued and no staged paths (receipt `4929a3`).
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight using actual
  four design paths: exempt/pass. Adding the five explicitly planned production
  and test paths: covered/pass, one valid work order. REQ/AC/design links and
  four-file unstaged boundary passed (receipt `9afa0e`). Planned-path coverage
  is a design check; actual changed implementation paths must be checked later.

Implementation verification:

- Conditional frozen install: handle 23241 joined exit 0, pnpm 9.15.9,
  923 reused packages; receipt `cfdeb4`.
- Exact five-file Vitest command in the work order, one worker/4 GiB heap:
  permanent real-hook RED handle 9407 joined exit 1, 11 failures/49 passes.
  The native harness was then corrected to restore URLs after mount so the
  cache stays populated. RED handle 91757 joined exit 1 with the same counts,
  including beta hydration failure; receipt `975384`.
- First GREEN handle 78259 joined exit 1: 59/60 passed, native option selector
  lacked the real control's remote badge. Correcting its exact accessible name
  produced final GREEN handle 12978 joined exit 0: all 60 tests in five files
  passed in 5.14 s; receipt `84dc8e`. Both native pickers select their own
  distinct feature branches. No hook/coordinator/chip/registry mock was used.
- Exact five changed-file ESLint command with zero warnings: handle 98012
  joined exit 0; receipt `777b26`.
- `pnpm run typecheck` with 4 GiB heap: handle 67225 joined exit 0;
  receipt `03ad65`.
- `pnpm run i18n:check`: handle 64290 joined exit 0; receipt `08c08f`.
- `pnpm run i18n:ratchet`: handle 30756 joined exit 0; receipt `c7b7db`.
- Unchanged owning specification contents retain the successful catalog,
  36 spec-linter unit tests, and all-spec lint receipts above. These passing
  checks were not repeated for reassurance; normal commit hooks still validate
  their applicable gates.

All command logs are owned temporary files under
`/tmp/kandev-child19-711b6d54/`, with live/terminal receipts in the task plan.
Actual changed-path documentation coverage passed for all nine changed paths
with one complete work order; whitespace and status boundary passed
(receipt `c616c3`). Normal pre-commit and commit-msg hooks are active;
no bypass is permitted. No full suites, browser builds, E2E, or public guide
changes were needed under the recorded state/data exception. Local implementation
is complete; remote current-head review, required CI, actual merge proof and
joined cleanup remain externally pending until verified in the task plan.

## Delivery checkpoint

Task `711b6d54-2dc5-45c5-bf56-36387ed5ec1c`, session
`3c39efb6-e048-41c2-b083-92766ad07178`, parent
`14825981-b175-411d-999a-31ddc2aa5fc3`. The four-artifact design handoff preceded
the later explicit implementation release. Implementation and normal delivery
remain in this primary session. Parent messages to this child interrupt; child messages to parent are
queued. Critical parent-question tool calls end the turn.

After local checks, load normal commit/push/PR skills, preserve active hooks,
and publish a ready PR. Freeze the published SHA unless a real finding needs
remediation; never rebase for moving main. Join one managed `scripts/pr-await`
all-terminal monitor before replacing it. Require authenticated CodeRabbit
App347564 substantive full review of every changed path at current head,
`sourceCommitId = coveredCommitId = head`, `kind = reviewed`; skipped,
incremental, boilerplate or disabled reviews are insufficient. Inspect completed
automatic full coverage before at most one necessary exact-head full request.
Disposition all threads; defer optional comment style without head churn.
Normal expected-head squash requires terminal required gates and fresh complete
review/policy evidence, with no admin/bypass. Independently verify actual merged
SHA/time, fetched merged tree and owned blobs, and authoritative remote main.
Task completion requires that merge proof, all handles joined and exact owned
temporary-resource cleanup. Leave clean managed worktree/dependencies/shared
caches for parent archive. No crash-immunity promise.

## Risks

- Resetting sequences to zero could let an old request match a new one; advance
  them and test late success/error/finalization against a pending replacement.
- Removing only loaded markers would leave obsolete inspection visible when
  no provider remains; clear visible loader state too.
- The existing registry signal is coarse; retain current version-driven
  semantics rather than introducing provider revision machinery.
- Native rendered integration may expose unrelated dependencies. Isolate API
  transport, retain real hooks/registry/chips, and seek bounded parent direction
  if a real infrastructure blocker prevents that evidence.
