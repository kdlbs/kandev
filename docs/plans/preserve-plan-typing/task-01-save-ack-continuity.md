---
id: "01-save-ack-continuity"
title: "Preserve own-save draft continuity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DOCUMENTS-003
acceptance_criteria:
  - AC-TASKS-DOCUMENTS-003.1
  - AC-TASKS-DOCUMENTS-003.2
  - AC-TASKS-DOCUMENTS-003.3
  - AC-TASKS-DOCUMENTS-003.4
  - AC-TASKS-DOCUMENTS-003.5
  - AC-TASKS-DOCUMENTS-003.6
  - AC-TASKS-DOCUMENTS-003.7
system_design:
  - ../../specs/tasks/system-design/plan-write-lifecycle.md
---

# Task 01: Preserve own-save draft continuity

## Summary

Keep newer plan typing when its own older save acknowledges. Add faithful
deferred-transport regressions through real hooks/store, make the smallest
local draft correction, and verify the actual panel/TipTap editor consumer.
ROOT reviewed the design package and explicitly released implementation to
this primary on 2026-10-08.

## In scope

- `usePlanDraft`'s own-attempt registration, same-view acknowledgement
  consumption, editor key and draft continuity, and follow-up autosave.
- Attempt/task-view identity protecting overlap callbacks and size suppression.
- Real store/hook and real panel/editor test fixtures with transport-only
  deferred save responses, plus existing compatibility suites.
- Accurate results/status in this work order and the plan manifest.

## Out of scope

- Backend/API/store/global-event/version-policy changes or transport reordering.
- Save queues/refcounts, arbitrary external-writer reconciliation, text merging,
  draft persistence across navigation, layout/touch/navigation/copy changes.
- Additional production edits, packages, permanent test helper modules, broad
  browser/build/full verification, delegates, sessions, or tabs. Checkpoint ROOT
  for necessary scope changes or infrastructure blockers.

## Acceptance

1. New tests first fail causally because B is replaced by A after the real own
   save publication. The correction preserves B, editor identity/selection/
   focus, dirty state, and exact next autosave arguments; unchanged success
   becomes clean. Cover update and initial create.
2. Real-hook controls preserve external replacement/deletion, latest-started
   same-task saves, different/identical overlapping callbacks, background
   publication after navigation, null/equal/no-plan/round-trip task changes,
   failed saves, size-only suppression, edited/explicit retries, and generic
   retries. Earlier callbacks cannot clean unpublished drafts or clear later
   suppression. Keep current assertions and debounce timings intact.
3. Run the exact focused checks below under ROOT's lease, join original
   handles, record actual command exits/counts and owned process-group absence,
   and update both delivery statuses. No production change outside the local
   draft hook, public docs, or new rendering is required by this package.

## Inputs and evidence

- [Plan and test matrix](plan.md#tests).
- [Owning requirement](../../specs/tasks/requirements/documents.md#req-tasks-documents-003-plan-draft-continuity-during-successful-saves).
- [Draft acknowledgement design](../../specs/tasks/system-design/plan-write-lifecycle.md#plan-draft-acknowledgement).
- [Size rejection requirements](../../specs/tasks/requirements/plan-content-size-limit.md)
  and [design](../../specs/tasks/system-design/plan-content-size-limit.md),
  especially AC-003.2 through AC-003.8 of that capability. They remain controls,
  not a new duplicate requirement.
- Real `useTaskPlan.savePlan`, `usePlanDraft`, `TaskPlanPanel`,
  `PlanPanelHeader`, `TipTapPlanEditor`, `StateProvider`, `createAppStore`, and
  `buildTaskPlanActions.setTaskPlan` at design base
  `f61fd4e5fda79c0bb28f94e6af1e988898be9561`.
- Nearest fixture patterns: `use-task-plan.test.ts`,
  `task-plan-panel-draft.test.ts`, `task-plan-panel.refresh.test.tsx`, and
  `tiptap-plan-editor.test.tsx`. Existing mocks are compatibility evidence,
  not the pattern for new own-save proofs: new tests must use the real hooks
  and provider/store. Do not replace the editor with a fake component.
- ROOT archive receipt/hash and original joined handles are in [the plan](plan.md#evidence-and-assumptions).
  Inspect it read-only only; independently author permanent tests from source.

## Execution sequence

1. After explicit implementation delivery, re-read caller constraints and
   current worktree/base/consumers. Obtain ROOT's local-heavy lease before
   dependency or product checks. Mark this work order `in_progress` and keep
   the manifest synchronized. No delegation.
2. Author the new hook regression and controls with a real StateProvider/store
   and real hooks. Mock only plan transport. Deferred responses must drive the
   actual store publication, saving true/false transitions, and next debounce.
   Rerender may change task ID; it may not synthesize acknowledged plan props.
3. Run the exact focused RED below. An import, fixture, missing-method,
   resource, timeout, or unknown command failure is not RED. The accepted
   archive is discovery evidence; this run records the new permanent regression.
4. Correct local own-save synchronization before dispatch/publication and
   consume scoped recognition once. Do not merely move ownership into a late
   `.then()` or infer provenance/order from `isSaving`. Preserve the observed
   plan as baseline and live draft independently. Same-text submissions need
   attempt identity; task round trips need view identity.
5. Add the actual panel/TipTap integration case using production onChange and
   editor transactions. Assert the same DOM/editor remains after A acknowledges
   while B's text and selection/focus remain. Verify B reaches transport.
   Reuse narrow editor-test browser shims, the real dynamic adapter, and unrelated
   chrome/service mocks only. Exercise phone/fine-pointer inputs where practical.
6. Run focused GREEN and remaining checks once. Ordinary causal own-fixture,
   type, or lint minimum repairs rerun only affected checks. Checkpoint ROOT
   for resource/timeout/unknown/transport/out-of-scope results without retries.
7. Record actual exits/counts/original terminal chunks and owned-group absence,
   update plan/work-order results and statuses, and return the local-heavy
   resources explicitly. Subsequent commit/PR/hosted/merge work follows the
   live task plan and ROOT grants; this package does not grant a merge.

## Verification

Commands below are repository-root-relative unless enclosed in a subshell.
During design run only the final cheap documentation/reference/whitespace
block. No product test, install, lint/typecheck/i18n, browser or build now.

After ROOT grants execution, use the pinned toolchain environment and verify
Node 24 and pnpm 9.15.9 before product checks. The design shell lacks Node on
PATH, but read-only discovery found the existing Node/Corepack binaries below.
The repository `mise.toml` pins Node 24 and pnpm 9.15.9. Environment setup is
local to the execution shell:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24/bin:$PATH"
node --version
(cd apps && corepack pnpm --version)
```

If `apps/node_modules` is absent, run exactly one frozen install:

```bash
(cd apps && corepack pnpm install --frozen-lockfile)
```

The reported pnpm version must be exactly 9.15.9. Skip the install when
dependencies are present; do not delete/reinstall dependencies or retry a failed
install autonomously. An unavailable pinned toolchain checkpoints ROOT.

RED, before production changes (expected actual Vitest exit 1 from the named
content assertion; preserve the original handle until terminal):

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm exec vitest run --maxWorkers=1 --project=browser-locales hooks/domains/session/use-plan-draft.save-ack.test.tsx -t 'keeps newer typing after its own autosave acknowledgement')
```

GREEN, including every planned changed test suite and affected consumers:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm exec vitest run --maxWorkers=1 --project=browser-locales hooks/domains/session/use-plan-draft.save-ack.test.tsx components/task/task-plan-panel.save-ack.test.tsx components/task/task-plan-panel-draft.test.ts hooks/domains/session/use-task-plan.test.ts components/task/task-plan-panel-header.test.tsx components/task/task-plan-panel.session-switch.test.tsx components/editors/tiptap/tiptap-plan-editor.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm exec eslint --max-warnings 0 hooks/domains/session/use-plan-draft.ts hooks/domains/session/use-plan-draft.save-ack.test.tsx components/task/task-plan-panel.save-ack.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm run i18n:ratchet)
```

Retain every original native handle, actual command verdict, and terminal chunk;
wrapper exit 0 is not a product verdict. No overlapping commands under the
local-heavy lease. If minimum causal repairs change an existing test, rerun
that exact suite and add its path to affected lint and recorded changed-path
coverage; do not silently omit newly changed files.

Cheap spec/catalog/actual changed-path reference/whitespace checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' });
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' });
const changedFiles = [...new Set((tracked + untracked).split('\0').filter(Boolean))];
const documentPaths = [
  'docs/specs/tasks/requirements/documents.md',
  'docs/specs/tasks/system-design/plan-write-lifecycle.md',
  'docs/plans/preserve-plan-typing/plan.md',
  'docs/plans/preserve-plan-typing/task-01-save-ack-continuity.md',
];
const fileContents = Object.fromEntries(documentPaths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
git diff --check
git status --short
```

No new public-doc files are planned, so public-doc validators are unnecessary.
No broad verification, build, or Playwright command is authorized here.

## Files likely touched

Production ownership:

- `apps/web/hooks/domains/session/use-plan-draft.ts`

Permanent tests (new):

- `apps/web/hooks/domains/session/use-plan-draft.save-ack.test.tsx`
- `apps/web/components/task/task-plan-panel.save-ack.test.tsx`

Design/delivery records (the four-file design package):

- `docs/specs/tasks/requirements/documents.md`
- `docs/specs/tasks/system-design/plan-write-lifecycle.md`
- `docs/plans/preserve-plan-typing/plan.md`
- `docs/plans/preserve-plan-typing/task-01-save-ack-continuity.md`

Existing affected consumer suites above remain unchanged unless a minimum
fixture/type/lint adjustment is necessary and recorded. Do not edit production
`use-task-plan.ts`, the store, API, panel/header/editor markup, or localization
as optional cleanup.

## Dependencies

None. ROOT review and later explicit implementation delivery plus local-heavy
lease are execution prerequisites, not additional work orders.

## Risks

- Misclassifying an external update as own acknowledgement or retaining a
  historical content exemption; cover receipt expiry and differing external data.
- Old same-text success clearing newer size suppression; cover both completion
  orders with real latest-started store behavior.
- Task-ID-only guards accepting A-to-B-to-A stale closures; cover task view
  generation and legitimate background store publication separately.
- TipTap browser setup failing before the causal assertion; checkpoint exact
  infrastructure gaps instead of weakening selection/focus/identity evidence.

## Parallelism

`sequential`. Use this existing primary only.

## Results

Completed under ROOT's exclusive local-heavy lease on 2026-10-08 at base
`f61fd4e5fda79c0bb28f94e6af1e988898be9561`. Only `use-plan-draft.ts` changes
production behavior. Own snapshots are registered before dispatch, consumed
once when observed, and scoped to the attempt and task view. Newer typing and
the editor key survive while the real store advances its persisted baseline.

- Node 24.21.0 / pnpm 9.15.9: one frozen install, actual exit 0; original
  handle `51415` joined in terminal chunk `df11ac`.
- Permanent anchored RED before the hook change: actual exit 1; original
  `54679` joined `422d5b`. A reached the real store, then the assertion
  observed A replacing newer B. This was a causal regression failure.
- Scoped GREEN: 64 passing tests across the seven planned suites. The first
  run joined `78213` / `a12487`: 61 passed, three panel fixtures failed for
  missing providers. Real ToastProvider and TooltipProvider repaired that
  fixture; affected panel-only rerun joined `80122` / `c8f5f5`, exit 0,
  three passing cases. The intermediate missing-TooltipProvider attempt
  joined `31346` / `d809ad`, exit 1, and is not a behavioral failure.
- Real-hook coverage: 21 cases, including create/update, newer typing,
  unchanged success, external updates/deletion/receipt expiry, overlapping
  saves, retries, and task/null/round-trip isolation. After the duplicated
  test literal lint repair, affected hook-only rerun joined `88815` /
  `2e973a`, exit 0, 21 passing tests. Existing passing suites were not repeated.
- Actual panel/dynamic adapter/TipTap coverage: desktop, phone, and Ctrl+S
  preserve DOM/editor identity, text, focus and selection, then save B.
- Changed-file eslint: exit 0 (`16353` / `1adbab`) after one test-only
  duplicate-string repair. Prettier: exit 0 (`1d464e`).
- The first normal commit attempt joined `36350` / `5a9b50`, exit 1:
  formatting exposed a test-group line-limit warning. Splitting the describe
  group preserved all assertions and timings. The affected hook-only rerun
  joined `15808` / `8ba3fa`, exit 0, with 21 passing tests; final active-hook
  validation is recorded in the live task plan.
- Web typecheck: exit 0 (`67277` / `59341d`). Both i18n checks: exit 0
  (`1072` / `13ce6b`, `61767` / `7516aa`).
- Catalog: exit 0, 363 decisions / 1437 specifications (`bd6368`). All spec
  lint: exit 0 (`19e789`). Actual seven changed-path coverage: exit 0,
  `status=covered`, `errors=[]` (`f72121`).

All listed original handles are actually joined; runner receipts recorded
empty owned process groups at completion. Logs, actual exits and native
chunks are retained in `/tmp/kandev-child80-execution-20261008`; the live task
plan owns final resource return, publication, hosted gates and crash recovery.
No browser/build/full suites or proof archive replay/mutation occurred.
Publication and merge remain separate gates; this work order grants no merge.
