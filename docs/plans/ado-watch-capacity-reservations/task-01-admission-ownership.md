---
id: "01-admission-ownership"
title: "Settle Azure preclaims at admission boundaries"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ADO-WATCH-ADMISSION-001
acceptance_criteria:
  - AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.1
  - AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.2
  - AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.3
  - AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.4
system_design:
  - ../../specs/integrations/system-design/ado-watch-admission.md
---

# Task 01: Settle Azure preclaims at admission boundaries

## Summary

Add a failing regression for the Azure poller reserving matches that the shared
capacity gate refuses. Make reservation ownership explicit and release only
only that event's preclaim when the maximum in-flight gate refuses admission.

## In scope

Existing Azure event/source claims, capacity-refused shared dispatch admission, real temporary
watch-store regression, cap and duplicate-prevention compatibility.

## Out of scope

No new cap control, no provider writes, no task reset, no historical row deletion,
no operator binary or configuration installation. Commit/push/draft PR are already
requested by the caller, after the design handoff and later implementation request.

## Acceptance

1. The regression fails before the fix because refused items retain reservations;
   after the fix, cap-two with five matches plus completion or archive admits a previously refused item.
2. Capacity refusal settles only owned preclaims; both Azure source types and
   an unclaimed non-Azure source retain their reservation ownership.
3. Azure PR, unclaimed provider, disabled and uncapped behavior remains covered.

## Verification

Run from the repository root after reading backend test and lint guidance:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestDispatchWatcherEvent_|TestAcquireWatcherSlot_|TestAzure.*WatcherSource' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/watcher_dispatch_wiring.go apps/backend/internal/orchestrator/source_azuredevops.go apps/backend/internal/orchestrator/watcher_preclaimed_reservation_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files changed

- apps/backend/internal/orchestrator/watcher_dispatch_wiring.go
- apps/backend/internal/orchestrator/source_azuredevops.go
- apps/backend/internal/orchestrator/watcher_preclaimed_reservation_test.go

## Dependencies

None. Read current source and backend-tests.md before implementation. No delegation.

## Risks

Releasing an unowned dedup row can duplicate an existing task. Do not infer a
preclaim from a provider name alone. Keep the post-create reservation boundary.

## Parallelism

sequential

## Inputs

- The paired requirement and design in this package.
- Existing TriggerWorkItemWatch publish-failure regression and shared throttle tests.

## Results

Implemented after the explicit implementation request. The regression fails before
the fix at the missing item-three assertion in both completion and archive cases.
The focused race run passes afterward, including claimed/unclaimed Azure work-item
and pull-request ownership cases and the existing concurrent admission checks.
This changes no UI, schema, provider query, public control, or deployment behavior.
