---
id: "01-preserve-initial-publication"
title: "Preserve initial executor catalogue publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-INITIAL-CATALOGUE-001
acceptance_criteria:
  - AC-EXECUTORS-INITIAL-CATALOGUE-001.1
  - AC-EXECUTORS-INITIAL-CATALOGUE-001.2
  - AC-EXECUTORS-INITIAL-CATALOGUE-001.3
  - AC-EXECUTORS-INITIAL-CATALOGUE-001.4
  - AC-EXECUTORS-INITIAL-CATALOGUE-001.5
  - AC-EXECUTORS-INITIAL-CATALOGUE-001.6
system_design:
  - ../../specs/executors/system-design/initial-catalogue-loading.md
---

# Task 01: Preserve initial executor catalogue publication

## Summary

Fence only admitted executor initial reads against live catalogue publication.
Prove selection continuity through real hook/provider/transport/registered
events/options/selector and retain ordinary initial-load and warm-state behavior.

## In scope

- Implement [invocation-local publication](../../specs/executors/system-design/initial-catalogue-loading.md#invocation-local-publication-fence)
  and its admission, failure, loaded and cleanup sections.
- Independently author every case in the [test matrix](plan.md#tests-and-acceptance-coverage),
  with caught failures and current inventory assertions.
- Update all four artifacts' statuses/results after actual scoped success.

## Out of scope

No other executor/profile writer, useRefreshProfiles remigration, backend,
events, store action redesign, global versions/resources, auth/lifetime,
agent-list, selection-default or launch redesign. No copy/layout/interaction,
new dependencies, browser/build/full-suite checks, protected proof access or
foreign-resource cleanup. No implementation during the design turn.

## Acceptance

1. The new suite independently reaches causal loss after held successful GET
   with the selected live profile usable before settlement, and the TWO ordinary
   initial-GET/prepopulated controls passing, zero setup/unhandled errors. After
   correction every AC .1 through .6 case passes across the actual pipeline.
2. Every current owner/profile entry, value and order survives contested
   settlement; removed rows stay absent. Catch preservation has independent
   evidence before its necessary bounded change. Gates/loaded/ordinary empty
   results and agent discovery compatibility retain their existing meanings.
3. Exact scoped checks and complete actual-path coverage pass, all original
   handles/results are retained and joined, foreign resources remain untouched,
   and the manifest/statuses distinguish design, implementation and delivery.

## Implementation sequence

1. Wait for ROOT actual-file review, later explicit implementation interrupt
   and exclusive GLOBAL heavy lease. Read all four files and scoped web/TDD
   guidance. Re-resolve exact remote main/candidate refs and source; preserve
   foreign changes. Mark this order in_progress only after release.
2. Check dependencies read-only. If absent, install once with the frozen command
   below; do not alter the lockfile or reuse/clean another worktree's deps.
3. Independently author the first three matrix cases (held successful GET and
   TWO ordinary controls) without reading/copying the protected
   candidate. Render actual StateProvider, useSettingsData and selected React
   draft. Import actual real API adapter/client, event registrations,
   useExecutorProfileOptions and ExecutorProfileSelector. Isolate external
   fetch and required platform capabilities only. Provide faithful settled
   ancillary agent/probe transport, or seed their unrelated loaded boot state;
   do not mock internal agent/executor APIs.
4. Start empty/unsettled, verify admitted GET/cache and hold its external
   successful Response. Deliver registered owner.created then profile.created
   payloads lacking profile owner display fields. Use the exact production
   flatten/fallback projection. Select that eligible live option through the
   actual selector, keep its draft ID, and assert current label/metadata/options
   before releasing the stale response. Assert entire state, actual options,
   disabled eligibility and selected trigger afterward. Test mixed current
   owner/profile changes and order, and add/remove back to empty.
5. Run the three-case RED before any production edit: ONE causal failure and
   TWO passing ordinary initial GET/warm catalogue controls. Record reached
   causal assertions and zero unhandled/setup errors. Then independently author
   remaining cases in the same bounded suite. Prove failure with live events before
   changing catch; empty failure is a control. Setup/transport/collection or
   resource failures are not causal RED; checkpoint ROOT before alternatives.
6. Add useAppStoreApi for fresh admission and the private sticky observer.
   Subscribe before real listExecutors. Admit only existing enabled empty
   unsettled state; any observed membership invalidates this read permanently.
   Successful uncontested GET publishes synchronously; contested/failure leaves
   current catalogue intact. Dispose in invocation finally, keep existing
   loaded finalization and effect fast path, and never detach observation merely
   because live arrival reruns the effect. No global registry or new policy.
7. Run GREEN and affected existing hook tests serially, then scoped lint,
   typecheck, full i18n and final staged ratchet after adding new TS helpers.
   No passing suite replay absent a new failure/change. Keep zero unhandled
   errors; settle held promises/unmount/restore transport and subscriptions.
8. Run lightweight specification/link/whitespace and independent coverage
   commands; inventory ALL tracked, cached and untracked NUL paths without
   allowlist filtering. Compare expected paths only after capturing actual
   inventory. Preserve/report foreign unexpected paths rather than hiding them.
   Actual production coverage must be covered/ok/errors=[] with ONE order.
9. Mark order done, manifest implemented and paired draft specs active/current
   only after all scoped success. Follow the manifest's already-authorized later
   ordinary hooks/commit/push/ready-PR/hosted gates. No merge without separate
   ROOT expected-head grant; no extra END/WFI approval ceremony.

## Verification

These are FUTURE implementation commands, rooted from repository root, under
ROOT release/lease. Do not run now. Use the already installed Node 24/pinned
pnpm 9.15.9 toolchain; inspect its location rather than installing a new runtime.
If needed for this host:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
```

Only when apps dependencies are absent, once after release/lease:

```bash
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

RED, then GREEN with the same original causal-suite command after correction:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run hooks/domains/settings/use-settings-data-executor-publication.test.tsx --maxWorkers=1 --no-file-parallelism)
```

Affected existing discovery/gate compatibility, one worker/files sequential:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run hooks/domains/settings/use-settings-data.test.tsx --maxWorkers=1 --no-file-parallelism)
```

Scoped static checks (add every actual new TS helper/test-helper to ESLint):

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/settings/use-settings-data.ts hooks/domains/settings/initial-executor-catalogue-read.ts hooks/domains/settings/use-settings-data-executor-publication.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
```

Stage every new TS helper and actual owned implementation file using explicit
paths before final `i18n:ratchet`; preserve foreign staging. Test copy belongs
in recognized test files, not production/helper literals with bypass markers.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet)
```

Lightweight checks also permitted during design:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
git status --short
```

Inventory/preflight uses Node builtins only, no dependencies/product tests.
Use the inspected implementation base below only if still authoritative; after
rebase/main changes update it to the exact reviewed base, preserving history.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage, parseFrontmatter } = require('./.github/scripts/pr-docs.cjs');
const path = require('node:path').posix;
const base = '53a00c274a677e579695c8d90052755372787c95';
const readPaths = args => cp.execFileSync('git', args, {encoding:'utf8'}).split('\0').filter(Boolean);
const actual = [...new Set([
  ...readPaths(['diff','--name-only','-z',base]),
  ...readPaths(['diff','--cached','--name-only','-z']),
  ...readPaths(['ls-files','--others','--exclude-standard','-z']),
])];
const docs = [
  'docs/specs/executors/requirements/initial-catalogue-loading.md',
  'docs/specs/executors/system-design/initial-catalogue-loading.md',
  'docs/plans/executor-initial-catalogue-preservation/plan.md',
  'docs/plans/executor-initial-catalogue-preservation/task-01-preserve-initial-publication.md',
];
const expected = [...docs,
  'apps/web/hooks/domains/settings/use-settings-data.ts',
  'apps/web/hooks/domains/settings/use-settings-data-executor-publication.test.tsx',
  'apps/web/hooks/domains/settings/initial-executor-catalogue-read.ts',
];
const fileContents = Object.fromEntries(docs.map(p => [p,fs.readFileSync(p,'utf8')]));
const changedFiles = actual.map(filename => ({filename,status:fs.existsSync(filename)?'modified':'removed'}));
const coverage = validateCoverage({changedFiles,fileContents});
const order = parseFrontmatter(fileContents[docs[3]]).data;
const design = parseFrontmatter(fileContents[docs[1]]).data;
const manifest = parseFrontmatter(fileContents[docs[2]]).data;
const resolve = (owner, ref) => path.normalize(path.join(path.dirname(owner),ref));
const referenceErrors = [];
const definitions = [...fileContents[docs[0]].matchAll(/\*\*(AC-[A-Z0-9-]+\.\d+):\*\*/g)].map(m=>m[1]);
if (order.acceptance_criteria.length !== definitions.length || definitions.some(ac=>!order.acceptance_criteria.includes(ac))) referenceErrors.push('ALL AC definitions must be covered by the ONE order');
for (const req of order.requirements) {
  if (!fileContents[docs[0]].includes('### '+req+':') || !design.requirements.includes(req) || !manifest.requirements.includes(req)) referenceErrors.push('Missing requirement mapping: '+req);
}
for (const ref of order.system_design) {
  const target = resolve(docs[3],ref);
  if (!fileContents[target] || !manifest.system_design.some(r=>resolve(docs[2],r)===target)) referenceErrors.push('Missing design mapping: '+ref);
}
if (resolve(docs[3],order.plan)!==docs[2]) referenceErrors.push('Missing sibling plan');
const planned = validateCoverage({changedFiles:[...changedFiles,{filename:expected[4],status:'modified'}],fileContents});
const orderCount = actual.filter(p => /^docs\/plans\/[^/]+\/task-\d{2}-[^/]+\.md$/.test(p)).length;
const unexpected = actual.filter(p => !expected.includes(p));
console.log(JSON.stringify({actual,unexpected,orderCount,referenceErrors,coverage,planned_trigger_validation:planned},null,2));
if (!coverage.ok || coverage.errors.length || referenceErrors.length || orderCount !== 1 || unexpected.length) process.exitCode = 1;
if (!planned.ok || planned.status !== 'covered' || planned.errors.length || planned.workOrders.length !== 1) process.exitCode = 1;
if (coverage.requiresCoverage && (coverage.status !== 'covered' || coverage.workOrders.length !== 1)) process.exitCode = 1;
NODE
```

Expected optional private helper/test-helper paths must be added to the
comparison and exact checks only if actually needed; never filter them out.
Docs-only actual classification is exempt and does not prove production
coverage. Independently validate ONE order, ALL AC definitions and design
references even during exemption. Check changed Markdown links resolve.
Retain full outputs/exit results, all returned session IDs and actual joins;
no invented join for directly completed commands. Require current owned absence
before heavy RETURN. No broad suite/browser/build for this state/data scope.

## Files likely touched

- `apps/web/hooks/domains/settings/use-settings-data.ts` (executor effect only).
- New `apps/web/hooks/domains/settings/use-settings-data-executor-publication.test.tsx`.
- Only if size limits require it: private adjacent
  `initial-executor-catalogue-read.ts` and/or
  `use-settings-data-executor-publication.test-helpers.tsx`; record exact paths,
  lint and semantic tests in the same order before completion.
- These four requirement/design/manifest/work-order files.

Read-only consumers: StateProvider/store/action, HTTP settings API/client,
registered executor/profile handlers, real options/selector and task fallback
mapping. Existing `useRefreshProfiles` stays unchanged.

## Dependencies

None. Execution prerequisites are ROOT actual-file review, later explicit
implementation interrupt and GLOBAL exclusive heavy grant. No additional
session, worker or persistent task is authorized.

## Risks

See [manifest risks](plan.md#risks). Any material unexpected source/actor impact,
three repeated check failures, timeout/resource/transport/setup failure or
foreign ownership conflict checkpoints ROOT before changing scope or rerunning.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/executors/requirements/initial-catalogue-loading.md),
  REQ-EXECUTORS-INITIAL-CATALOGUE-001 and ALL AC .1 through .6.
- [Design](../../specs/executors/system-design/initial-catalogue-loading.md),
  especially admission, local fence, cleanup and consumer/mobile boundaries.
- [Evidence and operational gates](plan.md), scoped web guidance, TDD skill,
  mobile-parity state/data exception and docs-maintainer impact audit.
- Existing `use-settings-data.test.tsx` for store/hook fixtures only; its
  internal API mocks are insufficient evidence for the new executor regression.

## Results

Historical design package was `DESIGN_READY`: catalogue validation
(369 decisions, 1513 specs), specification lint, Markdown links/anchors and
tracked/cached/untracked whitespace passed. Full NUL inventory contains exactly
the FOUR owned new docs and no unexpected/foreign paths. Independent ONE-order,
ALL-six-AC and design/manifest references passed. Actual coverage is docs-only
`exempt`, errors=[]; separately labelled planned-trigger validation is
`covered`, ONE order, errors=[], and is not actual implementation coverage.

Design receipt originals: `d18ea4` and `92d257` exit 0 (catalogue/spec/status,
links/whitespace); `f9fbf0` exit 0 (actual inventory and independent coverage);
`7a6307` exit 0 (link anchors). Node was absent from the default shell PATH in
`36b080`, exit 1; using the existing Node 24.21.0 path resolved that lightweight
preflight without installation. All commands completed directly; no native
session handle or inferred join exists.

ROOT subsequently completed full actual-file review and sent the later explicit
implementation release with exclusive GLOBAL local-heavy lease 110. The one
work order is implemented and locally validated; normal hooks/publication and
hosted review/check evidence remain separate delivery gates.

All originals are archived at `/tmp/kandev-initial-executor-24418c94`, including
full native tool returns, complete logs and timeout-wrapper group receipts.

- Frozen apps install original `522349/session56779`, actual join `169549`,
  exit 0. Dependencies were absent; installed once with frozen lockfile, unchanged.
- Independent primary RED `e0d16d/session97017`, join `9273ac`, exit 1:
  ONE causal failure/TWO ordinary controls passed. Actual current owner/profile,
  option and selected trigger were visible while real HTTP GET was held;
  late success lost inventory/option and reverted the trigger to placeholder.
  The selected draft ID remained, demonstrating why ID alone misses the bug.
  Zero unhandled/setup errors. No protected proof source accessed.
- Separate rejection RED `2ab75a/session63811`, join `b5c26c`, exit 1:
  actual HTTP 500 through the real adapter independently cleared live inventory,
  option and selected trigger. Zero unhandled/setup errors. Production was
  unchanged until both REDs qualified.
- Minimal initial-effect/private-helper correction. `green-new`
  `c09030/session8499`, join `0b42d8`, exit 0: 13 new cases passed. Existing
  compatibility `503a92/session17071`, join `ae9ca8`, exit 0: 6 passed.
- Initial scoped ESLint `527d35/session51630`, join `efff2b`, exit 1:
  oversized describe callback only. Split test groups, without changing case
  semantics; corrected `952782/session10118`, join `3e3bc2`, exit 0.
- Initial typecheck `612135/session30530`, final join `430f6c`, exit 2:
  fixture registry circular type inference. Replaced only its type with an
  explicit read/unexpected-path shape. Affected final suite
  `ba3d69/session4420`, join `d50faf`, exit 0: all 13 passed; existing six
  controls were not replayed. Formatting changed whitespace only.
- Final changed ESLint `65dbdd/session63266`, join `32e953`, exit 0.
  Final typecheck `d04725/session57293`, join `ee3b4e`, exit 0.
- Full i18n `842068/session50908`, join `d721ca`, exit 0 (existing orphan-key
  warnings only). New source/helper/test and docs were explicitly staged before
  final ratchet `1ed3b4/session95713`, exit 0; exact final native join retained
  in `i18n-ratchet.join.native.json`.
- Actual unfiltered NUL inventory and coverage `8a43fb`, direct exit 0:
  seven owned paths, unexpected=[], production covered, ONE order, ALL six ACs,
  referenceErrors=[] and errors=[]. Final docs/link/whitespace checks follow
  this results/status update before the normal commit.

Every yielded session above was polled to its actual original completion.
All timeout-owned command groups were absent at join; directly completed
commands have actual exit receipts, without invented session handles.
Protected/foreign resources were preserved. No broad suite, browser or build
was run for this data-only mobile exception; public executor instructions and
screenshots remain accurate. Parent progress callback queue_full was optional
and non-gating; no alternate task/session was created.
