---
created: 2026-11-07
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
legacy_specs: []
---

# Fix plan: recover retained standalone journal owners

## Overview

Correct the standalone continuation defect reproduced from
[issue #5165](https://github.com/kdlbs/kandev/issues/5165).
Resolve the retained manager before another instance opens its session journal.
Then prove adoption, stopped-manager replacement, and repeated backend recovery.
Execute the work orders sequentially after a later explicit implementation request.

## Evidence and root cause

Investigation baseline: `52309e613fc9a76d38a9f891b74b4d8b32bff265`.
It matched the remote main head on 2026-11-07.
The same unconditional standalone creation path exists in the reported release,
v0.99.1 at `813a741b21108ea8ed10ab58954d85c2e5205999`.
The issue is open and assigned to `carlosflorencio`.

`StandaloneExecutor.CreateInstance` resolves the session journal path, then
unconditionally sends POST through `ControlClient.CreateInstance`.
It does not inspect `PreviousExecutionID` or reconcile a retained same-session
manager. The new process manager tries to open the same bbolt database.
The retained manager still owns its exclusive handle, so acquisition times out.

`process.Manager.Stop` permits process restart and retains that handle.
Exact instance disposal uses `StopForTeardown`, which closes it.
Backend client closure alone releases neither manager nor journal.
The Docker reconnect path already resolves prior managers, but standalone
creation does not enter that path.

A temporary lifecycle test exercised the real standalone executor and control
HTTP client. Its control fixture used a real bbolt journal lock.
Both stopped-status and live-status cases recorded only:

```text
CREATE requested-execution
failed to create standalone instance: failed to create instance:
failed to create new instance: open delivery journal: lock_timeout (status 500)
```

Both assertions failed as expected. No inventory or DELETE request preceded
the failed acquisition. The [reproduction record](reproduction.md) contains
the command, fixture source, results, and limits.

This proves the continuation defect. It does not establish why the reporter's
startup recovery missed each owner, which survival configuration applied, or
whether each agent child remained alive. The fixture models those child states.
No reporter database, journal, runtime, or credentials were accessed.

## Requirement conformance and ownership

Platform owns exclusive journal access, authenticated adoption, and durable
submission admission. Executors owns control-server adoption and local process
survival. Agents owns the native conversation contract.
Keep this repair in the existing Platform capability.

Reuse [durable delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md):

- `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001`: criteria `.2` and `.3`.
- Requirement `002`: criterion `.1`.
- Requirement `003`: criterion `.2`.
- Requirement `004`: criterion `.3`.
- Requirement `006`: criteria `.1` through `.3`, `.20`, `.21`, `.23`, and `.24`.
- Requirement `007`: criteria `.4` and `.6`.

These existing criteria define the intended behavior. No requirement rewrite
or separate repair specification is necessary. The owning
[journal design](../../specs/platform/system-design/durable-agent-journal-ownership.md)
adds the missing standalone path. The
[reattachment design](../../specs/platform/system-design/durable-agent-reattachment.md)
retains authority over delivery reconciliation and native restore.

The assumption check found no material product choice that requires a question.
The issue and existing contracts require preserved ownership, unchanged native
identity, and blocked uncertain work. The
[durable-session decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md)
and [environment-ownership decision](../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
already define these boundaries. No new ADR is necessary.

## Scope

### In scope

- Standalone session-owned journals for local and worktree executors.
- Ordinary Resume and workspace execution creation after lost host tracking.
- Verified live adoption and exact disposal of stopped managers.
- Complete candidate classification, partial failure, cancellation, and races.
- Actual retained execution identity through persistence, callbacks, and Stop.
- Existing delivery admission, restore checkpoints, and peer-preserving rollback.
- Real journal tests, restart integration, and existing desktop/phone recovery flows.

### Out of scope

- Changing local survival defaults or remote executor lifetime policy.
- Restarting the shared control server or killing its process to release a lock.
- Removing journals, changing their path, or increasing the open timeout.
- Automatic prompt resend, tool replay, or replacement conversations.
- New public endpoints, configuration, feature flags, schemas, or UI controls.
- Repairing the reporter's installation or attributing its restart caller.
- Changing unrelated startup duplicate/orphan policy or remote-Docker recovery.

## Technical approach

### Resolve ownership before creation

Add `executor_standalone_reconnect.go` beside `executor_standalone.go`.
Call its resolver after acquiring the current control binding and before POST.
Inventory the requested ID, recorded predecessor, and all same-session owners.
An intermediate failed execution must not hide the original manager.
Use the existing eight-candidate bound and authenticated status operations.

Verify every relevant candidate before any disposal or creation. A matching
candidate plus another unknown candidate must block. Preserve different-session
instances. Missing inventory, unknown status, conflicting identity, duplicate
identifiers, and excess candidates must remain non-mutating failures.
Keep one runtime lease through the resulting instance-client binding.

### Live adoption and actual identity

Return the verified retained ID as both `ExecutorInstance.InstanceID` and
`StandaloneInstanceID`. Use its actual port and authenticated client identity.
Set `MetadataKeyReuseExistingProcess` before returning the selection.
Keep the retained workspace, environment, source roots, and provider identity.
Do not copy launch overrides into a surviving peer.

`ToAgentExecution`, `buildExecutionFromInstance`, and
`initializeCreatedExecution` must use that actual identity consistently.
For adoption, use `ExecutorInstance.Env` and `WorkspaceSourceRoots` instead of
the request environment and roots. Keep the saved native identity for validation.
Remove assumptions that an unused requested UUID owns callbacks or tracing.
At the launch boundary, preserve `CandidateExecutionID`: a pinned restore
candidate cannot silently become the retained execution.

Use `StartAgentProcess`'s existing `AgentStartupReattachedExisting` branch and
`adoptExistingAgentSession`. Full association, SQL generation, cursor, and
submission validation remain mandatory before prompt readiness.
Do not duplicate that reconciler in the executor helper.

Fence all post-selection cleanup with immutable adoption provenance.
Cover pre-construction failure, registration/persistence failure, race-lost
workspace creation, MCP preparation failure, and failed agent startup.
Release only attempt-owned host resources. Keep peer processes, journal,
runtime inventory, and MCP claims available for retry and explicit Stop.

### Stopped managers and repeatability

If all relevant managers are proven stopped and replacement admission passes,
DELETE those exact instances before POST creates the replacement.
Keep retained journal records and the owner marker.
Reconcile a lost DELETE response through authenticated exact-instance reads.
An unconfirmed delete cannot authorize another open.

Reuse existing per-session launch serialization and recovery admission.
Concurrent resumes and workspace reads must converge on one owner.
Recheck state after partial teardown, cancelled callers, Stop, or epoch change.
The existing typed journal error remains the final admission guard.
Disposal does not acknowledge an uncertain instruction or admit new work.

### Compatibility

| Boundary | Required evidence | Result | Verification and fallback |
| --- | --- | --- | --- |
| Standalone, live initialized owner | Current lease, task/session/workspace, native and delivery association | Adopt actual execution without ACP lifecycle calls | Real-lock and lifecycle tests. Unknown evidence blocks |
| Standalone, stopped retained manager | Complete candidate set, stopped status, replacement admission | DELETE exact manager, then open the same journal | Real manager teardown/reopen. Failed disposal blocks |
| Standalone, previous ID now names a failed attempt | Complete same-session control inventory | Resolve the original owner | Intermediate-ID regression. Ambiguity blocks |
| Different session on the shared server | Independent session and journal owner | Preserve that instance | Parallel unrelated-session control |
| Positively identified legacy peer | Existing authenticated association and generation contract | Keep eligible legacy adoption | Existing legacy tests. Missing evidence is not legacy proof |
| Local survival off | Existing shutdown policy | Preserve default process lifetime | Existing shutdown controls |
| Docker, SSH, Kubernetes, Sprites | Existing executor contracts | No changed recovery selection | Shared adoption tests only when shared code changes |

No provider-specific restore capability is added. Native ACP and other
transports use the same authenticated association contract when supported.
An unsupported peer remains blocked with its existing recovery feedback.

## Tests

New permanent test names below are implementation targets.

| Acceptance criteria | Test and file |
| --- | --- |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2`, `001.3`, `002.1` | `TestStandaloneReconnectDisposesStoppedJournalOwner` in `executor_standalone_journal_reconnect_test.go`. Retained records and marker survive reopen |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3`, `007.6` | `TestStandaloneReconnectAdoptsLiveJournalOwner` and `TestStandaloneRetainedOwnerUsesActualExecutionIdentity`. Assert the actual ID, unchanged stream and no lifecycle dispatch |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2`, `006.1`, `006.2`, `006.3`, `007.4` | `TestStandaloneReconnectBlocksUnresolvedJournalOwners`. Include a verified owner plus an unknown candidate, conflicts, incomplete inventory, and unavailable disposal |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6` | `TestStandaloneRetainedAdoptionFailurePreservesPeer` in `standalone_retained_owner_lifecycle_test.go`. Exercise rollback before and after registration |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23`, `006.24` | `TestStandaloneRetainedOwnerConcurrentAdmission` and `TestStandaloneRetainedOwnerRejectsPinnedCandidateSubstitution`. Include epoch replacement and cancelled waiters |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20`, `006.21`, `006.23`, `006.24` | `TestStandaloneRetainedJournalRestartRecovery` in `standalone_retained_journal_restart_test.go`. Use real control/instance managers, journal, SQL, and backend-side recreation |

Task 01 owns RED tests for selection, disposal, and adoption safety.
Task 02 owns restart and retained-record integration.
Keep test fixtures synthetic and restricted to disposable session roots.

## E2E tests

Task 02 runs the existing `agent-survival-restart.spec.ts` and
`mobile-agent-survival-restart.spec.ts` in their owning projects.
They cover the visible restart outcome and preserved conversation.
Strengthen their existing scenarios only if an outcome above lacks coverage.
The new Go integration must force the pre-creation path with an owner retained
and no host execution tracked. Normal startup adoption alone does not prove
that regression. Do not add a production fault endpoint for the test.

Assert unchanged session/native identity, ordered retained output, no extra
prompt or message, and one accepted later explicit instruction after admission.
Keep Stop usable and preserve uncertainty on blocked recovery.
The phone test uses the configured mobile project and existing touch controls.

This package changes no rendered UI, layout, navigation, or copy.
No ASCII preview or new mobile composition is necessary.
The existing task chat is the desktop and phone exemplar.

## Work orders

- [x] [Task 01: Reconcile standalone owners and preserve live adoption](task-01-reconcile-standalone-journal-owner.md)
- [x] [Task 02: Prove retained journal recovery across restart](task-02-prove-restart-recovery.md)

Task 02 depends on Task 01. Both run sequentially in the primary conversation.
This package does not authorize delegation, implementation, or publication.

## Companion implementation records

- [Remote-Docker recoverable resume](../remote-docker-recoverable-resume/plan.md): completed shared Docker owner correction.
- [Durable agent sessions](../durable-agent-sessions/plan.md): storage ownership and protocol adoption.
- [Delivery reconciliation](../durable-agent-session-reconciliation/plan.md): persisted identity and replay.
- [Surviving-agent reattachment](../durable-agent-reattachment/plan.md): immutable startup disposition and no ACP initialization.
- [Journal shutdown recovery](../agentctl-journal-shutdown-recovery/plan.md): teardown and restore-only behavior.

Those packages' historical results do not prove this standalone defect.
Preserve their statuses. If shared behavior changes during implementation,
append the affected evidence and link this package from the relevant record.

## Public documentation impact

The existing [executor guide](../../public/executors.md#agent-lifetime-and-reconnect)
already describes the required behavior. The repair adds no operator procedure
or supported configuration. No public text changes are necessary at planning.
Task 02 rechecks that guide and the configuration reference against the final diff.

## Verification results

- Canonical issue and all comments read. The issue has no images or comments.
- Authenticated GitHub identity: `carlosflorencio`. Assignment verified live.
- Temporary reproduction: two expected failures with real `lock_timeout`, exit 1.
- Related existing ownership/adoption tests: six passed across lifecycle and instance packages.
- Catalog validation passed: 437 decisions and 1937 specifications.
- Specification-linter tests passed: 36 tests. Full specification lint passed.
- Documentation coverage passed for both work orders with the planned production entry point included.
- Package Markdown links exist. Diff whitespace checks passed.
- The temporary Go test was removed. Only this package and the owning design changed.
- Production implementation completed in the standalone pre-creation resolver, adoption provenance, and rollback preservation. See both work orders for exact commands and results.
- Review remediation added a nonterminal failed-adoption detach, exact retained built-in MCP binding restoration and GET-only verification, and provenance-based empty retained evidence. Manager workspace-restore and task-service binding regressions passed.
- Desktop and mobile survival E2E suites passed with their existing scenarios.

## Risks

- A live agent and a stopped manager require different actions. Status errors cannot authorize disposal.
- Returning the actual retained ID affects callbacks, registration, tracing, and Stop. Test each boundary.
- Existing early rollback assumes fresh allocation in some paths. Adoption provenance must precede those paths.
- A restore checkpoint owns its candidate identity. Live adoption cannot silently change that allocation.
- The exact reporter startup failure remains unverified. This package proves and repairs the duplicate-open path.
- Linux reproduction does not establish native macOS or Windows survival behavior. Preserve existing platform restrictions.
