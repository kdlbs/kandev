---
created: 2026-10-29
status: complete
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-005
  - REQ-PLATFORM-INTERACTIVE-READS-007
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
legacy_specs: []
---

# Implementation plan: Quiet task refresh recovery

## Overview

Keep usable task content visible through short outages and show refresh warnings only after automatic recovery fails.
First bound sidebar recovery. Then apply the same warning policy to task details and cold task navigation.
Platform owns shared availability. UI retains sidebar query semantics, and Tasks retains record and permission authority.

## Evidence and scope decisions

The live backend rejected task and sidebar reads during two thirteen-second persistence-admission outages on October 29, 2026.
At 19:48:35.324 and 19:49:35.317 UTC, the writer health probe exceeded its two-second deadline.
Both samples showed one occupied writer connection and no occupied reader connections.
The gate recovered at 19:48:48.317 and 19:49:48.317 UTC.

During this task's creation, logs contain 39 failed sidebar queries, four failed task reads, and five failed session-list reads.
The sidebar queries repeat approximately every 250 ms.
These are bounded page-query POSTs, not transcript pagination or message backfill requests.
Existing WebSocket task notifications continued during the API admission failure.
Server logs normalize route paths and omit query bodies and caller identifiers.
The cadence matches the reproduced provisional-page loop. Those logs alone cannot attribute each request to that loop.

`useProvisionalSidebarRefresh` schedules another refresh whenever a provisional page has no pending request.
A rejected refresh leaves that page provisional and sets pending to null, which rearms the timer.
`loadPage` clears the error before each request and restores it after failure.
This cycle explains both repeated requests and the disappearing warning.

A temporary hook reproduction made eight attempts in two seconds without another event.
Its assertion, `expected 8 to be less than or equal to 1`, failed as expected.
The temporary test was removed. Task-detail source separately confirms first-error notice presentation during automatic recovery.

The running build was `f2e47b55d3814d8d2b6a399df525abcaf80b246e`.
Its relevant health, admission, sidebar, and task-read files match this checkout.
Bundle `e8f286a2d16f3cc36f47ed9e25cf9f48` contains both incidents despite older-backend truncation and bounded browser history.
A sanitized local receipt is `.kandev/diagnostics/task-refresh-2026-10-29T1948Z.json`.
The package does not depend on that ignored file.

The exact operation holding the writer remains unknown.
Retention deadlines occur nearby, but they do not prove that retention owns the blocked connection.
This delivery addresses recovery pressure and warning presentation. It cannot claim to repair the writer stall.

The user requested warnings only as a last resort, without an exact delay.
This proposal reuses the existing thirty-second visible-tab budget and five-attempt schedule.
No additional setting or architecture decision is necessary for that numerical policy.

## Scope

### In scope

- Shared, bounded sidebar recovery with terminal failure admission and scope cancellation.
- Quiet automatic recovery with retained rows, task details, and authorized task projections.
- Stable terminal warnings, manual Retry, and foreground recovery on desktop and phone.
- Focused regressions, affected existing fixtures, and public recovery guidance during implementation.

### Out of scope

- Database writer attribution, health-probe changes, SQL work, or additional database connections.
- Mutation retries, queued writes, page reloads, new settings, dependencies, or feature flags.
- Sidebar filter semantics, reusable-page budgets, task/session creation, and navigation redesign.

## Technical approach

Task 01 keeps `SidebarTaskPageCache` as the per-store shared owner.
Its private runner reuses `TaskNavigationRecoveryClock` and classifies known temporary failures through one neutral HTTP helper.
Consumer leases retain failed-cycle state across settlement. Background triggers cannot erase or restart terminal failure.
The hook keeps a request pending throughout recovery and preserves the existing provisional reconciliation path after accepted success.

Task 02 adds exhausted-warning state to the existing `TaskNavigationReads` snapshot.
`TaskNavigationReadFeedback` uses that state instead of treating every temporary failure as a visible warning.
Cold routes keep ordinary progress until exhaustion. Existing task projections, selected sessions, and drafts retain their current eligibility guards.
Manual or foreground recovery keeps an existing terminal notice visible until success.

The [technical design](../../specs/platform/system-design/interactive-read-availability.md#quiet-task-refresh-recovery) defines ownership, deadlines, cancellation, and responsive behavior.
Preserve the [persistence admission decision](../../decisions/2026-09-05-required-internal-persistence.md).
There is no HTTP, WebSocket, schema, or operator configuration change.

## ASCII UI preview

UI-01: Automatic recovery with usable content. Entry: sidebar refresh, task navigation, or foreground return.

```text
Desktop                          Phone task picker / task route
TASKS [View] [Filters]            [Tasks] [Close]
Task A                           Task A
Task B                           Task B

[Task header]                    [Task header]
[Existing conversation]          [Existing conversation]
[Draft remains]                  [Draft remains]
```

No visible warning row, strip, or extra gap exists during automatic recovery.
One screen-reader status announces the cycle. The existing list and conversation retain their scroll owners.
Phone navigation remains its current task picker or app menu. No new drawer appears for recovery.
This structure maps to AC-007.1, .3, .6, and .7 under `AC-PLATFORM-INTERACTIVE-READS`.

UI-02: Exhausted recovery. The sidebar and detail regions each report their own failed resource.

```text
Desktop sidebar                 Phone Tasks picker
TASKS [View] [Filters]           Tasks
Tasks could not be refreshed.   Tasks could not be refreshed.
[Retry]                         [             Retry             ]
Task A                          Task A

Desktop task detail             Phone task detail
[Task header]                   [Task header]
[Refresh notice] [Retry]        [Refresh notice]
[Conversation]                 [             Retry             ]
                               [Conversation]
```

Keep the existing in-flow status locations, localized copy, and phone safe areas.
Retry remains visible and disabled during explicit recovery. Success removes the warning.
Fine-pointer controls retain 28px sizing. Phone and coarse-pointer Retry targets measure at least 44px.
This maps to AC-007.4, .5, and .7. Spacing and text wrapping are illustrative.

UI-03: No eligible content. Desktop and phone share ordinary loading progress and task-overview access.
Only exhausted recovery changes that surface to its existing unavailable copy and enabled Retry.
This maps to AC-007.8 and existing AC-005.2, .5, .6, and .10.

## Tests

| Acceptance | Focused regression |
| --- | --- |
| AC-007.1-.3 | Cache/runner/hook tests: provisional failure, five attempts, thirteen-second recovery, and shared consumers |
| AC-007.3-.4 | Hook tests: all trigger families respect delay and exhaustion; explicit recovery keeps a warning stable |
| AC-007.5-.6 | Owner tests: access denial, invalid filters, late results, hidden tabs, Retry-After, attempt deadlines, and final release |
| AC-007.1, .4, .7-.8 | Component/route tests: quiet progress, terminal strip, cold load, geometry, and selected-session/draft continuity |

Every abbreviated AC in this package uses the `AC-PLATFORM-INTERACTIVE-READS` prefix.
Work-order frontmatter contains the full identifiers.

## E2E tests

Extend desktop and phone sidebar pagination fixtures for a timed outage, exhaustion, and manual recovery.
Update existing fixtures that currently expect a warning after one failed request.
Keep filter validation, access denial, disclosure continuity, and conversation-preservation scenarios.

Extend `task-transient-read-recovery.spec.ts` and its mobile counterpart.
Use a retained task projection and a loaded secondary session with an unsent draft.
After the first failure, assert no visible warning and no content displacement.
Release the outage after thirteen seconds and require recovery without another event.
For exhaustion, await the fifth response barrier before checking the warning and Retry.
Preserve the existing cold-load, manual recovery, and permanent-unavailable scenarios.

Use causal HTTP waits and request counters. Do not replace them with large assertion timeouts.
Use the existing sanctioned latency/negative-observation helpers when the scenario requires elapsed time.
Capture desktop and phone quiet and exhausted states, comparing their geometry to UI-01 through UI-03.

## Work orders

- [x] [Task 01: Bound sidebar refresh recovery](task-01-bound-sidebar-recovery.md)
- [x] [Task 02: Defer task refresh warnings](task-02-defer-task-warnings.md)

Execute sequentially. Task 02 uses the shared classification introduced by Task 01.
Existing task-read and sidebar-loading packages retain their historical completed statuses and verification results.
Record changed scenarios and new results here instead of rewriting those receipts.

## Verification results

Investigation: the temporary hook reproduction failed for the intended eight-attempt observation. No production or permanent test changes exist.

Design checks passed on October 29, 2026:

- `python3 scripts/list-docs.py validate`: 417 decisions and 1,842 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation coverage: the actual documentation diff is exempt. Both work orders cover a synthetic sidebar source change without errors.
- `git diff --check`: passed. New work orders remain unstaged and uncommitted.
- Plain-language review: descriptive sentences in the new plan files stay within twenty-five words.

The temporary hook test, raw diagnostic archive, and extraction directory were removed after the sanitized receipt was saved.
No isolated instance or browser session was launched. The live instance was not restarted or mutated.

Implementation results on 2026-10-29:

- `apps/web` typecheck passed with the repository test configuration.
- Focused Vitest suites passed for classification, the sidebar recovery cycle, the page cache, the sidebar hook, disclosure and removal fixtures, task-navigation reads, the recovery clock, task-detail routes, and task-page components.
- Full `apps/web` Vitest run for `lib`, `hooks`, `components`, and `src`: 2,966 files passed, 27,073 tests passed, four skipped.
- ESLint reported no errors. One soft file-size notice remains in a test file; no production warning was added.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run e2e:sleep-ratchet` passed without new catalog or sleep debt.
- `node scripts/validate-public-docs.mjs` validated 54 pages, and its 63 unit tests passed.
- Playwright passed for desktop task transient read recovery (4), desktop task loading state (4), desktop sidebar pagination (3), phone task transient read recovery (3), and phone sidebar pagination plus drawer recovery (5). Exact scenarios and timings are recorded in each work order.
- Specification validation, all 36 linter tests, specification lint, and the PR-documentation coverage preflight passed. `git diff --check` is clean.

Review remediation on October 30, 2026:

- A foreground event observed during an active sidebar cycle is now claimed by the shared recovery record, so it cannot start another cycle after exhaustion. Queued refreshes are discarded once a terminal warning latches; only a genuinely later foreground episode or manual Retry admits a new cycle.
- Post-remediation suites passed: `lib/sidebar`, `hooks/domains/kanban`, and `lib/state` (241 files, 2,503 tests) and the focused 10-file suite (126 tests).
- The manual-Retry E2E helper now requires an enabled Retry, waits for the corresponding successful sidebar response, and fails on interaction errors. Both recovery scenarios first wait for the fifth rejected attempt before asserting exhaustion.
- Final Playwright rechecks with the remediated code: desktop sidebar pagination 3 passed (2.1 minutes); phone sidebar pagination and drawer recovery 5 passed (3.6 minutes).

## Risks

- Clearing an error at request start can recreate warning flicker after exhaustion.
- Discarding the failure record at promise settlement can let another consumer restart a failed cycle.
- A delay without scope cancellation can issue reads for an obsolete account or query.
- Quiet retained content must not conceal permission loss or reuse another task's conversation.
- The backend writer incident remains unresolved after this frontend correction.
