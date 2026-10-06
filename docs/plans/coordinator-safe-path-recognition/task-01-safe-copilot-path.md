---
id: "01-safe-copilot-path"
title: "Safe copilot path recognition"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-004
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-004.12
system_design:
  - ../../specs/coordinator/system-design/copilot-panel.md
---

# Task 01: Safe Copilot Path Recognition

## Summary

Reject undecodable workspace/coordinator components at the existing copilot
reset boundary without throwing, preserving exact once-decoded valid identity
and slot semantics. Add causal permanent helper and real mounted bridge tests,
then apply the local helper correction. This task remains pending until a
later explicit same-primary implementation interrupt and LOCAL-HEAVY grant.

## In scope

- Capture both components in `coordinatorIdFromPath`, reuse
  `safeDecodePathSegment` locally once per capture, return coordinator only
  if both are valid; keep optional Queue and trailing slash grammar.
- Extend helper tests and replace the bridge test's pathname mock with the
  actual native subscription/history/popstate, real store and React boundary.
- Prove causal RED/GREEN, preserved valid entries and independent reset/close
  controls, then run only the specified affected checks.
- Record actual command results and synchronized delivery status after checks;
  completion of this work order is distinct from verified merge/task completion.

## Out of scope

Global router/policy/fallback, `useParams`, generic malformed-URI audit,
store model or shared decoder changes without causal evidence, backend,
transport, workflows, cache, conversation or persistence changes. No new
layout, copy, navigation, mobile geometry, browser/build/E2E or full SPA setup
repair. No delegation, sessions/tabs/model switch, install or heavy check now.
Publication and merge follow the later ROOT gates in the live Kandev plan;
this work order provides no merge authorization.

## Acceptance

1. The production helper returns `null` without throwing for invalid decoding
   in either component and for unsupported/default paths, and returns the exact
   once-decoded coordinator for valid encoded Needs you/Queue paths with the
   existing optional final slash. Tests explicitly distinguish literal percent,
   encoded slash and double-encoded content.
2. A mounted real bridge driven by native history/popstate clears a freshly
   seeded real slot for each malformed component/departure while its application
   marker survives and the error fallback stays absent. Same-identity Needs
   you/Queue transitions preserve the full entry; close/reopen preserves chip
   and draft; other coordinator and ordinary page controls independently reset.
3. Permanent regressions fail causally before the local helper correction and
   the authorized targeted selection passes afterwards. All affected checks
   and original command receipts are recorded; the shared decoder, public
   routes, real store and viewport-independent behavior retain their contracts.

## Implementation order

1. Read the governing instructions and package again. Compare current helper,
   bridge/router/store/decoder/SPA resolver with the recorded baseline without
   replaying the ROOT archive. Mark this work order in progress only after the
   explicit later instruction. Obtain exclusive heavy admission before package
   commands. If dependencies are absent, the sole permitted install is
   `(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)` under the
   admitted install bound; no duplicate installation or lockfile mutation.
2. Write the helper and mounted regressions first. Run the exact bounded
   selection below for RED; classify each failure at the actual bridge/helper
   boundary. Require malformed coordinator decoding failures and malformed
   workspace stale-state failure, while valid and ordinary-reset controls pass.
   A setup/import/resource failure is not RED and requires ROOT checkpoint.
3. Make the minimum correction in `coordinator-path.ts`; the bridge need not
   change. Run the approved targeted selection for GREEN. Apply only routine
   causal owned-fixture or affected-lint repairs; every timeout/resource/
   transport/unknown/out-of-scope failure ends at ROOT checkpoint without retry.
4. Run the affected verification below serially under the later grant. Record
   each original native handle/PID/group/start/absolute cutoff/log and actual
   join/gone before starting another heavy command. Update results and the plan
   only from observed evidence. Keep publication head frozen after later READY
   publication except actual corrective findings; normal hooks and a new
   Conventional Commit, never amend or bypass.

## Regression matrix

Use explicit expected values rather than reproducing the parser in tests.
Cover malformed percent `%`, non-hex `ws%ZZ`/`co%ZZ` and incomplete UTF-8
`%E0%A4%A` in workspace and coordinator positions, independently, on both
Needs you and Queue; include Queue with final slash. Reseed the real slot
before each case, assert it was seeded before dispatch, then assert ownership
is `null`, entry is `{open: false, chip: null, draft: ""}` and draftsSwept is
false. Assert boundary fallback absent and marker still mounted after effects.

Valid controls include `ws%2Fone/co%2F1` yielding `co/1`, workspace and
coordinator `%25` identities yielding literal `%`, `co%252F1` yielding
`co%2F1`, and encoded Unicode. Assert both views with optional trailing slash,
Needs you to Queue and back, complete chip including ref/draft/open and slot
identity. A valid decoded `%` is not malformed. Do not reject it by a second
decoder pass or interpret a decoded slash as a route separator.

Independently reseed for a different coordinator, `/`, ordinary workspace
tasks, generic `/workspaces/ws-1/coordinator`, missing/empty components,
unsupported `/settings`, and extra `/queue/extra`. Close and reopen on the
same recognized path using real `setOpen`, asserting draft/chip preservation.
Run existing store tests for ownership, null reset and draftsSwept separately
from the helper's correctness. Reset location/store and unmount subscriptions
after each case; suppress only expected boundary console errors with restored
spies. No production module mocks or full SPA import.

## Verification

Commands below are future checks, not authorized during design. Run from repo
root; each package command roots its own working directory. Preserve one
original active process per check with a receipt and retained log. The targeted
Vitest selection is fixed; no all-worker override, broad suite or duplicate run.

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/coordinator/coordinator-path.test.ts components/coordinator-copilot-reset-bridge.test.tsx hooks/domains/coordinator/copilot-store.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/coordinator/coordinator-path.ts hooks/domains/coordinator/coordinator-path.test.ts components/coordinator-copilot-reset-bridge.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:check)
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py specs --system coordinator --format paths
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 60s git diff --check -- docs/specs/coordinator/requirements/copilot.md docs/specs/coordinator/system-design/copilot-panel.md docs/plans/coordinator-safe-path-recognition apps/web/hooks/domains/coordinator/coordinator-path.ts apps/web/hooks/domains/coordinator/coordinator-path.test.ts apps/web/components/coordinator-copilot-reset-bridge.test.tsx
timeout --signal=TERM --kill-after=10s 60s git status --short
```

The original direct `pnpm exec tsc` check failed on missing generated assets.
ROOT authorized one corrected normal `pnpm run typecheck`: its tracked
pretypecheck scripts prepare the two ignored JSON files from `CHANGELOG.md`.
Preserve the original setup-failure receipt and the corrected receipt; this
does not authorize unrelated setup repairs. Node 24 is required for i18n;
absence is a setup checkpoint, not permission to install a runtime.
No new product copy means no catalog edits are planned.

Run the repository's actual pure documentation-coverage preflight without
GitHub calls or synthetic file changes:

```bash
timeout --signal=TERM --kill-after=10s 60s node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage, resultSummary } = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' }).split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0'),
].filter(Boolean))];
const fileContents = Object.fromEntries([
  'docs/specs/coordinator/requirements/copilot.md',
  'docs/specs/coordinator/system-design/copilot-panel.md',
  'docs/plans/coordinator-safe-path-recognition/plan.md',
  'docs/plans/coordinator-safe-path-recognition/task-01-safe-copilot-path.md',
].map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles: paths, fileContents });
console.log(resultSummary(result));
if (!result.ok) process.exitCode = 1;
NODE
```

Coverage preflight proves local reference compatibility; hosted exact-head PR
coverage remains a later independent gate. Design-only path exemptions do not
prove production coverage; rerun against actual implementation changes later.

## Files likely touched

- `apps/web/hooks/domains/coordinator/coordinator-path.ts`
- `apps/web/hooks/domains/coordinator/coordinator-path.test.ts`
- `apps/web/components/coordinator-copilot-reset-bridge.test.tsx`
- The four existing design-package artifacts, for delivery results/lifecycle.

Read-only dependencies/controls: `apps/web/lib/routing/path.ts`,
`apps/web/lib/routing/client-router.ts`, `apps/web/src/spa-routes.tsx`,
`apps/web/src/app-shell.tsx`, production reset bridge, copilot store/test and
`apps/web/app/coordinator/copilot/use-coordinator-copilot.ts`.

## Dependencies

None. One sequential vertical correction; no install or product validation
until the later explicit same-primary/exclusive-heavy admission.

## Mobile and documentation scope

The helper/bridge correction is pure state normalization with no layout,
touch, scrolling, navigation or viewport-dependent interaction change.
Targeted helper and mounted component evidence satisfies the mobile-parity
pure-state exception; no new mobile E2E or ASCII layout preview is needed.
Public Coordinator guidance and public route structure stay accurate; the
audit and internal-doc scope are recorded in the plan.

## Risks

Double decoding valid percent identities; wrong regex capture index; weak
chained invalid tests that observe an already empty slot; boundary console
spies hiding an unrelated failure; full-SPA dependency leakage into the fixture.
Every case must assert its own seeded state and actual native effect result.
`useParams` remains unproved and excluded; never report universal route safety.

## Parallelism

`sequential`. No delegation, new sessions/tabs or model switches.

## Inputs

- [Copilot requirements](../../specs/coordinator/requirements/copilot.md#req-coordinator-copilot-004-copilot-panel), `AC-COORDINATOR-COPILOT-004.12`.
- [Path reset boundary](../../specs/coordinator/system-design/copilot-panel.md#path-reset-boundary).
- [Plan and accepted proof provenance](plan.md#grounded-defect-and-evidence).
- ROOT's read-only accepted fixture and receipts, never replayed/modified/removed.
- Existing helper, bridge and store tests; native router history/popstate pattern.
- Live Kandev task plan with identities, user edits, ownership/completion gates
  and later CI/review/merge restrictions.

## Results

Implemented in the same primary after ROOT's reviewed-package release. The
production correction reuses existing `matchDouble` locally and changes only
`coordinator-path.ts`; shared decoder, bridge, store, native router and SPA
resolver are unchanged. Permanent helper and mounted bridge tests use real
native pathname/history/popstate and Zustand state, independently seeded
malformed/departure cases, complete-entry preservation, close/reopen and
once-decoded percent/slash/Unicode controls. No production module mocks,
accepted proof replay, full-SPA import, browser/build/E2E or global sweep.

Actual serial verification (all original processes joined; groups gone):

| Check | Actual result | Original native evidence |
| --- | --- | --- |
| One pinned frozen apps install, absent dependencies only | exit 0, 2.020s, 935 packages reused, lockfile unchanged | 68211, completion 2871b3 |
| Exact three-file Vitest RED selection | exit 1, 12 causal failures and 44 passing controls (56 total), 7.130s | 14436, completion cb8df5 |
| Affected Prettier | exit 0, source unchanged, helper-test wrapping only | bb7747 |
| Exact three-file Vitest GREEN selection | exit 0, all 56 tests/3 files pass, 7.080s | 97726, completion 7d2c2b |
| Affected three-file ESLint | exit 0, 1.719s | 64161, completion cce6b6 |
| Original direct tsc | SETUP FAILURE exit 2, missing generated changelog/release-notes modules, 63.150s; not a pass | 1718, completion cc57ec |
| ROOT-authorized corrected normal project typecheck | exit 0, pretypecheck generated the two previously absent ignored JSON assets from tracked changelog, 9.834s | 68517, completion 6fee5b |
| i18n:check | exit 0, 9.133s, catalogs/pseudo/indices/plurals/module-scope/copy checks pass; existing orphan-key warning retained | 10076, completion 08c69d |
| Owning catalog, catalog validation, all-spec lint | exit 0 each; 355 decisions/1396 specifications valid | 28eadd, 70f7a2, 062f41 |
| Actual changed-path documentation coverage | exit 0, covered, ok true/errors empty; helper requires linked COP004 work order/design | 329e98 |

Receipts and full logs: `/tmp/kandev-copilot-implementation-5573589e-7i_1pojv/`.
The design's actual 36 specification-linter tests passed previously; no unchanged
linter-test replay was added during implementation. Original design Node setup
failure and its authorized invocation correction remain intact in the design
receipt directory. Only two previously absent ignored generator outputs are
recorded as owned setup artifacts in `owned-setup-outputs.json`; no script,
source, tsconfig or lockfile change was needed for recovery. Normal command
execution uses existing Node 24 explicitly under Bash without a login shell.

The mobile pure-state exception and public-doc audit apply. No screenshots or
new public guide are needed for this viewport-independent recognition repair.
The wider Coordinator rollout's draft specifications and pending companion
panel plan are retained; this focused repair does not complete that rollout.

Local implementation is complete. Normal hooks, exact remote READY publication,
hosted checks/reviews and any later separately authorized merge are recorded in
the live Kandev plan; task completion still requires verified merge/cleanup.
