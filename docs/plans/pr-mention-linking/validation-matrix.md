# PR registration validation matrix

Status: planned product evidence. No permanent tests or production changes exist from this design turn.
All IDs below use prefix `AC-INTEGRATIONS-CHANGE-REGISTRATION-`.
Test paths below `internal/` are relative to `apps/backend`; browser paths are relative to `apps/web`.
New test names are proposed targets, not claims about existing tests.

## Backend and frontend evidence

| Scenario | AC suffixes | Planned test file and case |
| --- | --- | --- |
| Selected GitHub/GitLab reference versus pasted URL/plain #123 | 001.1, 001.6, 003.1 | New `internal/task/handlers/message_registration_test.go`: `TestSelectedReferenceOnly`; new `components/task/chat/reference-registration.test.tsx`: selected versus literal |
| Context-only, missing intent, recalled/historical reference | 001.6, 006.4 | `internal/entityrefs/reference_test.go` (existing), new `reference-registration.test.tsx`: context-only round trip |
| First message, description snapshot, title owner, competing first sends | 001.2, 005.3 | Existing `internal/task/handlers/message_handlers_blocked_test.go` plus new `message_registration_test.go`: `TestInitialBriefRegistration` |
| Direct message, steering, provider dispatch failure after acceptance | 001.2, 006.1 | New `message_registration_test.go`: `TestDirectRegistrationAfterCommit` |
| Cancelled draft, rejected admission, cancelled/expired editor | 001.3, 005.3 | New `internal/orchestrator/handlers/queue_registration_test.go`: `TestUncommittedEditNoRegistration`; frontend draft tests |
| Queue insertion, auto-fold, manual merge, dispatch, Send now | 001.2, 004.1 | New `internal/orchestrator/messagequeue/registration_test.go`: `TestRegistrationOriginSurvivesFold`; handlers and orchestrator tests |
| Queue Save adds/removes/toggles references; unchanged reference | 001.4, 001.5 | New `queue_registration_test.go`: `TestSavedEditIntentDelta`; new `hooks/domains/session/queue-registration.test.ts` |
| Removal/clear/cancel after acceptance; retained pending intent | 001.5, 005.3 | New `registration_test.go`: `TestRemovalPreservesAcceptedIntent`; cancellation race fixture |
| Multiple PRs and all stack layers; repeated active link | 003.1, 004.1 | Existing `internal/backendapp/task_change_link_coordinator_test.go`; new `task_change_registration_test.go`: `TestRegistrationPreservesSiblings` |
| Same number across repositories/hosts; fork target versus head | 002.1, 002.2 | New `internal/backendapp/task_change_registration_test.go`: `TestRegistrationURLIdentityMatrix`; existing identity tests |
| Rename with stable provider ID; reused path with different ID; credential rotation with stable scope | 002.1, 004.1, 004.2 | Provider `store_registration_test.go` and coordinator `task_change_registration_test.go`: `TestRegistrationImmutableIdentity`; SQLite/PostgreSQL variants |
| Missing legacy immutable ID; conflicting current path; exact association removal | 002.1, 002.4, 004.2 | Provider `store_registration_test.go`: `TestRegistrationLegacyIdentityRepair`; assert no startup HTTP request or path-only backfill |
| Selection ID round trip, reference binding, recall, replay after unlink | 001.3, 001.6, 004.1-004.3 | New `message_registration_test.go`: `TestRegistrationSelectionIdentity`; frontend `reference-registration.test.tsx` and serialization tests |
| Unchanged queued reference versus remove-and-reselect with identical final text | 001.4, 004.2-004.3 | New `messagequeue/registration_test.go`: `TestRegistrationQueueReselection`; SQLite/PostgreSQL, Cancel and second-unlink replay variants |
| Foreign/unattached/ambiguous/detached repositories; no first-repo fallback | 002.2 | `TestRegistrationURLIdentityMatrix`; assertion that repository/checkout mutation dependencies receive no calls |
| Azure/unsupported provider or scope | 002.3, 006.3 | New coordinator capability tests; frontend unsupported Details/Link state |
| Wrong workspace, forged host/ref, caller fields, origin spoof | 002.4, 003.4 | Existing `internal/mcp/handlers/task_change_link_test.go`; new `TestRegistrationAuthorization` table in coordinator tests |
| Missing credentials, revoked credentials, provider read failure/mismatch | 002.3, 002.4, 003.3 | New `internal/github/service_registration_test.go` and `internal/gitlab/service_registration_test.go`: `TestRegistrationReadFailureNoAssociation` |
| URL omitted/current task, explicit legacy target, conflicts, nulls | 003.2, 003.4 | Existing MCP server/handler tests plus new `TestRegistrationURLSchema`; both protocol-era schemas |
| gh/API creation URL; active review/fix/update; background exclusion | 003.1, 003.3 | New `internal/sysprompt/change_registration_test.go`: `TestRegistrationGuidance`; controlled fake-agent tool trace |
| Tool unavailable/restricted mode; launch/resume/later-turn injection | 003.1, 005.3 | Same guidance test, server tool catalog tests; assert no unavailable tool instruction |
| Concurrent sessions/retry/reconnect, identity collision | 004.1, 006.2 | New provider `store_registration_test.go`: `TestConcurrentRegistration`; SQLite and PostgreSQL variants |
| Crash before acceptance, after acceptance, after provider read, after association commit | 001.2, 004.1, 006.1 | New task repository `registration_intents_test.go`: `TestRegistrationCrashRecovery`; transaction fault injection |
| Unlink while read in flight; replay after unlink; watcher rediscovery | 004.2 | Provider store tests: `TestRegistrationUnlinkWinsStale`; orchestrator integration verifies no restored row/event |
| New intentional selection/relink after unlink; replay of relink after second unlink | 004.3 | `TestRegistrationIntentionalRelinkRevision`; browser relink flow; ordinary Retry retains old revision |
| Message/session deletion and nullable provenance; task purge | 004.4, 001.5 | Repository retention tests: `TestRegistrationOriginDeletion`; frontend UI-05 |
| All comparison effects on link/sync/rediscovery/restoration | 005.2 | Existing `internal/task/service/service_comparison_target_test.go` plus provider registration tests: `TestRegistrationObservationPreservesComparison`; fake pusher/base-reset assertions |
| No wake/fix/merge/workflow effects, existing automation preserved | 005.1, 005.3 | New `internal/orchestrator/change_registration_test.go`: `TestRegistrationObservationHasNoExecutionEffects`; seed task defaults enabled and per-association authority disabled |
| Result states, partial success, retry does not resend | 006.1, 006.3 | New `reference-registration.test.tsx`; backend result projection tests |
| Delayed update after deletion; bootstrap/reconnect/workspace switch | 006.2 | Existing `lib/ws/handlers/github.test.ts`, `gitlab.test.ts`, provider store tests: revision ordering |
| Keyboard/reader names, polite announcements, translations/phone | 006.4 | New chip tests and browser UI-01 through UI-05, pseudo-locale and geometry assertions |

All newly proposed files must exist before their work-order validation can count as complete.

## Rendered flows

`e2e/tests/chat/pr-mention-linking.spec.ts` (new, `chromium`) covers:

1. Initial/direct selection -> commit -> Linking -> Linked -> task list; AC 001.1-001.3, 006.1-006.2.
2. Context-only and plain URL; agent tool fixture links relevant plain-text work; AC 001.6, 003.1-003.3.
3. Mixed providers, unsupported Azure, foreign repo and credential failure -> recover without resending; AC 002.1-002.4, 006.3.
4. Two PRs -> unlink one -> reload/replay -> Link again; AC 004.1-004.3, 006.2.
5. Origin message deletion -> unavailable origin -> live task association; AC 004.4.
6. Queue insert/fold/edit Cancel/Save/remove/clear/Send now with question and lease barriers; AC 001.2-001.5, 005.3.
7. Link and status updates preserve manual comparison and automation settings; AC 005.1-005.2.
8. Queue reference -> unlink -> unchanged Save stays Unlinked; remove/reselect -> Save relinks; Cancel and replay do not; AC 001.4, 004.2-004.3.

`e2e/tests/chat/mobile-pr-mention-linking.spec.ts` (new, `mobile-chrome`) covers the same core values through UI-03/UI-04/UI-05.
It verifies touch targets, safe-area/viewport containment, internal drawer scroll, long translated names, and preserved draft across breakpoint changes.
Add keyboard-only desktop coverage and a narrow fine-pointer viewport case. Do not treat a tooltip opened by tap as a phone drawer.
Use causal event waits. For no-side-effect assertions, inspect the persisted settings and instrumented runtime calls after processing completes.

## Migration evidence

Provider store tests must cover fresh database, unambiguous legacy backfill, unresolved legacy rows, collision diagnostics, and restart.
Run both SQLite and the repository's PostgreSQL test mode. Assert tombstone retention and canonical host identity after repository metadata changes.
A restored row uses observation policy and disabled automation; an already-active row keeps its existing policy/options.
Credential rotation retains the built-in workspace scope. Repository rename retains the immutable key.
A different repository at the old path never inherits associations or tombstones. Unresolved legacy identity blocks automatic writes until verified repair.

## Plugin conformance extension

Added for the user-requested plugin scope. All cases use a disposable managed plugin fixture; production provider APIs remain mocked.
The existing matrix still applies to compatible plugin providers through the same initial/direct/queue user flows.
All new backend case names must include `Registration` so Task 04's scoped command runs them.

| Scenario | AC suffixes | Planned test file and case |
| --- | --- | --- |
| Active plugin source joins autocomplete; repository-only plugin does not supply results | 007.1 | Existing `internal/plugins/mentions_test.go`: `TestRegistrationSourceCapability`; browser plugin picker flow |
| Search-only/older plugin versus complete version-1 capability | 007.2, 007.8 | New `internal/plugins/change_registration_test.go`: `TestRegistrationCapabilityMatrix`; new `lib/plugins/change-request-registration.test.ts` |
| Initial/direct/queue insertion and saved edits; context-only/cancel preserve behavior | 007.2, 001.1-001.6 | New handler `TestPluginRegistrationAdmissionMatrix`; queue and browser fixtures |
| Plugin-only task exposes shared tools/guidance in both MCP protocol eras; restricted modes remain restricted | 007.3, 003.1-003.4 | New server/coordinator `TestPluginRegistrationCatalog`; shared sysprompt test |
| URL with no browser; full provider tuple; numeric/string ID conflicts | 007.3, 007.4 | New `internal/backendapp/task_change_registration_plugins_test.go`: `TestPluginRegistrationHeadlessURL` |
| Bitbucket Cloud/Data Center and Forgejo hosts; equal numbers across repositories/context roots/instances | 007.4, 002.1 | `TestPluginRegistrationIdentityMatrix`; include two Data Center paths on one authority |
| Two matching owners; source/provider alias mismatch; forged scope; foreign repository/workspace | 007.4, 002.2, 002.4 | `TestPluginRegistrationResolutionRejectsAmbiguity`; assert no import/checkout/association call |
| Response lost after plugin commit; readback failure; exact replay after restart | 007.5, 004.1 | New fixture `change_registration_test.go`: `TestPluginRegistrationLostReply`; host receipt tests with fault injection |
| Resolver unavailable at admission; accepted message survives; null-fence recovery after restart | 007.5, 006.1, 006.3 | New handler `TestPluginRegistrationResolutionUnavailable`; absent row creates, active row succeeds idempotently, tombstone suppresses |
| Unlink before resolve, between resolve/admit, and between admit/mutate | 007.5, 004.2-004.3 | Coordinator/fixture `TestPluginRegistrationAdmissionFence`; barrier-controlled schedules, exact operation receipt and immutable fence assertions |
| Unknown mutation outcome versus failed resolution; Retry versus fresh Link again | 007.5, 006.1, 006.3-006.4 | `reference-registration.test.tsx`, `pr-registration.spec.ts`, `mobile-pr-registration.spec.ts`; no message resend, no optimistic count |
| Concurrent link/unlink, branch rediscovery, new selected relink, replay after second unlink | 007.5, 004.2-004.3 | Fixture `TestPluginRegistrationTombstoneRace`; same fixture's transactional store restart tests |
| Stack layers/sibling links; same-provider replacement and failed compensation | 007.5, 004.1 | Coordinator `TestPluginRegistrationPreservesSiblings`; `TestPluginRegistrationReplaceFailure` |
| Existing authority preserved; no new wake/fix/merge/comparison effects | 007.5, 005.1-005.3 | Fixture/coordinator `TestPluginRegistrationObservationOnly`; instrument host runtime and plugin mutation dependencies |
| Disable during search, before submit, after admission, and after remote commit | 007.6, 002.4 | `TestPluginRegistrationDisableStages`; pre-admission authorization rejects unsafe refs, post-admission result stays unavailable/uncertain |
| Upgrade during dispatch; changed owner/connection; revoked access | 007.6 | `TestPluginRegistrationGenerationFence`; stale results cannot mutate host projection or dispatch to a replacement owner |
| Uninstall/reinstall; unavailable list/unlink | 007.6 | `TestPluginRegistrationInstallationIdentity`; no automatic replay into new installation, no false empty/success |
| Plugin manual link/unlink and create coexist with neutral tool path | 007.8, 004.2 | Fixture conformance `TestPluginRegistrationManualParity`; creation reply failure never retries remote creation to fix a link |
| Committed snapshot count/list/provenance, origin deletion, reconnect, workspace switch | 007.7, 006.1-006.3, 004.4 | Browser SDK/store tests and `pr-registration.spec.ts`; exact invalidation and readback assertions |
| Plugin desktop dropdown/phone drawer, keyboard, reader labels, translated long provider names | 007.7, 006.4 | `pr-registration.spec.ts` and `mobile-pr-registration.spec.ts`, projects chromium/mobile-chrome |
| Invalid/incomplete declaration, old/new host compatibility, malformed/oversized action result | 007.8 | Manifest/Go SDK tests `TestRegistrationContractCompatibility`; existing action-limit/generation checks |

The new browser files live in `apps/web/e2e/tests/plugins/` and implement UI-06 plus the shared UI-03/05 patterns.
Plugin list failure tests must assert partial/incomplete provider read results, not just an error toast.
Host SQLite/PostgreSQL tests cover intent/receipt persistence; plugin conformance separately proves its own atomic storage and operation receipts.
The external plugin adoption checklist is the capability/action/persistence contract in the [plugin design](../../specs/integrations/system-design/task-change-registration-plugins.md).
