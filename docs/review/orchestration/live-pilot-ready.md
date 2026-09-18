# Prepared first live pilot

Status: prepared, not applied. The implementation, packaged candidate and private
migration/replay/rollback rehearsal are complete. Live Kandev remains on v0.94.0.

| Review item | Prepared result |
| --- | --- |
| Candidate | `0.94.0-orchestration.20260918.sha69753564d0d7` |
| Source | `69753564d0d7c0121e8681a72e5b4ce8c384a106` |
| Base | Exact v0.94.0, `bf819a0228e742d069c528293d848c985a4d1bd1` |
| Bundle | Six immutable binaries; [SHA-256 receipt](candidate-receipt.json) |
| Qualification | [Completed checks and platform/provider limits](candidate-qualification.md) |
| Data rehearsal | [Migration, replay, synthetic Coordinator APIs and matched rollback](migration-rehearsal.md) |
| Service patch | Staged private override; merged user-unit validation passed |
| Initial flags | Coordinator on; Personal assistant off |
| Preserved settings | Port, authentication, data/home paths, provider PATH/accounts, identity and resource limits |
| Live mutation | None |

The local-only packet contains the exact staged override, current service-file
hashes, final-backup boundaries and ordered cutover/rollback steps. It selects the
frozen candidate executable and matching version/bundle metadata. It does not
rewrite provider accounts, settings or workflows. The current application home
uses about 11.55 GiB; capacity was checked before staging. Reserve several minutes
for quiescing work, the final cold backup and verification, then restart/smoke.
Isolated startup timings do not guarantee that whole-window duration.

After explicit deployment authorization, recheck active work and service/bundle
hashes, stop the service, create and verify the final matched cold backup, install
the reviewed override, reload/start and verify health/version/login/history.
Then complete one generic task/review/callback cycle with an explicit profile
and executor, followed by a controlled restart while quiescent. The pilot adds
no recurring schedule. Keep the normal upstream updater unapplied during this
custom-branch pilot; later upgrades require another qualified private bundle.

Rollback stops the candidate, preserves its new state privately and restores the
final pre-cutover data/key/blobs, old bundle and service configuration. Remove
only the new candidate override. Post-cutover writes remain in the diagnostic
copy for deliberate reconciliation; external effects are not automatically undone.

The reviewed [live-pilot work order](../../plans/orchestration-delivery/task-04-live-pilot.md)
requires an "explicit instruction to deploy the named qualified candidate."
That release action remains pending. Assistant implementation is complete, but
its live enablement follows the separate opt-in in the reviewed rollout plan.
Dogfood observations and the focused upstream PR/export remain after the pilot.
