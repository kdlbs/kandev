---
id: "04-live-pilot"
title: "Live coordinator pilot"
status: pending
wave: 6
depends_on: ["03-migration-rehearsal"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 04: Live coordinator pilot

## Summary

Run the reviewed private candidate in the actual installation after the user
explicitly requests cutover to that candidate. Repository publication and the
planning request do not themselves replace the running service.

## In scope

1. Present the candidate hash/version, passed qualification/rehearsal receipts,
   expected brief interruption and rollback location. An explicit deploy request
   is the final release action; no repeated permission request is needed afterward.
2. Record active work and drain/quiesce it through supported controls. Do not kill
   agent sessions without recording resumability and possible unfinished writes.
   Stop the service and take/verify a final consistent backup while quiescent.
3. Install the immutable bundle alongside the known-good bundle. Update the actual
   effective `ExecStart` override if one pins an older binary; preserve port,
   authentication, data paths, user identity, provider configuration and PATH.
   A generic service installer may leave a higher-priority override untouched.
4. Enable Orchestration and keep Personal assistant off. Reload the user service
   manager, start the service, verify health/version, actual executable and data
   location, and inspect bounded startup errors without exporting private logs.
5. In one deliberately selected workspace, configure a generic coordinator and
   explicitly chosen profile/executor. Verify chat, central task visibility,
   task creation/adoption, task link, native input UI and return callback. Confirm
   review state remains review. Do not add recurring schedules by default.
6. Check existing tasks/history/account choices, retained private conversation
   access and reconnect. Perform one controlled restart/recovery smoke when no
   work is at risk. Observe at least one normal task cycle before expanding use.
7. Roll back immediately on ownership leaks, wrong-account dispatch, duplicate
   side effects, startup/migration failure or unusable core workflows. Stop new
   work, retain a diagnostic copy and restore the matched previous bundle/data/
   config. Do not claim restoration undoes external actions already performed.

## Out of scope

Assistant enablement, automatic fleet rollout, changing default accounts/workflows,
moving live data into the repository, public media capture and upstream release.

## Acceptance

- The running service reports the reviewed candidate and intended flags, with
  production data preserved and a verified matched rollback available.
- One complete coordinator work cycle and controlled recovery succeed without
  wrong ownership/account selection, duplicate work or loss of existing records.
- The redacted result states whether the pilot continues or was rolled back,
  which candidate ran and which issues block wider use.

## Verification

Only after cutover is authorized and backup verified:

```bash
systemctl --user daemon-reload
systemctl --user start kandev.service
systemctl --user is-active kandev.service
systemctl --user show kandev.service -p MainPID -p ExecStart
```

These are not a standalone deployment script: execute the complete ordered
runbook, verify the configured health/version endpoint, and complete the UI/task
cycle matrix. Keep private service output out of the committed receipt.

## Files likely touched

Local service override and versioned install directory during authorized cutover;
redacted pilot receipt and work-order results in Git.

## Dependencies

Delivery 03 and explicit instruction to deploy the named qualified candidate.

## Risks

An existing override can silently keep the old binary running. Post-cutover writes
are not in the pre-cutover backup and need explicit reconciliation after rollback.

## Parallelism

`sequential`

## Inputs

[Dogfood runbook](dogfood-runbook.md), current effective service configuration and
the candidate/migration receipts.

## Results

Prepared, pending the named-candidate deployment instruction. The complete
candidate `0.94.0-orchestration.20260918.sha69753564d0d7` is qualified and passes
the private-data migration/replay/rollback rehearsal. The exact local service
override and rollback packet are staged; merged user-unit validation passes.
No override has been installed and live Kandev remains unchanged. See the
[reviewable pilot packet](../../review/orchestration/live-pilot-ready.md).
