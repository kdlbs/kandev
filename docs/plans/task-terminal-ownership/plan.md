---
created: 2026-11-06
status: implemented
requirements:
  - REQ-TASKS-TASK-TERMINALS-002
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
legacy_specs: []
---

# Implementation plan: Task terminal ownership

## Overview

Repair [issue #5105](https://github.com/kdlbs/kandev/issues/5105) across terminal
selection and attachment. Implement the backend correction, then browser scope
isolation, then panel recovery and focused browser regressions. Work remains
sequential in the primary session. This package does not authorize implementation.

The tasks system owns ordinary terminal identity and lifecycle. Its existing
[requirement](../../specs/tasks/requirements/task-terminal-persistence.md) now
defines `REQ-TASKS-TASK-TERMINALS-002`. The reusable read coordinator retains its
[UI requirements](../../specs/ui/requirements/task-navigation-responsiveness.md).

## Evidence and confirmed causes

Investigation used commit `628da65a0e0` on 2026-11-06. The issue has no image
attachments. The reporter's private database and logs were not available here.

1. `useUserShells` includes the task in request identity, but reads and writes
   rows and settlement flags under the environment alone.
2. `SessionRead.group` also grants shell publication ownership per environment.
   Distinct task readers can suppress each other and consume the same result.
3. `useEnsureDefaultTerminalOrdinary` selects the lowest ordinary sequence from
   that mixed cache. It never revalidates a non-`shell-default` ID. The wrong
   ID can therefore survive the next correct list and layout restoration.
4. The current shell WebSocket URL contains environment and terminal IDs,
   without a session ID. `resolveOrdinaryTerminalLaunch` instead supplies the
   transport execution's task to `ClaimShellLaunchInfo`. Environment execution
   lookup can return any task's execution in that shared environment.

The report's frontend session/terminal explanation is therefore incomplete for
this checkout. Both browser state and gateway ownership need correction.
`AuthorizeTaskEnvironmentAccess` already supports inherited and shared-group
environments through task/session bindings. Its check remains mandatory.

Temporary reproductions used real hooks/store coordination and real terminal
repository/service code. All temporary files were removed after diagnosis:

- A-to-B-to-A hook test: expected task A to exclude `shell-B`. It returned
  `shell-B` while A's refresh remained pending.
- Concurrent-reader hook test: expected independent A/B results. Both readers
  returned `shell-B`. All eight existing shell-read tests passed.
- Gateway test: a valid B terminal in the shared environment returned
  `terminal not found` with an A execution. A direct B ownership lookup passed.
- Four existing service tests passed: launch environment/task validation and
  cross-task rename, park, and destroy rejection.

## Scope

### In scope

- Resolve gateway ownership from persisted metadata before command consumption.
- Filter task/environment list responses and preserve task-only HTTP listing.
- Isolate ordinary rows, read writers, flags, tombstones, and mutations by scope.
- Preserve environment sharing for fixed, script, and unmanaged legacy terminals.
- Revalidate restored panels and retire stale selection publishers.
- Prove desktop and phone output through real shared-workspace task fixtures.

### Out of scope

- Issue #5104, agent recovery, or changes to task/workspace lifecycle.
- Database migrations, terminal numbering, new wire fields, or new permissions.
- Quick Terminal, terminal rendering, control placement, or retry-policy changes.

## Technical approach

The [task design](../../specs/tasks/system-design/task-terminal-persistence.md)
owns gateway resolution, result namespaces, and panel reconciliation. The
[read design](../../specs/ui/system-design/task-navigation-responsiveness.md#shared-session-reads)
owns coordination and obsolete-response protection.

Task 01 adds an internal owner lookup that validates the environment without
consuming commands. The gateway authorizes the resolved pair before claiming
launch metadata. Both local and remote paths reuse this boundary.

Task 02 introduces one typed task/environment scope. Ordinary results move to
`userShells.byScope`. Existing environment maps retain unmanaged resources only.
Shell publication groups use the same tuple. Every caller captures its scope
before an asynchronous action and uses it for result publication.

Task 03 gates ordinary attachment on valid ownership. It repairs the default
panel and removes invalid extra panels without stopping their referenced PTYs.
Default selection preserves a valid open ID, then prefers the lowest eligible
sequence, then creates once after a successful empty eligible list.

No new ADR is necessary. Persisted terminal ownership already defines the
authority. An environment-only cache or a browser-only repair cannot satisfy
the reproduced failure paths.

Compatibility coverage:

| Boundary | Required behavior | Evidence |
| --- | --- | --- |
| Local PTY | Correct row owner with a sibling task's execution | Gateway regression and desktop shell output |
| Remote shell bridge | Same authorized owner resolution before start/stream | Existing fake-agentctl fixture with shared-task ownership |
| Fixed/script terminals | Existing environment sharing and launch behavior | Store/hook tests and existing terminal E2E |
| Missing/foreign target | Existing denial, no command claim or PTY start | Gateway denial tests and missing-environment regressions |
| Same task, two sessions | Same owned terminal and scrollback | Existing `terminal-env-keyed.spec.ts` |

## ASCII UI preview

These previews describe selected content, not a new layout or new copy.
`Connecting` reflects the reported state. Prompts and identifiers are illustrative.

UI-01: Desktop, task A after opening its Terminal tab.

```text
Before: [Task A] [Terminal]   Connecting... (references B's shell)
After:  [Task A] [Terminal]   $ echo A-marker
                             A-marker
```

UI-02: Phone, Terminal navigation and existing terminal picker.

```text
+--------------------------+
| Task A                    |
| Terminal             [v]  |
| $ echo A-marker           |
| A-marker                  |
|                          |
| Ctrl Shift Tab Esc ...    |
+--------------------------+
```

Owned content and recovery are required. Desktop split geometry, phone picker
geometry, key-bar order, safe areas, and scrolling retain their shipped behavior.
The closest phone exemplar is `mobile/mobile-terminal-pane.tsx` with
`MobilePillButton` and `MobilePickerSheet`. There is no new surface or copy.
Task 03 verifies UI-01 and UI-02 through the corresponding browser projects.

## Tests

- `.002.1`, `.002.6`: new `terminal_ownership_test.go`, `launch_info_test.go`,
  and `shell_scope_test.go`. Cover both task owners, wrong environment, denied
  access before command claim, missing metadata, and unchanged mutation guards.
- `.002.2`, `.002.5`: deferred tests in `use-user-shells.test.tsx` and
  `session-read-coordinator.test.ts`. Cover both response orders, same-scope
  sharing, taskless transitions, failure, reconnect, and restoration retirement.
- `.002.2`, `.002.7`: store and real-consumer mutation tests. Cover ordinary
  isolation, shared scripts, scoped tombstones, and last-reference cleanup.
- `.002.3` through `.002.5`: default initializer and panel tests. Cover mixed
  open/parked/foreign inventory, restored IDs, empty success, failed reads,
  duplicate consumers, and create completion after navigation.
- Preserve `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2` and `.5` through existing
  coordinator suites. Exact commands belong to each work order.

## E2E tests

Task 03 adds `terminal-task-ownership.spec.ts` and
`mobile-terminal-task-ownership.spec.ts` under `e2e/tests/terminal/`.
Use a real parent and `workspace_mode: "inherit_parent"` child. Assert distinct
task IDs and equal environment IDs before the terminal stimulus.

Both projects cover A-to-B-to-A, page reload, real marker input/output, scoped
terminal menus, and no foreign terminal mutation. Desktop also restores a
contaminated default panel. Phone tests use its visible navigation and picker.
These flows cover `.002.1` through `.002.7`. Unit tests cover held reads and
authorization edges without artificial browser delays.

Retain existing `terminal-first-class.spec.ts`, `terminal-env-keyed.spec.ts`,
and desktop/phone missing-environment tests. Managed runs remain sequential,
with the configured worker budget and no full-suite invocation.

## Work orders

- [x] [Task 01: Resolve persisted terminal ownership](task-01-backend-terminal-owner.md)
- [x] [Task 02: Isolate browser terminal scopes](task-02-browser-terminal-scopes.md)
- [x] [Task 03: Reconcile panels and verify shared tasks](task-03-panel-recovery.md)

Task 03 depends on Tasks 01 and 02. Tasks 01 and 02 have independent verification
boundaries. The planned execution order remains 01, 02, 03.

## Verification results

Investigation: three temporary regressions failed for the expected causes.
Eight existing hook tests and four existing service tests passed. The package
was implemented on 2026-11-06. Implementation checks:

- `python3 scripts/list-docs.py validate`: 436 decisions and 1929 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed after shortening the
  modified navigation-design paragraphs to stay within the existing size limit.
- `.github/scripts/pr-docs.cjs::validateCoverage`: actual document-only changes
  are exempt. A second pass with the planned runtime paths validated all three
  work orders and their four requirement/design references as covered.
- `git diff --check -- docs/specs docs/plans/task-terminal-ownership`: passed.
  Working-tree status confirms all three new work orders remain untracked.

Implementation and browser verification passed: Go build, race tests, and
changed-file lint; web typecheck and lint; 324 focused unit tests; 12 Chromium
and 3 mobile-chrome E2E tests; and the documentation/specification checks.
The PR-documentation coverage preflight returned `covered` for all three work
orders and their requirement/design references. Exact commands and results are
recorded in each work order.

## Related packages and documentation

- [PostgreSQL terminal queries](../postgres-task-terminal-queries/plan.md)
  remains complete. Preserve repository rebinding and its verification record.
- [Task navigation responsiveness](../task-navigation-responsiveness/plan.md)
  owns the existing coordinator. This follow-up narrows shell publication scope
  without reopening its unrelated Files, layout, or performance work.
- [Stale terminal environment](../stale-terminal-environment/plan.md) owns
  permanent missing-target handling. Preserve that behavior.

Internal specifications change in this design turn. During Task 03, add one
explanation paragraph to `docs/public/developer-tools.md#integrated-terminal`:
ordinary terminals remain task-owned when tasks share a workspace. No new
control instructions or screenshots are necessary.

## Risks

- Gating on settlement alone can treat failed reads as a successful empty list.
- An omitted writer can reintroduce cross-task rows or late panel publication.
- Closing invalid restored panels can accidentally park a foreign PTY unless
  cleanup explicitly suppresses the terminal mutation.
- Owner lookup must not consume a one-shot command before pair authorization.
- Fake remote tests prove shared gateway behavior, not every executor platform.
  No macOS/PWA-specific runtime reproduction occurred during investigation.
