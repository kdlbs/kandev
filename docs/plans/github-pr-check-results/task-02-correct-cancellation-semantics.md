---
id: "02-correct-cancellation-semantics"
title: "Correct cancellation semantics across consumers"
status: done
wave: 2
depends_on:
  - "01-select-current-check-executions"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.6
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.7
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.8
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.1
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.2
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.3
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.4
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.5
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.6
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.7
system_design:
  - ../../specs/integrations/system-design/github-pr-check-results.md
---

# Task 02: Correct Cancellation Semantics Across Consumers

## Summary

Remove cancellation from failure counts and CI repair evidence. Connect the
selected-check identity to desktop and phone grouping, and prove the complete
repair through rendered tests using raw mock-provider data.

## In scope

- Cancellation handling in backend status/count/issue reducers and repair deltas.
- Prompt-free pruning of obsolete cancelled-check checkpoint entries.
- Cancellation-only non-success and existing strict merge-readiness guards.
- Web bucket counts, ignored-only groups, identity-aware group keys, and empty copy.
- Populated empty feedback, without inferred aggregate running counts.
- Existing details presentation of retained cancelled checks.
- Desktop/phone E2E scenarios listed in the plan, including refresh and reload.
- Implementation-time public docs check through `/docs-maintainer`.

## Out of scope

- Provider selection changes owned by Task 01, except defects exposed during integration.
- GitLab or plugin status interpretation, new overlays, and new cancellation groups.
- Changes to branch protection, automation budgets, user preferences, or polling.
- Global neutral/skipped/stale policy changes or rewriting historical messages.

## Acceptance

1. Cancellation-only and mixed-state regressions prove no false failure, passed
   result, repair prompt, or repair-round increment from cancellation alone.
2. Desktop and phone show selected results, omit ignored-only groups, preserve
   independent real failures, and expose retained cancellation details.
3. Raw-provider rendered tests pass through refresh/reload, and existing fresh-head,
   mergeability, review, attention, and repair-budget guards remain effective.

## ASCII UI preview

See [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).
The excerpt below covers `AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.1`, `.2`, `.7`.

```text
UI-01: desktop hover           UI-01: phone drawer
+------------------------+    +-----------------------------+
| #42 Current PR          |    | #42 Current PR       [Close] |
| Pass rate 2/3 (67%)     |    | Pass rate 2/3 (67%)          |
| Passed             2   |    | Passed                  2   |
| In progress        1   |    | In progress             1   |
|   Preview Environment  |    |   Preview Environment       |
|     1 running          |    |     1 running               |
| Review and automation  |    | Review and automation       |
+------------------------+    +-----------------------------+

UI-02: cancellation-only body, both surfaces
+--------------------------------+
| Checks not successful          |
| Review and automation          |
+--------------------------------+
```

Keep the existing fixed phone header, internal scroll owner, safe-area clearance,
dismissal, and details action. Remove superseded failure rows and their repair
controls. A genuine current failure still shows its normal failure row and action.
Spacing and fixture counts are illustrative; no surface geometry changes are planned.

## TDD and test design

Start with cancellation assertions in `check-buckets.test.ts` and
`TestCancelledCheckPolicy` in `client_helpers_test.go`.
Before changing production code, record the expected failure: `cancelled` is
currently counted as failed. Keep true-failure cases in the same collections.

Add a component regression in new `pr-ci-popover.checks.test.tsx` for zero
counted cancellation-only feedback. It must use existing non-success copy, omit
failure context controls, and avoid the no-checks-started label.

Add the planned automation tests in `event_handlers_github_ci_automation_test.go`.
Cover cancellation-only input, independent comments/failures, stale cancellation
checkpoints, and success siblings with required-check mergeability still blocked.
Verify no prompt-free checkpoint refresh consumes a round.

Rendered scenarios use the existing topbar and phone chip seed patterns.
Titles start with `current PR checks:`. Seed complete positive identities for
the old/new suites, not a precomputed feedback response.
Assert current links, no false failure actions, real failure visibility, same-head
refresh, reload, and phone drawer containment/dismissal.

## Verification

Run from the repository root. Install once before the first pnpm command in a
fresh worktree; later commands use that same installation.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/github -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(CIAutomation|CIFix|CIMerge)' -count=1)
(cd apps/web && pnpm exec vitest run lib/github/check-buckets.test.ts components/github/pr-ci-popover.test.ts components/github/pr-ci-popover.checks.test.tsx components/github/pr-ci-popover.automation.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/github/check-buckets.ts components/github/pr-ci-popover.tsx components/github/pr-ci-popover.checks.test.tsx lib/types/github.ts lib/types/github-checks.ts e2e/helpers/pr-checks.ts e2e/tests/pr/pr-topbar-popover.spec.ts e2e/tests/pr/mobile-pr-ci-chip.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-topbar-popover.spec.ts -- --grep 'current PR checks:')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts -- --grep 'current PR checks:')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/github-pr-check-results
```

The managed E2E runner builds fresh artifacts and enforces resource limits.
Run the desktop and phone commands sequentially.
If integration changes another suite, add its exact command to this work order
before completion. Record real RED/GREEN and final results rather than planned counts.
Run the local documentation-reference preflight described in the plan's package
validation record. Keep both work orders' requirement/design references covered.

## Files likely touched

- `apps/backend/internal/github/client_helpers.go` and `client_helpers_test.go`
- `apps/backend/internal/github/service_pr_status.go` and `service_test.go`
- `apps/backend/internal/github/service_pr_feedback_sync_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/web/lib/github/check-buckets.ts` and `check-buckets.test.ts`
- `apps/web/components/github/pr-ci-popover.tsx` and `pr-ci-popover.test.ts`
- New `apps/web/components/github/pr-ci-popover.checks.test.tsx`
- `apps/web/lib/types/github-checks.ts`, with its re-export from `github.ts`
- New `apps/web/e2e/helpers/pr-checks.ts` raw-provider fixture helper
- `apps/web/components/github/pr-checks-section.tsx`, only for cancellation-only summary/detail consistency
- `apps/web/e2e/tests/pr/pr-topbar-popover.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`
- `docs/public/automation-and-mcp.md`, only if its repair description needs correction

Any added product copy also owns the required locale catalogs and pseudo generation.
Prefer the existing `github:checksNotSuccessful` key.

## Dependencies

Task 01 supplies selected-check metadata, service wiring, and raw mock fixtures.
Read its recorded results before implementation.

## Risks

- A cancellation-only filtered list can accidentally show “No checks have started”
  or a green success state. Both require explicit regressions.
- Names alone can still merge independent workflows in the frontend. Use the
  selected execution identity for keys and the provider name only for labels.
- Backend and hover denominator policies differ intentionally for running checks.
  Do not expand this repair into a global count-policy change.
- Old cancelled-check checkpoints must clear without an extra repair round.
- Existing shared anatomy serves other providers. Keep this change in the GitHub
  adapter and preserve their enum and count behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-check-results.md), REQ-001 integration criteria and all REQ-002 criteria.
- [System design](../../specs/integrations/system-design/github-pr-check-results.md), Conclusion policy through Verification boundaries.
- Task 01 and the plan's UI-01/UI-02 previews.
- Existing `pr-ci-popover.automation.test.tsx` and `mobile-pr-ci-chip.spec.ts` patterns.
- Existing automation checkpoint refresh and fresh merge-readiness tests.
- `/mobile-parity`, `/e2e`, `/tdd`, and `/docs-maintainer`.

## Results

Completed. TDD red-phase checks first reproduced cancellation as a backend
failure, repair signal, failed hover bucket, and missing cancellation-only empty
state. After the reducer and presentation changes, cancellation is excluded from
failure counts, issue state, and auto-fix checkpoints. Prompt-free checkpoint
refresh prunes a stale cancellation without using another repair round, while
the strict merge-readiness guard still requires successful checks.

The existing github:checksNotSuccessful copy is used for cancellation-only
feedback. A loaded empty check list is authoritative, so a jobless active
workflow remains pending with zero fabricated job counts. Workflow groups use
execution identity internally and omit groups containing only ignored checks.
The PR details view continues to expose retained cancelled checks and their link.

The rendered desktop test passes raw mock check and workflow records through the
same provider selection path. It covers a superseded cancelled suite while the
replacement packages, current links, same-head success after reopen and reload,
cancellation-only details, a jobless active run, and an independent true
failure. The phone drawer test confirms the same selected active run and
cancellation-only state, then closes the drawer on both paths.

Verification:

- (cd apps/backend && go test ./internal/github -count=1): passed.
- (cd apps/backend && go test ./internal/orchestrator -run 'Test.*(CIAutomation|CIFix|CIMerge)' -count=1): passed.
- (cd apps/web && pnpm exec vitest run lib/github/check-buckets.test.ts components/github/pr-ci-popover.test.ts components/github/pr-ci-popover.checks.test.tsx components/github/pr-ci-popover.automation.test.tsx): passed, 44 tests.
- (cd apps/web && pnpm run typecheck): passed.
- (cd apps/web && pnpm exec eslint lib/github/check-buckets.ts components/github/pr-ci-popover.tsx components/github/pr-ci-popover.checks.test.tsx lib/types/github.ts lib/types/github-checks.ts e2e/helpers/pr-checks.ts e2e/tests/pr/pr-topbar-popover.spec.ts e2e/tests/pr/mobile-pr-ci-chip.spec.ts): passed.
- (cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet): passed.
- (cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-topbar-popover.spec.ts -- --grep 'current PR checks:'): passed, 1 test.
- (cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts -- --grep 'current PR checks:'): passed, 1 test.
- python3 scripts/list-docs.py validate: passed.
- python3 scripts/lint-spec-files.py --all: passed.
- Local work-order documentation coverage preflight: passed.
- git diff --check: passed.

Documentation impact: no public-doc change was needed. The docs search found no
repair instruction that classifies cancellation as a repairable failure.
