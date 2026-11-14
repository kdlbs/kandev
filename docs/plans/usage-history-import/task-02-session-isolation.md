---
id: "02-session-isolation"
title: "Isolate ineligible sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-005
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-005.1
  - AC-COSTS-USAGE-IMPORT-005.2
  - AC-COSTS-USAGE-IMPORT-005.4
  - AC-COSTS-USAGE-IMPORT-005.5
  - AC-COSTS-USAGE-IMPORT-005.6
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 02: Isolate ineligible sessions

## Summary

In `kandev-plugin-session-cost`, stop one ineligible session from aborting a collection pass, and give the composer an accurate message. This also fixes a reproduced outage where one session produced zero reports for every session.

## In scope

- Per-session error classification and `Skipped` pending entries in `collectOnce`.
- Removal of pending entries with no binding and no live session.
- `source_ineligible` in `unavailableSource`, and composer copy in `en` and `pt-pt`.
- Stale comments: the `manifest.yaml` "Read-only access" comment and the `report.go` waiter comment.

## Out of scope

- The import engine, which Task 03 owns.

## Acceptance

- With one `unknown` and one eligible active session, a pass reports the eligible session and returns no error.
- A skipped session is collected on a later pass after it becomes eligible, and a pass with only skipped sessions runs no report.
- A transient Host read error still aborts the pass and keeps saved work.

## ASCII UI preview

UI-03 from the [plan](plan.md#ascii-ui-preview). AC-005.5.

```text
+----------------------------------------------+
| Session cost                                 |
| Cost can't be attributed safely for this     |
| session because its transcript may include   |
| other work. Saved usage stays available.     |
+----------------------------------------------+
```

## Verification

Set `PLUGIN_ROOT` to the plugin checkout, with core checked out as its sibling `../kandev`.

```bash
(cd "$PLUGIN_ROOT" && go test ./server/ -run 'TestCollectorIsolation|TestComposer' -count=1)
(cd "$PLUGIN_ROOT" && go test -race ./server/... -count=1)
(cd "$PLUGIN_ROOT" && make test)
```

## Files likely touched

- `server/collector.go`
- `server/source_preflight.go`
- `server/report.go`
- `server/collector_isolation_test.go` (new)
- `server/source_preflight_test.go` (new)
- `ui/bundle.js`
- `test/responsive-bundle.test.mjs`
- `manifest.yaml`

## Dependencies

None. AC-005.4 is fully satisfied once Task 01 is on the Host; this task's test uses an eligible fixture.

## Risks

- Existing tests may assert that any resolve error aborts a pass. Keep that behavior for transient errors only.

## Parallelism

`parallel-safe`

## Inputs

- [System design](../../specs/costs/system-design/usage-history-import.md), Per-session isolation.
- `server/usage_history_test.go`: `historyPlugin`, `retainedProtocolFixture`.
- `server/review_source_display_test.go`: `sourceContextTestHost`.

## Results

Per-session `errUsageSourceUnavailable` and `errUsageScopeChanged` results now mark a pending session skipped while eligible sessions continue in the same pass.
Active skipped sessions are rechecked, unbound entries with no live session are removed, and a pass with no eligible session runs no report.
The composer maps `source_ineligible` to the safe-attribution copy in English and Portuguese.

Validation: `go test ./server/ -run 'TestCollectorIsolation|TestComposer' -count=1` and `go test -trimpath -race ./...` passed in the plugin checkout. `node --test test/*.test.mjs` passed 70 tests.
