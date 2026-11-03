---
created: 2026-11-03
status: completed
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-003
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-004
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
legacy_specs: []
---

# Fix plan: Startup and shutdown diagnostic integrity

## Overview

Make recovery progress, forwarded agentctl diagnostics, planned shutdown, and worktree conflict diagnostics accurately describe observed work.
This package addresses [issue #5018](https://github.com/kdlbs/kandev/issues/5018).
Four sequential work orders have separate regression boundaries.
Implementation and permanent tests are complete and verified.

Platform owns progress, child forwarding, and shutdown attribution.
Tasks owns cleanup obligations, immutable identity, and explicit reconciliation.
Existing requirement/design pairs retain those boundaries.

## Scope

### In scope

- Count conclusively resolved recovery candidates once, independently of retracking success.
- Preserve explicitly allowed child diagnostic fields and distinguish healthy pressure transitions from unhealthy transitions.
- Signal planned child shutdown before releasing the liveness writer.
- Identify each worktree conflict, its failed comparison, and a safe inspection action while preserving required retries.
- Add targeted regression coverage and operator documentation with the implementation.

### Out of scope

- Repairing the two actual macOS cleanup jobs, moving checkouts, updating their database, or restarting the operator's installation.
- Disabling orphan protection, extending shutdown grace periods, or changing shell/process escalation severity.
- Automatic identity adoption, refreshed expected commits, force deletion, or terminal failure for cascade-critical obligations.
- Listener/authentication settings, Office signing keys, plugin manifests, unused Auggie authentication, or unavailable Cursor effort.
- New provider recovery support, public APIs, browser controls, runtime flags, SQL schemas, or cleanup-state enums.

## Confirmed evidence

The investigation used current `main`, `4a8db14fa4eb2d1bea1621345d2ef8a41a24abc3`.
Both Git remote lookup and the GitHub API confirmed that base on November 3.
The issue contains no comments or image attachments.
GitHub assignment to `carlosflorencio` succeeded after reproduction.

Four temporary tests exercised real methods in four packages.
The cleanup test had two subcases, for five confirmed scenarios in total.
These tests asserted current faulty behavior and passed. They are diagnostic evidence, not future passing acceptance tests.
The temporary files were removed after their results were recorded.

| Finding | Smallest reproduction | Confirmed cause |
| --- | --- | --- |
| Recovery progress | One persisted standalone candidate and successful empty agentctl inventory yield absent=1, done=0, total=1. | `Manager.Start` advances returned instances but not conclusive omitted-candidate outcomes. |
| Child context loss | A valid pressure WARN with state and byte counts reaches `Launcher.pipeOutput`. The parent retains only four envelope fields. | `childJSONLogFields` skips other keys and `childJSONLogRecord` reconstructs only the envelope. |
| False parent-death warning | Close an owned pipe writer while its process remains alive. `monitorParentLiveness` and `waitForShutdown` emit the parent-death WARN. | `Launcher.Stop` closes the writer before signaling. Child EOF has unconditional death wording. |
| Repeated identity conflicts | Persist cascade jobs and drive typed conflicts through claimed attempts 13 and 14. Both remain `retry_wait` with repeated generic warnings. | Required infinite cascade retry has no actionable comparison/action fields. `errors.As` exposes only the first joined inspection error. |

The issue marks recovery progress complete, but the current main reproduction still shows the defect.
Recheck the issue, base, and pending work before implementing Task 01 to avoid duplicating a separately landed correction.
No evidence establishes why the operator's actual checkout identities diverged.
No journal saturation, delivery loss, failed overall shutdown, or data corruption is inferred.

Reproduction command, run from the repository root while temporary files existed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 2 ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle ./cmd/agentctl ./internal/task/service -run '^TestIssue5018Repro' -count=1 -v)
```

Results: all four top-level tests and both cleanup subcases passed.
The command intentionally does not reproduce those tests after temporary-file cleanup.
Work orders specify their permanent regressions separately.

## Contract reconciliation

- Extend attribution with AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7. Keep the existing reporter's genuine below-total warning.
- Amend AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.4 and add .5 for typed context. Preserve denial of arbitrary child fields.
- Add AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.5 and REQ-PLATFORM-DIAGNOSTIC-SIGNAL-004 for pressure severity and shutdown attribution.
- Extend REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002 with .4 through .6 for actionable conflict evidence and recovery.
- Preserve AC-TASKS-RUNTIME-CLEANUP-001.9, .32, and .45, including safe identity checks and unlimited capped cascade retry.

The completed [startup log corrections](../startup-log-corrections/plan.md) supplied detailed enumeration outcomes but deliberately preserved progress accounting.
Task 01 now uses that evidence channel to correct progress.
The completed [runtime log reliability](../runtime-log-reliability/plan.md) and [continuous draining](../agentctl-log-drain/plan.md) packages remain historical records.
Their statuses and results remain unchanged. Task 02 extends only their typed forwarding contract.
The existing [inventory repair package](../worktree-inventory-repair/plan.md) remains the explicit repair boundary.
Task 04 changes diagnostics, not its maintenance authority or platform support.

## Technical approach

### Recovery completion

Use a per-pass ledger tied to exact filtered input candidates.
Complete a candidate only after conclusive enumeration or reconstruction work, including required asynchronous guard/stop settlement.
Returned instances and later summary aggregation share that ledger.
Duplicate outputs cannot double-advance a candidate, and uncorrelated output cannot advance any candidate.
Enumeration failure, unknown omission, cancellation, and unfinished deadline work retain incomplete counts.
Keep unknown inventory opaque and retracking outcomes separate.
No provider interface or persisted data changes are required.

### Child diagnostics

Add fixed typed dispatch for the allowlist in the runtime diagnostic design.
Keep parsing and output under existing 64 KiB line bounds.
Invalid optional data is omitted without triggering raw-line fallback.
Update existing duplicate-component tests to retain allowed components while still rejecting sensitive context.
Add safe producer classifications for filesystem/submodule failures and info-level healthy pressure transitions.
Preserve existing forwarding severity, drain continuity, and provider-output exclusion.

### Shutdown

Keep the writer open through the launcher's graceful signal and existing wait/force paths.
Close it during owned descriptor cleanup on every return path.
Replace the child's death claim with a truthful liveness-pipe closure warning.
Planned signal handling stays informational.
No byte protocol, extra timer, acknowledgement, or longer grace period is needed.

### Cleanup conflicts

Carry bounded resource/comparison metadata through `CleanupInspectionError`, preserving its wrapped error.
Collect all classified resources from joined errors instead of selecting one.
Emit the reconciliation-required condition only for proven competing registration or commit identity conflicts.
Use safe comparison/action enums, resource identifiers, and the existing retry schedule.
Retain actionable context in bounded error formatting and `LastError` without changing snapshots or SQL columns.
Respect the claimed-transition result before attributing any new diagnostic.
Keep healthy siblings eligible and re-inspect original evidence on every retry.
Explicit repair uses existing preview, offline admission, backup, and linked supersession.

| Runtime or input | Transport/evidence | Intended result | Unsupported or uncertain fallback |
| --- | --- | --- | --- |
| Standalone detailed recovery | Agentctl enumeration and exact input correlation | Count confirmed absence or completed reconstruction once | Failed or ambiguous enumeration stays incomplete |
| Other recovery backends | Existing detailed outcomes or completed reconstruction | Count only proven outcomes | No provider support inferred from an omitted result |
| Managed child JSON | Bounded stdout/stderr and validated envelope | Retain severity plus typed context | Unsupported optional fields omitted, existing malformed-envelope fallback retained |
| Console/slog child output | Existing anchored parsers | Existing severity behavior | Existing stream fallback |
| Adopted or remote agentctl | No local launcher-owned pipe | Existing behavior | No forwarding or local parent-monitor coverage claimed |
| Unix planned stop | Existing process-group signal | Signal-triggered child cleanup before writer closure | Existing bounded force cleanup |
| Windows stop | Existing interrupt/job containment | Existing platform cleanup | No inherited Unix pipe or new portability guarantee |
| Survival-enabled launch | Existing no-pipe policy | Stop remains a no-op | Existing ownership and recovery gates |
| Cascade worktree cleanup | Immutable snapshot and real Git registration/ref | Actionable refusal plus capped retry | Unknown evidence never permits deletion |

## Tests

The named tests below are planned permanent regressions.
Each work order records its expected red result before implementation.

| Work order | Regression boundary | Acceptance mapping |
| --- | --- | --- |
| 01 | `TestRecoveryProgressCountsConfirmedAbsence`, `TestRecoveryProgressMixedResolvedCandidates`, `TestRecoveryProgressRetainsIncompleteOutcomes` in lifecycle | Attribution 001.4/.5/.7 and startup progress 001.3/.21/.22 |
| 02 | `TestPipeOutputPreservesSafeDiagnosticContext`, `TestPipeOutputRejectsUnsafeOptionalContext`, `TestDeliveryPressureHealthyTransitionSeverity`, `TestWorkspaceDiagnosticSafeClassification` | Diagnostic signal 001.1-.5, 002.5, 003.1-.4 |
| 03 | `TestLauncherPlannedStopKeepsLivenessUntilChildExit`, `TestParentLivenessEOFDoesNotClaimProcessDeath`, `TestAbruptParentLossCleansOwnedChild` | Diagnostic signal 004.1-.4 |
| 04 | `TestCleanupConflictsExposeEveryAffectedResource`, `TestCleanupConflictRetainsCascadeRetryAcrossRestart`, `TestApplyCleanupPreservesConflictDiagnosticEvidence` | Inventory repair 002.1-.6 and runtime cleanup 001.9/.32/.45 |

Use real logger observers, pipes/helper subprocesses, temporary Git repositories, and persisted cleanup jobs at the changed boundaries.
MacOS was the reported environment. Linux reproductions establish shared source paths, not platform-specific process behavior on macOS.
Task 03 requires the focused Unix stop tests on macOS before delivery.
No rendered UI changes are planned. A browser run does not prove these backend transport and process guarantees.

## Work orders

Execute in the primary conversation in manifest order. No delegation is authorized.

- [x] [Task 01: Count resolved recovery candidates](task-01-recovery-candidate-progress.md)
- [x] [Task 02: Retain safe child diagnostic context](task-02-safe-child-diagnostics.md)
- [x] [Task 03: Attribute planned and unexpected shutdown](task-03-shutdown-attribution.md)
- [x] [Task 04: Make cleanup identity conflicts actionable](task-04-cleanup-conflict-diagnostics.md)

Task 03 follows Task 02 because both own launcher and observer-test changes.
The other work orders have independent code boundaries but still execute sequentially.

## Package verification

Use each work order's exact targeted product checks after implementation.
Run these document gates from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/startup-shutdown-diagnostic-integrity
git status --short -- docs/specs docs/plans/startup-shutdown-diagnostic-integrity
```

Run the local documentation coverage preflight against working-tree artifacts:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const folder = 'docs/plans/startup-shutdown-diagnostic-integrity';
const documents = fs.readdirSync(folder).filter(name => name.endsWith('.md')).map(name => `${folder}/${name}`);
const contracts = [
  'docs/specs/platform/requirements/startup-progress-visibility.md',
  'docs/specs/platform/requirements/runtime-failure-attribution.md',
  'docs/specs/platform/requirements/runtime-diagnostic-signal.md',
  'docs/specs/tasks/requirements/runtime-cleanup.md',
  'docs/specs/tasks/requirements/worktree-inventory-repair.md',
  'docs/specs/platform/system-design/startup-progress-visibility.md',
  'docs/specs/platform/system-design/runtime-failure-attribution.md',
  'docs/specs/platform/system-design/runtime-diagnostic-signal.md',
  'docs/specs/tasks/system-design/runtime-cleanup.md',
  'docs/specs/tasks/system-design/worktree-inventory-repair.md',
];
const fileContents = Object.fromEntries([...documents, ...contracts].map(name => [name, fs.readFileSync(name, 'utf8')]));
const changedFiles = [...documents, 'apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go'].map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

The representative runtime path exercises coverage instead of the documentation-only exemption.
The preflight is read-only and does not require staging or a PR.

## Verification results

- Diagnosis: all four temporary method-level reproductions passed at the recorded base.
- `python3 scripts/list-docs.py validate`: passed, 427 decisions and 1,893 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: passed, all four work orders covered with no reference errors.
- `git diff --check` and package status inspection: passed. Seven specification files changed and five package files remain untracked.
- Cleanup: all temporary tests removed; the investigation package remains unchanged.
- Production implementation: all four work orders implemented with permanent regressions.
- Targeted Go suites: lifecycle, launcher, common filesystem diagnostics, agentctl process, agentctl command, worktree, task service, and inventory repair all pass.
- Review remediation added incremental recovery progress publication, operational-failure incompleteness, null-integer rejection, effective retry-policy metadata, real SQLite conflict persistence/resume coverage, and production-monitor abrupt-loss coverage.
- Local Go lint (`scripts/lint-go-changed`): passed for every changed Go file.
- Public documentation: `git-operations.md` and `tasks-and-workflows.md` updated; `node scripts/validate-public-docs.mjs` passed.
- macOS process verification: not available on this Linux host. The focused Unix Stop tests pass locally; the plan retains the macOS validation limitation for delivery.

## Risks

- A separately landed recovery correction can make Task 01 redundant. Reconcile the current source before executing it.
- Optional-field validation must never fall back to raw JSON or accept arbitrary text merely because it is short.
- Real provider/session identifiers outside the strict UUID shape are omitted. This preserves the forwarding boundary and limits correlation coverage.
- Changing duplicate-component assertions requires preserving sensitive-field negative controls and warning/error visibility.
- Signal ordering must preserve forced cleanup, survival behavior, and descriptor ownership on all exits.
- Diagnostic context cannot prove that a destructive operation is safe. Current ownership and immutable snapshots remain authoritative.
- Some actual conflicts need operator investigation or unsupported maintenance on macOS. This package does not claim to resolve those checkouts.
- Capped cascade retries continue by design. The actionable diagnostic distinguishes their remediation need without discarding the cleanup obligation.
