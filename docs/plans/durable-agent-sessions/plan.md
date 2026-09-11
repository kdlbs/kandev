---
created: 2026-09-10
status: complete
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-004
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
  - ../../specs/platform/system-design/durable-agent-delivery.md
legacy_specs: []
---

# Implementation plan: Durable agent sessions

## Overview

This package keeps Kandev session identity independent from native harness state.
It first delivers explicit continuity recovery, then durable delivery between the backend and agentctl.

The user requested implementation after the session-durability investigation.
The requirements and designs remain draft until review, and this branch contains
the implementation and its regression tests. The no-feature-flag and autonomous
park-and-surface decisions are implemented. Local release gates pass; final
evidence for browser, PostgreSQL, retained-executor, and live harness behavior is
environment-dependent.

## Source contracts

- [Harness continuity requirements](../../specs/agents/requirements/harness-session-continuity.md): six requirements and fourteen acceptance criteria.
- [Harness continuity design](../../specs/agents/system-design/harness-session-continuity.md).
- [Durable delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md): seven requirements and sixteen acceptance criteria.
- [Durable delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Boundary ADR](../../decisions/2026-09-10-durable-harness-session-boundaries.md).
- [Existing recovery requirements](../../specs/agents/requirements/agent-resume-runtime-recovery.md).
- [Existing queue authority](../../specs/tasks/system-design/resume-prompt-queue.md).

## Scope

### In scope

- Native-first restore with typed, non-destructive failure classification.
- Explicit context continuation from bounded canonical history.
- Original agent-visible CWD and native-state identity.
- Persistent recovery outcomes with desktop/mobile parity.
- A bounded agentctl journal, immutable submissions, and cursor replay.
- Backend inbox persistence, idempotent projection, and disconnect reconciliation.
- Default behavior without feature flags, retained executor storage, and operator documentation.
- Explicit recovery blocks for Office, CI, and PR/MR automation.

### Out of scope

- A Kandev replacement for the Codex, Claude Code, or OpenCode model loop.
- Universal native-state export or cross-harness migration.
- Automatic context continuation after native-state loss.
- Exactly-once external tool execution.
- Survival after deliberate executor-volume removal.
- Automatic autonomous context continuation and operator rollout toggles.
- Universal native-state migration across harnesses or deleted executor storage.

## Technical approach

### Milestone A: Explicit session continuity, tasks 01 through 07

Task 01 introduces the unconditional conservative restore contract.
Tasks 02 through 04 add durable continuation state, one restore coordinator, and workspace identity.
Task 05 completes the interactive recovery flow.
Tasks 06 and 07 add autonomous parking and operator recovery for Office and CI/PR/MR automation.

This milestone is implemented on the branch and requires no runtime toggle.
The code parks autonomous work when native recovery is unresolved. It does not
authorize context continuation without an operator action.

Lifecycle changes center on `createOrLoadSession`, `createReboundACPSession`, and the context-injection checkpoint.
Runtime configuration capture precedes native lifecycle mutation.
The coordinator preserves the prior token until candidate configuration and the generation commit succeed.

The task repository adds generation, restore-attempt, and continuation-snapshot records.
It uses canonical SQL messages rather than optional JSONL history.
The same repository owner provides both SQLite and PostgreSQL behavior.

The user selects context continuation explicitly.
The existing `resume_new_branch` action remains native-only.
A new conversation carries a persistent notice about lost private harness state and historical paths.

### Milestone B: Durable agentctl delivery, tasks 08 through 18

Task 08 adds the journal and its admission boundary.
Tasks 09 through 12 bind its lifetime to each executor's retained storage.
Tasks 13 through 16 implement submissions, replay, inbox projection, and workflow deduplication.
Task 17 reconciles failures without automatic prompt resend.
Task 18 activates negotiated v1 automatically for compatible peers and supported retained storage.

There is no delivery flag or hidden environment opt-in.
The durable path is enabled when negotiated capabilities and retained storage
are present. Final release evidence must pass before the path ships.
Legacy-peer compatibility is not a fallback for journal corruption, full storage, or authentication failure.

The journal is a bounded bbolt store, not a second canonical transcript.
It persists normalized events and submission identity before publication or dispatch.
It retains tombstones so cleanup cannot make an old submission executable.

The backend adds an inbox and separate received/projected cursors.
Its acknowledgment follows the inbox commit.
Canonical message projection and its cursor share a SQL transaction.
Workflow consumers use durable effect keys at their authoritative transition.

The runtime client replaces its unbounded event queue with bounded durable intake.
ACP cancellation and permission responses remain independent from slow event consumers.
Native-load replay suppression remains separate from transport replay.

### Ownership and migration

Backend tables remain within the task schema owner unless implementation proves a separate owner necessary.
Any new owner must enter requiredstores and the fixed storeconformance adapter set.
Schema remains required in both durable and legacy protocol modes.
Queue claims remain under the message-queue schema owner. Office owns its run recovery reference.
Additive migration must preserve existing ACP IDs, resume tokens, messages, and queue identity.

The journal schema has an explicit version.
Older writers reject newer journals without replacement.
Active v1 streams cannot downgrade to legacy admission.
Supported rollback retains data and reconciles pending submissions before new work.
Pre-v1 binaries are not a safe rollback target while unresolved durable work remains.
Without a feature flag, a regression requires a corrected release or a documented compatible rollback.

### Refreshed implementation baseline

The initial investigation used `28581093085ab7a903d7ee331ad7c713ea159a7e`.
The branch now includes verified main `65d64f6fa9f85b4977b8cdd62cf162380732effc` by fast-forward.
The original draft files survived the update.
This revision reconciles the following changes:

- Queue editing and dispatch recovery from #3480, including `queue_dispatch_claims`, dispatch attempts, and `WithSessionAdmission`.
- Conversational-only completed-task follow-ups from #3564.
- Inherited-environment detachment on executor mismatch from #3565.
- Fail-closed unattended Office budget admission from #3520.

Before implementation, compare the chosen base against this immutable baseline.
Reconcile later changes to these contracts before coding.

### Existing ownership to extend

- `internal/orchestrator/queue_dispatch_recovery.go`: reconcile unknown claims before restoring dispatch eligibility.
- `internal/orchestrator/messagequeue/repository_dispatch_recovery.go`: retain attempt identity and add protocol/submission association.
- `internal/orchestrator/messagequeue/service.go`: reuse admission serialization rather than introducing another queue fence.
- `internal/office/service/scheduler_integration.go`: park only affected runs and preserve normal claim ownership.
- `internal/office/service/budget_admission.go`: repeat `admitRun` with unchanged provenance after authorized recovery.
- `internal/agent/planinjection/reduce.go`: retain the 12,000-byte handover plan budget inside the 64 KiB snapshot.

These backend paths are relative to `apps/backend`.
The queue owns its claims, the task repository owns recovery blocks, and Office owns run references.
Cross-owner projection failure cannot grant prompt admission.

### Exact boundaries to preserve

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`: native CWD and replay barrier.
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`: immutable configuration capture and restoration.
- `apps/backend/internal/orchestrator/executor/executor_resume.go`: branch recovery and original task directory.
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`: disconnect state, completion, and callbacks.
- `apps/backend/internal/orchestrator/event_handlers_agent.go`: current turn and queue admission.
- `apps/backend/internal/task/repository/sqlite/message.go`: canonical message transactions.
- `apps/web/components/task/ensure-session-error.tsx`: shared recovery surface.
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`: native mobile interaction pattern.

## Companion contracts and packages

The refreshed baseline includes these adjacent packages:

| Package | Relationship |
| --- | --- |
| [Queued edit fencing](../queued-message-edit-fencing/plan.md) | Reuse admission and edit leases. Do not redefine their operation IDs. |
| [Queued edit auto-send](../queued-message-edit-auto-send/plan.md) | Add recovery eligibility to the existing post-save drain. Preserve Auto-run intent. |
| [Task completion](../task-completion/plan.md) | Preserve conversational-only completed-task follow-ups and their regression tests. |
| [Office agent recovery](../office-agent-recovery/plan.md) | Preserve status-only recovery. It does not resolve session recovery blocks. |

Those packages retain their recorded implementation results.
This package owns the new recovery behavior and tests.
It does not mark older work pending or relabel their passing tests as new evidence.

## Metrics ownership

The continuity design owns the `agent_restore_*` names.
The delivery design owns the `agent_delivery_*` names.
Tasks 03, 08, 13, 14, 15, and 17 implement their assigned producers.
Root `AGENTS.md` documents each family with the corresponding implementation.
`CLAUDE.md` remains its symlink.
Task 18 reconciles the final family and producer documentation.

## Tests

The following names are required future evidence, not tests that already pass.
Each work order starts with a failing test and records its result.
Test comments use the relevant `AC-*` ID where the mapping is not otherwise clear.

| Acceptance criterion | Proposed test file | Proposed test |
| --- | --- | --- |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1 | `apps/backend/internal/agent/runtime/lifecycle/session_restore_policy_test.go` | `TestRestorePolicyPreservesNativeIdentity` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2 | `apps/backend/internal/agent/runtime/lifecycle/session_restore_policy_test.go` | `TestRestorePolicyRequiresExplicitContinuation` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.1 | `apps/backend/internal/task/repository/sqlite/session_continuation_test.go` | `TestContinuationSnapshotBoundedCanonicalHistory` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.2 | `apps/backend/internal/task/repository/sqlite/session_continuation_test.go` | `TestContinuationCheckpointCrashSafety` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1 | `apps/backend/internal/agent/runtime/lifecycle/session_restore_coordinator_test.go` | `TestRestoreCoordinatorPreservesRuntimeConfiguration` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.2 | `apps/backend/internal/agent/runtime/lifecycle/session_restore_coordinator_test.go` | `TestRestoreCoordinatorPartialFailureBlocksDispatch` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.1 | `apps/backend/internal/agent/runtime/lifecycle/session_workspace_identity_test.go` | `TestResumeChecksDirectoryAndNativeState` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-004.2 | `apps/backend/internal/agent/runtime/lifecycle/session_workspace_identity_test.go` | `TestBranchRecoveryNeverFallsBackToNewConversation` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1 | `apps/web/lib/services/session-recovery-service.test.ts` | `persists typed recovery outcomes across reload` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.2 | `apps/web/e2e/tests/session/mobile-harness-session-continuity.spec.ts` | `mobile recovery preserves actions and containment` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1 | `apps/backend/internal/agentctl/journal/journal_test.go` | `TestJournalCommittedRecordsSurviveKill` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2 | `apps/backend/internal/agentctl/journal/journal_test.go` | `TestJournalStorageFailuresFailClosed` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1 | `apps/backend/internal/agent/runtime/lifecycle/executor_journal_lifetime_test.go` | `TestNativeJournalSurvivesAgentctlReplacement` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2 | `apps/backend/internal/agent/runtime/lifecycle/executor_journal_lifetime_test.go` | `TestNativeJournalLossAndCleanupSafety` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1 | `apps/backend/internal/agent/runtime/lifecycle/executor_docker_journal_test.go` | `TestDockerJournalSurvivesReplacement` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2 | `apps/backend/internal/agent/runtime/lifecycle/executor_docker_journal_test.go` | `TestDockerJournalLossIsExplicit` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1 | `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_journal_test.go` | `TestSSHJournalSurvivesReplacement` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2 | `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_journal_test.go` | `TestSSHJournalLossIsExplicit` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1 | `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_journal_test.go` | `TestKubernetesJournalSurvivesReplacement` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2 | `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_journal_test.go` | `TestKubernetesJournalLossIsExplicit` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1 | `apps/backend/internal/agentctl/server/process/submission_delivery_test.go` | `TestSubmissionRetryDispatchesOnce` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2 | `apps/backend/internal/agentctl/server/process/submission_delivery_test.go` | `TestSubmissionCrashWindowAndHashConflict` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1 | `apps/backend/internal/agentctl/server/api/durable_stream_test.go` | `TestDurableStreamReplayLiveBoundary` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2 | `apps/backend/internal/agentctl/server/api/durable_stream_test.go` | `TestDurableStreamCursorAndBackpressure` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1 | `apps/backend/internal/task/repository/sqlite/agent_delivery_inbox_test.go` | `TestInboxAckAfterCommitDeduplicatesMessages` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2 | `apps/backend/internal/orchestrator/agent_delivery_projection_test.go` | `TestReplayedTerminalIntentAdvancesOnce` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1 | `apps/backend/internal/agent/runtime/lifecycle/stream_reconciliation_test.go` | `TestDisconnectReconcilesBeforeTerminalOutcome` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2 | `apps/backend/internal/agent/runtime/lifecycle/stream_reconciliation_test.go` | `TestUncertainSubmissionBlocksQueueAndKeepsStop` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1 | `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation_test.go` | `TestDurableDeliveryLegacyPeerCompatibility` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2 | `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation_test.go` | `TestDurableDeliveryRollbackPreservesUncertainty` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3 | `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation_test.go` | `TestDurableDeliveryActivatesWithoutConfiguration` |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4 | `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation_test.go` | `TestJournalFailureNeverSelectsLegacyDelivery` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1 | `apps/backend/internal/office/service/session_recovery_test.go` | `TestOfficeSessionRecoveryParksDurably` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2 | `apps/backend/internal/office/service/session_recovery_test.go` | `TestOfficeRecoveryPreservesRunBudgetAdmission` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3 | `apps/backend/internal/office/service/session_recovery_test.go` | `TestOfficeRecoveryCannotBeClearedByBackgroundActors` |
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4 | `apps/web/e2e/tests/office/mobile-session-recovery-required.spec.ts` | `operator reaches session recovery from a parked run` |

Integration evidence supplements the table:

- `internal/task/repository/sqlite/session_continuation_test.go:TestContinuationComposedPlanBudget`: budget composition for continuity criterion 002.1.
- `internal/orchestrator/queue_dispatch_recovery_test.go:TestPendingQueueDispatchReconcilesOriginalSubmission`: delivery criteria 003.1 and 006.1.
- `internal/orchestrator/queue_dispatch_recovery_test.go:TestLegacyUnknownDispatchRequiresRecovery`: delivery criteria 003.2 and 007.1.
- `internal/orchestrator/automation_session_recovery_test.go`: `TestAutomationRecoveryPreservesPendingWork`, `TestAutomationRecoveryRequiresExplicitSettlement`, and `TestAutomationRecoveryBlocksRepeatedTriggers` cover continuity criteria 006.1 through 006.3.
- Desktop and mobile automation recovery scenarios cover continuity criterion 006.4 for CI, PR, and MR dispatchers.

- `internal/orchestrator/agent_delivery_projection_test.go`: `TestReplayedTerminalIntentAdvancesOnce` covers workflow effects for delivery criterion 005.2.
- `internal/agent/runtime/lifecycle/session_restore_coordinator_test.go`: stale action, archive, and prevent-auto-start subtests cover continuity criteria 001.2 and 003.2.
- `internal/agentctl/server/api/durable_stream_test.go`: permission and cancellation race subtests cover delivery criteria 004.1 and 004.2.
- `internal/agent/runtime/lifecycle/durable_delivery_activation_test.go`: default activation, genuine legacy support, and journal-failure refusal cover delivery criteria 007.1, 007.3, and 007.4.
- `internal/persistence/storeconformance/`: fresh, replay, and previous-stable upgrade coverage applies to tasks 02, 06, 07, 13, 15, and 16.

Backend test paths in this supplemental list are relative to `apps/backend`.
Unit fixtures prove deterministic policy, not live third-party behavior.
Harness compatibility changes also require opt-in smoke evidence against the shipped adapter/runtime versions.
That evidence records same-CWD, changed-CWD, and missing-state outcomes without real workspace mutations.

## E2E tests

All paths are relative to `apps/web/e2e/tests`.
New fixtures use disposable Kandev state and the repository mock agent.
They do not interrupt a developer's active session.

| Flow | Proposed file | Project | Acceptance |
| --- | --- | --- | --- |
| Native resume, explicit context continuation, blocked restore, reload, stale action | `session/harness-session-continuity.spec.ts` | chromium | Continuity 001.2, 004.2, 005.1 |
| Equivalent recovery actions, focus, touch targets, overflow, reload | `session/mobile-harness-session-continuity.spec.ts` | mobile-chrome | Continuity 005.1, 005.2 |
| Disconnect after acceptance, missing acknowledgment, terminal replay, queue held, Stop | `session/durable-agent-delivery.spec.ts` | chromium | Delivery 003.1, 005.1, 006.1, 006.2 |
| Reconnect, uncertainty, Stop, no duplicate notice | `session/mobile-durable-agent-delivery.spec.ts` | mobile-chrome | Delivery 006.2, 007.2 |
| Retained container storage and explicit storage loss | `docker/durable-journal.spec.ts` | containers | Delivery 002.1, 002.2 |
| Remote agentctl replacement with retained storage | `ssh/durable-journal.spec.ts` | containers | Delivery 002.1, 002.2 |
| Pod replacement with retained environment storage | `kubernetes/durable-journal.spec.ts` | containers | Delivery 002.1, 002.2 |
| Parked Office routine, repeated ticks, operator recovery, budget refusal | `office/session-recovery-required.spec.ts` | chromium | Continuity 006.1, 006.2, 006.3 |
| Office inbox/run navigation to recovery, reload, no duplicate notice | `office/mobile-session-recovery-required.spec.ts` | mobile-chrome | Continuity 006.4 |
| CI/PR/MR repeated triggers, parked queue, explicit settlement | `session/autonomous-session-recovery.spec.ts` | chromium | Continuity 006.1, 006.2, 006.3 |
| Automation run/task notice to recovery, touch controls | `session/mobile-autonomous-session-recovery.spec.ts` | mobile-chrome | Continuity 006.4 |

The acceptance column uses the full ID prefixes defined in the linked requirements.
Use full IDs in test annotations.

Existing regression gates include:

- `session/session-resume-recovery.spec.ts` and `session/mobile-session-resume-recovery.spec.ts`.
- `session/session-resume-prompt-queue.spec.ts`.
- `session/session-resume-keeps-review-state.spec.ts`.
- `session/session-resume-dedup.spec.ts`.

Run the targeted commands in each work order.
After task 18, run this integration regression gate from the repository root:

```bash
(cd apps/web && rtk pnpm e2e:run --project chromium tests/session/harness-session-continuity.spec.ts tests/session/durable-agent-delivery.spec.ts tests/session/session-resume-recovery.spec.ts tests/session/session-resume-prompt-queue.spec.ts tests/session/session-resume-keeps-review-state.spec.ts tests/session/session-resume-dedup.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-harness-session-continuity.spec.ts tests/session/mobile-durable-agent-delivery.spec.ts tests/session/mobile-session-resume-recovery.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/office/session-recovery-required.spec.ts tests/session/autonomous-session-recovery.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/office/mobile-session-recovery-required.spec.ts tests/session/mobile-autonomous-session-recovery.spec.ts)
```

The guarded runner controls worker and memory limits.
Do not overlap full suites or use all-worker overrides.
Container evidence requires `KANDEV_E2E_CONTAINERS=1`, Docker, and the supported SSH/Kind fixtures.
PostgreSQL evidence requires a provisioned test database through `KANDEV_TEST_POSTGRES_DSN`.
A skipped dependency-based suite remains incomplete evidence.

## Work orders

All work is sequential. Wave numbers do not authorize parallel agents.

- [x] [Task 01: Define the restore contract](task-01-restore-contract.md)
- [x] [Task 02: Persist continuation snapshots](task-02-continuation-snapshot.md)
- [x] [Task 03: Unify harness restore orchestration](task-03-restore-coordinator.md)
- [x] [Task 04: Preserve workspace identity during recovery](task-04-workspace-identity.md)
- [x] [Task 05: Expose explicit continuity recovery](task-05-continuity-feedback.md)
- [x] [Task 06: Park Office runs that require session recovery](task-06-office-recovery.md)
- [x] [Task 07: Park automation dispatches that require recovery](task-07-automation-recovery.md)
- [x] [Task 08: Add the agentctl delivery journal](task-08-agentctl-journal.md)
- [x] [Task 09: Bind journals to retained native storage](task-09-journal-ownership.md)
- [x] [Task 10: Retain the Docker delivery journal](task-10-docker-journal.md)
- [x] [Task 11: Retain the SSH delivery journal](task-11-ssh-journal.md)
- [x] [Task 12: Retain the Kubernetes delivery journal](task-12-kubernetes-journal.md)
- [x] [Task 13: Persist prompt submission outcomes](task-13-prompt-submissions.md)
- [x] [Task 14: Implement cursor-based event replay](task-14-event-replay.md)
- [x] [Task 15: Project replayed conversation events](task-15-inbox-projection.md)
- [x] [Task 16: Deduplicate projected workflow effects](task-16-workflow-effect-dedup.md)
- [x] [Task 17: Reconcile disconnected submissions](task-17-disconnect-reconciliation.md)
- [x] [Task 18: Activate negotiated durable delivery](task-18-protocol-activation.md)

## Verification results

Implementation is complete on 2026-09-11 against main
`65d64f6fa9f85b4977b8cdd62cf162380732effc`.

- Focused backend lifecycle, journal, task SQLite, agentctl, API, process, and orchestrator suites pass.
- Focused race coverage passes for orchestrator, agentctl API/process/journal, runtime agentctl, and lifecycle.
- Store conformance and persistence upgrade checks pass.
- Backend lint and SQL guard pass.
- Agentctl native and Linux/macOS cross-builds pass.
- Focused web recovery tests pass; web lint and i18n checks pass with all required locales synchronized.
- Public documentation validators and specification lint pass.
- Web typecheck reaches two unchanged duplicate-property errors in `lib/types/http.ts` and `lib/ws/handlers/workflows.ts`.
- PostgreSQL, Docker, SSH, Kind, retained-executor replacement, full browser matrix, and live harness compatibility evidence remain environment-dependent.

The earlier design-package checks also passed: spec lint, 36 linter tests,
package links, frontmatter references, dependency order, and whitespace checks.

## Risks

- Harness versions can change native CWD behavior. Capability fixtures require version-specific maintenance.
- Context continuation loses private harness state and can include historical paths that no longer exist.
- An external tool can run before agentctl records an event. Uncertainty is unavoidable across that boundary.
- Storage compaction, quota exhaustion, and executor cleanup can undermine durability unless fault tests cover them.
- Workflow effects require durable deduplication beyond message insertion.
- Mixed versions require retained-v1 reconciliation before supported rollback permits legacy work.
- No feature flag means no instant operator disable. Compatible-peer bugs require a corrected release or a supported binary rollback.
- Autonomous recovery must not confuse a session block with agent pause, provider backoff, or budget cancellation.
- A PostgreSQL, Docker, SSH, or Kind skip cannot count as executor or persistence acceptance.

## Handoff

The implementation branch is ready for commit and PR review. The remaining
environment-dependent checks are recorded above.
Automatic context continuation and release publication remain outside this
package.
