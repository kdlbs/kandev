---
id: "01-preserve-current-step"
title: "Preserve current step after a superseded cross-step move"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
acceptance_criteria:
  - AC-TASKS-KANBAN-TASK-REORDERING-001.13
  - AC-TASKS-KANBAN-TASK-REORDERING-001.28
---
# Task 01: Preserve current step

## Summary

Bound rollback to the pending move's own optimistic stage. Preserve accepted
current projections and backend-owned arrival position.

## In scope

Real-provider causal TDD, narrow production correction, browser entry-point
proof, internal contract clarification and recorded checks.

## Out of scope

See [plan scope](plan.md#scope). No delegation. Publication required the separate
parent grant, subsequently received after independent checkpoint validation.

## Acceptance

- Newer accepted task projections survive failed old moves, including equal
  targets and away/back; departed/replaced contexts are never recreated.
- Ordinary owned failure reverts only the stage; success/no-op, other tasks,
  request payload and captured error identity remain compatible.
- Journals and all owned fixtures/handles close on success and failure.

## ASCII UI preview

### UI-01: Cross-step move after a newer task event

Desktop entry: drag a card body onto another column. Phone entry: visible card
More options > Move to > destination, using the existing inset menu and focused
column navigator. Layout, scroll owners, safe areas and touch targets stay as
shipped (`MobileColumnTabs`, card menu and `SwipeableColumns`).

```text
Desktop: [Source] [Optimistic] [Authoritative: Remote title]
Phone:   [Workflow / Authoritative v]
         [Remote title] [More options]
         [Existing move error]
```

Before: rejecting the old request returns the card to Source.
After: the accepted authoritative projection remains, including when its stage
is Optimistic. Without a newer projection, failure returns the card to Source.
The structure is existing UI; spacing is illustrative. Covers AC .13 and .28.

Full context: [plan](plan.md#ascii-ui-preview).

## Verification

Run from repository root with Node 24 on PATH. Use the documented CI 8 GiB cap
for frontend checks if needed; do not broaden worker/shard limits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-swimlane-kanban-dnd.ts hooks/domains/kanban/use-cross-step-move.test.tsx hooks/domains/kanban/use-cross-step-move.test.ts hooks/domains/kanban/use-swimlane-kanban-dnd.test.ts e2e/helpers/cross-step-rollback.ts e2e/tests/kanban/cross-step-rollback.spec.ts e2e/tests/kanban/mobile-cross-step-rollback.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
NODE_OPTIONS=--max-old-space-size=8192 GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/kanban/cross-step-rollback.spec.ts -- --retries=0
NODE_OPTIONS=--max-old-space-size=8192 GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/kanban/mobile-cross-step-rollback.spec.ts -- --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --check -- docs/plans/cross-step-move-rollback-ownership
git status --short -- docs/plans/cross-step-move-rollback-ownership
```

Also run the repository PR-documentation coverage preflight over actual changed
paths and a normal commit with active pre-commit/commit-msg hooks. Record complete
per-check outcomes, actual diff, clean HEAD and resource closure for the parent.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-swimlane-kanban-dnd.ts`
- `apps/web/hooks/domains/kanban/{cross-step-move-ownership,workflow-snapshot-overviews}.ts`
- `apps/web/hooks/domains/kanban/use-cross-step-move.test.tsx`
- `apps/web/hooks/domains/kanban/use-cross-step-move.test-helpers.tsx`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/use-workflow-snapshot.ts`
- `apps/web/lib/sidebar/sidebar-task-page-cache{,.test}.ts`
- `apps/web/lib/state/slices/task-overview{,-types}.ts`
- Existing `use-cross-step-move.test.ts` and `use-swimlane-kanban-dnd.test.ts` fixtures
- `docs/public/tasks-and-workflows.md`
- `apps/web/e2e/helpers/cross-step-rollback.ts`
- `apps/web/e2e/tests/kanban/cross-step-rollback.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-cross-step-rollback.spec.ts`
- `apps/web/lib/ws/handlers/task-overview.ts`
- `apps/web/lib/state/slices/task-overview-merge.ts`
- `apps/web/lib/state/slices/kanban/{kanban-slice,snapshot-task-stages}.ts`
- Existing owning Tasks requirement/design pair and this package.

## Dependencies

None. Preserved parent receipts remain outside the checkout.

## Risks

Existing bounded journal invalidation fails closed. Partial unrelated fields
survive an owned stage rollback. This is client projection repair.

## Parallelism

`sequential`

## Inputs

[Requirement](../../specs/tasks/requirements/kanban-task-reordering.md),
[design](../../specs/tasks/system-design/kanban-task-reordering.md), and
parent receipts under `loop-receipts/attempt26-source/`.

## Results

- Frozen workspace install: passed with Node 24.18.0.
- Immutable-base causal RED: 2 intended stage failures, 1 passing ordinary
  failure control, 17 unselected cases; no setup/import failures. The source was
  restored byte-for-byte in `finally` (SHA-256
  `103532b5a51228b6bed4778e0801bd148a754e55ffb6146e59ff9aa696ab817a`).
- Partial title/position-only event RED: 1 intended behavioral failure before
  narrowing ownership to stage writes. Final causal GREEN: 20/20, including
  equal-value events without timestamp advancement, away/back, independent
  moves, deletion, membership loss/reinsertion, workflow/workspace replacement,
  journal overflow, unmount, captured request/error callback and no-ops.
- Full `hooks/domains/kanban` Vitest directory: 42 files, 384 tests passed after
  the final production edit. Existing fake-store tests were upgraded to the real
  app store; their missing-capability failures were setup failures, not RED.
- Final desktop browser: 3/3 passed (different/equal authoritative target through
  real pointer drag, plus injected callback-failure fixture cleanup). Final phone
  browser: 2/2 passed through card-menu taps and the existing focused-column
  navigator. Error dismissal leaves the authoritative card in the viewport.
  Phone screenshots inspected; no document horizontal overflow observed.
- Managed host production build passed with Node's documented 8 GiB cap,
  `GOMAXPROCS=2`, `GOFLAGS=-p=2`, one worker, one shard and retries zero. Later
  browser runs reused that exact unchanged production source/build.
- Disposable fixture cleanup passed on success and injected callback failure:
  owned workspace, executor and files removed; worker seed, disposable source
  and task checkout HEAD/full raw porcelain status preserved. No shared reset,
  operator instance, provider traffic or credentials were used.
- Final typecheck (including rich-output catalog) and focused ESLint passed with
  no warnings. Full i18n check and new-code ratchet passed; no product copy or
  catalog keys changed.
- Public docs: 63 validator tests and all 54 published pages passed. Added a
  short recovery explanation to the existing Tasks and Workflows guide.
- Documentation catalog: 396 decisions and 1707 specifications validated.
  Full specification lint passed. Existing near-limit spec prose was shortened
  without changing its contracts. PR-documentation coverage preflight passed
  over all actual changed paths and the single linked work order.
- `git diff --check`, package diff and status checks passed. The final normal
  commit must have active pre-commit and commit-msg hooks; its exact receipt and
  complete base-to-head diff are retained in the external checkpoint packet.
- Refreshed 69 open PRs and actual paginated target patches: #1512 remains at
  `a8655ad92faa050a68140a904500fc99ebc0a04b` (9 pages, 895 files), with an
  unconditional original-snapshot rollback in its Query producer. #4013 and
  #3440 have no matching target files. This is a bounded duplicate assessment,
  not a correctness claim about those PRs.
- Original proof base remains `0424e7885814b0e4a7c8950e05f957fe6c6aa5a7`.
  Current main `8ba5b6909139ec2e9e14b0e2eb2f96a4e342e88b` adds the inspected
  disjoint Jira repair. No branch rewrite or combined-delivery claim was made.

Initial message-expectation, fake-store, fixture-cleanup helper and doc-size
failures were corrected and followed by the passing checks above. An initial
public-doc test invocation from `apps/` failed on its root-relative fixture;
the final documented root invocation passed. No deadline was used as evidence.

This proves managed Chromium and phone-emulated interactions. It does not claim
physical-device, OS-shell, operator-incident reproduction or zero-flake coverage.
At that original checkpoint no push or PR had been attempted. Parent independent
validation and its separate same-session delivery grant were subsequently received.

## Review remediation results

Original-head check counts and parent review above are historical after remediation.

- Real-handler/store RED: four intended failures (discarded stale event, identical
  and changed-metadata equal-stage snapshots, stale event with independently fresh
  summary); two stale-snapshot controls passed. No setup/import failure.
- Minimal shared admission fix reuses existing freshness/discard rules and the
  bounded journal. Accepted explicit equal-stage WS events remain authoritative.
  A snapshot replacement asserts its stage; unchanged row identities do not.
  Stale task fields do not assert a stage, while independently fresh status
  summaries still merge. No revision framework or transport guarantee added.
- Shared GREEN: 48 files, 429 tests passed across Kanban hooks, task-overview
  state and task update ordering/status-summary/snapshot handlers. Final added
  sibling-identity control plus causal/isolated-slice run: 43 tests, two files.
  Final task-handler and task-overview suites: 157 tests across 25 files passed.
- Profile cleanup RED: actual profile GET returned 200 where cleanup required
  404. The regression's fallback removed that owned orphan. Explicit profile
  deletion now precedes executor deletion, and each cleanup is attempted even
  if another fails. Worker seed/source HEAD and raw status guards remain.
- Fresh production build and browser GREEN: desktop 3/3; phone 2/2. Both now arm
  the real authoritative `task.updated` wait before the move, await it before
  releasing the old failure, and retain native pointer/tap semantics. The
  failure-path fixture control passed. Phone screenshot inspected.
- Typecheck/catalog and focused ESLint passed without warnings after correcting
  the narrow slice context type and duplicate test literals. Localization,
  public-doc tests/pages, catalog and full-spec checks passed. Final normal-hook
  commit and complete diff are retained externally; no hook bypass or amend.

PR 4692 was opened under the separate parent grant. Initial CI had zero failed
checks and eight recovered native retries, with error contexts/traces inspected;
all five owned browser cases passed first attempt. These are historical initial
head results, not final-head signoff. Fresh final-head reviews/checks, exact
current-base combined validation and protected merge remain delivery gates.
Current base advanced by a disjoint agents planning package; its actual patch
was inspected. Do not infer publication or merge success from these local results.


## Pending-request chain results

- Parent real-provider RED confirmed both rejection orders. Replaying the
  published producer through two actual desktop pointer drags also failed at
  the final Source-card assertion after both exact stage-only POSTs returned
  HTTP 500. The producer was restored byte-for-byte with unchanged HEAD and
  full raw source status. Earlier pointer-target helper failures did not reach
  the second request and are not causal RED evidence.
- Request-local links now skip rejected intermediate optimism and restore a
  still-valid predecessor claim. Later accepted WS/snapshot stages, successful
  moves, independent tasks, original error identity, membership/scope departure
  and journal overflow remain protected. No admitted move is suppressed.
- Real HTTP producer RED exposed equal-row identity reuse losing stage admission.
  Existing read completion now receives original admitted rows and records only
  accepted stage claims. Stale task fields and in-flight superseded stages do
  not gain authority; independent fresh status summaries still merge. The
  existing legacy lifecycle backfill cannot inflate a stale HTTP timestamp into
  fresh stage evidence. Successful sidebar reads use the same boundary; cached
  retention and finally/cancellation closure assert no stage.
- Shared GREEN before the sidebar control: 600 tests across 70 files. Final
  causal and sidebar producer
  control: 57 tests across two files passed, including equal HTTP admission,
  cache reuse and rejected stale rows. The new test helper is fully typed and
  keeps the permanent causal test below the repository line limit.
- Fresh production build and native browser GREEN: desktop 5/5, phone 4/4,
  one worker/shard and retries zero. Both rejection orders pass through real
  desktop drags and phone card-menu taps. Existing authoritative different/equal
  stage cases and injected fixture-failure cleanup pass. Phone screenshot
  inspected; no physical-device or zero-flake claim.
- Final focused run: 556 tests across 63 files passed after the typed helper
  and sidebar control edits. The Kanban slice file additionally passed
  all 16 tests. Final ESLint, i18n/full ratchet, 63 public-doc tests,
  54 public pages, 396-decision/1707-spec catalog, full specification lint,
  documentation-coverage preflight and whitespace checks all passed.
- Final Node 24 typecheck and rich-output catalog passed at the documented 8 GiB
  cap after fixture-type corrections. A root pnpm invocation selected the wrong
  package-manager context; the documented apps/web invocation then passed.
- Current-main incoming production/docs patches were inspected, including VCS
  dialog ownership, parent workflow effect and SSH diagnostics. No Kanban owning
  file overlap was found. Fresh exact final-head/current-base frozen-install
  combined proof, normal-hook receipt, remote checks/reviews, assets and merge
  evidence remain external delivery gates, not implied by historical results.

The first pending-chain commit attempt failed its new-code i18n hook because the
extracted test-helper suffix is not excluded by that scanner. Fixture-only title
values now have explicit non-product-copy exemption comments. No hook was bypassed
and no product copy was introduced.

## Final review dispositions

The acceptance wording now explicitly forbids recreating departed tasks or
writing to replaced workspaces. This clarifies the existing sticky departure
contract; production and tests are unchanged. Skipping accepted equal-stage
snapshot claims would restore the demonstrated bug, so that suggestion was
rejected. Identity-retained siblings and stale rows already avoid claims; the
bounded journal deliberately fails closed on exhaustion.

The source HEAD/full raw status guards are required isolation checks and remain.
Cleanup guard failures must fail the injected callback-failure control rather
than masquerade as its expected callback error. Additional error aggregation is
optional diagnostics, not part of this repair. Suggested task-preview/topbar menu
selectors address other surfaces; these tests exercise the actual board-card
menu. Extra instrumentation is unnecessary for the admitted native flows.

The exact final-head CI, semantic review, current-base combined proof and merge
receipts remain external gates. Earlier successful checks retain their immutable
input boundaries and are not silently promoted to a later head or base.
