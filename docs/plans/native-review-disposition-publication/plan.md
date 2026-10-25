---
created: 2026-10-08
status: completed
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
system_design:
  - ../../specs/agents/system-design/native-code-review-action-publication.md
legacy_specs: []
---

# Implementation plan: Native review disposition publication

## Overview

Preserve a newer acknowledged review disposition when an older local action
finishes. One sequential work order supplies meaningful rendered regression
tests, the shared publication correction, and affected checks. ROOT reviewed the
four-document package and released implementation in this existing primary on
2026-10-08. Local implementation is complete; hosted delivery and the separate
ROOT merge grant are tracked in the current task plan.

## Evidence and assumptions

Base: `7302254a2371544a82ddca084001f101341cdd92`.
Confirmed: Resolve can remain pending while Undo and Dismiss are acknowledged;
the older Resolve rejection restores the original open blocker. The real
overview then shows that blocker again. No backend persistence loss is claimed.

ROOT's read-only disposable evidence is
`/tmp/kandev-root-review-disposition-discovery-20261008/candidate.test.tsx`,
regular file mode `0400`, SHA-256
`302bcdc2a2c119548886aba2825b3111c8019b9a6aae5ad73157456be8db2c63`.
`qualified-proof.json` records native original `20779`, start `7f662b`, actual
join `bb85cc`, wrapper exit `0`, actual Vitest exit `1`, one causal RED, two
passing current-action controls, source removed, ROOT clean, and fresh process
group `3664200` absent. `output.log` fails at the exact dismissed-versus-open
assertion. The temporary empty-selector warning did not prevent the assertion.
Read these artifacts only; never replay, copy, import, edit, chmod, or clean them.
Author the permanent fixture independently with a stable empty selector array.

Verified at the base: every `InlineReviewFinding` creates its own hook instance;
the shared hook unconditionally published full success/rollback rows. The
requirement owns native findings in Agents. Existing snapshot, clear,
supersession, and unseen WS update paths are distinct writers. Intended overlap
details absent from the original acceptance criteria are now `.9` through `.13`.
The existing backend atomicity design remains authoritative for persistence.
There is no material unresolved product decision within this bounded correction.

## Scope

### In scope

- Shared action-publication ownership by app store, task, and finding.
- Acknowledged rollback baseline across overlapping local actions, including
  older success before/after newer failure and older rejection after success.
- Existing optimistic feedback and every existing failure toast/error path.
- Real providers, rendered consumers, actual API transport/payload evidence,
  separate-instance overlap, independent scopes, and existing writer controls.

### Out of scope

- Backend versioning, database changes, server execution ordering, cross-browser
  reconciliation, all-writer ordering, cancellation, or lifecycle hardening.
- Layout, localization copy, touch sizing, navigation, component refactors,
  mobile feature completion from the historical native-review plan, or polish.
- Browser/build/full-suite work, new runtime flags, new sessions, delegation,
  recursive tasks, contacting child80, or automatic resource retries.

## Technical approach

Follow the [owning design](../../specs/agents/system-design/native-code-review-action-publication.md).
Add `lib/review/finding-action-publication.ts` for synchronous, store-scoped
bookkeeping. Keep transport and toasts in `use-finding-actions.ts`. A group's
local ordinals govern acknowledgements, its baseline excludes pending failures,
and exact current-row ownership protects other writers. Retain a group until
all original overlapping requests settle. Do not change the review slice or WS
handler to impose global ordering.

| Consumer/boundary | Identity | Result/evidence |
| --- | --- | --- |
| Anchored diff card and unanchored banner card | Same store/task/finding, separate hook instances | Shared admission and settlement; two real mounted cards in the regression fixture. |
| Existing review API over WS | Captured finding ID and requested status | Assert actual `task.review.finding.update` action and payload; mock request boundary only. |
| Real overview and toast provider | Store rows and existing rejection path | Assert membership/empty state, collapsed card status, full acknowledgement metadata, error text and reporting. |
| Other findings/tasks/stores | Distinct keys, including equal finding IDs across tasks/stores | No shared fence or baseline; independent controls. |
| Snapshot/clear/supersession/live updates | Existing slice/handler actions | Preserve replacement/deletion/unseen-row semantics; local group yields on observed row replacement. |

Unsupported transport shapes keep the existing API contract; no new capability,
fallback request, or product error behavior is introduced.

## Tests

Permanent suite: `apps/web/hooks/domains/review/use-finding-actions.test.tsx`.
Use a small fixture helper if necessary to meet file/function limits, not a
parallel production abstraction. All cases use the actual providers and rendered
finding/overview path with deferred WS requests. Proposed test names define the
observable contract rather than inspect private tokens.

| AC suffix under `AC-AGENTS-NATIVE-CODE-REVIEW-001` | Proposed cases |
| --- | --- |
| `.4`, `.9` | `retains acknowledged Dismiss after older Resolve rejects`; same trace with older Resolve success; assert complete Dismiss row and overview after draining. |
| `.9`, `.11` | `shares disposition ownership across two inline consumers`; start Resolve in card A and Undo/Dismiss in card B; old completion must not reopen either card/overview. |
| `.10` | `keeps newer optimism while older Resolve succeeds then rolls rejected Undo back to that acknowledgement`; `publishes older acknowledgement after newest failure`; `restores pre-overlap row when all actions fail`; `rolls failed Dismiss back to acknowledged Undo`. |
| `.12` | Current acknowledged Resolve publishes full returned metadata; current rejected Resolve restores exact prior row and actual toast/report; current acknowledged/rejected Undo and Dismiss preserve their states/errors; obsolete rejection still reports its own error. |
| `.11` | Interleaved different findings; equal finding IDs under distinct tasks; separate `StateProvider` stores; unrelated writes do not cancel target completion. |
| `.13` | Actual `setTaskReview`, `clearTaskReviewState`, and `addReviewFindings(..., supersededIds)` during pending action retain replacement/removal after both late success and rejection; actual handler still inserts unseen update; replacement admits fresh action with replacement baseline. |

Retain the existing card, overview, slice, and WS handler suites as affected
controls. The first causal test must fail on the real dismissed-versus-open
outcome against unchanged production code, with controls passing. Missing APIs,
collection failures, mock-only reducers, and predicate tests are not causal RED.

## E2E tests and mobile parity

No new browser test is required for this pure state/data correction. The
mobile-parity skill explicitly permits targeted unit/component coverage when
layout, touch behavior, scrolling, navigation, and viewport interactions do not
change. The existing phone Changes diff uses the shared annotation/card path;
the permanent rendered transport-to-provider-to-card/overview tests exercise
this contract on that shared path. Existing
`e2e/tests/review/mobile-review-findings-nav.spec.ts` covers phone navigation,
not disposition overlap, and is not claimed as causal evidence or run here.
No UI geometry preview is needed because the composition is unchanged.

## Documentation audit

Internal requirement and design updated. Public-doc search included
`docs/public`, `README.md`, and `docs/screenshots.md`; no public native finding
disposition timing wording requires correction. This repair adds no user step,
public API, configuration, screenshot, or terminology. No public docs change
is planned. Historical native-review and status-atomicity work orders were
audited; their completed/pending outcomes are not reclassified by this package.
No root/scoped AGENTS architecture statement becomes inaccurate.

## Work orders

- [x] [Task 01: Preserve acknowledged finding dispositions](task-01-preserve-dispositions.md)

## Verification results

Design checks on 2026-10-08 passed: `python3 scripts/list-docs.py validate`
(363 decisions, 1438 specifications), `python3 scripts/lint-spec-files.py --all`,
catalog discovery of the owning pair, and `git diff --check`. A pure Node call
to the repository's `validateCoverage` returned `ok: true`, `status: covered`,
and `errors: []` for this package with explicitly projected hook/helper paths;
this is reference validation, not a claim of an implemented runtime diff.
All four document files passed a separate read-only whitespace check, including
untracked files. The amended requirement is 20,242 bytes, below its 20,480-byte
limit; only duplicated introductory rationale was shortened for headroom.

The first Node preflight invocation ended `127` because Node was absent from
the default PATH. Using the existing configured Node 24.21.0 binary completed
the document-only preflight with actual exit `0`; no install/toolchain change.
Those design-turn checks used no heavy resources. After ROOT's separate
implementation release and exclusive lease, the independently authored permanent
fixture produced one causal dismissed-versus-open RED with two passing current
controls. The expanded pre-production matrix produced 12 behavioral failures
and eight controls passing. The implemented helper/hook passed all five affected
suites, 54 tests; after minimal lint-only test constants, the corrected fixture
passed all 20 cases. Production did not change after the 54-test run.

Scoped ESLint, package typecheck, i18n check/ratchet, catalog/spec lint, and
`git diff --check` passed. Actual workspace PR-documentation coverage, including
untracked implementation/test files, returned `covered` with `errors: []`.
See [Task 01 results](task-01-preserve-dispositions.md#results) for command evidence
and the current task plan for original native handles and process-group receipts.
No browser, build, full suite, or product runtime was used. Normal commit hooks
and exact-head hosted CI/review receipts are delivery gates tracked externally.

## Risks

- A latest-token-only fence fixes the headline trace but loses the correct
  acknowledged baseline when a newer pending action fails.
- A per-instance fence misses overlapping consumers. A global finding-ID fence
  wrongly joins separate tasks/stores.
- Live events can cede ownership even when echoing a local request. That is the
  existing independent-writer boundary; this package does not repair event order.
- Deferred fixtures must drain all requests, scope repeated controls, use stable
  selectors, and retain actual command exit evidence. The ROOT proof is immutable.

## Handoff

Local checks complete in task `287fbd04-8e15-44cf-91d6-08d4922bd357`, session
`24ae35e9-41e6-4370-acc0-86b13c0ec44d`. ROOT's review/implementation and exclusive
heavy releases were separate from the design handoff. Standing delivery gates,
joined heavy RETURN, hosted collector, and crash/next-action receipts live in
this task's versioned Kandev plan. Merge requires a separate serial ROOT grant.
