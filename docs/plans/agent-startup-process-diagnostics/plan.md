---
created: 2026-11-07
status: completed
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
system_design:
  - ../../specs/agents/system-design/startup-process-diagnostics.md
legacy_specs: []
---

# Fix plan: Agent startup process diagnostics

## Overview

Preserve a confirmed subprocess exit and its safe toolchain cause through startup
failure, backend diagnostics, and desktop and phone recovery.
Issue [#5128](https://github.com/kdlbs/kandev/issues/5128) is assigned to
`carlosflorencio`. GitHub readback confirmed the assignment on November 7.
All three implementation work orders are complete.

The agent system owns this contract because it interprets process evidence and
recovery causes. Tasks retain durable state, history, and admission ownership.
The [recovery requirements](../../specs/agents/requirements/session-recovery-failures.md)
add criterion 006.36 to the existing recovery contract.
The [managed runtime requirements](../../specs/agents/requirements/managed-npm-runtime-recovery.md)
retain retry exclusions and bounded diagnostic behavior.
The [design](../../specs/agents/system-design/startup-process-diagnostics.md)
covers the complete path without a parallel UI specification.

Implementation proceeds sequentially: safe source evidence, backend propagation,
then localized presentation with end-to-end proof. No subagents are authorized.

## Confirmed evidence

Investigation base: `5264536ba9845f966966c2a371c7e8136ccdf119`.
The canonical issue and its one comment contain no image attachments.
Related [#5129](https://github.com/kdlbs/kandev/issues/5129) describes a separate
uncommitted durable-delivery checkpoint defect.

Three temporary Go reproductions ran against the production boundaries:

| Reproduction | Observed result | Intended regression |
| --- | --- | --- |
| `TestRepro5128ImmediateExitCause`, agentctl API | A real helper wrote the mise refusal and exited 1. Initialize returned `peer disconnected before response`; `startup_evidence` retained ordinary exit 1 but no toolchain reason. | `TestHandleWSInitializeProjectsEarlyProcessExit` |
| `TestRepro5128WrapperDropsMiseDiagnostic`, process reader | The reader observed stderr, but a configured launch wrapper left the safe ring empty. | `TestStartupDiagnosticMiseTrustProjection` |
| `TestRepro5128BootstrapIgnoresExitEvidence`, runtime lifecycle | `InitializeError` containing ordinary exit 1 became bootstrap cause `unknown` with generic safe detail. | `TestStartupProcessBootstrapCause` |

Each reproduction failed its expected-cause assertion for the reported reason.
The helper simulates the reported mise signature. It does not execute mise or
prove a particular mise installation's behavior. No developer instance was accessed.
All temporary files were removed after the runs.

Source inspection confirms these additional boundaries:

- `handleWSInitialize` already attaches exact-generation evidence after a bounded wait.
- `managed_runtime_startup.go` consumes evidence for npm retry, while `bootstrapFailureFor` ignores general process exits.
- `launcher/child_log_context.go` forwards validated optional fields, including exit status, but excludes `recent_stderr`.
- `buildBootstrapLastAgentError` preserves a safe typed cause but keeps a generic outer message.
- `waitForSessionReady` rebuilds an error from `session.ErrorMessage`. Request errors lose typed detail.
- Recovery history and UI already have attempt/stamp ownership. Distinct attempts must remain distinct.

The immediate-exit defect is reproduced. The exact multi-generation incident in
#5129 is not reproduced and is not necessary to validate this diagnostic repair.

## Scope

### In scope

- Confirmed ordinary pre-initialize exit status, including zero.
- A fixed safe projection for the reported mise trust refusal, including wrapper mode.
- Generation-bound initialize evidence, backend cause normalization, logging, and persistence.
- Same-attempt preservation across readiness and recovery request wrappers.
- Distinct history after a later failed recovery operation.
- Localized recovery copy, safe details/copy, and desktop and phone coverage.

### Out of scope

- The journal/checkpoint recovery algorithm in #5129 or new database migrations.
- Automatic `mise trust`, command execution from stderr, or project configuration changes.
- New retry budgets, cache repair, provider fallback, or conversation replacement policies.
- Arbitrary stderr forwarding or unverified toolchain signatures.
- Host capability-probe behavior changes, native protocol expansion, or passthrough redesign.
- Implementation, commits, pushes, PR creation, or persistent platform subtasks in this design turn.

## Technical approach

### Work order 01: Safe process evidence

Add `internal/common/startupdiag` for exact signature recognition and fixed markers.
Integrate it before wrapper filtering in `process.Manager.readStderr`.
Extend `ManagedStartupEvidence` additively, keeping npm completeness independent.
Carry the fields through initialize/client decoding and emit validated log context.
Extend the launcher's existing optional-field allowlist rather than forwarding raw stderr.

### Work order 02: Backend propagation and history

Validate process generation at the failed initialize boundary before releasing
the observed client. Produce a `BootstrapFailure` with code `process_exited`,
closed reason, and optional numeric status. Normalize that evidence in task models.
Carry it through both bootstrap projection paths and matching recovery feedback.
Audit DTO, bounded task-summary, event, and historical copies for field loss.
Preserve current atomic settlement and stale-attempt guards.

The readiness wrapper can recover a safe cause only from its matching durable error.
Keep a prior resume failure in history after a separate fresh-start failure.
Do not make older evidence the current failure or attach it to a successor.
Existing npm recovery retains its budget, exclusions, and final-attempt evidence.

### Work order 03: User-visible explanation

Extend cause parsing and the shared bootstrap recovery model.
Use `SessionRecoveryCard`, existing detail/copy primitives, and current action eligibility.
Localize all guidance and labels. The first supported toolchain reason is mise trust refusal.
The prerequisite names the execution host and requires user review before trust.
No new trust control or recovery surface is necessary.

### Compatibility matrix

| Provider/transport/context | Evidence | Intended behavior | Proof or fallback |
| --- | --- | --- | --- |
| Built-in ACP through a mise-backed npx shim | Matching ordinary exit and recognized stderr signature | `process_exited`, `mise_config_untrusted`, observed status | Real private subprocess and API/browser tests |
| Other ACP process using the same supervisor | Matching ordinary exit, no supported reason | Exit summary and status without toolchain attribution | Parameterized backend tests |
| Managed npm transient failure | Existing complete npm evidence | Existing single retry, then actual final cause | Existing managed-runtime tests plus exit-evidence assertions |
| Cancellation, stop, signal, stale generation, or later phase | Ineligible evidence | Preserve existing specific handling and no invented ordinary exit | Negative and replacement-generation tests |
| Older agentctl or malformed optional fields | Missing or rejected evidence | Existing safe fallback | Wire-decoding tests |
| Native Codex transport or passthrough | No new validated ACP initialize contract | Existing behavior | Explicitly outside the new classifier |
| Docker/SSH/other agentctl-backed executors | Same authenticated instance evidence when present | Same diagnostic projection; recovery permissions stay executor-owned | Shared contract tests; no new remote recovery claim |

## ASCII UI preview

### UI-01: Startup refusal, task Chat recovery region

Before, as reported and supported by the current classifier:

```text
Agent startup failed
The agent could not start.
[Existing eligible recovery actions]
> Technical details: Phase bootstrap; Cause unknown
```

Proposed desktop composition:

```text
Agent startup blocked by mise
mise did not trust this workspace's configuration.
Review the configuration on the execution host before trusting it.
[Resume session] [Start fresh session] [Restore workspace]
> Technical details
```

Proposed phone composition:

```text
Agent startup blocked by mise
mise did not trust this
workspace's configuration.
Review configuration on
the execution host first.
[ Resume session        ]
[ Start fresh session   ]
[ Restore workspace     ]
> Technical details
```

Labels and spacing are illustrative. Only existing eligible actions appear.
The nearest mobile exemplar is `SessionRecoveryCard` with `RecoveryActions`.
The cause precedes actions and collapsed details on both surfaces.
Desktop uses a wrapping row. Phone uses full-width actions with 44-pixel targets.
The existing inline recovery region owns expanded diagnostic scrolling and
safe-area clearance. The transcript keeps its separate scroll owner.

### UI-02: Expanded details and later failure

```text
Technical details                  [Copy details]
Operation: Resume attempt
Phase: bootstrap
Cause: process_exited
Reason: mise_config_untrusted
Exit code: 1

Earlier history: Resume failed, mise configuration was not trusted.
Current failure: A later recovery attempt failed for its own reason.
```

The earlier cause is history, not a second active card.
Missing status or unsupported reason stays absent or uses the safe fallback.
Pending controls and recovery success use the current shared presentation.
UI-01 and UI-02 map to 006.2, .13, .24-.26, .28-.29, .34, .36 and 007.5.

## Tests

| Criteria | Permanent regression targets |
| --- | --- |
| 006.26, .36 | `common/startupdiag/startup_test.go`: `TestStartupDiagnosticMiseTrustProjection`; wrapper reader and hostile-line cases |
| 006.36; managed 004.2-.3, .8 | `server/api/startup_process_diagnostics_test.go`: `TestHandleWSInitializeProjectsEarlyProcessExit`; current/stale generation and delayed-drain cases |
| 006.26, .36; managed 004.8 | `launcher/child_log_context_test.go`: `TestPipeOutputPreservesStartupExitContext`; malformed/duplicate fields and raw-stderr exclusion |
| 006.24-.26, .36; managed 004.2-.4, .7 | `lifecycle/startup_process_diagnostics_test.go`: `TestStartupProcessBootstrapCause`; context, phase, retry, and replacement tests |
| 006.24, .26, .28-.29; 007.1-.2, .5 | `orchestrator/startup_process_diagnostics_test.go`: `TestStartupProcessRecoveryCauseSurvivesReadiness` and `TestStartupProcessDistinctRecoveryHistory`; executor/model/projection tests |
| 006.2, .13, .25-.26, .34, .36 | `session-last-agent-error.test.ts`, bootstrap recovery model/card tests, mounted recovery card tests, and details tests |

The first permanent regressions must fail before production edits.
Use channel barriers and real helper subprocesses for ordering.
No sleep-based race assertion or real mise trust operation is necessary.

## E2E tests

Add one shared real-launch fixture in
`apps/web/e2e/helpers/agent-startup-process-diagnostics.ts`.
Follow `copilot-plugin-startup-failure.ts` for private fake CLI setup, backend
environment isolation, durable privacy checks, and cleanup.
The fixture must reach the actual process/start/initialize path before rendering.
It must not seed the final `last_agent_error` as proof of startup propagation.

| Flow | Criteria | Spec/project |
| --- | --- | --- |
| Start and a subsequent fresh-start recovery attempt encounter mise refusal; show cause, status, prerequisite, and safe copy; reload retains evidence | 006.2, .13, .24, .26, .34, .36; 007.5 | `tests/session/agent-startup-process-diagnostics.spec.ts`, `chromium` |
| The same flow with touch actions, stacked controls, wrapped details, and no horizontal overflow | 006.6, .14, .34, .36 | `tests/session/mobile-agent-startup-process-diagnostics.spec.ts`, `mobile-chrome` |
| A later distinct failure retains the original dated cause in history; current controls use the latest attempt | 006.5, .8-.9, .28-.29 | Both specs through the shared fixture |
| Unknown ordinary exit and malformed toolchain reason do not invent mise attribution | 006.25-.26, .36 | Backend and model tests; one browser unknown-exit case |

## Work orders

- [x] [Task 01: Preserve safe subprocess evidence](task-01-safe-process-evidence.md)
- [x] [Task 02: Carry process causes through recovery](task-02-process-cause-propagation.md)
- [x] [Task 03: Explain process failures on desktop and phone](task-03-process-cause-presentation.md)

All work is sequential: 01, then 02, then 03.
Each work order contains exact commands from the repository root.

## Verification results

Design investigation:

- `go test -tags fts5 ./internal/agentctl/server/api -run '^TestRepro5128' -count=1 -v`: expected failure. Immediate exit loses its explanation.
- `go test -trimpath -tags fts5 ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run '^TestRepro5128' -count=1 -v`: expected failures. Wrapper drops the signature; bootstrap cause becomes unknown.
- `go test -trimpath -tags fts5 ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl/launcher -run '^(TestHandleWSInitializeCarriesExactProcessEvidence|TestManagedStartupEvidence|TestInitializeFailurePhase|TestPipeOutputPreservesSafeDiagnosticContext|TestPipeOutputRejectsUnsafeOptionalContext)$' -count=1`: passed in all four packages after temporary-file removal.

Package validation passed:

- `python3 scripts/list-docs.py validate`: 436 decisions and 1932 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans`: passed.
- `git status --short`: only the requirement, new design, and this package changed.
- `.github/scripts/pr-docs.cjs::validateCoverage`: all three work orders and their
  requirement, criterion, design, and plan references passed. Documentation-only
  changes are normally exempt. A prospective `bootstrap_failure.go` trigger
  forced the reference-validation path without changing production code.

The existing companion packages remain completed implementation records:
`managed-npm-startup-resilience`, `session-startup-failure-explanations`,
`session-error-recovery-ui`, `error-scope-and-history`,
`contribution-resume-recovery`, `resume-cancellation`,
`resumed-turn-cancellation`, and `start-model-transport-failure`.
Their historical scope, results, and statuses remain accurate for that work.
This package owns only the added process diagnostic coverage.

Implementation and browser execution are complete for work orders 01-03.

Implementation results:

- `go test -trimpath -race -tags fts5` passed for the work-order 01 and 02 package sets.
- Full non-race suites passed for every changed backend package.
- `bash scripts/lint-go-changed` passed in package groups for all changed Go files.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- The focused Vitest suites passed: 147 tests across 10 files.
- `pnpm e2e:run --host --project chromium tests/session/agent-startup-process-diagnostics.spec.ts` passed.
- `pnpm e2e:run --host --project mobile-chrome tests/session/mobile-agent-startup-process-diagnostics.spec.ts` passed.
- The browser fixture drives the real custom-agent subprocess, real initialize failure, task projection, reload, safe copy, and a distinct later fresh-start failure. Resume readiness cause preservation is covered by `TestStartupProcessRecoveryCauseSurvivesReadiness`.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, and `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

Review remediation (November 8):

- `startupdiag.ClassifyLine` now rejects overlapping prefix/suffix ranges before slicing; one-space, truncated, empty-path, and prefix/suffix-only lines return no attribution without panicking.
- `exit_code` decoding in both agentctl evidence and `LastAgentError` uses a pointer target, so explicit JSON null and missing values stay nil while numeric zero survives.
- Malformed supplied npm code or safety flags now invalidate npm diagnostic completeness at the wire boundary while preserving bounded exit status and recognized reason. The real retry-classification path rejects malformed present, unclassified, and npm-code witnesses; valid empty evidence keeps its single retry.
- Managed-runtime retry failures with a final `process_exited` cause now render the process title, summary, prerequisite, and exit-code details instead of generic managed-runtime copy.
- Explicit branch, guard, and distinct manual recovery refusals now keep their own visible explanation and suppress the earlier process prerequisite; the process cause remains in technical details and history.

Remediation verification:

- `go test -trimpath -race -tags fts5` passed for startupdiag, agentctl types, process, API, agentctl client, launcher, lifecycle, and task models.
- Full non-race suites passed for every changed backend package.
- Frontend typecheck, lint, i18n checks, and 152 focused Vitest tests passed.
- Both desktop and phone Playwright specs passed again.

## Risks

- Toolchain signatures can change. Unsupported lines must degrade to an exit summary without raw text.
- Exit and initialization race. Reuse the existing bounded evidence wait and capture exact process identity.
- New optional fields can disappear in typed copies. Verify persistence, summary, events, reload, and equality.
- Same-attempt correlation can hide a different failure if inferred from text. Require existing identity/stamp proof.
- Windows statuses exceed Unix's byte range. Keep the current nonnegative child-log range and optional status zero.
- #5129 can still block a session after the startup prerequisite is resolved. This package cannot promise checkpoint recovery.

## Documentation impact

Internal requirements, design, plan, and work orders change in this turn.
Public docs describe released behavior and remain unchanged during planning.
Task 03 updates `docs/public/sessions-and-review.md` during implementation with
the new startup explanation and execution-host prerequisite. No new command,
configuration option, feature flag, recovery permission, or ADR is required.
