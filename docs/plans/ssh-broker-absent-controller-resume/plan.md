---
created: 2026-11-12
updated: 2026-11-16
status: in_progress
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
  - ../../specs/executors/system-design/ssh-agentctl-reconciliation.md
legacy_specs: []
---

# Fix plan: Resume SSH sessions after controller exit or host reboot

## Overview

Resolve both reports in [issue #5287](https://github.com/kdlbs/kandev/issues/5287).
The issue remains assigned to `carlosflorencio`.
The user approved the expanded scope on 2026-11-16 and requested a simple fix.

Keep two sequential work orders. Task 01 corrects the existing helper and
resume callers. Task 02 proves recovery over real SSH and updates public guidance.
Task 01 is implemented. Task 02 remains in progress until its full E2E gate passes.

## Requirement and decision alignment

The executor system owns controller identity and replacement.
[REQ-EXECUTORS-SSH-EXECUTOR-001](../../specs/executors/requirements/ssh-executor.md),
AC-001.9 now explicitly includes a verified host reboot.
AC-001.10 and AC-001.18 through AC-001.24 retain their ownership and failure guards.

The [reconciliation design](../../specs/executors/system-design/ssh-agentctl-reconciliation.md)
and [existing launch-proof ADR](../../decisions/2026-11-07-ssh-controller-launch-proof.md)
describe the same rule: a changed boot proves the old launch gone.
It grants no right to signal a current PID holder or remove a directory.

## Confirmed causes and evidence

Investigation used `9b2b8e98e2b7a510560c65ac572d412fa3394eb0`,
which matched remote `main` on 2026-11-12.

### Controller exit

Both `resetManagedBrokerResume` and `resetTrackedManagedBrokerResume` require
`verifyRecordedControllerIdentity` to accept the old controller before replacement.
That verifier accepts only `OutcomeVerified`.
A valid `OutcomeAbsent` therefore fails broker resume.
Its empty reason/detail produce `controller reconciliation unresolved:  ()`.

The temporary `TestReproIssue5287ConfirmedAbsentController` drove the real
resume entry point with fake SSH and a validated absence result.
Ordinary SSH passed. Both broker paths failed with the reported error.
Each case issued one verification and zero stop calls.

### Host reboot

The [November 13 comment](https://github.com/kdlbs/kandev/issues/5287#issuecomment-6861981027)
reports `identity_conflict (recorded launch belongs to a previous boot)`.
The helper's `absentOutcome` refuses a previous-boot receipt.
A live holder of a reused PID also reaches a conflict before the old launch
can count as absent.

The temporary `TestReproIssue5287PreviousBootController` used an exited
owned process and the real helper. The original receipt returned `absent`.
Changing only its boot component reproduced the exact reported conflict.
Existing identity-change and PID-reuse guards passed.

These are focused reproductions, not a live GitHub or host-reboot run.
Both temporary test files were removed. Commands and historical results appear below.

## Scope

In scope:

- Confirmed absence in both broker paths, including a verified host reboot.
- Receipt-bearing ordinary SSH resume after reboot, including PID reuse.
- Existing verified-live stop followed by absence proof.
- Directory preservation when a persisted receipt-bearing controller is already absent.
- Focused tests, real SSH recovery evidence, and the existing public explanation.

Out of scope:

- New outcomes, wire fields, SQL columns, public actions, flags, or recovery services.
- Background repair, new ownership stores, generic executor refactors, or forced cleanup.
- Clearing durable stop metadata as an independent persistence change.
- Missing runtime-secret repair, weaker authentication, or unknown-evidence bypass.
- Changes to terminate-mode boot policy, other runtimes, or rendered UI.

## Technical approach

### 1. Classify boot changes inside the existing helper

In `reconcileProcess`, verify mode calls `BirthBootMatches` after receipt
and helper-user validation, before `openProcessHandle` or PID inspection.

- Different, verified boot: return existing `OutcomeAbsent`.
  The old controller and its children cannot survive that reboot.
  Signal nothing and change no directory.
- Matching boot: use the existing process, generation, and child-drainage checks.
- Invalid or unreadable evidence: return an existing unresolved result.

Keep terminate mode's current `absentOutcome` guard.
Do not globally remove that guard or change `processidentity`.
`SessionMembersRemain` already recognizes that an old boot has no surviving children.

### 2. Let resume use the existing absence path

Both broker paths accept a validated `absent` response.
They skip `stopRemote`, clear stale controller metadata with
`clearSSHResumeRuntimeMetadata`, and let normal creation run.

For `verified`, retain the guarded stop and separate post-stop absence check.
Every unexpected, unresolved, or malformed response preserves metadata and fails.
Include the actual outcome in errors and preserve `errors.Is` behavior.

Ordinary receipt-bearing SSH resume applies the same live/absent distinction.
It verifies the receipt before any numeric PID probe, so a reused current PID
after reboot cannot block the helper's confirmed-absence reset. Keep the
numeric liveness probe for legacy records only.

Keep `verifyRecordedControllerIdentity` live-only for adoption and stop callers.
Reuse the existing result type and, where duplicated, a small local resume helper.
Keep the existing tracked-resource teardown, preflight, and owner checks.

### 3. Preserve directory ownership without another protocol

`stopPersistedRemoteAgentctl` currently allows directory removal after a verify
response of `absent` and a matching numeric pidfile.
A pidfile match cannot distinguish boots.

For receipt-bearing already-absent controllers, preserve the session directory.
Do not add another wire field or boot probe to authorize removal.
Normal verified-live stop keeps its existing directory cleanup.
Legacy records keep their existing guarded behavior.

This small caller adjustment prevents reboot verification from granting
directory-removal authority. It can leave disposable runtime files until
separately authorized task cleanup.

### Metadata and conversation

Use the existing runtime reset key set and launch persistence.
Keep target configuration, workspace, and unrelated metadata.
A replacement receives a new receipt and cannot inherit the predecessor's proof.
Startup recovery already maps `OutcomeAbsent` to `RecoveryOutcomeNoMatchingInstance`.

Resume preserves the provider conversation.
Fresh start retains its existing conversation-replacement policy.
Both actions use the corrected preflight.
Missing authentication and unrelated ownership conflicts remain errors.

### Compatibility matrix

| Case | Expected behavior | Evidence |
| --- | --- | --- |
| Broker SSH, absent controller, tracked or untracked | Skip stop and launch a replacement | Lifecycle unit tests and SSH E2E |
| Receipt from an earlier verified boot, PID absent or reused | Return `absent` without inspecting/signalling that PID | Real helper tests |
| Ordinary receipt-bearing SSH after reboot | Verify receipt before numeric PID checks; reset before forwarder/adoption | Lifecycle tests with absent, reused, and permission-denied PIDs |
| Same-boot matching live controller | Existing stop/adoption policy | Existing and new focused tests |
| Same-boot PID/generation conflict or unknown evidence | Preserve proof and fail | Existing guard tests |
| Receipt-bearing persisted stop, controller already absent | Preserve session directory | Lifecycle regression |
| Terminate mode with a previous-boot receipt | Existing unresolved refusal | Helper regression |
| Legacy/Darwin SSH or an old incompatible helper | Existing guarded behavior | Existing tests, no weaker fallback |
| Other runtimes | Existing behavior | No runtime dispatch changes |

## Tests

All criteria below belong to `REQ-EXECUTORS-SSH-EXECUTOR-001`.
New test names are planned, not implemented.

| Criteria | Regression evidence |
| --- | --- |
| AC-001.9, .18, .22 | `TestSSHBrokerResumeConfirmedAbsentController`, both broker paths |
| AC-001.9, .20, .24 | `TestReconcileVerifyPreviousBootController`, absent and reused PID with directory markers preserved |
| AC-001.9, .24 | `TestSSHResumePreviousBootController`, ordinary SSH without stale adoption |
| AC-001.10, .21, .23 | `TestReconcileVerifyInvalidBootEvidence` and existing transport/preflight refusal tests |
| AC-001.19, .22 | `TestSSHBrokerResumeVerifiedController` and `TestSSHBrokerResumePostStopRequiresAbsence` |
| AC-001.20, .22, .24 | `TestSSHAbsentReceiptPreservesDirectory` and existing PID/generation guards |
| AC-001.20, .24 | `TestReconcileTerminatePreviousBootRefuses` and `TestSSHVerifyRecordedControllerIdentityRequiresLiveController` |
| AC-001.21, .23 | `TestSSHBrokerResumeRejectsUnsafeOutcome` and `TestSSHBrokerResumeDiagnostics` |

Use a synthetic older boot component in a valid receipt.
A live owned process at that PID proves that verification preserves a reused holder.
No test needs a real reboot or a product test flag.

## E2E tests

Extend `apps/web/e2e/tests/ssh/recovery.spec.ts` in project `containers`.
Keep its four existing recovery cases and add four under `broker-backed SSH`:

1. `agent.stop` followed by `session.recover` with `resume`.
2. External SIGTERM, confirmed controller/child exit, then resume.
3. A synthetic previous-boot receipt, then resume.
4. A synthetic previous-boot receipt, then `fresh_start`.

The reboot cases use the isolated fixture's existing SQLite database.
Stop the owned controller and fixture backend, then change only the selected
receipt's boot component. Use `node-sqlite.ts` and the existing backend
restart pattern. Scope the update to the expected session and execution.
Close the database before restart. Do not alter the host boot ID.
Label this as synthetic reboot evidence.

Each case proves that a managed broker URL and lease activated the production path.
Reuse the existing plugin credential fixture and reachable HTTPS broker setup.
The ordinary SSH seed alone does not activate a managed broker.

Require a new controller generation, preserved workspace sentinel, and a new
accepted-turn response. Resume retains the conversation. Fresh start uses its
existing policy. Preserve a sibling controller and its workspace data.
Assert results through the existing task page and persisted runtime record.

Run the related `cleanup-reconciliation.spec.ts` cases as well.
Planned discovery: eight recovery cases and two existing cleanup cases.
A skipped broker case is not acceptance.
No rendered UI changes or mobile composition changes are planned.

## Work orders

- [x] [Task 01: Correct absence and reboot handling](task-01-handle-confirmed-absence.md)
- [ ] [Task 02: Prove SSH recovery and document it](task-02-prove-session-recovery.md), after Task 01

Both are sequential. No delegation is authorized.

## Related delivery records

The completed [directory-loss package](../ssh-controller-directory-loss/plan.md)
owns the original receipt and helper implementation.
This follow-up changes verify-mode reboot classification and preserves
receipt-backed absent-session directories. Those results describe its original implementation.

The [idle-reclaim](../ssh-agentctl-absence-idle-reclaim/plan.md)
and [orphan sweep](../ssh-orphaned-agentctl-sweep/plan.md) packages have independent triggers.
They gain no authority to kill a reused PID from this change.

## Verification results

Historical investigation on 2026-11-12:

```sh
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestReproIssue5287ConfirmedAbsentController$' -count=1 -v)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^(TestSSHResumeRejectsReceiptPIDMismatch|TestSSHResumeRejectsUnverifiedLiveController|TestSSHBrokerResumeVerifiesControllerBeforeStop|TestSSHResumeRetainsProofWhenAbsentChildUnconfirmed|TestSSHBrokerResumeRetainsProofWhenChildUnconfirmed|TestSSHExecutorResetManagedBrokerResume|TestSSHLaunchReceiptReplacementDoesNotInheritPriorProof)$' -count=1 -v)
```

The reproduction failed in both broker cases and passed the ordinary control.
Seven top-level guard tests passed, including four reset subcases.

Historical follow-up on 2026-11-16:

```sh
(cd apps/backend && go test -trimpath ./internal/agentctl/sshreconcile -run '^(TestReproIssue5287PreviousBootController|TestReconcileRejectsIdentityChanges|TestReconcileRejectsReusedPID)$' -count=1 -v)
```

The synthetic previous-boot case failed with the reported conflict.
The same-boot control and both existing guard tests passed.
The temporary reproduction sources are no longer present.

Revised package validation on 2026-11-16:

- `python3 scripts/list-docs.py validate`: passed, 450 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: covered.
  Both work orders resolve their requirement, acceptance, plan, and design references.
- Package links, whitespace, and `git diff --check`: passed.
- Seven documentation files are modified or untracked. The staging area is empty.
  No production or permanent test files changed.

Task 01 implementation and focused verification completed on 2026-11-16.
Task 02 recovery coverage and public guidance were added on 2026-11-16.
The container suite discovered 10 tests: six passed and the four broker cases
were skipped because the isolated HTTPS broker URL was unavailable. Task 02
remains in progress until all four broker cases pass.

## Risks

- A boot change proves the old launch gone, not ownership of a current directory.
- The boot comparison must precede old-PID inspection in verify mode.
- Invalid evidence and genuine same-boot conflicts must remain errors.
- Preserving already-absent receipt directories can leave disposable runtime files.
- Real SSH broker E2E needs Docker and the existing reachable HTTPS broker URL.
- Synthetic reboot coverage must not be reported as a real host reboot.
