---
id: "01-observation"
title: "Show unknown budget usage"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-COSTS-001
acceptance_criteria:
  - AC-OFFICE-COSTS-001.9
system_design:
  - ../../specs/office/system-design/costs-02.md
---

# Task 01: Show Unknown Budget Usage

## Summary

Correct the real budget card's optional-observation projection and prove the
first-party policy-only consumer through its real API. Keep measured values and
policy management intact. Implementation was authorized after design review
in the same primary session.

## In scope

- Independently authored rendered regression and explicit-value controls.
- Unknown/unavailable rendering in `BudgetPolicyCard`, preserving geometry.
- Verify configured fields and delete behavior through the actual consumer.
- Record exact task-defined results and synchronize this work order/manifest.

## Out of scope

No backend, spend API, polling, schema, pricing, enforcement, known threshold,
color or cap fixes, synchronization/races, search/security, new dependencies,
locales or broader UI changes. No private evidence source read/copy/replay or
cleanup. No permanent production/test edits before the later grant.

## Acceptance

1. Real `BudgetsTab` through actual API normalization with the captured-shaped
   policy-only HTTP response renders truthful unknown/unavailable values and
   no Healthy, measured zero/percentage, full remaining or colored usage fill.
2. Real shared-card omitted/undefined, zero, positive, warning and exceeded
   controls demonstrate AC-001.9 without changing existing known-spend rules.
3. Scope, limit, period, threshold, action and existing delete control remain
   usable; real tab deletion issues its existing HTTP request and removes the
   card. Existing row/grid/touch/scroll/navigation structure is retained.

## ASCII UI preview

UI-01: Office Costs > Budgets, unavailable observation. Full composition and
before state: [plan preview](plan.md#ascii-ui-preview), AC-001.9.

```text
Desktop (card within existing two-column grid)
| workspace: ws-demo   [Unknown]       [Delete] |
| Observed                        Unavailable |
| [neutral track with no colored usage fill]   |
| Budget: $10.00       Remaining: Unavailable  |

Phone (card within existing one-column list)
| workspace: ws-demo   [Unknown]       [Delete] |
| Observed                        Unavailable |
| [neutral track with no colored usage fill]   |
| Budget: $10.00       Remaining: Unavailable  |
```

Existing metadata row remains below both excerpts. Labels/spacing are
illustrative; Delete denotes the existing icon. Same shared projection and
unchanged rows/classes/control geometry support the mobile state/data exception.
Compare rendered test content/structure with UI-01. Layout or interaction changes
invalidate that exception and require a revised desktop/phone Playwright plan.

## Implementation sequence

Read this packet and the observation design, then mark this work order
`in_progress`. Under the separately granted local-heavy lease, prepare missing
workspace dependencies once. Author the real consumer regression and controls
from captured JSON and public production source, never private candidate source.
Mock external fetch only; render real `TooltipProvider`/UI/translations and
serve the existing default-ceiling request normally from the HTTP fixture.

First RED must fail on the false Healthy/zero/remaining claim, not provider or
test setup. Keep the controls' actual outcomes. Then remove the default-zero
projection and branch on missing observation in the card. Prefer existing
Badge neutral tokens and locale keys, without new helper layers. The tab keeps
omitting observation. Run affected GREEN and the listed checks once; repeat
only for a real change/failure. Preserve failures and original process handles.

## Verification

Run from repo root, with the existing runtime tools:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
node --version
(cd apps && corepack pnpm --version)
# Only if this fresh worktree lacks dependencies; one install after the grant.
(cd apps && timeout --signal=TERM --kill-after=10s 600s corepack pnpm install --frozen-lockfile)

# RED then GREEN on the independently authored suites.
(cd apps/web && timeout --signal=TERM --kill-after=10s 240s corepack pnpm exec vitest run app/office/workspace/costs/budget-policy-card.test.tsx app/office/workspace/costs/budgets-tab.test.tsx)
# GREEN also checks existing API normalization compatibility.
(cd apps/web && timeout --signal=TERM --kill-after=10s 240s corepack pnpm exec vitest run lib/api/domains/office-budget-api.test.ts)
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s corepack pnpm exec eslint --max-warnings 0 app/office/workspace/costs/budget-policy-card.tsx app/office/workspace/costs/budget-policy-card.test.tsx app/office/workspace/costs/budgets-tab.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 timeout --signal=TERM --kill-after=10s 300s corepack pnpm run typecheck)
```

After passing product checks, synchronize owned status/results. Stage only
actual owned files, including all additions, under the authorized delivery
phase and active normal hooks. Inspect actual indexed paths. If ownership
differs from the list below, reconcile it before changed-line checks; never
stage foreign edits or include nonexistent files. Keep the actual baseline SHA
in `BUDGET_OBSERVATION_BASE_SHA` (initial reviewed base below).

```bash
export BUDGET_OBSERVATION_BASE_SHA=71688b0f178839052be1e19cfaf0afca23fc7e99
git add -- apps/web/app/office/workspace/costs/budget-policy-card.tsx apps/web/app/office/workspace/costs/budget-policy-card.test.tsx apps/web/app/office/workspace/costs/budgets-tab.test.tsx docs/specs/office/requirements/costs.md docs/specs/office/system-design/costs-02.md docs/plans/unknown-budget-observation/plan.md docs/plans/unknown-budget-observation/task-01-observation.md
git diff --cached --name-status
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s corepack pnpm run i18n:ratchet --base "$BUDGET_OBSERVATION_BASE_SHA")
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s node scripts/check-nonjsx-copy.mjs 'app/office/workspace/costs/budget-policy-card.tsx')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
git status --short
```

The ratchet must see actual staged/new paths; confirm its path evidence.
Adjust scoped ESLint/non-JSX commands to every actual owned production addition
if implementation needs one. No backend tests or giant unrelated suite.

Documentation coverage preflight, also used prospectively in design. The script
reads actual staged paths after implementation and anticipated paths only in
design; referenced documents are actual workspace content. The design run must
be `covered` (not only document exemption). Run exactly this block from root:

```bash
node - prospective <<'JS'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/office/requirements/costs.md',
  'docs/specs/office/system-design/costs-02.md',
  'docs/plans/unknown-budget-observation/plan.md',
  'docs/plans/unknown-budget-observation/task-01-observation.md',
];
const anticipated = [...docs,
  'apps/web/app/office/workspace/costs/budget-policy-card.tsx',
  'apps/web/app/office/workspace/costs/budget-policy-card.test.tsx',
  'apps/web/app/office/workspace/costs/budgets-tab.test.tsx',
];
const paths = process.argv[2] === 'prospective' ? anticipated
  : cp.execFileSync('git', ['diff', '--cached', '--name-only', '-z'])
    .toString().split('\0').filter(Boolean);
assert(paths.length > 0, 'Expected actual staged or prospective paths');
const result = validateCoverage({ changedFiles: paths,
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])) });
console.log(JSON.stringify(result, null, 2));
assert.equal(result.ok, true);
assert.equal(result.status, 'covered');
assert.deepEqual(result.errors, []);
JS
```

For final actual coverage use `node - staged` instead of `node - prospective`.
Design-only documentation checks use catalog/spec lint, prospective coverage
and diff/status; do not run installation, frontend checks or staging then.
Record results for the actual run; no passing-suite replay for optional prose.

## Files likely touched

- `apps/web/app/office/workspace/costs/budget-policy-card.tsx`
- `apps/web/app/office/workspace/costs/budget-policy-card.test.tsx` (new)
- `apps/web/app/office/workspace/costs/budgets-tab.test.tsx` (new)
- The linked existing Office requirement/design and this manifest/work order.

Read-only integration boundaries: `budgets-tab.tsx`, `office-budget-api.ts`,
`office-api.ts`, `BudgetPolicy` and existing UI/i18n providers. No public docs
edit: Office budgets remain documented as in progress, without a published
policy-card metrics claim. No new locale keys or package/lockfile change.

## Dependencies

None. Explicit implementation and local-heavy authorization were granted
after design review. Same primary session; preserve all foreign resources.

## Risks

Use a presence check rather than truthiness so explicit zero stays measured.
Missing TooltipProvider is setup-only failure. Keep the neutral track distinct
from a zero utilization claim. Do not infer policy spending from workspace
rollups or change existing thresholds/caps. Preserve every pre-existing AC.

## Parallelism

`sequential`

## Inputs

- [REQ-OFFICE-COSTS-001 / AC-001.9](../../specs/office/requirements/costs.md).
- [Budget policy observation design](../../specs/office/system-design/costs-02.md#budget-policy-observation).
- Actual card/tab/API source and existing `office-budget-api.test.ts` fetch
  fixture pattern; reuse real UI/providers rather than its API mocks.
- Qualified evidence's four permitted JSON/log files, as recorded in the plan.

## Results

Completed on 2026-10-14. Every required command in Verification passed:

- Node 24.18.0, pnpm 9.15.9; one frozen workspace install succeeded because
  this fresh worktree lacked dependencies. Package/lockfile unchanged.
- Independently authored corrected RED: three causal false-Healthy failures
  (omitted, undefined and real policy-only consumer), six controls passed.
  The initial run also had one incorrect empty-state copy expectation; it was
  corrected before production edits. Both failed receipts were retained.
- Final affected Vitest run: both new files, nine tests passed after the final
  simple-branch edit. Existing budget API suite: three tests passed.
- Scoped ESLint with `--max-warnings 0`: no errors or warnings. Earlier
  nested-ternary warnings were corrected before the final run.
- Normal web typecheck: passed with the documented 8 GiB Node heap. The first
  default-4-GiB attempt exited 134 with heap exhaustion; its process joined
  before retry and its failure receipt was retained.
- Actual staged i18n ratchet: zero added production files, one modified file
  clean; guard allowlist intact (643 entries). New tests were staged and are
  excluded from copy scanning by repository convention. Scoped non-JSX check:
  one guarded production file passed; existing common locale keys only.
- Catalog: 381 decisions and 1607 specifications validated. Specification
  lint: all files passed. The unchanged validator's 36 design-phase tests were
  not repeated. Actual staged documentation coverage: seven owned paths,
  `covered`, `errors: []`, correct requirement/design/work-order linkage.
- Working-tree and staged diff checks passed. All 29 pre-existing acceptance
  criteria remain verbatim; requirement remains 20453 bytes.

Rendered real card and consumer checks match UI-01: same rows and neutral
progress track, retained configuration/delete control and known-value behavior.
No layout/touch/scroll/navigation change, backend tests, Playwright/build,
locale additions or public-doc edits. The state/data mobile exception applies.
