---
id: "03-session-cost"
title: "Session Cost collection and saved display"
status: done
wave: 3
depends_on: ["02-usage-apis"]
plan: "plan.md"
requirements:
  - REQ-COSTS-TOKEN-USAGE-002
  - REQ-COSTS-TOKEN-USAGE-003
acceptance_criteria:
  - AC-COSTS-TOKEN-USAGE-002.1
  - AC-COSTS-TOKEN-USAGE-002.2
  - AC-COSTS-TOKEN-USAGE-002.3
  - AC-COSTS-TOKEN-USAGE-002.4
  - AC-COSTS-TOKEN-USAGE-002.5
  - AC-COSTS-TOKEN-USAGE-002.6
  - AC-COSTS-TOKEN-USAGE-002.7
  - AC-COSTS-TOKEN-USAGE-003.1
  - AC-COSTS-TOKEN-USAGE-003.2
  - AC-COSTS-TOKEN-USAGE-003.3
  - AC-COSTS-TOKEN-USAGE-003.4
system_design:
  - ../../specs/costs/system-design/token-usage.md
---

# Task 03: Session Cost collection and saved display

## Summary

Connect the plugin to core usage storage. Show saved values during refresh and collect optional background updates through one worker.

## Companion work package

The [plugin-local plan](../../../../kdlbs-kandev-plugin-session-cost/docs/plans/token-usage/plan.md) expands this task into four sequential work orders.
This task completes only after all four finish and both packages record matching verification results.
Plugin-local scope and settings previews live there. The core requirements and design remain authoritative.

## In scope

- Add settings, one in-flight report, changed-record batches, final collection and restart checkpoints.
- Implement separate saved-read and refresh actions, stale/error status and session response fencing.
- Add an explicit bounded historical import action and documented local transcript coverage.
- Add installed-plugin desktop and phone E2E with deterministic tokscale fixtures.
- Update the plugin README and release notes for settings, compatibility and retained data.

## Out of scope

- Native stats page and new protocol collectors.

## Acceptance

- Concurrent tabs and timers share one subprocess, and idle or disabled collection starts none.
- Saved values remain visible during delayed or failed refreshes without crossing session boundaries.
- The installed plugin works through keyboard and touch, including the phone drawer.

## ASCII UI preview

### UI-04: Session Cost refresh states

Entry: chat cost icon. Desktop uses a popover. Phone uses an explicit touch drawer.

```text
Saved + refresh                 No saved measurement
SESSION COST                    SESSION COST
$1.24 estimated                 Calculating cost...
Input / Output / Cache
Saved 2 minutes ago             Failed initial calculation
Refreshing...                   Could not calculate. [Retry]

Saved + failed refresh
$1.24 estimated
Saved 2 minutes ago
Refresh failed. [Retry]
```

The amount is illustrative. The structure and preservation of saved values are required.
Desktop controls retain standard density. Phone drawers have one scroll body and a close control.
Keyboard focus returns to the cost trigger after dismissal.


See the [full preview](plan.md#ascii-ui-preview). Covers AC-COSTS-TOKEN-USAGE-003.1 through .4.

## Verification

Validation was run from the Kandev repository root; the executed commands and results are recorded below.

```bash
(cd ../kdlbs-kandev-plugin-session-cost && make test vet build package-host)
(cd ../kdlbs-kandev-plugin-session-cost && go test -race ./server/...)
(cd ../kdlbs-kandev-plugin-session-cost && node --test test/bundle.test.mjs)
```

## Files likely touched

- `apps/web/e2e/tests/plugins/session-cost-collection.spec.ts (new)`
- `apps/web/e2e/tests/plugins/mobile-session-cost-collection.spec.ts (new)`

- `../kdlbs-kandev-plugin-session-cost/manifest.yaml`
- `../kdlbs-kandev-plugin-session-cost/server/plugin.go`
- `../kdlbs-kandev-plugin-session-cost/server/tokscale.go`
- `../kdlbs-kandev-plugin-session-cost/ui/bundle.js`
- `../kdlbs-kandev-plugin-session-cost/test/bundle.test.mjs`
- `apps/web/e2e/tests/plugins/session-cost-refresh.spec.ts (new)`
- `apps/web/e2e/tests/plugins/mobile-session-cost-refresh.spec.ts (new)`

## Dependencies

02-usage-apis.

## Risks

The npx wrapper can outlive a timed-out child unless process-tree cancellation works. Delayed transcript writes need final retries.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/token-usage.md) and the acceptance IDs in frontmatter.
- [System design](../../specs/costs/system-design/token-usage.md).
- [Core projection ADR](../../decisions/2026-09-13-core-session-usage-projections.md).
- Existing source paths and test patterns listed in the [plan](plan.md#tests).

## Results

Implemented the Session Cost adapter, shared tokscale report coordinator, saved-read and refresh actions, nullable token preservation, optional background collection, final retries, restart-safe state, historical import, owner-scoped settings and touch-accessible saved details.

Validation: the plugin `make test vet build package-host` block passed; `go test -race ./server/...` passed; the bundle suite passed 12 tests; and `node --check ui/bundle.js` passed. Installed-plugin browser E2E and a live tokscale benchmark were not run.
