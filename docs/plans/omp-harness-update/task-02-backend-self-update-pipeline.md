---
id: "02-backend-self-update-pipeline"
title: "Integrate harness updates into the backend pipeline"
status: done
wave: 2
depends_on:
  - "01-omp-update-capability"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-003.1
  - AC-AGENTS-RUNTIME-UPDATES-003.3
  - AC-AGENTS-RUNTIME-UPDATES-003.4
  - AC-AGENTS-RUNTIME-UPDATES-003.5
  - AC-AGENTS-RUNTIME-UPDATES-003.6
  - AC-AGENTS-RUNTIME-UPDATES-003.7
  - AC-AGENTS-RUNTIME-UPDATES-003.8
  - AC-AGENTS-RUNTIME-UPDATES-003.9
  - AC-AGENTS-RUNTIME-UPDATES-003.11
  - AC-AGENTS-RUNTIME-UPDATES-003.13
  - AC-AGENTS-RUNTIME-UPDATES-003.14
system_design:
  - ../../specs/agents/system-design/harness-self-update.md
---

# Task 02: Integrate harness updates into the backend pipeline

## Summary

Extend discovery, status, preview, enqueue, and maintenance-job handling to support the harness-owned update mode alongside pinned npm runtimes. OMP must run only its trusted updater and existing ACP probe; no version selection, npm cache repair, or fallback package-manager command is permitted.

## In scope

- Add the `update_mode` contract to runtime-update, status, preview, and job DTOs.
- Project OMP's ACP-reported current version and stable latest reference into status and preview; resolve package metadata directly from the trusted HTTPS npm registry without invoking `npm`. Preview sets `stable_latest_version` and leaves `target_version` unset for self-update.
- Infer update mode from built-in agent capability metadata. Accept `{}` only as an approval request for a self-update agent; continue requiring the existing target/default fields for pinned runtimes. Reject a non-empty target version or `use_default: true` for self-update.
- Run the trusted updater as a streamed job, ACP-probe it, publish only on success, and preserve old capabilities on probe failure.
- Test successful and target-free updates, up-to-date approval no-job behavior, unknown status, registry metadata resolution when `npm` is absent, mixed-agent isolation when OMP metadata fails, rejected target/default requests, updater-failure output retention plus one trusted updater invocation and no fallback or follow-up mutation, probe-failure preserving the previously active version and capability catalogue, and untrusted-request behavior.
- Include a configured-channel case where the ACP version after `omp update` differs from `stable_latest_version`; assert Kandev records the probed version and neither passes the stable reference as a target nor changes the configured channel.
- Test a self-update preview whose registry metadata lookup fails; assert the preview returns the resolution error and no update job is enqueued.
- Test a self-update preview when stable metadata resolves but the current ACP version is absent; assert the structural operation is `repair`, `target_version` is unset, and preview enqueues no job before approval.
- Test POST `{}` with the current ACP version equal to or newer than stable latest; assert HTTP 202 returns terminal `up_to_date` (`status: succeeded`) with empty `job_id` and `current_version` equal to the preflight version, no stored job or maintenance claim, no start/finish event, and neither the trusted update nor ACP probe command is invoked.
- Test the decision boundary: preflight sees an outdated OMP version, then an external update makes it current before the worker check; assert POST returns a job ID, the retained job completes with operation `up_to_date` without invoking the update or probe command, and start/finish notifications use the same job ID rather than replacing it with the pre-enqueue no-job response.
- Test stable metadata resolution failure during approval revalidation; return the resolution error before creating a job or maintenance claim and without running the update or ACP probe command.

## Out of scope

- Frontend presentation and Playwright coverage.
- Changes to the pinned npm update behavior, OMP launch command, install script, or remote runtime setup.
- Exact-version pins, rollback, Kandev staging, or Nix-specific recovery.

## Acceptance

- Available OMP appears in discovery and update status with `self_update` mode; stable status compares against its ACP-reported version and resolves metadata without the `npm` executable.
- If approval preflight classifies update or repair, it creates a job. Unless the worker observes an intervening up-to-date version, the job runs `omp update` without a version or channel argument, streams output, then probes `omp acp`; success publishes capabilities without persisting a version selection. If the worker observes the version became up to date after enqueue, the existing job finishes as a no-op without invoking `omp update` or `omp acp`, while retaining its ID and lifecycle.
- Updater failure returns a failed job retaining the updater's own output in the job output and existing output stream; assert the trusted updater runs once and no package-manager fallback or follow-up mutation command runs. Probe failure returns a failed job, leaves the prior capability catalogue and active/current version published, and does not report the candidate version as active. API-supplied command/package/registry/version cannot change the trusted recipe.
- When OMP metadata lookup fails in a multi-agent status response, OMP is `unknown` and a second agent with valid metadata retains its correct status.
- A configured-channel update may report an ACP version different from `stable_latest_version`; Kandev records that actual version without changing OMP's channel or treating the stable reference as the update target.
- A self-update preview metadata failure returns the resolution error and enqueues no update job; this remains separately tested from the `unknown` status result.
- When stable metadata resolves but the current ACP version is absent, preview returns structural `repair` with no target version and enqueues no job before approval.
- If the approval-time check sees a current ACP version equal to or newer than stable latest, POST returns the terminal `up_to_date` result with an empty `job_id` and creates no job record, maintenance claim, start/finish notification, trusted update command, or ACP probe command. This pre-enqueue comparison is the decision point; if an external version change is observed only by the worker after enqueue, the existing job retains its ID and lifecycle and may complete as a no-op without running either command.
- If trusted stable metadata cannot be resolved during approval revalidation, POST returns the resolution error before creating a job or maintenance claim and does not run the trusted update or ACP probe command.

## Verification

```bash
cd apps/backend
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/agents
```

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_discovery.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/controller/agent_update_status.go`
- `apps/backend/internal/agent/settings/controller/agent_update_harness_test.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`

## Dependencies

Task 01 defines the trusted agent capability and OMP metadata.

## Risks

- OMP's updater mutates its installed executable before ACP validation; Kandev cannot roll back that mutation.
- Nix-owned installs intentionally fail through OMP's updater. Do not add a package-manager fallback.

## Parallelism

`sequential`

## Inputs

- Requirement AC-AGENTS-RUNTIME-UPDATES-003.1, .3-.9, .11, .13, and .14.
- [Harness self-update design](../../specs/agents/system-design/harness-self-update.md), especially Control flow, Failure and recovery, Persistence, and Security.
- Existing `agent_update.go`, `agent_update_job.go`, `agent_update_status.go`, and their tests.

## Results

Implemented self-update discovery, direct HTTPS stable metadata, status, preview, target-free approval, no-job preflight, and streamed maintenance with ACP probe-before-publication. RED evidence: harness status absent; preview unsupported; direct resolver absent; approval unsupported; job worker unavailable; canary misclassified. GREEN: `TMPDIR=/home/clem/.kandev/tasks/omp-test-tmp GOCACHE="$PWD/.go-build-cache" CGO_ENABLED=1 go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/agents` from `apps/backend` (pass). Focused `go test -race -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers -run 'HarnessUpdate|HarnessStableLatest' -count=1` (pass). `TMPDIR` avoids the unrelated `/tmp/package.json` npm-fixture interference.
