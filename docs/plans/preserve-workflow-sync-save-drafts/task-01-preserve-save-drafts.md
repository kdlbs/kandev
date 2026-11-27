---
id: "01-preserve-save-drafts"
title: "Preserve workflow source drafts during Save"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.1
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.2
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.3
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.4
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.5
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.6
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.7
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.1
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.2
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.3
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.4
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.5
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.6
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.7
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.8
system_design:
  - ../../specs/integrations/system-design/workflow-sync-settings-lifetime.md
---

# Task 01: Preserve workflow source drafts during Save

## Summary and release barrier

Implement the design's complete raw acknowledgement inside the real workflow
source controller and actual dialog save caller. Accepted canonical config and
truthful success coexist with a retained newer draft; canonical editable reset
and dismissal remain admitted for unchanged raw drafts.

ROOT reviewed the four-file package and released this same-primary implementation
after actual END DESIGN. CHILD90 returned localheavy and is hosted-only; CHILD91
holds the exclusive global local-heavy lease for normal delivery. Implementation
and scoped verification are done; hosted readiness and merge remain gated.
Review receipt: `/tmp/kandev-root-child91-design-review-release-20261009.json`.
Same primary, no delegates/models/tasks/sessions/tabs. Merge authority NONE;
normal ready-PR delivery is authorized, with a separate ROOT serial merge grant
still required. All delivery/process/protected-resource gates remain binding.

## In scope and ownership

- `apps/web/hooks/domains/settings/use-workflow-sync.ts`: local immutable full
  draft and latest snapshot, pure draft transformations, submitted raw snapshot,
  conditional canonical acknowledgement and optional read-only consumer signal.
- `apps/web/components/settings/workflow-sync-dialog.tsx`: invocation-local
  Save adoption admission combined with existing completion guard and bool.
- `apps/web/components/settings/workflow-sync-section.tsx` only if immediate
  consumer plumbing proves necessary; preserve keyed dialog/composition.
- New independently authored tests:
  `apps/web/hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx` and
  `apps/web/components/settings/workflow-sync-section.save-drafts.test.tsx`.
  Existing dialog controller fixtures may receive only the minimal shape update
  required by real no-argument/optional-notification compatibility.
  `apps/web/hooks/domains/settings/use-workflow-sync.lifetime.test.tsx` owns one
  existing GitLab compatibility case whose old in-flight reset expectation is
  superseded by REQ003; retain payload/accepted-config/bool/pending assertions.
- Update this work order/manifest results and owning requirement/design if
  private implementation detail changes; retain historical package results.
  Extract one private sibling draft helper only if lint limits require it; name
  it and include its actual test/lint/format paths before running checks.

## Out of scope

No backend/schema/API/transport abort, generic concurrency/revision/coordinator,
global framework, provider/poller/auth/refresh redesign, contributor integration,
Office edits, persistence, migration, layout/copy/touch/scroll/breakpoints,
runtime harness/cache changes, browser/build/full suites or optional polish.
Do not inspect/copy/import/replay/edit/chmod/remove ROOT's protected candidate.
Only the two authorized discovery summaries may be read from that directory.

## Acceptance

1. Independent real Section/Dialog/hook/providers transport-only regressions
   first fail causally on newer full raw draft loss and actual dialog dismissal.
   Then all 003 cases pass, including invalid/equivalent URL, provider/scalars,
   reverting old baseline, next Save and unchanged normalized close.
2. Current accepted config and feedback still publish; admitted Save remains
   truthful `Promise<boolean>` with existing no-argument behavior. Latest raw
   draft preservation and pre-reset close admission do not depend on dirty
   against config, normalized payload, or naive post-await render comparison.
3. Relevant existing lifetime/pending/dialog/removal/Sync-now/load/background,
   parsing/API and contributor controls pass. Mobile uses the unchanged
   composition state/data exception at 390/1024; docs/copy/API remain bounded.

## TDD and causal matrix

After release, read `/tdd`, mark this work order in_progress, and independently
author the permanent tests. Real StateProvider creates the actual app store;
real ToastProvider supplies actual feedback. Only external fetchJson is mocked;
do not mock form, store, lifetime, coordinator, reporter, API functions, hook or
components. Existing transport-only lifetime tests are patterns, not ROOT proof
imports. Keep test fixture text in recognized `.test.tsx` files or test-owned
parameters, not unsupported `.test-helpers` exemptions.

Name the actual Section regression `retains newer branch and keeps the real
dialog open after an older accepted Save`; name the hook outcome regression
`records accepted canonical config while preserving complete raw edits`.
Obtain one causal RED before the minimal correction: with initial config ready,
edit branch to submitted value, admit POST and assert its payload; edit enabled
Branch again while that POST is deferred; resolve old accepted canonical config.
Reach BOTH soft assertions on newer value and real open dialog. Use phone and
desktop variants for the shared path. Failure must be product behavior, not
fixture/setup/resource error. Keep independent unchanged-success closing and
current-rejection draft-preservation controls alongside the RED. Accepted ROOT
exit1 evidence is not a substitute for this independently authored RED.

The new suites' matrix must include:

| Stimulus | Product observations |
| --- | --- |
| Current success after branch/directory/each identifier/interval/poll/provider edit | Entire raw draft retained, real dialog open, saving settled; hook true, accepted canonical config and success toast |
| Valid new URL; invalid URL keeping parsed form unchanged; same-target alternate URL spelling | Raw URL retained with corresponding form/invalid state, no canonical partial overwrite; invalid input keeps Save disabled |
| Multiple raw changes before React rerender/within one act batch, then ACK | Latest complete admitted draft observed; no stale render snapshot decides adoption |
| Submit a changed branch then revert to old stored baseline while pending | Baseline draft preserved despite previously not being dirty; accepted server branch remains submitted; next POST uses reverted draft |
| Unchanged raw draft with server-trimmed branch/directory and canonical URL | Canonical editable values and displayed URL adopted; actual dialog closes and reopens showing canonical values |
| First Save and unchanged provider-switch Save | Own accepted config/target changes do not block closing; real provider payload shape preserved |
| Edit away then back to exactly submitted raw values | Canonical adoption and close permitted by value equality |
| Current failure with newer full draft, then correction/retry | False, error toast, draft/dialog preserved, saving cleared; next valid POST uses retained values and closes on success |
| Explicit close/reopen while pending; real workspace retirement; independent providers/stores | No old dismissal/reopening or retired publication; existing bool/pending/lifetime invariants remain |
| Background status-only refresh while editing; initial load, removal, forced sync | Existing reset/status/feedback/refresh semantics remain; no global result ordering added |

Use `@covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.<N>` as needed. Hook tests
assert public config/form/url/feedback/outcomes and transport payloads; component
tests assert real visible controls/dialog. Do not prove a private equality helper
by mirroring it. Clear/drain all owned deferred transport, timers and providers.
Do not add a browser or navigation mock to the new Save evidence. Existing suites'
reload spies remain existing controls, not mocked internal Save behavior.

## Verification and process ownership

All package commands below are LATER-LEASE ONLY. Run commands separately, one
original at a time. For each record actual argv/cwd/start, native session ID,
PID/group/start identity, actual terminal exit, joined state and fresh owned
group absence. Retain/poll each native handle; detached/unknown/timeout is NO
VERDICT. Checkpoint ROOT before replacement, alternatives or duplicates.

Conditional prerequisite only after release: if this worktree lacks usable
dependencies, run `(cd apps && pnpm install --frozen-lockfile)` with the existing
pinned pnpm9.15.9 under the same lease. No install in design; no package-manager
upgrade or cache/harness mutation. Keep normal hooks active.

RED and later focused new-suite GREEN (run from repo root):

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 280s pnpm exec vitest run --maxWorkers=1 hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx components/settings/workflow-sync-section.save-drafts.test.tsx)
```

After minimal correction, use this scoped GREEN once to include all new evidence
and relevant existing controls. Do not replay passing runs without a new causal
change/failure; if this scoped run provides new-suite GREEN, omit a duplicate
focused GREEN:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 280s pnpm exec vitest run --maxWorkers=1 hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx components/settings/workflow-sync-section.save-drafts.test.tsx hooks/domains/settings/use-workflow-sync.test.ts hooks/domains/settings/use-workflow-sync.lifetime.test.tsx components/settings/workflow-sync-section.lifetime.test.tsx components/settings/workflow-sync-dialog.test.tsx lib/utils/github-repo-url.test.ts lib/utils/gitlab-repo-url.test.ts lib/api/domains/workflow-sync-api.test.ts components/settings/settings-save-provider.test.tsx components/settings/settings-save-revision.test.ts)
```

Scoped lint/format use ACTUAL changed frontend files, not a guessed list; validate
that the raw changed/untracked paths are exactly the owned production/test files
above (and an explicitly recorded private helper if needed) before executing:

```bash
python3 - eslint <<'PY'
from pathlib import Path
import subprocess, sys
paths = set(subprocess.check_output(['git', 'diff', '--name-only', 'HEAD', '-z']).decode().split('\0'))
paths.update(subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z']).decode().split('\0'))
web = sorted(p for p in paths if p.startswith('apps/web/') and p.endswith(('.ts', '.tsx')))
assert web and all(Path(p).is_file() for p in web), web
allowed = {
    'apps/web/hooks/domains/settings/use-workflow-sync.ts',
    'apps/web/components/settings/workflow-sync-dialog.tsx',
    'apps/web/components/settings/workflow-sync-section.tsx',
    'apps/web/hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx',
    'apps/web/components/settings/workflow-sync-section.save-drafts.test.tsx',
    'apps/web/components/settings/workflow-sync-dialog.test.tsx',
    'apps/web/hooks/domains/settings/use-workflow-sync.lifetime.test.tsx',
}
assert set(web) <= allowed, web
print('\n'.join(web))
args = [p.removeprefix('apps/web/') for p in web]
subprocess.run(['pnpm', 'exec', *sys.argv[1:], *args], cwd='apps/web', check=True)
PY
```

Exact format invocation with the same actual raw path selection, as a separate
original command:

```bash
python3 - prettier --check <<'PY'
from pathlib import Path
import subprocess, sys
paths = set(subprocess.check_output(['git', 'diff', '--name-only', 'HEAD', '-z']).decode().split('\0'))
paths.update(subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z']).decode().split('\0'))
web = sorted(p for p in paths if p.startswith('apps/web/') and p.endswith(('.ts', '.tsx')))
assert web and all(Path(p).is_file() for p in web), web
allowed = {
    'apps/web/hooks/domains/settings/use-workflow-sync.ts',
    'apps/web/components/settings/workflow-sync-dialog.tsx',
    'apps/web/components/settings/workflow-sync-section.tsx',
    'apps/web/hooks/domains/settings/use-workflow-sync.save-drafts.test.tsx',
    'apps/web/components/settings/workflow-sync-section.save-drafts.test.tsx',
    'apps/web/components/settings/workflow-sync-dialog.test.tsx',
    'apps/web/hooks/domains/settings/use-workflow-sync.lifetime.test.tsx',
}
assert set(web) <= allowed, web
print('\n'.join(web))
args = [p.removeprefix('apps/web/') for p in web]
subprocess.run(['pnpm', 'exec', *sys.argv[1:], *args], cwd='apps/web', check=True)
PY
```

If a causally required private helper is extracted, record its ownership and add
its exact path to both allowed sets before running. Any required format write is
a separate causal command and normal-hook correction; never bypass hooks or
hide fixture copy with fake exemptions.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Light actual-path documentation coverage preflight from repo root, also usable
at the design checkpoint (actual diff/untracked files, not proposed code paths):

```bash
node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = cp.execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...changed, ...untracked])].map(filename => ({ filename, status: untracked.includes(filename) ? 'added' : 'modified' }));
const docs = [
  'docs/specs/integrations/requirements/gitlab-workflow-sync.md',
  'docs/specs/integrations/system-design/workflow-sync-settings-lifetime.md',
  'docs/plans/preserve-workflow-sync-save-drafts/plan.md',
  'docs/plans/preserve-workflow-sync-save-drafts/task-01-preserve-save-drafts.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
JS
```

Confirm links, all 15 ACs, both REQs declared in design, plan/work-order design
membership, and exactly one work order. Before documenting success check
`git diff --check -- docs/plans/preserve-workflow-sync-save-drafts` and
`git status --short -- docs/plans/preserve-workflow-sync-save-drafts` to include
untracked order evidence. Public-guide audit only; no public validator suite
unless a causally required public edit is admitted. No browser/build/full local
test command. Any broader check needs actual cause and ROOT release.

## Files likely touched

The three production boundaries and two new test suites named under ownership,
plus minimal existing `workflow-sync-dialog.test.tsx` fixtures if necessary, and
the four package artifacts. Private helper only for a real lint limit.

## Dependencies

None. Earlier provider/lifetime packages are complete compatibility baselines;
this work does not reopen them or reuse their old passing counts as proof.

## Risks

Raw URL/form divergence, canonical trim/reset interfering with close admission,
stale/batched snapshots, draft equality accidentally measured against config,
and own-save config target changes confusing removal/dismissal guards. The
matrix above names observable regressions for each. Respect resource/checkpoint,
hosted exact-head, review, separate serial merge and protected-resource gates in
the manifest/live plan throughout delivery.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/integrations/requirements/gitlab-workflow-sync.md#req-integrations-gitlab-workflow-sync-003-source-draft-preservation-during-save).
- [Existing design's acknowledgement boundary](../../specs/integrations/system-design/workflow-sync-settings-lifetime.md#save-draft-acknowledgement).
- [Manifest](plan.md), existing actual Section/Dialog/hook and transport-only
  lifetime suites; scoped web AGENTS and applicable skills.
- Accepted ROOT summaries only; protected candidate never read or replayed.

## Results

Implemented following ROOT's later reviewed release. Independently authored 33
real hook/Section/Dialog/provider regressions mock only external fetchJson.
Causal RED65715 exit1 and affected RED23094 exit1 reach BOTH raw-branch and
actual-dialog soft failures at 390/1024; unchanged normalized success and current
failure controls pass. The protected ROOT candidate was never read or replayed.

Minimal correction publishes immutable complete raw snapshots outside render and
state updaters, compares raw form+displayed URL before canonical reset, always
records accepted canonical config and truthful true/feedback for current owners,
and signals unchanged draft adoption to the immediate Dialog caller. Optional
read-only notification failure cannot report a successfully accepted write as
false. A private dialog completion helper satisfies the existing lint limit;
no framework/API/schema/Section/layout/copy changes.

Original scoped GREEN81525: 159/161 passing. Real provider mouseDown fixture and
one earlier GitLab test's explicitly excluded in-flight reset expectation were
corrected causally; affected GREEN35561 passes44/44. Other nine unchanged suites
retain117 passing cases from the original, yielding161/161 combined, including
33 new cases. After lint helper extraction, affected real dialog controls pass
43/43 in GREEN57394. No passing full matrix replay.

Actual six-file formatting and scoped eslint pass (affected lint6395 zero
warnings/errors). Typecheck35307, i18n72006, ratchet29458 and final format-check
exit0. Conditional frozen dependency install33833 used pinned9.15.9 only after
lease. All actual originals joined and fresh exact owned wrapper/child groups
were absent; task-owned `/tmp/kandev-child91-<label>.json`/`.log` retain argv,
identity, native handles, actual exits and joins. No resource/timeout/transport
or out-of-scope alternative, cache wipe, hook bypass or foreign process action.

Catalog/spec/link/actual-path implementation coverage and whitespace validation
are recorded in the current live plan. Normal active commit hooks and hosted
checks/review remain required before delivery readiness; merge authority NONE.
The public guide remains accurate; pure state/data mobile exception retains
composition and uses real 390/1024 outcome tests. No browser/build/full local run.

Historical design-only validation passed catalog365 decisions/1478 specs, spec
lint, four-document exempt coverage(errors[]), one order/two REQs/15 ACs/design
membership/18 links and whitespace. Design artifacts were uncommitted at that
checkpoint. Its queue_full callback was not retried; ROOT later reviewed exact
hashes and released implementation. Do not mistake design-exempt coverage for
implementation coverage or that callback result for approval.
