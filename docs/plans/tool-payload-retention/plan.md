---
created: 2026-09-14
status: completed
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
legacy_specs: []
---

# Tool Payload Retention Implementation Plan

## Overview

Add optional cleanup of old tool payloads to `/settings/system/data-storage`.
Keep tool messages and metadata. Provide savings analysis, a backup choice,
initial cleanup, and daily cleanup under the saved policy.

The user subsequently authorized implementation and native subagent support.
The primary session owns integration and verification. No production database
maintenance, commit, push, or deployment is included.

## Inputs and assumptions

- [Requirements](../../specs/system-page/requirements/tool-payload-retention.md)
- [System design](../../specs/system-page/system-design/tool-payload-retention.md)
- [Decision](../../decisions/2026-09-14-tool-payload-removal-policy.md)
- Age basis: task inactivity, with conservative task/session update protection.
- Initial age: three calendar months. Cleanup remains disabled by default.
- Initial engine: SQLite. Analysis is optional. The backup choice is explicit.

Previous copy-only audits motivated the feature but do not provide its live
savings estimate. Different removal rules and eligibility change the result.
Do not reuse prior database-wide estimates as a promise for this policy.

## Scope

Include policy persistence, supported field removal, analysis, backup preparation,
guarded writes, replay protection, removed-output rendering, scheduling, desktop
and phone settings, access control, localization, and public documentation.

Exclude compression, cold storage, attachment deletion, full-message deletion,
automatic compaction, and changes to Office/filesystem retention settings.

## Technical approach

Use one reducer for analysis and cleanup. Ground candidate queries in the existing
session message indexes. Preserve marker state at the repository write boundary.
Persist only singleton policy/runtime records and bounded progress.

Compose `internal/system/toolretention` through the system service. Reuse backup
creation and job notifications, while retaining durable preparation state.
Keep all analysis and cleanup work outside startup readiness.

Add an independent settings card and save contributor. Use inline preparation
review and existing backup/compaction controls. Reuse the same state model for
desktop and phone. No extra release flag is proposed: the setting defaults off.

## ASCII UI preview

These views specify hierarchy, actions, and states. Spacing and sample values
are illustrative. Product copy must use localization keys.

### UI-01: Desktop, disabled policy with optional estimate

Entry: Settings > System > Data & Logs. The existing Database section precedes
the retention area. Office history retains its independent card.

```text
Tool payload cleanup                              [Off]
Keep tool messages and summaries. Remove old details.
Tasks inactive for  [3] [Months v]    [Analyze savings]

Estimated payload reduction: <size>
<messages> tool messages across <tasks> tasks
Analyzed <time> | <skipped> unsupported/protected items
Database file shrinking requires a separate compaction.

Last cleanup: Never              Next check: Disabled
                                         [Run cleanup now] (disabled)
```

The existing shared Save/Discard bar owns persisted changes. Analyze uses the
draft and does not save or enable it. Policy edits mark the estimate stale.

### UI-02: Phone, first-cleanup review

Entry: direct Data & Logs route or the existing phone settings navigation.
Use the route scroll owner. Age controls and actions stack for thumb access.

```text
< System           Data & Logs

Tool payload cleanup     [On*]
Tasks inactive for
[3                         ]
[Months                   v]
[Analyze savings           ]
Estimate: <size> of payloads
<messages> messages

Before the first cleanup
Removed details cannot be undone
without restoring a database backup.
( ) Create backup (recommended)
( ) Continue without backup
Backups can take time and disk space.

Last cleanup: Never

[Discard]              [Save]
```

`On*` denotes an unsaved draft, not effective enablement. Neither radio choice
is submitted automatically. The shared Save bar respects the safe area and
keyboard. Controls have 44px touch targets. No nested confirmation dialog.
Desktop uses the same inline review below its estimate.

### UI-03: Progress, failure, and completed states

```text
Analysis: <scanned> messages scanned       [Cancel]
Analysis failed: <localized reason>       [Retry analysis]
Estimate: Not analyzed / Stale / No eligible payloads

Preparing backup: cleanup blocked         [Cancel preparation]
Backup failed: <localized reason>         [Retry] [Cancel]

Cleanup: <count> messages processed       [Cancel run]
Last cleanup: Partial | <size> removed | <remaining> remaining
Next check: <time>                        [Run cleanup now]
```

Read-only users see status and an admin-only explanation. Unsupported engines
show an unavailable state. Neither state mounts enabled mutation controls.
Partial results remain visible with their failure reason. Canceling a run keeps
the policy enabled for the next daily check. Disabling stops later batches.

### UI-04: Conversation with removed payload

```text
[tool icon] <original tool title>          <original status>
Tool details removed by the retention policy on <date>.
<retained summary, such as exit code or file count>
```

The same row appears on desktop and phone. It replaces loading/expand-output
controls for removed details. Existing message grouping and navigation remain.

UI-01/02 map to AC-001.1–001.7 and AC-002.1/002.2. UI-03 maps to
AC-002.6 and AC-003.1–003.6. UI-04 maps to AC-002.3/002.4.
All abbreviated AC references use the `AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-` prefix.

## Work orders

The primary session integrates dependent contracts in order. With explicit user
authorization, native subagents implemented separate reducer/replay, backup, UI,
and browser-test files. Shared runtime integration remains in the primary session.

| Order | Work order | Result |
| --- | --- | --- |
| 1 | [Policy, reducer, and analysis](task-01-policy-and-analysis.md) | Tested read-only analysis and eligibility boundary |
| 2 | [Guarded cleanup and replay](task-02-guarded-cleanup.md) | Backup gate, bounded mutation, durable marker |
| 3 | [Settings and conversation UI](task-03-settings-and-transcript.md) | Usable desktop/phone workflow and removed-output state |
| 4 | [Scheduling and integration](task-04-scheduling-and-integration.md) | Daily operation, restart evidence, docs, full feature verification |

## Tests

The test files below cover the implemented behavior. Work orders record
verification evidence and assertions about committed database state.

| Criteria | Test location and evidence |
| --- | --- |
| 001.1–001.3, 001.7, 003.6 | `internal/system/toolretention/policy_test.go`: defaults, ranges, revisions, authorization, engine gating |
| 001.3–001.5, 003.2 | `internal/system/toolretention/analysis_test.go`, `internal/task/repository/sqlite/tool_payload_retention_scan_test.go`: UTC boundaries, no writes, paginated scale, malformed data |
| 002.3/002.4 | `internal/task/models/tool_payload_retention_test.go`: exact field preservation, unknown structures, idempotence, byte accounting |
| 002.1/002.2/002.5–002.7, 003.3/003.4 | `internal/system/toolretention/runner_test.go`: backup failure/crash, transactional activity race, policy disable, partial progress |
| 002.4 | `internal/task/repository/sqlite/tool_payload_retention_scan_test.go`, `internal/task/service/service_messages_payload_retention_test.go`: stale replacement, replay, resume |
| 003.1–003.4 | `internal/system/toolretention/scheduler_test.go`: fake clock, startup readiness, daily schedule, busy backoff, restart/cancel |
| 001.4–001.7, 002.1/002.2, 003.4/003.5 | `components/settings/system/tool-payload-retention-card.test.tsx`: draft, choice, error, job states, independent Office settings |
| 002.4 | Transcript renderer tests and `hooks/domains/system/use-tool-payload-retention.test.ts`: removed marker, output cache invalidation |

## E2E Tests

- `e2e/tests/system/tool-payload-retention.spec.ts`: analyze while disabled,
  enable with backup, wait for cleanup, reload settings, inspect original tool
  metadata and removed details in a seeded old task. Protect a recently active task.
- Same desktop file: explicit skip, stale estimate, disabled policy, and independent
  Office retention behavior. Component and backend tests cover backup failure,
  cancellation, partial results, and restart boundaries.
- `e2e/tests/system/mobile-tool-payload-retention.spec.ts`: repeat the activation
  and conversation flow at 390px and 320px, including keyboard and safe-area behavior.
- Extend existing auth data-storage member-gating tests for denied analysis and
  mutations. Use the `auth` project and its existing mobile project ownership.

Use disposable fixture databases only. Seed payloads through test fixture helpers,
never the user's production database or retained backups. Restore shared settings
in teardown. Assertions must prove payload removal, not just a success toast.

## Verification

Each work order contains exact commands. Run focused checks after its changes.
The final validation covers backend, frontend, browser flows, and documentation.

Design-package checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/tool-payload-retention docs/decisions
```

## Risks

- Conservative activity and ownership checks can retain old legacy records.
  Eligibility deadlines also retain large tasks and produce partial results.
- Large legacy JSON needs bounded parsing. Oversize/unknown rows stay intact and
  reduce achievable savings. Do not conceal these skips.
- Backup and compaction can take substantial time and disk. Keep preparation
  explicit and do not add either operation to automatic startup work.
- Stale message writes can restore details unless the repository enforces markers.
- SQLite page reuse does not immediately shrink files or existing backups.
- Keep the tool payload and Office save contributors independent. The route
  design now records both without changing the Office policy.

## Verification results

Implementation and final integration validation are complete.
No production database or retained production backup was opened for this work.

Design validation on 2026-09-14:

- `list-docs.py validate`: passed, including the new requirement/design pair and ADR.
- `lint-spec-files.test.py`: all 36 tests passed.
- `lint-spec-files.py --all`: passed.
- New-document relative links and whitespace: checked before handoff.

Runtime verification results are recorded below.


### Implementation verification (2026-09-14)

- Broad backend tests passed for system, task models, SQLite repositories,
  task service, handlers, and persistence, including store conformance.
  Focused race checks passed for the changed reducer/replay, eligibility,
  backup, maintenance, and retention paths.
- Full backend lint and SQL portability guard passed. Full web lint passed with
  zero warnings/errors. Typecheck, localization validation, and the i18n ratchet
  passed. The focused frontend run passed 90 tests; 29 affected tests passed
  again after lint refinements.
- Fresh managed backend and frontend builds and fixture plugin packaging passed.
  All eight browser tests passed sequentially: two desktop retention flows,
  two phone flows (320px/390px), and four desktop/mobile member/admin checks.
- `go test -race ./internal/system/toolretention -run TestAdmission -count=1 -v`
  proves both transaction orderings and foreground commits between batches.
  The disposable 307-message fixture took four batches with three intervening
  foreground commits. It removed 2,101,108 logical bytes and freed 307 pages.
  The main file stayed at 2,650,112 bytes after cleanup; explicit VACUUM reduced
  it to 278,528 bytes. These measurements describe this fixture only.
- A 250,000-message eligibility fixture hit its 200ms budget and retained the
  task. The pass exposes that skip as partial. Very large or ambiguous tasks
  can therefore save less than the configured age alone suggests.
- The optional PostgreSQL replay integration test was skipped because no test
  DSN was configured. SQLite is the supported retention engine. An initial full
  task-service race run exceeded its ten-minute limit; the isolated affected
  tests and the later broad non-race suite passed.

The final cancellation fix passes deterministic regression tests and the full
retention package race suite (4.477 seconds). It covers writer-held snapshots,
pending preparation, operation-ID handoff, invalid requests, transactional CAS,
and failed persistence. Scoped lint reports zero issues. No production database
or retained backup was read or modified. No commit, push, or deployment occurred.


Final confirmation after the cancellation fix:

- Fresh managed backend/frontend builds and fixture packaging passed again.
- Both desktop backup/skip browser tests passed against the final backend.
  The preceding eight-scenario desktop/phone/auth run remains green.
- Full backend lint passed again with zero issues. SQLguard, document index,
  specification lint, published-page validation, and whitespace checks passed.
- All four work orders are complete. Changes remain uncommitted for review.

## Review remediation

Both P2 findings from the uncommitted-workspace review are resolved:

- A normal worker tick recovers orphaned preparation after final-state persistence
  fails. Revision/operation CAS protects replaced work; cleanup remains disabled
  and preparation becomes retryable without restart or manual cancellation.
- Guarded replay preserves shell exit-code summaries in live-event metadata and
  API projections. Unknown large numbers retain precision; removed output stays
  absent.

Focused RED tests reproduced both failures. Focused race tests cover recovery,
replacement protection, failure handling, real SQLite replay, and projections.
The full retention and model package race suites passed (5.085s and 1.448s).
Logs: `/tmp/retention-review-red.log`, `/tmp/retention-review-replay-red.log`,
`/tmp/retention-review-green.log`, `/tmp/retention-review-package-race.log`.
No broad build/test rerun, commit, push, deployment, or production data access
was performed during remediation.
Scoped lint passed with zero issues (`/tmp/retention-review-lint.log`). The
additional running/terminal backup failure-handler race test passed
(`/tmp/retention-review-terminal-race.log`). Whitespace checks passed.

## Approved UX refinement (implemented)

This refinement supersedes the earlier layout previews. Messages compaction
uses the existing shared Save changes/Discard controls and explicit backup
consent; the automatic schedule is unchanged.

Desktop (bounded card width):

```text
Messages compaction
Remove old tool inputs and outputs; keep message metadata.
Tasks inactive for [3] [Months v] [Analyze savings]
Analysis outcome and timestamp
> Analysis details
------------------------------------------------------
[off] Automatic compaction
Checks every 24 hours while Kandev is running.
First run starts after backup preparation.
[Backup choice appears here when enabling]
Last run: <status and time>   > Run details
Next check: <time or Disabled>
[Compact messages now]
Space reuse and database file compaction explanation
```

Phone: number and unit remain adjacent; Analyze savings takes its own full-width
row. Controls and disclosure summaries have 44px touch targets. The existing
settings page owns scrolling and the shared Save bar. Partial results, stale
estimates, and errors remain visible outside collapsed details.

Validation: focused component/hook tests, desktop backup/skip flows, and phone
flows at 320px and 390px, including disclosure and control geometry checks.
User requested no commit.

## Follow-up presentation package

The [settings storage tabs package](../settings-storage-tabs/plan.md) owns the planned header tabs and maintenance presentation changes.
This package retains its historical implementation results. The follow-up owns new route, copy, and regression verification.
