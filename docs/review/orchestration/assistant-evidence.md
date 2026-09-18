# Assistant acceptance evidence

Task 11 is complete. This inventory maps all 39 current acceptance criteria
to executable checks. It does not turn a deterministic test into a provider,
deployment or production-data qualification claim.

The implementation starts from exact v0.94.0
(`bf819a0228e742d069c528293d848c985a4d1bd1`); assistant tasks 01–10 are in private
commit `41de43a14ae6d08a65d4b44167f4779f5cd2306d`. Task 11 adds fixture-isolation
repairs, combined browser evidence, provider integration regressions and media.
The source fixes and tests are in clean private commit
`c357f9ab94d3790a471d9db587c0a063c7f8d838`. The final candidate receipt will
identify the qualified source and binary hashes.

## Criterion matrix

Every criterion below has the prefix `AC-ORCHESTRATION-ASSISTANT-` from the
[requirements](../../specs/orchestration/requirements/personal-assistant.md).
`runtime/` means [orchestration runtime tests](../../../apps/backend/internal/orchestration/runtime/),
`backendapp/` means [native adapter tests](../../../apps/backend/internal/backendapp/),
and browser specs live in [the orchestration suite](../../../apps/web/e2e/tests/orchestration/).
These are durable test references, not a claim that all scenarios call a real model.

| Criterion | Executable evidence | What it establishes |
| --- | --- | --- |
| 001.1 | `runtime/assistant_privacy_test.go`, `assistant_authority_test.go`, `assistant_intake_privacy_test.go` | Binding switch/removal retains ownership; foreign claims and stale launches fail. |
| 001.2 | `runtime/intake_test.go`, `intake_recovery_test.go`; `personal-assistant-attention.spec.ts` | Accepted intake survives unavailable queues/restart; duplicate identity is not a second effect; a lost response converges. |
| 001.3 | `TestAssistantIntentExactSourceSurvivesNewerComments`, `TestAssistantIntentNewMessageRevokesOldRunWrites` | Exact triggering comment survives bounded history; newer intent revokes old mutations. |
| 001.4 | `runtime/objectives_test.go`, `backendapp/adapters_workspace_tasks_test.go`; `coordinator-view.spec.ts` | Inspect/answer creates no delivery card; native workflow selection is checked; review/turn completion needs current acceptance evidence. |
| 002.1 | `runtime/assistant_memory_contract_test.go`, `context_memory_test.go`; desktop/mobile assistant specs | Owner/scope pagination, correction, provenance, expiry and forget behavior. |
| 002.2 | `TestAssistantContextConfirmedPreferenceSurvivesActivity`, `TestAssistantContextScopesBudgetAndMandatoryOverflow` | Required context survives unrelated activity; mandatory overflow is explicit. |
| 002.3 | `runtime/credentials_test.go`, `credential_validation_test.go`; `personal-assistant-capabilities.spec.ts` | Two eligible workers receive descriptors only; canary secrets stay absent and unavailable accounts show their specific unblock action. |
| 002.4 | `runtime/context_dispatch_test.go`, `backendapp/assistant_context_test.go` | Forget/narrowing invalidates queued and resumed packets at native dispatch. |
| 003.1 | `runtime/capabilities_test.go`, `backendapp/assistant_capabilities_test.go`; capability browser spec | Scoped health/revision directory and native pagination, including browser page two. |
| 003.2 | `TestAssistantCapabilitiesScopeAndRedaction`, `TestAssistantCapabilitiesSchemaSizeAndSecretProjection`; capability browser spec | Secret configuration is not projected; advertised capability does not grant invocation authority. |
| 003.3 | MCP profile/plugintools and plugin manifest conversation-surface tests; capability native adapter tests | Task-only declarations stay task-only; opted-in tools retain individual native dispatch identities. |
| 003.4 | `TestAssistantCapabilitiesRevocation`, `TestAssistantCapabilitiesNativePaginationAndRevocation`, `TestAssistantCapabilitiesSessionAttachmentRevocation` | Cached directory results do not survive live deauthorization at invocation. |
| 004.1 | `runtime/authority_test.go`; agentctl config/ACP/API assistant-policy tests; `backendapp/assistant_authority_test.go` | Unknown effects and unsupported providers fail closed; session new/load/reset attach only the managed broker. Real-provider qualification remains separate below. |
| 004.2 | `TestAssistantAuthorityRevocationAtDispatch`, `TestAssistantWorkspaceNativeEffectGuard`, `TestAssistantPreparedSessionPersistsRestriction` | Current owner/profile/intent/grant is checked at the native effect; the persisted session restriction survives preparation. |
| 004.3 | `TestAssistantReadOnlyDefaultsAndMutationCounters`, `TestAssistantReadOnlyEffectMatrix` | Inspect allows internal receipts but no delivery, repository, credential or unknown external writes. |
| 004.4 | `runtime/operations_test.go` | Restart, concurrent calls and lost acknowledgement preserve one attempt/unknown receipt instead of replaying effects. |
| 005.1 | `runtime/attention_test.go`, `backendapp/assistant_attention_test.go`; attention browser spec | All relevant sessions are inspected, including older pending questions; idle is not fabricated as input. |
| 005.2 | `TestAssistantAttentionRestartDedup`; attention browser spec | Restart and duplicate source events converge without duplicate notification/action. |
| 005.3 | `TestAssistantAttentionLatencyAndNoop` | Controlled-clock event/reconciliation budgets and no model wake for unchanged state; this does not measure model latency. |
| 005.4 | `TestAssistantAttentionPausedAndExpired`, `TestAssistantAttentionExpiredQueueDoesNotLaunch`; permission browser spec | Paused/rate-limited/expired attention stays visible without repeated dispatch or account fallback. |
| 006.1 | `backendapp/assistant_input_test.go`; assistant and permission browser specs | Central human answers use exact native request/session/options and converge with task state. |
| 006.2 | `runtime/known_answer_test.go` | Only explicitly delegable questions use confirmed scoped evidence; permissions cannot be approved by runtime identity. |
| 006.3 | `runtime/attention_resolution_test.go`; attention browser spec | Stale/wrong-session/expired answers fail; duplicate or unknown delivery does not replay a turn. |
| 006.4 | `TestAssistantInputPauseAndStop`, `TestAssistantInputStopReportsUnknown`, `TestAssistantInputStopRechecksBindingPerSession`; permission browser spec | Pause and per-worker stop differ; paused assistant still permits authorized human native resolution. |
| 007.1 | `personal-assistant.spec.ts`, `mobile-personal-assistant.spec.ts`; assistant API/hook tests | Navigation selects the existing private conversation without creating a new workspace/workflow/card. |
| 007.2 | Assistant, maintenance and workspace-link desktop/mobile specs; assistant panel tests | Objectives, evidence, input, capability, memory provenance and maintenance receipts are inspectable centrally. |
| 007.3 | `use-assistant.test.ts`, `use-assistant-actions.test.ts`, `use-assistant-input.test.ts`; attention browser spec | Stale owner/binding responses are discarded; request identities, pagination and delivery state survive retries. |
| 007.4 | All four `mobile-*.spec.ts`; assistant page/action tests | Essential controls at 390px, labels, one-column navigation, draft/focus retention and distinct unavailable/paused states. |
| 008.1 | `runtime/friction_test.go`, `backendapp/assistant_friction_test.go` | Typed native incidents meet the three-events/two-tasks/seven-days threshold without retroactively regrouping old identities. |
| 008.2 | `runtime/maintenance_test.go`, `maintenance_patch_test.go`, `maintenance_recovery_test.go`; maintenance browser specs | No grant means proposal only; a current scoped grant permits isolated preparation/checks/local commit. |
| 008.3 | `TestAssistantMaintenanceBoundaryCannotUseGeneralDelegation`, `TestAssistantMaintenanceBoundaryNativeLaunch`, `TestAssistantImprovementPreparedIsNotResolved` | The repair boundary cannot grant itself general delegation/publication; prepared is not resolved and classifier internals remain unknown. |
| 009.1 | `runtime/workspace_grants_test.go`, `workspace_broker_test.go`; workspace-link browser specs | Home-only default, explicit owner link and receiving-profile consent; no installation-wide automatic adoption. |
| 009.2 | `runtime/workspace_dispatch_test.go`, `workspace_attention_test.go`, `backendapp/assistant_workspaces_test.go` | Target/grant/profile changes are rechecked at reads, queued wakes and actual native effects. |
| 009.3 | `TestAssistantWorkspaceBrokerIsolation`, `TestAssistantWorkspaceNativeProjection`; workspace-link specs | Old coordinator credentials remain workspace-scoped; export identifies the receiving profile. |
| 009.4 | `runtime/workspace_history_test.go`; workspace-link specs | Receiver changes need reconsent; revoke and stored-copy forget are distinct and cannot recall already delivered text. |
| 010.1 | `backendapp/assistant_feature_gate_test.go`, runtime flag/profile tests | Four flag combinations; default-off assistant does not disable ordinary coordinator behavior. |
| 010.2 | `runtime/assistant_feature_gate_test.go`; assistant-off browser route checks | Direct APIs, queued work and new runtime tools stay blocked with the flag off. |
| 010.3 | Retained-owner privacy tests, required-store/conformance tests | Disabling/unregistering preserves schema/history/privacy; no legacy fallback exposes owned conversations. |
| 010.4 | Feature-settings registry/API tests; `TestAssistantReadOnlySupportedMatrix` | Effective source/restart metadata uses existing settings; toggles do not make an unsupported provider eligible. |

## Combined browser runs

The ordered runner copies specs to unique, sorted, temporary filenames and runs
both cycles in one managed Playwright worker. It records original source hashes,
disables retries and removes only its own copies. `--repeat-each` is insufficient
for this claim because Playwright replaces the worker across repeat indexes.

```bash
python3 apps/web/e2e/scripts/orchestration-evidence.py --project chromium --order forward --cycles 2
python3 apps/web/e2e/scripts/orchestration-evidence.py --project chromium --order reverse --cycles 2 --no-build
python3 apps/web/e2e/scripts/orchestration-evidence.py --project mobile-chrome --order forward --cycles 2
python3 apps/web/e2e/scripts/orchestration-evidence.py --project mobile-chrome --order reverse --cycles 2 --no-build
```

The completed matrix passed 18/18 desktop checks in each order and 8/8
phone checks in each order, without retries. It exercises coordinator chat,
Automations, central task view, assistant, maintenance and workspace links.
It includes the provider-boundary fixes and media hooks. The subsequent
attention-event terminal-status correction is separately verified against the
native lifecycle and affected coordinator/Automation browser flows.

Real worker question/permission fixtures use the native mock protocol. They do
not pretend the unsupported mock profile is an eligible restricted central
assistant. The capability fixture creates 51 generic profiles, projects no
canary configuration and verifies the explicit unavailable-account action.
Reset preserves ordinary execution profiles, shared role references and other
workspace data, while deleting fixture-owned coordinator/assistant state.

## Provider and deployment boundary

An isolated trial uses a fresh application home/database/workspace and generic
requests. Native provider authentication is consumed through its normal reader;
no credential value is copied into prompts, fixtures, receipts or Git. The trial
captures ordinary-task/repository/workflow counts and digests before/after, native
tool receipts, message retry identity and teardown. It never opens live history.

The first trials exposed integration gaps missed by narrower adapter tests:
provider database UUID versus canonical provider name, discarded session metadata,
and generic MCP attachment during session creation/load/reset. Regression tests
now exercise each actual boundary. The pinned ACP/SDK launch option correction
is recorded in the [broker ADR](../../decisions/2026-09-17-restricted-assistant-broker.md).

The four-turn real trial passed on Linux x86-64 with managed Claude ACP 0.75.1
and its pinned SDK. Native evidence records one successful `workspace_tasks`
call and one failed `create_task` call with `assistant_effect_denied`; the model's
explanation is not the source of the denial claim. The direct-answer request
created no card. No built-in Write/Bash call occurred and the requested synthetic
file remained absent. All ordinary task/repository/workflow counts and digests
matched their baseline. Four duplicate message submissions returned their
original comment identities. The fixture backend exited normally, no owned
provider processes remained, and the temporary credential-reader link was removed.
No real vault, external service mutation, remote executor or alternative provider
was tested. This establishes the narrow enforced tool path, not general model
quality or an operating-system sandbox against a malicious provider binary.

The reusable [trial script](../../../scripts/orchestration/assistant-provider-trial.py)
requires native Claude authentication and a private output directory outside the
checkout. It fails unless native read and denial receipts are observed. Run only
after building the intended backend and agentctl binaries:

```bash
python3 scripts/orchestration/assistant-provider-trial.py --artifacts-dir /tmp/kandev-assistant-trial
```

The trial keeps raw generic transcript/debug data local. Only reviewed aggregate
receipts belong in Git. The executed prototype trial is not an immutable bundle
qualification; delivery 02 records the final clean source and artifact hashes.

## Affected native checks

The final affected filter passed **168 top-level tests with the race detector**
and no selected test skips. Packages containing no tests are not counted.

```bash
cd apps/backend
go test -race -tags fts5 -count=1 -json ./internal/orchestration/... ./internal/backendapp ./internal/agentctl/server/api ./internal/agentctl/server/config ./internal/agentctl/server/acp ./internal/agentctl/server/adapter/transport/acp ./cmd/agentctl -run 'TestAssistant|Test.*E2E.*Reset|TestE2EReset|TestHandleWS(New|Load|Reset)Session|TestInjectKandev|TestConversationRunsWithoutOffice|TestStreamingReplyIsBridgedOncePerTurn'
```

Typecheck, scoped ESLint and Go lint against the v0.94.0 comparison base passed.
New provider regressions were first observed failing at their real boundary,
then passed after each fix. The browser's missing account unblock text was
observed before rendering the existing native `unblock_action` field.
`TestAssistantAttentionEventsDoNotFinishConversationRun` additionally proves that
ten attention-only event types leave an active run claimed, allowing the actual
terminal failure to retain its error instead of being mislabeled finished.

Candidate qualification, private migration replay/rollback and live cutover have
separate [delivery work orders](../../plans/orchestration-delivery/plan.md).
Screenshots/video use fictional mock fixtures only; copied live data and real
provider logs are excluded from media.

## Inspected media

The [coordinator](media/coordinator-view/README.md) and
[assistant](media/assistant/README.md) packets contain eight desktop/phone
screenshots and two silent clips from clean source `c357f9ab9`. All twelve source
video frames were visually reviewed. Capture and artifact receipts identify the
exact test, frame and output hashes. The eight desktop/phone capture tests pass
without retries. The mock assistant's unsupported-profile banner is retained;
the real-provider execution claim comes only from the separate native trial.
