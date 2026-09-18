---
id: "03-migration-rehearsal"
title: "Private migration rehearsal"
status: done
wave: 5
depends_on: ["02-candidate-qualification"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 03: Private migration rehearsal

## Summary

Prove that the qualified candidate preserves the current installation's records
and can be rolled back with a matched binary/data backup. Use a copy in a separate
environment with external dispatch blocked before first startup.

## In scope

1. Read effective service configuration, current binary/version/hash, config/data
   locations, database engine and backup procedures into a local-only manifest.
   Inventory required stores, active runs/queues/schedules and ownership mappings.
2. Create a consistent supported backup. For SQLite include the supported online
   backup/checkpoint procedure rather than copying a hot main file without its
   WAL. For PostgreSQL use its supported consistent backup/restore. Verify restore
   before treating the backup as protection. Do not stop live service for this
   read-only rehearsal unless separately instructed or a maintenance window exists.
3. Restore into a new rehearsal home/database with separate ports and credentials.
   Enforce network/provider isolation before launch; block inherited schedules,
   webhooks, Office/Automation dispatch and workers. Feature flags alone do not
   prevent every inherited background job. If isolation is unavailable, do not
   start the copied database; fix the rehearsal environment first.
4. Start the candidate with coordinator/assistant off for migration. Check required
   stores, IDs, row counts, role/assignment/conversation mappings, ownership, task
   links, Automation history, workflow/profile references and indexed search.
   Compare identifiers/counts/digests locally without exporting message bodies.
5. Restart and replay migration. Confirm transfer markers prevent duplicate roles,
   copied comments, schedules or runs. Enable coordinator only inside the isolated
   environment and use synthetic new work for UI checks; never capture copied
   real conversations or permit real model calls.
6. Stop the rehearsal, preserve its diagnostic copy, restore the original matched
   binary/config/database backup into another test location and start it. Confirm
   its version, health and reference counts. This is the rollback drill; merely
   having an archive is insufficient.
7. Record safe aggregates and bundle/backup hashes in the review receipt. Store
   actual backups, service paths, raw logs and identity maps locally with existing
   access restrictions. Rehearse again after any schema/ownership change.

## Out of scope

Live cutover, model/provider evaluation on private history, public screenshots of
the copied database and destructive migration testing against the live store.

## Acceptance

- A restored private copy migrates and replays with expected IDs/history/ownership
  intact, and no external work is dispatched.
- The old binary successfully starts a restored pre-candidate backup with its
  original configuration; rollback time and lost-write boundary are understood.
- A redacted receipt identifies the candidate and backup hashes, coverage and
  unresolved migration differences without disclosing private content.

## Verification

Use the exact commands established by the installation's existing backup tooling
and captured effective service config. Do not invent deployment flags. The
read-only service checks and artifact comparison include:

```bash
systemctl --user show kandev.service -p FragmentPath -p DropInPaths -p ExecStart
systemctl --user is-active kandev.service
git rev-parse HEAD
sha256sum '<candidate-bundle>/bin/kandev' '<known-good-bundle>/bin/kandev'
```

Keep command output local if it includes private paths. Follow the runbook's
engine-specific backup/restore, isolation and before/after count matrix; record
the actual successful commands locally and only redacted results in Git. Do not
call a successful migration an automatic proof of authorization correctness.

## Files likely touched

Redacted rehearsal receipt in `docs/review/orchestration/`; operational runbook
corrections. Private backups/manifests are outside the repository.

## Dependencies

Delivery 02. The candidate hash must remain unchanged throughout the rehearsal.

## Risks

Copied schedules can perform real actions unless blocked before startup. A schema
downgrade is not promised; rollback restores matched data and binary, potentially
requiring reconciliation of work created after the cutover backup.

## Parallelism

`sequential`

## Inputs

[Dogfood runbook](dogfood-runbook.md), current backup implementation and required
store/migration tests. Read operational docs before choosing backup commands.

## Results

Complete for candidate `0.94.0-orchestration.20260918.sha69753564d0d7` on the
current SQLite installation. A consistent Online Backup API snapshot preserves
implicit row IDs and all task/search mappings. Candidate migration, replay,
Coordinator-only synthetic API/page checks and matched v0.94.0 rollback pass in
network-isolated copies. Existing task/session/message history, ownership/auth
records and references survive; no inherited agent work starts. The original
key and all 21 claimed attachment blobs match in every restored home.

The first vacuumed snapshot was rejected after application-level search checks
found row-ID mismatches, reproduced by the unchanged original bundle. The
replacement online backup and final cold-backup procedure avoid that issue.
Rendered UI evidence uses the separately isolated generic candidate fixtures;
no private copied page was captured. See the full
[rehearsal record](../../review/orchestration/migration-rehearsal.md) and
[redacted receipt](../../review/orchestration/migration-receipt.json).
The live service remains unchanged. Its exact proposed override and rollback
packet are staged; delivery 04 still requires the named-candidate deploy request.
