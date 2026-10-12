---
id: "01-bind-refresh-owner"
title: "Bind workspace refresh to its current editor"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
acceptance_criteria:
  - AC-UI-FILE-EDITOR-MUTATION-001.7
  - AC-UI-FILE-EDITOR-MUTATION-001.8
  - AC-UI-FILE-EDITOR-MUTATION-001.9
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
---

# Task 01: Bind Workspace Refresh to Its Current Editor

## Summary

Publish only the latest eligible workspace read into its originating live editor,
using live dirty state after asynchronous hashing. One sequential TDD pass owns
helper ordering, both caller lifetimes and faithful integration evidence.
ROOT reviewed the four-file package and released implementation in this primary
session on 2026-10-08, with the exclusive local-heavy lease. Production changes
follow independently authored behavioral RED evidence.

## In scope

- Shared per-key publication token and existing instance/host ownership in
  `syncOpenFileFromWorkspace`, checked before/after fetch and hashing.
- Existing committed visit forwarding for `useOpenFileWorkspaceSync` and local
  committed activation-subscription guard in `useResyncOnTabActivate`.
- Final live-buffer read for dirty/clean reconciliation and owned panel sink.
- Independently authored real helper/store/hash, hook/provider and actual
  panel-callback regressions; current behavior and independence controls.
- Minimal compatibility changes in the directly affected existing fixtures,
  durable specs/plan status and actual documentation coverage.

## Out of scope

All exclusions and delivery barriers in [the plan](plan.md) apply. No new store
action/API/persistence/dependency, generalized ownership framework, fetch abort,
retry, editor rename/layout/copy/touch/navigation change, independent tablet/phone
read repair, save-versus-refresh policy, browser/build/E2E/full-suite run or merge.
Never modify/replay/remove/copy/import ROOT's protected proof.

## Acceptance

1. Independently authored named behavioral REDs prove read regression, identical
   reopened-buffer overwrite and typing loss on the uncorrected production
   helper, using the real Dockview actions, request normalization and hash.
2. Shared helper/caller ownership satisfies `.7` and `.8` across both actual
   triggers, preserving independent readers/files/repos and future valid reads.
3. Final live reconciliation and actual callback/panel positive controls satisfy
   `.9`; all directly affected checks pass with joined handles and actual results.

## Source and consumer inventory

Only two production calls invoke `syncOpenFileFromWorkspace`: the Git-status
effect in `hooks/file-editors-sync.ts` and activation callback in
`components/task/file-editor-panel.tsx`. `use-file-editors.ts` is the sole
production caller of `useOpenFileWorkspaceSync`.

All audited production `useFileEditors` consumers (some files contain several
instances) are:

- `hooks/use-panel-actions.ts`, `hooks/use-lsp-file-opener.ts`.
- `app/office/tasks/[id]/advanced-panels/chat-panel.tsx`.
- `components/diff/walkthrough-step-card.tsx`, `walkthrough-floating-window.tsx`.
- `components/task/file-editor-panel.tsx`, `use-review-dialog.ts`,
  `passthrough-toolbar.tsx`, `dockview-shared.tsx`, `dockview-panel-content.tsx`.

Each inherits the Git-status helper; reader unmount must retire only its own
pending publication. Desktop and compact fine-pointer layouts render Dockview
file editors. `TaskLayout` selects separate coarse-pointer tablet and phone
layouts; `TaskCenterPanel` restoration requests file content independently,
and `SessionMobileLayout` uses `fetchAndOpenFile` with selected-file state and a
keyed `MobileFileViewerPanel`. `usePanelActions` may still mount the hook there,
but gates Dockview opening on `usesDesktopWorkbench`. Do not describe tablet or
phone viewers as helper consumers, and do not modify their requests.

Inventory commands (read-only, repo root):

```bash
rg -n 'syncOpenFileFromWorkspace|useOpenFileWorkspaceSync|useFileEditors\(' apps/web
rg -n 'usesDesktopWorkbench|SessionTabletLayout|SessionMobileLayout' apps/web/components/task/task-layout.tsx apps/web/hooks/use-responsive-breakpoint.ts
rg -n 'requestFileContent|fetchAndOpenFile|MobileFileViewerPanel|TaskCenterPanel' apps/web/components/task/task-center-panel-restoration.ts apps/web/components/task/mobile
```

## Implementation constraints

Capture existing instanceId, repo/path and Dockview API before any await. Use
one local shared pending map per file key, not per caller. A retired/missing
owner cannot admit or supersede a live read. Latest admission suppresses all
earlier results, including when latest fails or its entry is removed. `finally`
deletes only its own current entry. Keep bookkeeping bounded to outstanding
publication and avoid content retention.

Use `activeEditorVisitRef` for the Git-status caller; do not add render-time ref
writes. The activation effect owns a committed token captured by its callback,
with layout cleanup and captured portal API validation. Preserve initial-active
read, true-only activation, same-session rerenders and disposal. Validate again
after fetch/hash; re-read the live buffer immediately before publication without
another await. Recheck ownership before panel title/dirty publication after store
updates. Retain dirty matching/nonmatching and no-op metadata branches.

## Permanent tests

New `hooks/file-editors-sync.ownership.test.tsx` owns real helper/store/request/hash
and real `useFileEditors`/provider evidence. New
`components/task/file-editor-panel.workspace-sync.test.tsx` owns the actual
registered initial-active/activation callbacks and their retirement. Required
test names for the three first REDs:

- `keeps newer acknowledged content after an older refresh completes`.
- `keeps an identical reopened buffer after an old refresh completes`.
- `preserves typing while the real content hash is pending`.

Both completion orders and newest failure/old success must deliberately overlap
actual requests. Assert real content, baseline, hash, dirty and remote fields,
not only call counts or a copied predicate. Same-key tests must use remove/set
or replacement set so the real action creates a new instanceId, with identical
initial content/hash to rule out content-based guards.

Additional cases: independent keys/repos; two live readers/triggers sharing one
buffer; current peer still works after another reader unmounts; API replacement;
retirement during fetch and hash; committed A-B-A, null and unmount; StrictMode
cleanup/replay when the actual consumer can start a request on mount. Ordinary
same-session rerender and typing must remain writable. Current success/empty,
binary normalization, real content hash, resolved-path clearing, already-dirty
matching/nonmatching, repeated remote no-op, transport failure and a subsequent
valid trigger preserve compatibility. Matching dirty text must actually clear
panel dirtiness/title; nonmatching text must retain typing and real reload must
apply the offered content/hash. Assert stale callbacks cannot start transport,
clear current bookkeeping or publish into the surviving editor.

At the hook boundary use real `StateProvider`, AppStore/actions, Dockview
actions, connection singleton, `useSessionGitStatus`, normalization and hashing.
Supply actual session/environment mapping and Git status through real store
actions, plus real `ToastProvider` and other required providers. Substitute only
the transport's `client.request`; record and join every deferred request. At
the panel boundary mount real `FileEditorPanel` and acquire a real portal-manager
entry with a minimal Dockview API event double; fire its registered callback,
inspect real store/panel effects and verify disposal. Retain positive controls
for initial active, later true activation, inactive no-read and dirty reload.
Do not mock the consumer hook, state provider, sync helper or affected panel.
The rendered text controls select the shipped CodeMirror provider through the
real editor-resolver store and restore the previous setting; the repository's
Vitest Monaco stub lacks `editor.create`. Include the real command-registry
provider. No production editor implementation or global stub is changed.

Hash interleaving uses a temporary instrumented `crypto.subtle.digest` boundary:
hold completion, delegate to the saved genuine digest and return genuine bytes.
Keep `calculateHash` real; restore the original descriptor and join all pending
work even after assertions fail. Do not use sleeps, global helper/hash/store
mocks, production testing hooks or source-string assertions. Existing mocked
`file-editors-sync.test.ts` controls may receive necessary caller/instance fixture
metadata; they are supplementary and cannot supply the new owner evidence.

## Mobile and rendered verification

Explicit `/mobile-parity` state/data exception: this changes no layout, copy,
touch, scrolling, navigation or breakpoint rule. The source inventory above
truthfully distinguishes Dockview from independent tablet/phone reads. Faithful
unit/hook/panel evidence suffices; no new browser/mobile E2E or ASCII layout
preview is warranted. This is not a claim of executed mobile viewer coverage.

## Verification

Run only after ROOT grants this work order's implementation/local-heavy lease.
Every command starts from repo root or its own isolated directory. Join every
returned process handle before the next heavy command. Record actual exit and
test counts, including original failures. No blind retry after resource/transport
timeouts; persist the blocker and end for ROOT reads.

```bash
export PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH
export NODE_OPTIONS=--max-old-space-size=4096
# apps/package.json pins pnpm 9.15.9; verify before any package command.
(cd apps && corepack pnpm --version)
# Only if apps/node_modules is absent; exactly one frozen installation.
(cd apps && corepack pnpm install --frozen-lockfile)

# Independently authored RED, before production edits; only these three cases.
(cd apps/web && corepack pnpm exec vitest run hooks/file-editors-sync.ownership.test.tsx --maxWorkers=1 -t 'keeps newer acknowledged content after an older refresh completes|keeps an identical reopened buffer after an old refresh completes|preserves typing while the real content hash is pending')

# GREEN once after correction: both new suites and directly affected controls.
(cd apps/web && corepack pnpm exec vitest run hooks/file-editors-sync.ownership.test.tsx components/task/file-editor-panel.workspace-sync.test.tsx hooks/file-editors-sync.test.ts hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-editors.open-action.test.tsx hooks/use-file-editors.test.tsx components/task/file-editor-panel.image.test.tsx components/task/file-editor-panel.download.test.tsx --maxWorkers=1)
(cd apps/web && corepack pnpm exec eslint hooks/file-editors-sync.ts hooks/use-file-editors.ts components/task/file-editor-panel.tsx hooks/file-editors-sync.ownership.test.tsx hooks/file-editors-sync.test-helpers.tsx components/task/file-editor-panel.workspace-sync.test.tsx hooks/file-editors-sync.test.ts --max-warnings 0)
(cd apps/web && corepack pnpm run typecheck)
(cd apps/web && corepack pnpm run i18n:check)
(cd apps/web && corepack pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/editor-workspace-refresh-ownership

# Actual local documentation coverage, including unstaged/untracked artifacts.
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = '1257838968f5c92305a858427cdf723a87b0a882';
const tracked = execFileSync('git', ['diff', '--name-only', '-z', base, '--']);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']);
const paths = [...new Set(Buffer.concat([tracked, untracked]).toString().split('\0').filter(Boolean))];
const artifacts = [
  'docs/plans/editor-workspace-refresh-ownership/plan.md',
  'docs/plans/editor-workspace-refresh-ownership/task-01-bind-refresh-owner.md',
  'docs/specs/ui/requirements/file-editor-mutation-ownership.md',
  'docs/specs/ui/system-design/file-editor-mutation-ownership.md',
];
const fileContents = Object.fromEntries(artifacts.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles: paths.map(filename => ({ filename, status: 'modified' })), fileContents });
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
NODE
```

If an additional owned fixture/helper becomes necessary, update this exact lint
and GREEN selection before execution; run its changed suite once. Inspect
typecheck generators and lockfile status. Do not include unrelated/generated
changes. Normal active commit hooks run only after the release and checks, with
no bypass; stage/commit/PR delivery follows the local skills and ROOT barriers.

## Files likely touched

- `apps/web/hooks/file-editors-sync.ts`.
- `apps/web/hooks/use-file-editors.ts` (existing visit forwarding only).
- `apps/web/components/task/file-editor-panel.tsx` (activation lifetime only).
- `apps/web/hooks/file-editors-sync.ownership.test.tsx` (new).
- `apps/web/hooks/file-editors-sync.test-helpers.tsx` (new shared faithful fixture).
- `apps/web/components/task/file-editor-panel.workspace-sync.test.tsx` (new).
- `apps/web/hooks/file-editors-sync.test.ts` (minimal fixture compatibility).
- Owning requirement/design and this manifest/work order.

Existing `FileEditorState.instanceId`, `dockview-file-state.ts` actions,
`requestFileContent`, `calculateHash` and portal manager remain production inputs,
not planned edits. Tablet/phone independent read consumers remain outside scope.

## Dependencies

None. Existing instanceId and committed visit implementation are included in
baseline `1257838968f5c92305a858427cdf723a87b0a882`. No moving-main dependency or
synthetic merge check is required.

## Risks

See the plan's concrete ownership/cleanup/fixture risks. A new material contract,
out-of-scope change or repeated targeted failure requires a persisted checkpoint
and ROOT review, not a guessed workaround or broadened suite.

## Parallelism

`sequential`. Same primary session only; no native agents or new tasks/sessions.

## Inputs

- Existing editor requirement `.7`-`.9` and design sections Workspace refresh
  admission and ordering, Refresh consumer lifetime and Live reconciliation.
- Real mutation-owner visit/instance patterns, Files per-path reply-token pattern,
  current sync branches and actual portal activation registration.
- ROOT's accepted receipts and protected namespace in the manifest; no replay.

## Results

Completed after ROOT's explicit implementation release. The production change
is limited to the shared sync helper and its two existing caller boundaries.
New tests use genuine store/actions, request normalization, hashing, providers
and actual panel callbacks. Only transport is deferred; hashing barriers forward
the genuine digest. No browser/build/E2E/full-suite result is claimed.

- Three named helper REDs: actual Vitest exit 1, all three causal assertions failed
  before production edits. Original native 22638 joined at 0c5e3a.
- Initial caller RED: actual exit 1; panel-unmount assertion was causal, three
  Git tests lacked the initial real repository status. Repairing that fixture
  produced three causal Git lifetime REDs (original 79016 joined 9b11d6).
- The eight-suite command in Verification: actual exit 1, 94/96 tests passed.
  The two rendered text controls lacked CommandRegistryProvider. After adding
  it, the affected panel run passed 8/10 and exposed the Monaco test-stub limit.
  Selecting the actual shipped CodeMirror provider fixed those two controls;
  the affected panel run passed all 10, no skips (95670 joined faf104).
- Final affected GREEN after test-only lint constants/group splitting:
  `corepack pnpm exec vitest run hooks/file-editors-sync.ownership.test.tsx components/task/file-editor-panel.workspace-sync.test.tsx --maxWorkers=1`:
  exit 0, 2 suites / 31 tests passed, zero skips, native 53355 joined 00ae9b.
  Combined final evidence covers all 96 tests in eight affected suites without
  replaying six unchanged passing controls.
- The exact affected ESLint command initially reported only nine warnings in
  the two new suites. Constants/group splitting repaired those; affected-only
  ESLint on those two suites passed exit 0 (8202 joined b1b1a2). Other files
  were clean in the original lint run. Production behavior did not change.
- `corepack pnpm run typecheck`: exit 0, 68145 joined dca774; generators left no
  unexpected tracked diff. `i18n:check`: exit 0, 46219 joined 720a4e;
  `i18n:ratchet`: exit 0, 58942 joined 449a24. No new user-facing copy.
- Exactly one pnpm 9.15.9 frozen installation from `apps/` passed; no lock change.
- `python3 scripts/list-docs.py validate`: exit 0, 363 decisions/1437 specs;
  `python3 scripts/lint-spec-files.py --all`: exit 0. Whitespace clean.
- Actual product-diff `validateCoverage` includes all tracked/untracked paths
  and the four real artifact contents: exit 0, `ok: true`, `status: covered`,
  `requiresCoverage: true`, `errors: []`, one accepted work order and existing
  owning design/requirement. Receipt retained in the live task plan.

All original command handles above were actually joined; runner receipts report
empty owned process groups. Full argv/start/deadline/PID/PGID/stdout/stderr/exit
receipts remain under `/tmp/kandev-child76-editor-refresh-20261008/` and in the
live task plan. Normal hook/PR/hosted gates are separate delivery evidence; no
merge authority is inferred from implementation completion.

### Design checkpoint, 2026-10-08

- `python3 scripts/list-docs.py validate`: exit 0, 363 decisions/1437 specs.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specs passed.
- `git diff --check`: exit 0; four owned docs paths unstaged/uncommitted and
  no staged paths, production/permanent-test edits or child-owned pending handles.
- Real `validateCoverage` with actual four changed paths: exit 0, errors empty,
  docs-only exempt. This does not claim future product-diff coverage.
- Repository `parseFrontmatter` cross-reference check: exit 0, one pending
  sequential order; all requirement/acceptance/design/manifest and plan links
  resolve. No synthetic source paths or invented coverage verdict.
- Dependencies absent and current PATH lacks Node; existing Node 24.18.0 and
  Corepack located at the exact path in Verification. No installation attempted.

Next ROOT action: independently read this package, then send a later explicit
implementation interrupt/local-heavy lease to the same primary session. End
this design turn now; autopilot and package creation do not release that barrier.
