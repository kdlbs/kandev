---
created: 2026-09-24
status: implemented
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLUGINS-PLUGINS-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-PLATFORM-STARTUP-PROGRESS-001
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/plugins/system-design/plugins-02.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/platform/system-design/startup-progress-visibility.md
legacy_specs: []
---

# Implementation Plan: Recent Runtime Log Remediation

## Overview

Fix the reproducible late queue-status event after task deletion. Add bounded
diagnostics at four other failure boundaries so a future occurrence identifies
the cause without changing established safety policy. The September 24 logs
showed a 13-second persistence outage, six earlier webhook 503s, one PR
identity rejection, and startup recovery at 0/39. They do not prove the
underlying cause of the first three. The 39 startup records were attached to
cancelled, failed, or missing sessions, so zero retracked is not evidence of a
lost live agent. Implement in the order below; each work order passes alone.

## Scope

### In scope

- Drop cached task summary state on verified deletion after a queue-status event.
- Attribute failed persistence probes, plugin webhook 5xx responses, PR identity
  mismatches, and startup non-recovery with safe structured diagnostics.
- Keep the current readiness, webhook response, PR identity, and recovery
  decisions unchanged; use captured evidence to decide any later cause-specific fix.

### Out of scope

- Securing the live listener, per the user's explicit exclusion.
- Raising the two-second database probe timeout, bypassing fail-closed health,
  suppressing the 0/39 warning, deleting unknown-liveness records, or relaxing
  PR identity checks without evidence and a separate reviewed contract change.
- Changing plugin-owned manifest declarations in this monorepo. Installed
  plugin packages live in dedicated repositories; any needed release is a
  separate follow-up after webhook origin is known.
- Changing intentional `KANDEV_LOG_LEVEL=debug` volume or the isolated cancel
  escalation that settled successfully.

## Technical approach

1. In `internal/task/statussummary/Projector.handleEvent`, handle verified
   missing-task from `LoadLaunchQueue` for queue-status events as the same
   terminal condition already handled at workspace resolution. Discard cached
   state under the task lock and publish nothing. Preserve transient errors.
2. In `internal/persistence/requiredstores/Health`, measure the existing probe
   stages and capture writer/reader `sql.DB.Stats` at failure. Emit one bounded
   warning per failed periodic check. Keep the current 15-second schedule,
   two-second timeout, maintenance lease, and all-store health transition.
3. In `internal/plugins/Controller.webhook`, classify 5xx outcomes at the host
   dispatch lease, RPC, and plugin-response boundaries. Do not log body, key,
   query, headers, or error strings that may contain remote data.
4. In `internal/orchestrator/executor`, return a fixed mismatch reason alongside
   the current PR identity decision, then log it at the rejection boundary.
   Keep exact existing bound/unbound and fork rules and launch behavior.
5. In `internal/agent/runtime/lifecycle`, report one aggregate recovery outcome
   after the pass. Classify only proven reasons; treat missing backend returns
   as unknown. Preserve the startup step's existing done/total and warning.

After deployment, correlate a repeated persistence failure with pool wait and
stage data before proposing a contention or host-IO fix. Correlate a webhook
503 with its origin before changing host code or an external plugin. Reassess
the PR association only when a mismatch reason recurs with trustworthy task
binding evidence. These are evidence gates, not unimplemented promises in this
package.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.11` | `projector_queued_test.go`: cold and cached deleted tasks plus a transient queue-loader error; no publication or retained state on deletion, while transient failures propagate. |
| `AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1` | `requiredstores/health_test.go`: blocked writer or reader, stage/elapsed/pool fields, one warning, unchanged health recovery. |
| `AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.2` | `plugins/handlers_webhook_lifecycle_test.go`: host lease failure, RPC error, and plugin-supplied 503 retain response semantics and log only safe fields. |
| `AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.3` | `executor_pr_base_identity_test.go`: all mismatch reasons and valid fork cases preserve the current decision. |
| `AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4` | `manager_recovery_sessions_step_test.go`: 0/N stale/unknown outcomes, partial recovery, and progress warning semantics. |

## Work orders

- [x] [Task 01: Drop deleted task projection state](task-01-drop-deleted-task-projection.md)
- [x] [Task 02: Attribute persistence probe failures](task-02-attribute-persistence-probes.md)
- [x] [Task 03: Attribute plugin webhook failures](task-03-attribute-plugin-webhooks.md)
- [x] [Task 04: Attribute PR identity rejection](task-04-attribute-pr-identity.md)
- [x] [Task 05: Summarize startup recovery outcomes](task-05-summarize-recovery-outcomes.md)

## Verification results

All five work orders are implemented, and each focused check passes:

- Task 01: `go test -tags fts5 ./internal/task/statussummary -run 'TestProjectorQueueEvent' -count=1`
- Task 02: `go test -tags fts5 ./internal/persistence/requiredstores -run 'Test(RuntimeHealth|StartupHealth|HealthCheck|ProbeTables)' -count=1`
- Task 03: `go test -tags fts5 ./internal/plugins -run 'TestWebhook' -count=1`
- Task 04: `go test -tags fts5 ./internal/orchestrator/executor -run 'Test.*PRBaseIdentity|Test.*PRBase' -count=1`
- Task 05: `go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*Recovery' -count=1` and `go test -tags fts5 ./internal/startup -run 'Test.*Step' -count=1`

The complete affected-package suite also passes:
`go test -tags fts5 ./internal/task/statussummary ./internal/persistence/requiredstores ./internal/plugins ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle`.
`make -C apps/backend build` passes. `git diff --check` is clean.

Review follow-up tightened deletion detection to `errors.Is` with the typed
task-not-found sentinel, updated gateway nil-task fallbacks, and added the
`invalid_resolved_pr_base` PR diagnostic. Verification after these changes:
`go test -tags fts5 ./internal/task/statussummary ./internal/orchestrator/executor ./internal/backendapp`
and `make -C apps/backend build` both pass; `git diff --check` remains clean.

PR review follow-up added a cold projection regression for a deleted task and
kept the persistence-loop test alive until its first failure warning, then
canceled it deterministically. `Service.Get` uses the in-memory registry and
returns only a record or `store.ErrNotFound`, so the proposed webhook lookup
500 path cannot occur and requires no code change. Verification after these
follow-ups:

- `go test -tags fts5 ./internal/task/statussummary -run 'TestProjectorQueueEvent' -count=1`
- `go test -tags fts5 ./internal/persistence/requiredstores -run 'TestRuntimeHealthProbeFailureLogsBoundedStageAndRecovers' -count=3`
- `go test -tags fts5 ./internal/task/statussummary ./internal/persistence/requiredstores ./internal/plugins ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/startup ./internal/backendapp`
- `make -C apps/backend build`

The focused and complete affected-package checks passed. The backend build
passed; Darwin helper binaries were left unsigned because neither `codesign`
nor `rcodesign` is installed in this environment. `list-docs.py validate`,
`lint-spec-files.py --all`, and `git diff --check` also passed.

## Risks

- A single pool snapshot cannot prove the cause of a timeout; compare repeated
  stage and wait measurements before changing database policy.
- Plugin-generated 503s may require a change in a dedicated plugin repository.
- Recovery inventory rows with unknown liveness may still be live; summary
  generation must never become a cleanup trigger.
- Diagnostic fields must remain bounded and exclude secrets and request data.
