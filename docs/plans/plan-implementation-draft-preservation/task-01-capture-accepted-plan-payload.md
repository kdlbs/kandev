---
id: "01-capture-accepted-plan-payload"
title: "Use captured accepted-payload clearing for plan implementation"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.3
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.4
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.6
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 01: Use Captured Accepted-Payload Clearing

## Summary

Capture the initiating composer payload and accepted-payload callback before
starting either implementation request. Successful completion may clear only
that matching committed visit; newer or successor drafts and no-ref toolbar
drafts survive. Use independently authored actual shared composer regressions.

## In scope

- Both action hooks and removal of their direct editor/storage clear writes.
- Two new rendered test suites and at most one local fixture helper. Mount
  actual ChatInputArea with valid production state/providers and real TipTap;
  click real desktop/phone Implement and fresh menu controls. Defer only external
  WS/fetch. Include the no-ref PlanPanelHeader path with a separate real composer.
- Existing isolated runner/fresh tests: update obsolete raw-clear expectations,
  retain request/guard/success/error controls. Those are supplementary evidence.
- Package status/results and the paired specification clarification.

## Out of scope

The manifest's exclusions and phase/resource/delivery barriers apply verbatim.
No new coordinator, handle API, backend/storage schema, transport, navigation,
plan-mode/start/activation policy, context cleanup, copy or layout changes.
No protected-proof reuse or internal hook/editor/store/storage mocks in the new
regressions. No generic broad verify/QA/review task.

## Acceptance

1. Both paths settle through the captured original accepted-payload callback
   and unaugmented raw text/attachment snapshot, with no mutable-ref or direct
   storage clear fallback. No-ref actions cannot clear browser-tab drafts.
2. The manifest's new/unchanged/rejected, rich/attachment, captured owner/editor,
   session/replacement/unmount/reopen and fresh/destination scenarios pass actual
   live editor, saved text/rich JSON/attachments and restoration assertions.
3. Existing request identities, UUID, payload/timeout/readiness truth and
   successful marker, plan-mode, primary/profile/executor and activation behavior
   retain their controls; all listed exact checks pass with actual receipts.

## Execution sequence

1. Wait for a later explicit ROOT implementation interrupt in this same primary
   and exclusive heavy lease. Re-read actual artifacts/user edits and mark this
   task in_progress. Do not treat this file as release.
2. Check Node24.21.0 PATH, pnpm9.15.9 and dependency presence. If absent, perform
   exactly one pinned frozen install from apps; no cache wipe or foreign cleanup.
3. Independently author the two suites and real production fixture. Never copy,
   import, execute, edit or release the protected candidate. Seed valid store
   readiness so external transport requests, not setup errors, explain outcomes.
4. Run the exact RED command before production changes. Prove causal newer
   actual editor loss and directly evaluate persisted preservation too; record
   controls separately. Never relabel fixture/import/setup failure as product RED.
5. In each hook capture the original handle/callback and raw payload before the
   first await. After existing successful side effects invoke that captured
   callback only. Remove competing direct null writes. Missing callback means
   no clear, not operation failure. Do not move fresh activation or augment the
   comparison payload. Existing visit/payload ownership remains authoritative.
6. Update affected isolated mocks/expectations and run exact serial GREEN gates.
   Routine affected fixture/lint repairs are allowed. Timeout/resource/transport/
   unknown/out-of-scope failures checkpoint ROOT before alternatives/retry.
7. Record real native starts/full responses, every session handle, actual terminal
   chunk/exit, wrapper PID/PGID/start identity, wait/reap and fresh owned absence.
   Join all originals; never infer pass from output or duplicate runs. Update
   Results and manifest only after required evidence; return heavy explicitly.
8. Normal commit/push/ready PR and hosted wait/review/merge follow the manifest's
   standing gates and separate ROOT merge grant. Do not end at partial delivery
   after an implementation release unless a real gate blocks the task.

## Verification

All commands run serially from repo root in Bash, login=false, after release.
Each needs its own task-owned log and process receipt. Use Node4GiB and one Vitest
worker. The 180s Vitest limit is GNU wall time with kill10; original native
handles must be joined beyond that grace. Checkpoint ROOT on resource/timeout/
transport/unknown failures before any retry, replacement command or foreign kill.
Never overlap another local-heavy owner.

Environment and the conditional single install:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
# Only if apps/node_modules is absent; confirm pnpm reports 9.15.9 first.
(cd apps && timeout --kill-after=10s 10m pnpm install --frozen-lockfile)
```

RED, before any production edit:

```bash
(cd apps/web && timeout --kill-after=10s 180s pnpm exec vitest run --maxWorkers=1 hooks/domains/kanban/use-plan-implementation-draft.test.tsx hooks/domains/kanban/use-implement-fresh-draft.test.tsx)
```

GREEN and the affected compatibility controls:

```bash
(cd apps/web && timeout --kill-after=10s 180s pnpm exec vitest run --maxWorkers=1 hooks/domains/kanban/use-plan-implementation-draft.test.tsx hooks/domains/kanban/use-implement-fresh-draft.test.tsx hooks/domains/kanban/use-plan-actions.runner.test.ts hooks/domains/kanban/use-implement-fresh.test.ts hooks/domains/kanban/use-plan-actions.test.ts components/task/chat/use-chat-input-state-accepted-payload.test.ts components/task/chat/chat-input-draft-ownership.test.tsx components/task/task-plan-panel-header.test.tsx)
(cd apps/web && timeout --kill-after=10s 180s pnpm exec eslint --max-warnings=0 hooks/domains/kanban/use-plan-actions.ts hooks/domains/kanban/use-implement-fresh.ts hooks/domains/kanban/use-plan-implementation-draft.test.tsx hooks/domains/kanban/use-implement-fresh-draft.test.tsx hooks/domains/kanban/plan-implementation-draft.test-helpers.tsx hooks/domains/kanban/use-plan-actions.runner.test.ts hooks/domains/kanban/use-implement-fresh.test.ts)
(cd apps/web && timeout --kill-after=10s 10m pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 5m pnpm run i18n:check)
(cd apps/web && timeout --kill-after=10s 5m pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/plan-implementation-draft-preservation
```

If the single optional fixture helper is needed, add its exact path to ESLint
and Results. If causal immediate glue is required, checkpoint scope, include
its actual affected test/lint paths and record why. Do not broaden test suite or
weaken assertions/gates for convenience. No browser/build/full suite/backend/PG.

Cheap standalone documentation coverage preflight (also permitted at design
time; representative future source paths do not constitute implementation proof):

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/plan-implementation-draft-preservation/plan.md',
  'docs/plans/plan-implementation-draft-preservation/task-01-capture-accepted-plan-payload.md',
  'docs/specs/ui/requirements/session-refresh-efficiency.md',
  'docs/specs/ui/system-design/session-refresh-efficiency.md',
];
const sources = [
  'apps/web/hooks/domains/kanban/use-plan-actions.ts',
  'apps/web/hooks/domains/kanban/use-implement-fresh.ts',
];
const result = validateCoverage({
  changedFiles: [...docs, ...sources].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors?.length) process.exitCode = 1;
NODE
```

For delivery substitute all actual changed paths, preserving linked docs.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-plan-actions.ts`
- `apps/web/hooks/domains/kanban/use-implement-fresh.ts`
- `apps/web/hooks/domains/kanban/use-plan-implementation-draft.test.tsx` (new)
- `apps/web/hooks/domains/kanban/use-implement-fresh-draft.test.tsx` (new)
- `apps/web/hooks/domains/kanban/plan-implementation-draft.test-helpers.tsx`
  (optional new real-fixture helper only)
- `apps/web/hooks/domains/kanban/use-plan-actions.runner.test.ts`
- `apps/web/hooks/domains/kanban/use-implement-fresh.test.ts`
- Paired requirement/design and this manifest/work order.

Existing ChatInputArea, shared state/visit/container/editor, desktop/mobile toolbar
and PlanPanelHeader are inspection and real test dependencies. No production
edits to them are indicated by current causal evidence.

## Dependencies

None beyond the delivered shared composer API. Operational gates are later ROOT
implementation authorization and exclusive heavy release. Read the manifest's
identity, question barrier, task plan editing, delivery and final-action rules.

## Risks

Captured callback versus newly fetched handle, raw versus augmented prompt,
independent storage writers, late fresh activation, and mocked-editor false
confidence. See manifest evidence limitations and scenario matrix.

## Parallelism

`sequential`. Same primary only; no agents/tasks/sessions/tabs/model switches.

## Inputs

- [Owning requirement](../../specs/ui/requirements/session-refresh-efficiency.md),
  `004.3`–`.6` and implementation-action scope paragraph.
- [Paired design](../../specs/ui/system-design/session-refresh-efficiency.md),
  Composer draft settlement ownership / Plan implementation action settlement.
- [Manifest](plan.md), complete inventory, test matrix and standing gates.
- Root/web/chat AGENTS, local fix/plan/mobile-parity skills; load TDD at release.
- The two action hooks, real shared composer/imperative callback, launch service,
  API and request contracts, existing isolated and rendered test patterns.
- ROOT's read-only proof receipt and already inspected protected source; no reuse.

## Results

ROOT released implementation after actual package review in this same primary.
ROOT subsequently granted exclusive global local-heavy after qualifying CHILD87's
corrective RETURN (original ROOT terminal `6334fa`, exit 0). Task 01 is now in full
implementation: one conditional frozen pnpm9.15.9 install, independent first-party
permanent RED, then production correction and exact serial gates. At lease receipt
no install/product checks or production correction had run; dependencies were
absent. Corepack resolves the existing apps packageManager pin to pnpm9.15.9.
Meaningful permanent RED must precede production edits. Separate merge authority
remains pending; all receipt, heavy RETURN and delivery gates still apply.
Design receipts and history remain intact in the manifest/platform task plan.


Independent permanent RED is qualified: original `red-03` native `740a92`,
session `13076`, actual terminal `7e395a`, exit 1, 15 causal failures and 6
passing controls, with no unhandled errors or unexpected transports. Initial
red-01/02 were fixture repair runs only. Direct persisted content assertions
were reached independently, without claiming the protected proof's unreached
assertion. The two-hook correction followed RED; no immediate glue was needed.

Exact eight-suite green-03 (`4a617f` / `90718` / `892181`, exit 0) passed 96
tests. Scoped ESLint02 passed after fixture/function grouping repairs.
Typecheck02 passed after correcting three fixture type errors; i18n checks and
ratchet passed. Final GREEN after those fixture edits and remaining cheap gates
are pending. The only extra test dependency is the listed local fixture helper.

Public guidance: five lines added to the existing tasks-and-workflows plan
how-to. Mobile parity uses the state/data-only exception with actual desktop
1280px/phone390px action tests; no layout, touch, navigation or visual changes.
Raw logs, original native responses and wrapper wait/reap/group checks are under
`/tmp/kandev-child88-plan-draft-implementation-20261008/`. Publication, hosted
review and separate ROOT merge authorization remain pending.

Final local validation passed: exact GREEN04 `d0f863` / session13636 /
terminal `ec6615`, exit0, 96 tests; final affected helper ESLint `bfde47`, exit0;
typecheck02 `f27494`, exit0; i18n check `1dea5b` and ratchet `d5a728`, exit0;
catalog `59e001`, spec lint `28147c`, reference coverage `1d110d` and whitespace
`011ee2`, all exit0. Coverage evaluated all 12 actual changed paths, with
`covered`, `ok:true`, `errors:[]`. Publication/hosted review/merge are still
pending at this recorded checkpoint; current delivery status lives in the
platform task plan. No browser/build/backend/full-suite run was performed.
