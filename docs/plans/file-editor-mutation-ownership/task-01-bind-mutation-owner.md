---
id: "01-bind-mutation-owner"
title: "Bind mutation completion to its editor owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
acceptance_criteria:
  - AC-UI-FILE-EDITOR-MUTATION-001.1
  - AC-UI-FILE-EDITOR-MUTATION-001.2
  - AC-UI-FILE-EDITOR-MUTATION-001.3
  - AC-UI-FILE-EDITOR-MUTATION-001.4
  - AC-UI-FILE-EDITOR-MUTATION-001.5
  - AC-UI-FILE-EDITOR-MUTATION-001.6
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
---

# Task 01: Bind mutation completion to its editor owner

## Summary

Use faithful deferred hook/store/panel regressions to prove and repair obsolete
save/delete publication. Bind replies and pending cleanup to the live session
visit and editor incarnation while preserving the existing action contracts.

## In scope

- Implement the paired design's local ownership rule in save/delete/reload,
  immediate hook lifecycle/saving-state glue and file-state installation.
- Apply the same captured-owner rule to the tablet TaskCenterPanel mutation
  consumer and immediate tab installation/typing/pending state glue. Prove
  deferred navigation save/delete against the real hook and React tab state;
  cover return, reopen/replacement, disposal, typing, repos and current errors.
- Real AppStore/StateProvider + Dockview store + production hook publication,
  panel removal/persistence wiring and observable LSP arguments with mocked
  transport. Reconcile the existing repo-threading harness as necessary.
- Criteria `.1` through `.6` in the plan's test matrix; current and obsolete
  success/failure, same-session typing, repo independence and actual replacement.
- Synchronize docs/results and complete normal authorized PR delivery after the
  later explicit implementation release.

## Out of scope

- Backend, new API/coordinator, runtime/harness files, viewport markup/copy.
- Generic refresh or same-editor concurrency redesign; local full suites,
  build/browser/E2E runs absent new concrete evidence.
- Delegation, additional tasks/sessions, foreign process/cache/worktree cleanup.

## Acceptance

1. Permanent save-navigation and delete-navigation tests fail meaningfully on
   the uncorrected production path, then pass with the minimal fix.
2. Every replacement/failure/cleanup case in the plan is covered by real state
   and panel publication; positive controls prove preserved behavior and routing.
3. All exact local checks pass and docs/results agree with actual evidence.
   Subsequent authorized delivery requires hosted gates/full semantic review
   before verified merge and joined cleanup, tracked in the live task plan.

## Verification

Run independent commands from the repository root, sequentially, retaining and
joining any returned handles. One heavy command at a time; never rerun a passing
check without a new change or failure that justifies it.

```bash
# This execution shell omits the existing local Node/pnpm runtime from PATH.
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"

# Only if worktree dependencies are absent; exactly one install.
(cd apps && pnpm install --frozen-lockfile)

# RED before production changes: only the two deferred navigation regressions.
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run hooks/use-file-editors.mutation-ownership.test.tsx --maxWorkers=1 -t 'rejects old save after session navigation|keeps replacement panel after old delete')

# GREEN after the correction: all directly affected hook/store suites once.
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-save-delete.test.ts hooks/use-file-editors.open-action.test.tsx hooks/use-file-editors.build-state.test.ts hooks/file-editors-sync.test.ts lib/state/dockview-file-state.test.ts --maxWorkers=1)

(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec eslint hooks/use-file-save-delete.ts hooks/use-file-editors.ts hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-save-delete.test.ts lib/state/dockview-store.ts lib/state/dockview-file-state.ts lib/state/dockview-file-state.test.ts --max-warnings 0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/file-editor-mutation-ownership

# Actual local coverage preflight, including committed and untracked changes.
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = '5f08b1b3e3c17bb2688d7473f2a9ec32f3eed7cb';
const tracked = execFileSync('git', ['diff', '--name-only', base, '--'], { encoding: 'utf8' });
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' });
const paths = [...new Set((tracked + untracked).trim().split('\n').filter(Boolean))];
const docs = [
  'docs/plans/file-editor-mutation-ownership/plan.md',
  'docs/plans/file-editor-mutation-ownership/task-01-bind-mutation-owner.md',
  'docs/specs/ui/requirements/file-editor-mutation-ownership.md',
  'docs/specs/ui/system-design/file-editor-mutation-ownership.md',
];
const fileContents = Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles: paths.map(filename => ({ filename, status: 'modified' })), fileContents });
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
NODE
```

The coverage command uses the actual `validateCoverage` with changed paths and
real artifact contents; also inspect the actual hosted PR documentation coverage
status. No synthetic passing coverage verdict.
If implementation needs another directly owned file, include that file in
changed-file lint and its changed suite in the targeted GREEN command. Generated
release/changelog work from typecheck must be inspected and kept outside the
logical diff unless this repair actually requires it.

## Files likely touched

- `apps/web/hooks/use-file-save-delete.ts`
- `apps/web/hooks/use-file-editors.ts`
- `apps/web/lib/state/dockview-store.ts` (internal buffer incarnation type only)
- `apps/web/lib/state/dockview-file-state.ts`
- `apps/web/hooks/use-file-editors.mutation-ownership.test.tsx` (new)
- `apps/web/hooks/use-file-save-delete.test.ts` (existing harness compatibility)
- `apps/web/lib/state/dockview-file-state.test.ts` (new)
- `apps/web/components/task/task-center-panel-restoration.ts`
- `apps/web/components/task/task-center-panel-file-tabs.ts`
- `apps/web/components/task/task-center-panel.tsx`
- Their targeted tablet mutation/restoration/file-tab tests.
- Owning requirement/design, UI boundary note and this plan/work order.

`file-editor-panel.tsx` is a consumer to preserve; no markup or loader/resync
rewrite is planned. `file-editors-sync.ts` is preserved and its affected control
suite verifies that incarnation metadata does not change current remote sync.

## Dependencies

None. Later explicit implementation release is required; no production or
permanent tests in the design turn. Do not replay the accepted parent RED.

## Risks

See [plan](plan.md#risks). Preserve StrictMode cleanup, multi-consumer restoration,
repo identity, current errors, local persistence and the existing LSP disk/live
boundary. Matching absent tokens must not authorize a replacement.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/file-editor-mutation-ownership.md)
- [Design](../../specs/ui/system-design/file-editor-mutation-ownership.md)
- `use-file-save-delete.test.ts` and `use-file-editors.open-action.test.tsx` for
  deferred transport patterns; real store actions replace their state mocks in
  the new integration suite.
- Parent proof `/tmp/kandev-editor-session-save-repro.test.ts`, read-only archive.
- Task/session IDs and routing/completion barriers in the Kandev task plan.

## Mobile parity and public docs

State/data publication only. No composition/geometry change; targeted real
hook/store/panel evidence satisfies mobile-parity's narrow exception. Phone
Files/document flow remains the nearest shipped surface. No browser/build or
new mobile E2E. Internal docs updated; existing public editor capability,
terminology and screenshots need no change.

## Results

Implemented after the parent's later explicit release. Production changes are
limited to save/delete/reload publication, the immediate visit/pending-save hook
glue and transient buffer incarnation assignment/preservation. Existing public
action signatures, descriptor/request shapes and remote/LSP boundaries remain.

- Frozen install: one run, 923 packages, exit 0; handle 90923 joined.
- Navigation RED command above: three meaningful assertion failures, exit 1;
  handle 1404 joined. Save changed B's baseline/hash to A; pinned and preview
  delete removed B's panel. Parent proof/archive was not replayed or changed.
- Additional focused RED of `rejects a save after the panel host is replaced`:
  spinner expected zero, received one; handle 10372 joined, exit 1. Marker host
  identity and exposed saving Set were corrected within the same owner rule.
- Final GREEN command above: six test files, 57 tests passed; handle 20034
  joined, exit 0. Includes StrictMode, multiple consumers, preview promotion,
  A-B-A, unmount/no session, identical reopen, replaced host, initially absent
  delete panel, current/obsolete errors, typing, repositories, pending cleanup,
  descriptor/request exclusion and remote-hash retirement.
- Final changed-file lint, typecheck, `i18n:check` and `i18n:ratchet`: passed;
  sequential handle 54128 joined, exit 0. A function extraction satisfied lint
  limits; its affected tests passed again. Generated files stayed outside diff.
- Final docs checkpoint: catalog validated 343 decisions/1319 specifications;
  36 spec-linter tests and specification lint passed. Actual-diff coverage
  accepted all 12 changed paths with the linked work order and zero errors;
  whitespace/status checks passed. Handle 2654 joined, exit 0.

Local implementation status is `done`. PR creation, exact-head full review,
hosted gates, actual merge and resource cleanup remain delivery gates in the
live Kandev task plan, with queued parent callbacks. No hosted result is claimed
by this pre-publication record.

PR 4177 review correction: the parent authorized the reachable tablet
TaskCenterPanel handler within this same work order. Navigation save/delete
RED failed twice with A baseline/hash published into B and B's tab removed
(handle 29870 joined, exit 1). The local tablet visit/tab-incarnation guard,
guarded functional publication and per-action pending markers now cover that
consumer. Existing typing/preview updates preserve incarnations; installation
and restoration create them. Explicit descriptor/request projections retain
their existing shapes. No viewport markup or responsive interactions changed.

Focused tablet GREEN:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run components/task/task-center-panel-mutation-ownership.test.tsx components/task/task-center-panel-restoration.test.ts components/task/task-center-panel-file-tabs.test.ts --maxWorkers=1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec eslint components/task/task-center-panel-restoration.ts components/task/task-center-panel-file-tabs.ts components/task/task-center-panel.tsx components/task/task-center-panel-mutation-ownership.test.tsx components/task/task-center-panel-restoration.test.ts components/task/task-center-panel-file-tabs.test.ts --max-warnings 0)
```

Three files/27 tests passed, including 14 faithful real-hook/React-tab-state
cases (handle 90916 joined, exit 0). Tablet lint and typecheck passed (handle
52105 joined, exit 0). The unchanged six Dockview suites were not replayed.
The reviewer-requested store test now compares reopen against the immediately
preceding incarnation; its one test and lint passed (57943 joined, exit 0).
Requirement terminology now includes panel host and clean-save wording retains
content while clearing dirty indicators. Optional callback-identity polish was
deferred. Corrective-head hosted review/checks and merge remain live-plan gates.

Corrective i18n check/ratchet passed (22132 joined, exit 0); docs catalog/spec
lint/whitespace passed (43638 joined, exit 0). Actual-diff PR coverage accepted
all 18 paths, including the tablet consumer, with zero errors. No generated or
foreign paths entered the diff.
