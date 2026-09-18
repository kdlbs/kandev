# First live Orchestrator pilot

Status: applied and running. The implementation, packaged candidate and private
migration/replay/rollback rehearsal completed before the authorized cutover.

| Review item | Prepared result |
| --- | --- |
| Candidate | `0.94.0-orchestration.20260918.sha901e94e18` |
| Source | `901e94e18` |
| Base | Exact v0.94.0, `bf819a0228e742d069c528293d848c985a4d1bd1` |
| Bundle | Six immutable binaries; [SHA-256 receipt](candidate-receipt.json) |
| Qualification | [Completed checks and platform/provider limits](candidate-qualification.md) |
| Data rehearsal | [Migration, replay, synthetic Coordinator APIs and matched rollback](migration-rehearsal.md) |
| Service patch | Staged private override; merged user-unit validation passed |
| Initial flags | Orchestrator on; Office off |
| Preserved settings | Port, authentication, data/home paths, provider PATH/accounts, identity and resource limits |
| Live mutation | Unified Orchestrator candidate installed |

The local-only packet contains the exact staged override, current service-file
hashes, final-backup boundaries and ordered cutover/rollback steps. It selects the
frozen candidate executable and matching version/bundle metadata. It does not
rewrite provider accounts, settings or workflows. The current application home
uses about 11.55 GiB; capacity was checked before staging. Reserve several minutes
for quiescing work, the final cold backup and verification, then restart/smoke.
Isolated startup timings do not guarantee that whole-window duration.

The authorized cutover rechecked active work and service/bundle hashes, stopped
the service, created and verified the final matched cold backup, installed the
reviewed override, reloaded/started and verified health/version/login/history.
Controlled restart recovery also passed while quiescent. The pilot adds no
recurring schedule. Keep the normal upstream updater unapplied during this
custom-branch pilot; later upgrades require another qualified private bundle.

Rollback stops the candidate, preserves its new state privately and restores the
final pre-cutover data/key/blobs, old bundle and service configuration. Remove
only the new candidate override. Post-cutover writes remain in the diagnostic
copy for deliberate reconciliation; external effects are not automatically undone.

The reviewed [live-pilot work order](../../plans/orchestration-delivery/task-04-live-pilot.md)
received the explicit instruction to deploy the named qualified candidate. The
deployment and recovery checks are recorded in [live-pilot-receipt.json](live-pilot-receipt.json).
The Orchestrator includes the owner-level assistant capabilities under the same
flag. Workspace conversations use the backend-owned scoped broker surface, so
they can inspect workspace/task state without user API keys. Office remains
disabled. The first authenticated generic task/review/callback observation and
focused upstream PR/export remain follow-up work; no private prompts or account
data are included in this packet.
