# Ownership checkpoint: default selection must not publish private history

Status: repaired and verified on 2026-09-16, after explicit authorization for the privacy repair. The accepted boundary is [ADR-2026-09-16-private-conversation-ownership](../../decisions/2026-09-16-private-conversation-ownership.md).

## Reproduced defect

The earlier development implementation stored the private owner only on the currently selected assistant binding. `ConversationUserOwner` queried that binding; `privateConversationAllowed` treated an absent binding as a legacy shared conversation. Selecting another orchestrator updated the binding's conversation ID, making the previous conversation readable by another authorized workspace user.

The regression test `TestAssistantBindingSwitchKeepsPreviousConversationPrivate` in `apps/backend/internal/orchestration/runtime/assistant_privacy_test.go` uses disposable SQLite, synthetic identities and `PRIVATE_HISTORY_CANARY`. Before the repair, a foreign user received 404 before the switch and **200 with the canary afterward**. The owner switch itself returned 200. No production or development-instance database was queried or changed for this reproduction.

Run the regression from `apps/backend` (it now passes):

```sh
GIN_MODE=release go test -tags fts5 -count=1 ./internal/orchestration/runtime -run TestAssistantBindingSwitchKeepsPreviousConversationPrivate -v
```

The original failing assertion is retained, not skipped or weakened. The broader assistant feature is still not ready to deploy.

## Implemented ownership boundary

The additive `orchestration_conversations.owner_user_id` column now retains ownership independently of the default binding. Selection claims that owner in the same transaction as binding CAS; failed/conflicting selections roll back the claim. Migration backfills current bindings and preserves existing owners on replay and legacy import. Switching/deleting a default or unregistering a persona cannot publish its history.

History stays available to its owner, subject to workspace access. Private inactive conversations require reselection before new turns. Runtime reads (including memory) and launch recheck binding ID/version; native task/session authorization remains wired with the feature disabled. Configuration/reimport and legacy Office access preserve the same boundary. Background automations cannot target private assistants without durable owner authorization, which is not implemented yet.

Intake rechecks ownership/current selection inside its transaction. Selection marks accepted instructions that lost authority as superseded; queue repair cannot grant a former shared-chat sender the new owner's authority. No comments or history are transferred to another account or conversation.

Alternatives considered:

- Retain ownership on the existing conversation registry (selected).
- Retain historical binding records with a separate current-default pointer.
- Disallow changing the default once selected. This avoids a migration but removes intended functionality and is not recommended.

Do not infer that checking only the human history endpoint fixes every runtime/memory path. The existing intent snapshot does revoke a previously bound run after a switch; retain that protection.

The previous escalation checkpoint was honored; the user's subsequent repair request authorized implementation. No model switch is claimed. The record skill captured the cross-system ownership rationale; fix/TDD drove the failing-then-passing tests; docs-maintainer updated the experimental public reference without claiming overall readiness.

## Verification after repair

The original regression failed again before production edits, with 200 instead of 404 and the synthetic private-history canary. Additional red tests demonstrated ownership loss on removal, stale runtime memory reads, stale queued launch, private automation/configuration access, native-session bypass, legacy Office fallback and foreign pending intake inheriting new authority. All are now passing.

From `apps/backend`, using the repository Go 1.26 toolchain:

```sh
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp ./internal/task/service -run 'TestAssistant(Binding|Privacy)|TestConversationRunsWithoutOffice|TestScheduledDeliveryReusesConversationAndSelectedAccount|TestOrchestratorsUseProfilesAndScopeConfiguration|TestImportPreservesAssistantIdentityAndRejectsOtherWorkspaces' -v
GIN_MODE=release go test -tags fts5 -count=1 ./internal/orchestration/repository/sqlite ./internal/orchestration/runtime -run 'TestAssistant(Intake|Binding|Operation|Intent)|TestRuntimeAPIScopesWritesAndRevokesFinishedRuns|TestRestartDoesNotReplayAnInterruptedConversation' -v
GIN_MODE=release go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistant(Memory|Context|Credential)|TestEachTurnUsesCurrentGlobalRoleAndWorkspaceContext' -v
GIN_MODE=release go test -race -tags fts5 -count=1 ./internal/orchestration/repository/sqlite ./internal/task/service ./internal/task/share -run 'TestRegistryWithoutOfficePreservesExistingRows|TestGlobalRoleMigrationPreservesDistinctAssignments|TestWorkspaceScoping|TestRedactor'
```

Results: **23**, **27** and **10** top-level tests respectively in the first three commands; the fourth passed migration, workspace-scoping and existing redaction tests. Empty package selections were not counted as tests. The unchanged redactor moved to `internal/common/redaction`, preserving the sharing API and removing the test import cycle exposed by reusing sharing backends from the earlier memory repository work.

## Implementation state to preserve

Task 01 is done again after the ownership repair. Task 02's routing/objective checks passed. Task 03 has implemented scoped memory, descriptor-only credential knowledge, bounded context packets, idle-task refresh, durable per-message context references and native dispatch guards; it remains in progress.

Before adding the failing privacy regression, the task-03 exact filter selected **10 tests** (8 runtime, including one existing compatibility test, and 2 backend adapter tests), all passing. Full orchestration and agentctl race tests, full messagequeue/executor race tests, and scoped orchestrator/backendapp queue/routing race tests also passed. See task 03 for exact commands.

Remaining task-03 work includes completing its API/metadata contract details, coverage and complexity checks. Ownership is reconciled and its task checks passed again. Tasks 04–11, the central UI, enforced provider/tool read-only execution, native attention/input handling and live-provider trials have not been implemented in this turn. Public API/CLI reference documentation is explicitly marked in development. The repair cannot reconstruct ownership already lost before migration or recall text previously delivered to a provider.

Public docs updated: `docs/public/personal-assistant-api.md` (reference, experimental and explicitly not production-ready), `orchestration-personas.md` (how-to), navigation and coverage. Documentation checks passed: **61 validator unit tests** and **43 published pages**. Two stale documentation references were corrected: the orchestration page extension and the browser test's moved directory.

No commits, pushes, service restarts, deployments, real vault reads or external-provider experiments occurred. Preserve the large pre-existing dirty worktree; do not stage all changes.
