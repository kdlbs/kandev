---
created: 2026-10-20
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
legacy_specs: []
---

# Implementation Plan: GitHub rate-limit integration repair

## Overview

Repair the reproduced defects in PR #3143 while retaining principal-wide
admission and Workflow Sync ownership. The original four work orders are
complete. Task 05 contains the subsequent independent-review remediation.
Classification and state fixes precede the polling integration repair.
Implementation remains in the primary session. The user separately authorized
an independent read-only review session on 2026-10-22.

## Evidence and assumption check

The user requested repairs after the review on 2026-10-20. The reviewed PR head
was `00fce40d68869ed476f4602962ad2d61f32efa76`.
Issue #3176 and the accepted rate-coordination ADR establish the intended boundary.
The review inspected the complete diff, design records, and existing discussions.

Four temporary probes failed at that head:

1. `TestReview3143SecondaryDeadlineCannotMoveEarlier`: a ten-minute refusal,
   followed by a one-minute refusal, admits a request after two minutes.
2. `TestReview3143SuccessfulGraphQLDataIsNotRateLimit`: real CLI ExecuteGraphQL
   rejects successful data containing the title `Handle RATE_LIMITED errors`.
3. `TestReview3143LegacyIdentityChangeDoesNotShareQuota`: Alice's throttle blocks
   Bob after both clients pass through legacy coordination.
4. `TestReview3143IssueFiltersEventuallyReachEveryRepository`: three polling
   cycles issue three requests for the first repository and zero for the second.

The temporary source is `/tmp/pr3143-probe_test.go`. Logs are
`/tmp/pr3143-probes-all.log` and `/tmp/pr3143-cli-probe.log`.
These files are review evidence, not required implementation dependencies.
Work orders specify permanent regression tests that reproduce each condition.

Existing GitHub and Workflow Sync suites passed with the race detector during
review. That result does not cover the four failures. Required E2E CI failed
on mobile Quick Chat cancellation. Causality to this PR remains unproven.

The integration system owns the repair. RATE-001.4 and RATE-001.6 already cover
the refusal and payload defects. New RATE-002.6 and RATE-002.7 make human
isolation and eventual polling progress explicit. No unresolved product choice
blocks implementation. No new ADR is needed because the accepted ownership
and admission boundaries remain unchanged.

## Scope

### In scope

- Keep secondary refusal boundaries monotonic until success or expiration.
- Restrict GraphQL payload error evidence to the error array.
- Isolate legacy human trackers while retaining same-principal restrictions.
- Resume issue/review filters and distribute polling opportunities among watches.
- Add regression coverage at transport, service, and polling boundaries.

### Out of scope

- Replacing the coordinator, distributed admission, or a new scheduler framework.
- UI changes, database migrations, new settings, or quota dashboards.
- Unrelated mobile cancellation or walkthrough model configuration failures.
- GitHub comments, reviews, and merge. The user authorized commit, push, and CI
  verification after implementation.

## Technical approach

Task 01 changes `RateTracker.ObserveSecondary` in `ratelimit.go` under its existing
mutex. Preserve the maximum active retry time and the matching source.
Task 02 changes `classifyGitHubResponse` and the GraphQL payload path in
`rate_error.go` and `graphql.go`. Use structured error evidence for HTTP 200.
Task 03 changes legacy binding in `service.go` and its startup/token consumers.
Audit `provider.go`, `service_tokens.go`, and `auth_resolver.go` so subsequent
attachment cannot overwrite canonical state.
Task 04 adds scoped cursor state in `watch_poll_progress.go`.
`poller_watch_queue.go` rotates the combined issue/review inventory. It integrates the
issue/review fetch loops and `poller.go`. Return an internal outcome that retains
completed discoveries and distinguishes admission deferral from a complete sweep.
Keep public service signatures compatible where possible. Use the existing
batch-progress lifecycle and poller cursor patterns as references, not a generic
continuation framework.

### Compatibility matrix

| Path | Identity and transport | Required behavior and evidence |
| --- | --- | --- |
| PAT and App token | Verified principal, HTTP REST/GraphQL | Error-array classification and isolated canonical state, real fake-HTTP tests |
| Named CLI | Explicit host/login, gh subprocess | Same classification via subprocess fixture, preserve explicit account selection |
| Legacy PAT/CLI | Provisional identity, then verified human | Private provisional state, safe canonical binding, token/startup integration tests |
| Same human across workspaces | Canonical login, separate credential/cache scopes | Share quota only, retain workspace authorization tests |
| Unknown or unsupported identity | Existing resolver fallback | Keep observations provisional, never invent a known identity or broaden access |

## Tests

Task files contain full AC IDs. Test methods listed here are planned additions.

| Acceptance | Regression evidence |
| --- | --- |
| RATE-001.4, .5 | `ratelimit_secondary_order_test.go:TestSecondaryRefusalsPreserveLatestBoundary` and success/expiry cases |
| RATE-001.6, RATE-004.3 | `graphql_rate_error_test.go:TestGraphQLSuccessfulDataDoesNotThrottle` across PAT and CLI |
| RATE-002.1, .4, .5, .6 | `service_legacy_rate_identity_test.go:TestLegacyAccountReplacementIsolatesRateState` plus same-account rotation |
| RATE-002.2, .3, .7 | `poller_watch_progress_test.go:TestWatchPollingEventuallyReadsEveryFilter` plus mixed watches/principals |
| RATE-003.5, .7 | Existing Workflow Sync continuation suite in final focused package run |

## End-to-end evidence

Task 04 exercises poller -> workspace resolver -> real admission -> fake provider
transport -> discovery event and task reservation. Cover issue and review watches.
Check observable progress and provider request counts, not cursor internals alone.
No rendered UI changes require browser coverage in this package.

## Work orders

- [x] [Task 01: Preserve secondary refusal deadlines](task-01-secondary-deadlines.md)
- [x] [Task 02: Classify structured GraphQL errors](task-02-graphql-evidence.md)
- [x] [Task 03: Isolate legacy rate identity](task-03-legacy-identity.md)
- [x] [Task 04: Resume background watch polling](task-04-watch-progress.md)

Tasks are independent repair boundaries. Use the listed order in this single
session. Task 04 includes final integration checks across all four repairs.

- [x] [Task 05: Complete polling isolation and credential recovery](task-05-polling-and-credential-recovery.md)

An independent review on 2026-10-22 found remaining integration gaps in PR
workspace ordering, legacy quota gates, and credential-recovery fingerprints.
The user authorized remediation and delivery. Task 05 addresses those findings
within the existing admission and recovery boundaries.

## Companion records

The original `github-rate-limit-coordination` and September
`github-rate-limit-pr3143-repair` plans remain historical receipts. Their earlier
completed work orders do not certify the current head. This package tracks the
integration findings. Preserve those historical counts and scopes.

## Verification results

The first four work orders and their package verification are complete. All four original
failures have permanent regression coverage. Token replacement also invalidates
cached workspace clients, as proved by an additional service regression.

- Full GitHub race suite: passed in 271.518s.
- Full Workflow Sync race suite: passed in 1.302s.
- CI-pinned golangci-lint v2.9.0: zero issues.
- Specification catalog, specification lint, public docs, work-order coverage,
  and whitespace checks passed.

[Task 04](task-04-watch-progress.md#results) records the exact final commands.
All changed Go files in the task workspace match the tested isolated checkout.
Local verification is complete. Delivery and current-head CI are tracked in the
conversation because each push invalidates the previous CI snapshot. No live
GitHub quota test or browser E2E run was performed locally.
Design-package checks passed on 2026-10-20:

- `python3 scripts/list-docs.py validate`: 378 decisions, 1547 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `git diff --check`: passed.
- `node /tmp/pr3143-plan-preflight.cjs`: all four work orders covered, no errors.
  This calls the repository's `validateCoverage` with local documents and a
  representative planned backend change. It does not publish a GitHub status.
- `git status --short -- docs/plans/github-rate-limit-integration-repair`:
  the new package is untracked and remains unstaged for review.

Task 05 is complete. Its final focused race tests, full current-main integration
race suites, backend lint, documentation checks, and independent rereview passed.
[Task 05](task-05-polling-and-credential-recovery.md#results) records the evidence.

## Risks

- Wrong provisional-state transfer can either cross identities or erase a throttle.
- Watch cursors need configuration/credential invalidation and fair watch ordering.
- Deferral handling must retain completed discoveries and preserve orphan cleanup.
- In-memory progress resets on restart. Workflow Sync retry persistence must remain unchanged.
- CI evidence belongs to one head SHA. Every push requires a fresh result.

## Workspace

The repair is now in the task workspace on local branch
`fix/pr-3143-rate-limit-repair`. The earlier fetch supplied enough objects to
check out the reviewed head without altering the unrelated broken ref.
Implementation and test execution used `/tmp/kandev-review-3143`. The task
workspace received the identical repair diff. The user authorized delivery on
2026-10-21 after reviewing the implementation results.
The unrelated ref `refs/heads/feature/lead-pr-4371-to-merg-eeb` remains untouched.
