---
id: "01-scoped-list-publication"
title: "Scope and order automation list publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.12
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.13
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.14
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.15
system_design:
  - ../../specs/office/system-design/automations-settings-02.md
---

# Task 01: Scope and Order Automation List Publication

## Summary

Replace instance-local workspace-string list guards with workspace-scoped state
and request authority in the existing automations slice. Preserve ordinary
mutations and bounded legacy consumers, and prove the resulting settings list
through real hook/store and rendered page/table integration.

## In scope

- Scoped entries and store-owned atomic initial admission/refresh/settlement.
- Hook outputs, workspace/cache/null/lifecycle controls, shared and independent
  stores, current failure/recovery, obsolete result rejection.
- Breadcrumb scoped selection with legacy fallback; narrow mutation cache
  maintenance and secret stripping controls; flat invalidation references.
- Adapted permanent regression suites and unchanged existing consumer controls.
- Results/status synchronization in this work order and manifest; owning spec
  lifecycle reconciliation only after implementation matches the design.

## Out of scope

Backend runtime/DB/schema/API, trigger metadata, automation-run state, sidebar
cache consolidation, other writers, mutation/list or event coherence, layout,
copy/touch/navigation changes, public docs, browsers/build/E2E, broad suites,
delegation, new tasks/tabs/sessions, model switches, child30, or callback changes.

## Acceptance

1. Real scoped hook outputs satisfy AC-001.12/13 across cached A/B/A and both
   refresh completion orders, including latest empty/current failure/obsolete
   failure. No foreign row is visible, and obsolete settlements cannot clear
   current loading or change accepted rows.
2. All same-store/workspace consumers share initial requests and accepted
   state; explicit refresh from either wins across instances. Independent
   workspaces/stores, null selection, StrictMode, unmount/remount, failures and
   recovery satisfy AC-001.14/15 without stranded loading. Sequential mutations,
   caller responses, secret stripping, flat invalidation and unrelated state
   remain correct.
3. Adapted tests establish meaningful permanent RED before the production
   patch and GREEN after it. Real page/table integration shows scoped rows and
   navigation, with transport-only mocks. Every exact required verification
   completes successfully and its actual counts/status are recorded.

## Implementation sequence

1. Wait for ROOT's later explicit implementation INTERRUPT in this primary
   session. Read owning criteria/design and `apps/web/AGENTS.md`; invoke `/tdd`.
   Mark this order `in_progress`. Do not replay, modify, or remove ROOT's proof.
2. Use the conditional install prerequisite once if needed. Do not change
   lockfiles, configs, test-selection lists, or resource budgets.
3. Adapt the accepted proof into permanent real-provider regressions; add
   actual page/table integration in a separate file from existing mocked-hook
   tests. First run the RED command against unpatched production. Require
   collected tests, joined exit, and expected stale-row assertions; setup/no
   tests failures are not RED. Distinct automation fixtures, deferred requests,
   and real store are mandatory.
4. Implement the minimum slice/types/hook/breadcrumb changes in the paired
   design. Start guarded state before transport; finish success/failure under
   the generation check. Do not use a late unconditional `finally` write.
   Keep request lifetime in the store, not component-local state.
5. Complete the scenario matrix below and run the exact GREEN once. Run each
   remaining check sequentially, actually joining every handle. A new change,
   failure, or unresolved concern must justify any repetition. Record results;
   reconcile statuses and paired draft spec lifecycle if conformance is proven.
6. Continue already authorized delivery only after all task checks pass, using
   manifest and versioned task-plan gates. Resource failure checkpoints for
   ROOT direction; no automatic retries or scope expansion.

## Required scenarios

Use the real provider/store, production hook/slice, and deferred automation
transport. Do not mock `useAutomations`, `useAppStore`, the store or table in the
new integration suites. Cover behaviors rather than helper token mechanics.

- Current initial load positive: rows, loaded/loading, and transport count.
- Loaded A -> B pending -> cached A: immediate A rows and flags, no redundant A
  load, B settlement remains confined to B. Pending A -> B and null before
  settlement also expose no foreign rows, including the render before effects.
- Two refreshes after a baseline, each completion order: obsolete success never
  publishes, older settlement cannot clear newer loading, latest empty wins.
- Current failure with older success, and obsolete failure with newer success,
  each settlement order: baseline and flags remain owned by newest admission.
- Current initial failure empty/loaded/not loading, explicit recovery;
  refresh failure retains baseline and explicit refresh later replaces it.
- Two same-workspace consumers share one initial transport and loaded rows;
  refresh from different instances is ordered across their common store.
- Simultaneous A/B consumers and separate stores requesting the same workspace
  remain independent. Mixed current/foreign response rows retain only eligible
  rows, and null never fetches or exposes cached rows.
- Unmount initiating consumer with a sibling pending, settle and remount;
  unmount all consumers then settle and remount; StrictMode pending effect replay
  deduplicates. No request is left unresolved in test cleanup.
- Sequential create/update/remove/enable/disable update existing scoped caches;
  full create response retains one-time secret but neither scoped nor flat
  cache does. Trigger result unchanged; no partial loaded list created from a
  mutation. Trigger/run state sentinel values survive list actions. Legacy
  initial state still receives defaults; accepted list/mutation changes retain
  flat reference invalidation. No concurrency claim about mutation races.
- Scoped breadcrumb name remains A after B settles; authoritative empty A does
  not fall back to stale flat A; absent cache supports workspace-checked legacy
  seeded rows. Use real breadcrumb hook in the hook integration suite.
- Real page/table: A/B/A and late B, first loading and accepted empty state,
  A's unique row name/ID and navigation target under the real browser-history
  router. Existing `SettingsSaveProvider`/`TooltipProvider` stay real; seed
  minimal legitimate workspace state. Restore history/storage and settle all
  started transport promises before cleanup.

## Mobile and public documentation assessment

Pure state/data repair; no composition, copy, layout, touch, scrolling,
navigation or breakpoint changes. Apply the mobile-parity unit/component
exception. No ASCII preview, browser/build or Playwright run is part of this
order. Existing workspace automation public guidance remains accurate; owning
internal specs/plans are the documentation change.

## Verification

Run from repository root in `/bin/bash`, `login:false`. Apply this environment
in each command invocation:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
```

Only after explicit implementation release, at most one conditional install:

```bash
if [ ! -d apps/node_modules ] || [ ! -x apps/web/node_modules/.bin/vitest ] || [ ! -x apps/web/node_modules/.bin/eslint ]; then
  (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

Permanent RED before production edits (adapted suites, no synthetic replay):

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run hooks/domains/settings/use-automations.test.tsx components/automations/automations-list-page.integration.test.tsx --maxWorkers=1)
```

Additional sequential-cache RED before its mutation adaptation:

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run lib/state/slices/automations/automations-slice.test.ts --maxWorkers=1)
```

Targeted GREEN and existing bounded consumer controls:

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run hooks/domains/settings/use-automations.test.tsx lib/state/slices/automations/automations-slice.test.ts components/automations/automations-list-page.integration.test.tsx components/automations/automations-list-page.test.tsx components/automations/automations-table.test.tsx components/settings/settings-layout-client.test.tsx components/settings/use-settings-breadcrumbs.plugin.test.ts hooks/domains/sidebar/use-sidebar-shortcut-catalog.test.ts --maxWorkers=1)
(cd apps/web && corepack pnpm@9.15.9 exec eslint hooks/domains/settings/use-automations.ts hooks/domains/settings/use-automations.test.tsx lib/state/slices/automations/types.ts lib/state/slices/automations/automations-slice.ts lib/state/slices/automations/automations-slice.test.ts components/settings/use-settings-breadcrumbs.ts components/automations/automations-list-page.integration.test.tsx --max-warnings=0)
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/automation-list-publication
```

Documentation coverage preflight (prospective paths in design; actual paths
after implementation) uses the repository's real evaluator, without creating
tests or mutating files:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/office/requirements/automations-settings.md',
  'docs/specs/office/system-design/automations-settings-02.md',
  'docs/plans/automation-list-publication/plan.md',
  'docs/plans/automation-list-publication/task-01-scoped-list-publication.md',
];
const sources = [
  'apps/web/hooks/domains/settings/use-automations.ts',
  'apps/web/lib/state/slices/automations/types.ts',
  'apps/web/lib/state/slices/automations/automations-slice.ts',
  'apps/web/components/settings/use-settings-breadcrumbs.ts',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...docs, ...sources].map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
```

Design additionally runs `python3 scripts/lint-spec-files.test.py` once as
required by `/spec`. No package command/install, production/permanent test edit,
staging, commit, browser/build/E2E or broad verification during design. Later
tests, typecheck and i18n are the only required heavier local checks, sequential
with joined handles. Preserve normal active commit hooks and hosted gates.

## Files likely touched

- `apps/web/hooks/domains/settings/use-automations.ts`
- `apps/web/hooks/domains/settings/use-automations.test.tsx` (new)
- `apps/web/lib/state/slices/automations/types.ts`
- `apps/web/lib/state/slices/automations/automations-slice.ts`
- `apps/web/lib/state/slices/automations/automations-slice.test.ts` (new)
- `apps/web/components/settings/use-settings-breadcrumbs.ts`
- `apps/web/components/automations/automations-list-page.integration.test.tsx` (new)
- `apps/web/AGENTS.md` (required scoped cache ownership guidance)
- `docs/specs/office/requirements/automations-settings.md`
- `docs/specs/office/system-design/automations-settings-02.md`
- `docs/plans/automation-list-publication/plan.md`
- `docs/plans/automation-list-publication/task-01-scoped-list-publication.md`

Existing page/table/editor/sidebar/trigger/run source and named control tests
are read-only inputs. If a required finding demands edits beyond these paths,
save the evidence and checkpoint for ROOT direction. Do not patch foreign work
or rewrite unrelated legacy specifications.

## Dependencies

None. Existing React/Zustand/Immer/Vitest/testing-library and browser-history
router suffice. No new package, lockfile, configuration or hydration migration.

## Risks

Shared loading/cache behavior must be atomic across instances. Legacy flat
items are an invalidation/compatibility projection, never the scoped data
source. Store-owned requests intentionally can settle after unmount into their
own workspace cache. Snapshot retention lasts for the store lifetime, and
concurrent mutation/list or backend-event coherence is excluded.

## Parallelism

`sequential`. Same primary session; no workers, tasks, tabs or sessions.

## Inputs

- [Owning requirements](../../specs/office/requirements/automations-settings.md),
  REQ-OFFICE-AUTOMATIONS-SETTINGS-001 / AC-001.12 through AC-001.15.
- [Owning design](../../specs/office/system-design/automations-settings-02.md#workspace-list-publication).
- [Manifest](plan.md): supplied evidence, consumer inventory, resource/delivery
  gates and explicit exclusions.
- Existing `useWorkspaceAutomations` and store-owned trigger metadata pattern.
- ROOT's read-only proof/receipt; adapt permanent tests without replay or removal.

## Results

Implemented in this primary session on 2026-10-05 after ROOT's explicit review
and release. All Node-heavy checks used Node 24.21.0,
`NODE_OPTIONS=--max-old-space-size=4096`, and tests used `--maxWorkers=1`.
Every started command handle was actually joined before the next heavy check.

- One conditional frozen pnpm 9.15.9 install: joined handle 50266, exit 0,
  923 packages reused, 1.7 seconds; no lockfile changes.
- Permanent two-file RED: joined 59727, exit 1, 23 collected tests. Fifteen
  meaningful assertion failures included the real B row rendered under A and
  newest refresh overwritten; seven positive controls passed. One incomplete
  sibling transport mock caused a TypeError (not RED); its promise fallback
  was corrected. Log: `/tmp/kandev-child32-permanent-red.log`.
- Narrow cache RED: joined 89873, exit 1, three collected assertion failures,
  including ordinary create absent from its seeded workspace cache. Log:
  `/tmp/kandev-child32-cache-red.log`.
- Exact eight-file GREEN: joined 27885, exit 0, eight files / 55 tests passed,
  11.78 seconds. Log: `/tmp/kandev-child32-green.log`.
- Focused seven-file ESLint initially found three new test-only warnings.
  Splitting a describe block and naming shared fixture/order strings resolved
  them. Changed hook suite joined 1575, exit 0, 23 tests; exact focused lint
  joined 13731, exit 0. Assertions and production code were unchanged.
- Required `pnpm run typecheck` initially found circular inference in the new
  page-test cleanup helper. An explicit promise/resolve type resolved it.
  Changed page suite joined 10189, exit 0, two tests; page-only ESLint exit 0.
  Final required typecheck joined 30212, exit 0 with no diagnostics.
- Required `pnpm run i18n:check` joined 31860, exit 0; key/catalog, Trans,
  plural, module-scope, em-dash and non-JSX gates passed. Existing orphan-key
  advisory is unchanged. Required `pnpm run i18n:ratchet` joined 54779,
  exit 0; all four changed production files clean, allowlist intact.

Only the four production paths, three new focused test files, four-document
package and scoped frontend ownership guide changed. No backend, runtime,
hydration, trigger/run-state, public copy,
browser/build/E2E, lockfile/config, delegate/task/tab/session, or child30 edits.
ROOT's proof remains read-only and unreplayed. Initial-state compatibility uses
an optional public `byWorkspace` field with an empty default map. The flat row
array is copied separately from scoped rows to avoid duplicate mutation writes.
Mobile parity uses the pure data/state exception with real rendered integration.
The owning requirement and Part 2 design are now active/current; Part 1 remains
untouched migration material.

Final documentation verification passed: `python3 scripts/list-docs.py validate`
(351 decisions / 1,348 specs), `python3 scripts/lint-spec-files.py --all`,
`git diff --check`, plan-directory status, and the exact repository
`validateCoverage` preflight over owning documents and actual production paths
(`covered`, no errors). No active local handles.
Delivery remains gated on normal active hooks, required terminal hosted checks,
current-head full authenticated semantic coverage, every finding disposition,
normal expected-head squash, independently verified merge and joined owned
cleanup. Task completion is not inferred from local checks alone.

PR review correction: the scoped frontend guide now records the workspace cache,
store-owned generations and legacy compatibility boundary, as required by the
root engineering guide. ROOT explicitly authorized this documentation-only
scope addition. Production and tests are unchanged; passing product checks are
not replayed. The guide stays at its existing 300-line boundary. Harness unit
tests (19), harness lint (201 files), spec-linter tests (36), spec lint,
documentation catalog (351 decisions / 1,348 specs), whitespace and the targeted
harness hook passed in joined handle 55535, exit 0. Exact commands:

```bash
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
wc -l -w -c apps/web/AGENTS.md
pre-commit run harness-lint --files apps/web/AGENTS.md
```

Guide counts: 300 lines, 4,452 words, 36,714 bytes (previously 300 lines,
4,384 words, 36,013 bytes). It adds one local cache boundary to existing key
state-path guidance; no common workflow or generic rule changes.
The sidebar optimization suggestion is outside this repair: the reviewed design
retains the flat reference invalidation signal and the sidebar's independent
workspace-scoped fetch.
