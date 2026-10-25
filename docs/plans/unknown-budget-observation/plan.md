---
created: 2026-10-14
status: implemented
requirements:
  - REQ-OFFICE-COSTS-001
system_design:
  - ../../specs/office/system-design/costs-02.md
legacy_specs: []
---

# Implementation Plan: Unknown Budget Observation

## Overview

Show unavailable budget usage when the policy list supplies configuration without
a spend observation. One sequential work order corrects the shared card and
proves the first-party consumer through its real API normalization.
Implementation was explicitly authorized after the completed design review.

Office owns this outcome because `REQ-OFFICE-COSTS-001` owns its cost explorer
and budget management. Add `AC-OFFICE-COSTS-001.9` to that existing requirement
and its [part 2 design](../../specs/office/system-design/costs-02.md#budget-policy-observation).
All pre-existing acceptance criteria, including selected-range AC-001.8, remain
verbatim. Only explanatory notification prose is shortened to respect the
requirement file's 20 KiB limit. No standalone repair spec or new ADR is needed.

## Confirmed evidence

The qualified observation directory is
`/tmp/kandev-root-budget-observation-20261014-ns2k9vbm`. Its
`qualified-proof.json`, `fixture_test.go.log`, `candidate-v2.log` and
`budget-list.json` are the permitted evidence. The real CostService and isolated
SQLite recorded 120000 subcents ($12) with a 100000-subcent ($10) lifetime
workspace policy. The real Gin list response contained policy configuration
only. The corrected rendered `BudgetsTab`, real API normalization and that
captured response showed Healthy, Observed $0 (0%), Remaining $10.

The cause is `BudgetPolicyCard` defaulting omitted `spentSubcents` to zero.
The corrected provider fixture failed causally; explicit zero and 120000
controls passed. The initial missing-TooltipProvider render failure was setup
only. These are supplied receipts, not checks executed in this design phase.
Never read, copy, replay or delete private fixture/candidate source. Later tests
must be independently authored. The evidence's seven relevant source blobs
matched authoritative main `71688b0f178839052be1e19cfaf0afca23fc7e99`.

## Scope

In scope: optional-observation rendering, neutral unknown status, unavailable
observed/remaining values, no invented percentage or colored utilization fill,
configured fields/delete control retained, and known-spend compatibility.

Out of scope: spending APIs, per-policy fetch/polling, backend models/schema,
enforcement/admission/pricing, threshold/color/cap corrections for known spend,
request races, synchronization, security/search work, dependencies, touch/layout
redesign and unrelated changes. Native backend evidence needs no re-run.

## Technical approach

In `apps/web/app/office/workspace/costs/budget-policy-card.tsx`, remove the
zero default and branch on absence before deriving measured values. Reuse
`common:unknown`, `common:unavailable` and `office:remainingAmount`; resolve
translations at render time. Keep the existing progress track height and
layout. Unknown has no colored fill. Explicit numbers retain dollar formatting,
percentage rounding and 100% cap, remaining floor and existing status/bar rules.

`budgets-tab.tsx` continues to omit observation when rendering configuration
from `listBudgets`. It needs no production edit. Keep the API client,
normalization and `BudgetPolicy` unchanged; integration coverage must consume
them rather than replacing them with mocks. Test HTTP at the external fetch
boundary, including the existing default-ceiling read and delete request.

Adjacent [measurement integrity](../../specs/office/requirements/budget-measurement-integrity.md),
[pre-launch enforcement](../../specs/office/system-design/budget-enforcement-01.md)
and notification contracts remain unchanged. The existing
[cost-window package](../office-cost-date-range/plan.md) is independent; this
repair changes neither its range criteria nor delivery results.

## ASCII UI preview

UI-01: Office Costs > Budgets, policy-only response. The supplied current
render claimed Healthy / Observed $0.00 (0%) / Remaining $10.00. Proposed:

```text
Desktop card excerpt (existing md two-column grid)
+---------------------------------------------------+
| workspace: ws-demo            [Unknown]    [Delete] |
| Observed                              Unavailable |
| [existing neutral progress track, no colored fill] |
| Budget: $10.00              Remaining: Unavailable |
+---------------------------------------------------+

Phone card excerpt (existing one-column list)
+----------------------------------------------+
| workspace: ws-demo     [Unknown]     [Delete]  |
| Observed                         Unavailable |
| [neutral track, no colored utilization fill]  |
| Budget: $10.00        Remaining: Unavailable  |
+----------------------------------------------+
```

Both retain the current period/alert/action metadata row below the excerpt,
scope title, limit and controls. Delete denotes the existing icon/tooltip,
not a new label. Spacing and abbreviated scope are illustrative, not pixel
requirements. Rows, classes, control sizing, scroll owner, navigation and
breakpoints stay as shipped. Known zero/positive/exceeded states retain their
current structure and semantics. UI-01 maps to AC-001.9 and the rendered tests.

## Tests

Independently author `budget-policy-card.test.tsx` and `budgets-tab.test.tsx`
beside the components. Render real UI, the real `TooltipProvider` and real
translation catalogs. Do not mock the card, tab, API normalization or labels.

- `policy-only HTTP response renders unavailable budget observation`: render
  real `BudgetsTab` through `listBudgets` using a literal independently authored
  fixture matching captured `budget-list.json`. Assert Unknown/unavailable,
  no Healthy/$0/0%/full remaining, configuration fields retained, and the actual
  delete action/request/removal. Intercept external HTTP only.
- `omitted and undefined observation are unknown`: verify the actual shared
  card's status, observed/remaining rows and absence of colored fill.
- `explicit zero remains measured`: $0.00 (0%), Healthy, $10 remaining.
- `positive warning and exceeded observations retain measured behavior`:
  25000 => $2.50 (25%), Healthy, $7.50 remaining; 85000 => Warning (85%);
  120000 => $12.00 (100%), Exceeded, $0 remaining. Existing known bar colors
  remain unchanged. These are controls, not threshold redesign.

Every case covers AC-001.9. Assert normal scope/limit/period/threshold/action
values and existing delete callback as well as the absence of false metrics.
No test should merely duplicate a new helper's implementation.

## E2E tests

The mobile-parity state/data exception applies: no layout, touch, scroll,
navigation, viewport or interaction changes. Closest shipped surface is this
same `BudgetsTab`/`BudgetPolicyCard`; adjacent `DefaultCeilingCard` exemplifies
existing inline budget settings without changing its behavior. Real rendered
component-to-HTTP tests prove the shared projection on both viewport paths.
No Playwright, host build or screenshot is required for this bounded change.
If layout or interaction changes become necessary, revise the package first
with two exact meaningful desktop/phone Playwright flows; no full suite.

## Public documentation assessment

No public docs edit is needed. `README.md` and `docs/public/feature-status.md`
describe Office budgets as in progress; there is no published policy-card
metric guide or relevant screenshot asserting measured usage. Configuration,
commands, APIs and supported feature status do not change. Update internal
specs/plans only; no locale additions or speculative public page.

## Work orders

- [x] [Task 01: Show unknown budget usage](task-01-observation.md)

One work order, sequential, no delegation. Exact tests, lint, typecheck, staged
i18n and documentation checks are in its [Verification](task-01-observation.md#verification).

## Verification results

Design checks passed on 2026-10-14: catalog validation (381 decisions, 1607
specifications), specification linter tests (36 passed), full specification lint,
prospective documentation coverage (`covered`, `errors: []`) for the seven
anticipated owned paths, and documentation diff/status checks. All 29 existing
acceptance criteria match HEAD verbatim; only AC-001.9 was added. The requirement
is 20453/20480 bytes. At that design checkpoint no installation, product tests
or heavy checks had run.

Implementation completed on 2026-10-14. Corrected RED had three causal failures
and six passing controls; final rendered card/real-consumer suites passed all
nine tests, and existing API compatibility passed three tests. Scoped ESLint
passed with zero warnings; normal typecheck passed with an 8 GiB heap after the
preserved default-4-GiB OOM receipt. Staged i18n ratchet and scoped non-JSX copy
check passed, as did catalog/spec lint, actual seven-path staged documentation
coverage (`covered`, `errors: []`) and diff checks. See the
[work-order results](task-01-observation.md#results) for exact check outcomes.
No backend, API, geometry or locale changes; shared rendered checks satisfy the
accepted mobile state/data exception. Public docs assessment remains unchanged.

## Risks

- Unknown is expected for every first-party policy-only card until a caller
  actually supplies observation. It says nothing about enforcement's result.
- A truthiness check would regress explicit zero. Guard presence, not value.
- The requirement is near its size limit; keep criteria intact and do not
  increase a lint exception or move unrelated contracts.
- Missing real TooltipProvider causes setup failures unrelated to this defect.
