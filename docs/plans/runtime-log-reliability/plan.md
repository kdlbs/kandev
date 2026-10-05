---
created: 2026-10-05
status: done
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
  - REQ-TASKS-CANCELLED-SIDEBAR-READS-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/workspace-git-capture-recovery.md
  - ../../specs/tasks/system-design/cancelled-sidebar-reads.md
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
legacy_specs: []
---

# Implementation plan: Runtime log reliability

## Overview

Reduce reproducible Git capture failures, canceled-read errors, and repetitive runtime logs.
Use controlled investigations for persistence stalls, worktree ownership conflicts, and voice-plugin responses.
Those three causes remain unknown. Their investigation results must define any later repair package before production changes.

Implementation proceeds sequentially in the listed priority order.
The package has four correction work orders and three investigation work orders.
Completing an investigation does not mean its production symptom is fixed.

## Scope

### In scope

- Reproduce shared-pool pressure and attribute the stalled database operation.
- Retry one transient basic Git capture race within existing observation ownership.
- Verify that successful enriched recovery reaches launch baseline capture.
- Stop abandoned sidebar enrichment and classify it through the existing 499 convention.
- Preserve recognized agentctl JSON severity in parent logs.
- Remove routine idle-reclaim repetition, repeated health info, and benign MCP alternative warnings.
- Diagnose the exact archived worktree conflict without changing installation data.
- Identify the voice plugin's 503 cause in its dedicated repository using synthetic requests.

### Out of scope

- Live database changes, restarts, deployment, plugin publication, or persistent Kandev task/session creation.
- Longer health timeouts, new pools, readiness hysteresis, or bypassing persistence middleware.
- Automatic worktree adoption, force cleanup, historical evidence replacement, or repair of the live installation.
- New Git endpoints, metadata-only baseline capture, frontend layout changes, or new retry controls.
- Provider/model substitution or suppressing real provider capacity failures.
- Generic review, QA, broad verification, and changes to intentional debug thresholds.

## Evidence and assumptions

The retained logs cover October 4 at 18:41 through October 5 at approximately 11:30, Lisbon time.
The running binary reports `v0.96.0-199-gca1492fafa5`.
The planning checkout starts at `f18fe2d9e3bafb567aff643f25356ac3ed651120`.
The reviewed probe, sidebar, Git, and webhook emitters match the running source revision.
Recheck the implementation base before execution because other packages can move.

| Finding | Established evidence | Remaining uncertainty |
| --- | --- | --- |
| Persistence | Four failed probes, 11–13 second recovery windows, and 54 observed middleware 503 responses. Busy reader/writer handles appear in failure snapshots. | Responsible operation, transaction duration, and host I/O contribution. Cumulative pool counters do not identify a query. |
| Git | Basic evidence validation fails during workspace changes. Enriched baseline capture sometimes aborts, including this session. | Exact mutation timing, handled by deterministic fixture barriers. |
| Sidebar | Four canceled enrichment requests produce error entries and HTTP 500. Optional reads generate cancellation warnings. | Which client trigger abandoned each historical request. That fact is unnecessary for the backend regression. |
| Voice | Five webhook failures have `origin: plugin_response`, including three this morning. | Plugin version, configuration eligibility, and dependency failure. |
| Worktree | One recorded cleanup request receives `competing_registration`; no second attempt was found in the retained logs. | Saved versus observed branch/path identity and whether another historical attempt occurred. |
| Logs | Approximately 244 MB across seventeen hours. About 150,000 routine idle-reclaim refusal entries. Agentctl JSON errors appear inside outer DEBUG records. | Aggregate byte reduction after targeted changes. |

Read-only evidence pointers:

- `/root/.kandev/logs/backend-logs-2026-10-05-000004.log:49108` identifies the writer probe timeout.
- `/root/.kandev/logs/backend-logs-2026-10-05-000006.log:56304` identifies this session's baseline capture failure.
- `/root/.kandev/logs/backend-logs-2026-10-05-000006.log:21673` identifies canceled sidebar enrichment.
- `/root/.kandev/logs/backend-logs-2026-10-05-000006.log:32894` identifies a plugin-supplied 503.
- `/root/.kandev/logs/backend-logs-2026-10-05-000005.log:20589` identifies the repeated cleanup conflict.

Logs remain local evidence. Do not copy raw requests, transcripts, audio, credentials, or installation databases into this package.

## Contract ownership

Platform owns health, Git observation validity, and diagnostic signal quality.
Tasks owns task-query enrichment and task-owned inventory recovery.
The voice implementation remains in `kdlbs/kandev-plugin-voice`.
Core owns generic webhook dispatch and attribution, not voice transcription behavior.

## Technical approach

### Persistence investigation

Use the existing two-second probe and fifteen-second schedule against a disposable shared pool.
Separate reader saturation, writer transactions, maintenance admission, and catalog-probe cost.
Use stage measurements and counter deltas, not cumulative counters alone.
Record deterministic reproductions in `persistence-evidence.md` with the responsible operation or an explicit unresolved result.
A workload repair needs its own concrete source scope and regression before production changes.

The completed [September diagnostic package](../recent-runtime-log-remediation/plan.md) already supplies pool-pressure fields.
The [statistics package](../stats-read-recovery/plan.md) protects analytics read capacity.
Neither package proves the cause of this instance's writer timeout.
Preserve the [persistence ADR](../../decisions/2026-09-05-required-internal-persistence.md) and
[maintenance ADR](../../decisions/2026-09-17-maintenance-health-probe-coordination.md).
The [writer transaction ADR](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md) retains authority over transaction entry.

### Git recovery

Add one typed evidence-change retry inside `computeGitStatusObservation`.
Keep the same shared context, class, ordinal, and budget.
Dispose of each failed index snapshot before another attempt.
Keep the enrichment worker's existing correction policy.
Verify the HTTP result and `Executor.captureBaseCommit` through successful enriched recovery.
No metadata-only exception or launch-side retry loop is part of this design.

| Caller or transport | Intended behavior | Evidence |
| --- | --- | --- |
| Basic agentctl HTTP status | One shared corrective basic capture. Existing payload shape remains. | Real Git tracker and HTTP router fixture. |
| Fresh detailed runtime client | Existing `fresh=true&details=wait` request consumes recovered enrichment. | Existing client tests and executor regression. |
| Completion, archive, Review | Existing enriched-value requirement remains. | Existing process/API package suites. |
| Multi-repository tracker | Retry applies only to the failed repository and admission class. | Concurrent repository fixture. |
| Missing repository, permission failure, stopped tracker | Existing failure. No new corrective attempt. | Negative controls. |

### Cancellation and diagnostics

Classify abandoned sidebar requests before query/enrichment error logging.
Stop optional reads once their request context is canceled.
Preserve wrapped error chains, active-request deadlines, and genuine database failures.

Recognize anchored agentctl JSON in the existing launcher parser.
Keep unknown-output fallbacks and console/slog support.
Remove only the three routine diagnostic sites named in the new design.
Do not introduce a global logger sampler or a per-session suppression registry.

### Worktree and voice investigations

Reproduce ownership conflict with synthetic Git and inventory fixtures.
Use the completed [inventory repair package](../worktree-inventory-repair/plan.md) as the recovery boundary.
Do not apply its offline repair command to the active installation.
Record identities needed for an explicit later repair in `worktree-evidence.md`.

For voice, preserve the host's plugin-response classification.
Inspect a pinned checkout of the dedicated plugin repository and its declared authenticated webhook.
Use synthetic provider responses and non-sensitive audio fixtures without billable external calls.
Record cause, version, error classification, and a plugin-owned regression proposal in `voice-evidence.md`.
Any plugin code change needs the dedicated repository's instructions and a separate concrete repair package.

## Tests

| Work order | Acceptance mapping | Required evidence |
| --- | --- | --- |
| 01 | Store parity 007.1, .3, .4, .6–.8 and attribution 001.1 | Controlled reader/writer barriers, maintenance control, missing-table control, pool reuse, and real middleware rejection/recovery. |
| 02 | Git capture recovery 001.1–.6 | New `workspace_git_status_capture_recovery_test.go`, API route integration, and `executor_base_capture_recovery_test.go`. |
| 03 | Canceled sidebar reads 001.1–.5 | Handler cancellation integration including body decoding, optional-read counters, message-queue log observers, committed-summary event publication, and a successful successor query. |
| 04 | Diagnostic signal 001.1–.4 | Parser fixtures and scanner/observer tests at info threshold, including exact allowlisted output. |
| 05 | Diagnostic signal 002.1–.4 | Health transition observers, existing reclaim decision tests, MCP transport fixtures, and synthetic log-byte comparison. |
| 06 | Inventory repair 001.1–.3 and 002.1–.3 | Real Git/SQLite preview and refusal fixtures. No live repair. |
| 07 | Attribution 001.2 | Generic webhook fixture preserves plugin-supplied status. Dedicated plugin investigation isolates its cause. |

## End-to-end evidence

No rendered UI or copy changes are proposed.
Backend integration tests traverse the real HTTP status, sidebar, persistence middleware, and plugin dispatch boundaries.
Git fixtures include a real tracker and executor baseline consumer.
No artificial browser test is required for the logging-only changes.
If implementation adds a rendered UI outcome, amend the package with mobile-parity and targeted desktop/phone Playwright coverage first.

## Work orders

- [x] [01: Attribute persistence contention](task-01-persistence-contention.md)
- [x] [02: Recover transient Git capture races](task-02-git-capture-recovery.md)
- [x] [03: Classify canceled sidebar reads](task-03-sidebar-cancellation.md)
- [x] [04: Preserve agentctl JSON severity](task-04-agentctl-json-severity.md)
- [x] [05: Reduce routine diagnostic repetition](task-05-routine-log-volume.md)
- [x] [06: Diagnose worktree ownership conflict](task-06-worktree-conflict.md)
- [x] [07: Diagnose voice-plugin webhook failures](task-07-voice-webhook.md)

Work orders 01 and 06–07 produce evidence and record whether it justifies a separate repair package.
Work orders 02–05 produce regression-tested corrections.
They share no hard dependency and execute sequentially in priority order.
Shared launcher/orchestrator files make parallel implementation unsuitable without a revised ownership split.

## Documentation and package checks

This design turn changes internal specifications and plans only.
Public documentation remains unchanged until behavior ships.
During implementation, review existing Git troubleshooting and HTTP/API documentation for the final behavior.
Any new operator command or plugin recovery step belongs in its owning public guide with the corresponding repair.

Each work order lists its exact product checks.
After artifact changes, run:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/runtime-log-reliability
git status --short -- docs/specs docs/plans/runtime-log-reliability
```

Run the repository documentation coverage validator against work-order references.
Documentation-only diffs are exempt, so also exercise the validator with a representative planned source path.

## Verification results

Planning validation passed on 2026-10-05:

- `python3 scripts/list-docs.py validate`: 349 decisions and 1,349 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Repository `validateCoverage` preflight: seven work orders covered with no reference errors.
- Documentation-only coverage classification: exempt, as expected.
- Existing paths, requirement/design ownership, and linked work-order references passed inspection.
- New prose passed the simple-English sentence-length check.

Implementation verification passed:

- All work-order product checks passed. Exact race-enabled commands and investigation outcomes are recorded in tasks 01–07.
- `make -C apps/backend build` passed, including the main backend binary and agentctl build targets.
- The pinned voice-plugin Go/UI tests and build passed in the disposable environment described in [voice evidence](voice-evidence.md); all 57 UI tests passed.
- `python3 scripts/list-docs.py validate`, all 36 spec-linter tests, and `python3 scripts/lint-spec-files.py --all` passed.
- Repository `validateCoverage` reported all seven work orders covered with no errors when exercised against a representative planned source path.
- `git diff --check` and whitespace/local-link checks for the new plan documents passed.

No live database, installation state, or runtime settings changed. The voice plugin was not modified, released, or deployed.

## Risks and completion

- Busy pools demonstrate contention but do not identify its producer. No timeout or middleware change is pre-authorized by that observation.
- Extra Git work must remain inside the existing shared deadline and preserve index cleanup and publication order.
- A nested canceled context can belong to another operation. Request cancellation must be established before returning 499.
- Removing routine debug entries must preserve real failed-probe and successful-reclaim evidence.
- Competing worktree registration can be a correct refusal. Do not convert it into automatic adoption or deletion.
- Voice failures can require configuration recovery rather than code. Plugin response origin alone does not establish a plugin defect.
- Record each investigation as attributed, expected refusal, or unresolved. Never record an investigated issue as fixed without a repair regression.
- Keep unresolved production symptoms visible in the final implementation report. Plan status records delivered work, not installation health.
