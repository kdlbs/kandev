---
created: 2026-09-10
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
legacy_specs: []
---

# Implementation Plan: GitHub Workflow Attention

## Overview

Collected and persisted workflow attention, then exposed the reason through the existing desktop and phone PR surfaces.
Task 01 and Task 02 are complete.

## Confirmed defect

On 2026-09-10, GitHub returned these values for `failsafe-go/failsafe-go#143`:

| Observation | Value |
| --- | --- |
| PR head | `93538b58322527d5d86c684d85c66538c91ff680` |
| PR state / merge state | `OPEN` / `UNSTABLE` |
| GraphQL check rollup | Empty |
| Actions run | `34494307522`, "Run tests" |
| Run status / conclusion | `completed` / `action_required` |
| Run event / attempt | `pull_request` / `1` |
| Job count | `0` |
| Check suite | `93444456632`, `github-actions`, `action_required`, zero checks |
| Head repository | `carlosflorencio/failsafe-go` |
| Run PR associations | Empty |

The supplied GitHub screenshot explicitly says that one workflow awaits maintainer approval.
Read-only API calls corroborate the underlying run and missing jobs.
Access to the linked Kandev task returned `FORBIDDEN`, so its stored record was not inspected.

`ListCheckRuns` reads check runs and commit statuses, but not Actions workflows.
The GraphQL batch reads `statusCheckRollup` without workflow attention.
`deriveCIRow` omits an empty check state. `deriveMergeRow` falls back to raw `unstable`.
These source paths reproduce the screenshot from the observed provider data.
Changing only the label cannot identify the approval gate.

## Scope

In scope: provider evidence, additive stored observation, status convergence, desktop/phone copy, and targeted regression coverage.
Out of scope: workflow approval or reruns, credential changes, deployment gates, and provider-neutral automation redesign.
No live GitHub or Kandev task state changes are part of this package.

## Technical approach

The [system design](../../specs/integrations/system-design/github-workflow-attention.md) owns collection and interpretation details.
The implementation adds workflow evidence to REST feedback/status and batched sync before persistence.
`SyncTaskPR` publishes the new field through existing events and reads.
Approval evidence remains separate from check counts and failure snapshots.
The UI consumes the stored reason through shared summary and detail components.

No ADR is necessary: the existing integration owner, credential boundary, and reviewed-head merge rule remain authoritative.
This design records the local transport and presentation choices.
Public documentation changes were completed with Task 02 because the behavior is now available.

## ASCII UI preview

UI-01: Desktop task-row hover or keyboard focus, approval-only state.

```text
Before                         After
PR #143                        PR #143
Add bulkhead permit...         Add bulkhead permit...
by carlosflorencio              by carlosflorencio
Merge  (o) unstable             CI  (!) Awaiting maintainer approval
```

UI-02: Phone task navigation, then existing PR status chip drawer.
The desktop detailed popover uses the same content order.

```text
+----------------------------------+
| PR #143                          |
| Add bulkhead permit...            |
| by carlosflorencio                |
|                                  |
| CI: Awaiting maintainer approval  |
| Run tests                        |
| This workflow has not started.    |
| [View on GitHub]                  |
|                                  |
| Existing review and automation    |
| controls                         |
+----------------------------------+
```

UI-03: Mixed and unavailable states, shared content within existing surfaces.

```text
CI  (!) Failed checks
    (!) 1 workflow awaits approval

CI  (?) Workflow status unavailable

CI  (!) Last known: awaiting approval
        Workflow status refresh unavailable
```

Grouping, separate failure/approval facts, and existing phone navigation are required.
Spacing and icon sketches are illustrative. Titles and translated copy wrap within the existing surface.
The drawer retains its scroll owner and safe-area padding. The provider link has a 44px touch hit area.
The tooltip remains informational. It does not gain interactive links or start provider requests.
Previews map to criteria 001.3, 001.4, 001.6, and 001.8.

## Tests

All criterion suffixes refer to `AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION`.

| Criteria | Planned regression evidence |
| --- | --- |
| 001.1, 001.6, 001.7 | `workflow_attention_test.go`: jobless fork workflow, ambiguous action, empty checks, denied and incomplete reads |
| 001.2, 001.4 | Provider rollup and orchestrator automation tests: zero fake failures, mixed failure, strict readiness |
| 001.5 | Store and service tests: migration, REST/batch/feedback convergence, rerun, head change, stale reads |
| 001.3, 001.8 | `pr-task-status-summary.test.ts`, chip and popover tests: readable reason, no raw unstable, localized fallback |

## E2E tests

Extend `apps/web/e2e/tests/pr/pr-status-badge.spec.ts` in the `chromium` project.
Cover sidebar hover/focus, the detail surface, provider link, reload, and a status update after approval.
Extend `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts` in `mobile-chrome`.
Cover task navigation, a real chip tap, the drawer reason/link, dismissal, and no horizontal overflow.
Use provider fixtures rather than browser-only state injection for the approval observation.
Run the existing scenarios in both files with the additions.

## Work orders

- [x] [Task 01: Collect workflow attention](task-01-collect-workflow-attention.md) (complete)
- [x] [Task 02: Explain workflow attention](task-02-explain-workflow-attention.md) (complete)

## Verification results

Implementation and verification on 2026-09-10:

- `go test ./internal/github -count=1`: 1,741 tests passed.
- `go test ./internal/orchestrator -run 'GitHub|Github|PRCI|PRCIAutomation' -count=1`: 51 tests passed.
- The focused frontend suite passed 149 tests, including workflow interpretation, notice, summary, chip, popover, and detail coverage.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Desktop PR E2E passed 10 tests; mobile PR E2E passed 8 tests.
- Public documentation tests passed 62 tests; the validator accepted 46 published pages.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
- Dedicated desktop and mobile approval-attention screenshots were captured and visually reviewed from `.pr-assets`.

PR fixup remediation on 2026-09-10 moved batched workflow enrichment inside the shared singleflight, kept unwatched lifecycle refreshes Actions-read-free, corrected workflow-run ordering and GraphQL schema usage, hardened legacy/null and mock-provider paths, and separated duplicate summary-row test IDs. The final local checks passed backend race testing, backend lint, frontend typecheck/lint/i18n gates, the focused 149-test frontend suite, and the targeted desktop/mobile browser scenarios.

## Risks

- The Actions API requires read permission that some existing credentials lack. The unavailable path is required.
- `action_required` is broader than approval. Classification requires the full evidence described in the design.
- Empty PR associations occur in the observed case. Rejecting all empty associations would preserve this defect.
- Latest-run selection must exclude obsolete attempts without hiding independent workflows on the same head.
- Both GraphQL and REST writers must carry the observation, or opening the detail panel can erase the reason.
