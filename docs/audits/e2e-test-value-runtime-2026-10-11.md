# Runtime-focused E2E cleanup

Status: Changes implemented and focused verification complete; draft publication and Actions measurement requested  
Reviewed: 2026-10-11; last verified: 2026-10-12  
Baseline: `e8c1761cb5b7006b905c0aa995925372ce8664e1`, plus the [first cleanup pass](e2e-test-value-cleanup-2026-10-11.md)

The remaining suite is not certified irreducible. The first pass favored proven assertion equivalence and removed 121 configured instances. This follow-up targets elapsed time, and records measurements rather than treating test-count reduction as a speedup.

## Measured candidates

The [completed CI sample](https://github.com/kdlbs/kandev/actions/runs/38162790010) identifies LSP file intelligence (439 seconds), message queues (362 seconds), clarification (340 seconds), last-prompt scrolling (250 seconds), Quick Chat (246 seconds), Git changes (244 seconds), session tabs (215 seconds) and config-mode MCP (`settings/config-management.spec.ts`, 202 seconds) as substantial Chromium suites. These values sum final test-attempt durations within each file; they exclude fixture setup and earlier attempts and are not workflow elapsed time. A newer run, 38174659644, was still in progress when checked, so its incomplete timings were not substituted for a completed sample.

The normal shards in that sample ran for approximately 27.7 to 36.0 minutes. A 125-second wait can matter to the shard holding that case, but total test-time savings must not be divided or translated into a guaranteed CI reduction without rerunning the pipeline and observing allocation.

| Selection                                                                                 | Baseline                 | First passing follow-up                                             |
| ----------------------------------------------------------------------------------------- | ------------------------ | ------------------------------------------------------------------- |
| LSP idle release, archive shutdown, Quick Chat queue controls/delivery and task clear-all | 7 passed in 3.7 minutes  | 4 passed in 56.9 seconds                                            |
| Session-tab close confirmation/cancellation and active/non-active context-menu deletion   | 4 passed in 53.4 seconds | 3 passed; summed test bodies 36.9 seconds in the final combined run |

The first selection is about 165 seconds faster locally. The original reporter rounds the baseline duration to tenths of a minute, so this is approximate. Both runs use the same host, one worker, existing product artifacts and equivalent behavioral checks. The first passing follow-up replaces the four redundant Quick Chat launches with one. It does not exclude a failed baseline case. No baseline failures occurred in either selection.

## Changes and retained coverage

### LSP browser idle release

`apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`, “releases the retained language server after the last editor stays idle,” now installs the Playwright clock before navigation and advances it 125 seconds after the last editor closes. Its duration fell from approximately 132 seconds to 7.2 seconds in the passing comparison.

The two-minute deadline belongs to `LspClientManager.scheduleIdleCleanup` in `apps/web/lib/lsp/lsp-client-manager.ts`, using `LSP_IDLE_TIMEOUT` from `lsp-client-config.ts`. The existing continuity unit test checks the timer and `editor_idle` control. The E2E case still starts a real task-host language-server process, closes the real editor through its menu, verifies the tab disappears, waits for the original PID to exit and checks a shutdown event for that exact PID. Neither the WebSocket release path nor process shutdown is mocked. Requirement AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.8 remains covered.

Playwright's installed clock advances browser timers. It cannot advance a Go reaper, task runtime or detached backend lease deadline. The separate browser-close/real-reaper continuity test, task archive, restart, capacity, attachment and explicit Stop paths remain. The idle test originated with the browser-continuity feature in PR #3899 on 2026-09-24; this change accelerates its existing proof rather than dropping that feature boundary.

### Quick Chat queue journey

Four cases in `apps/web/e2e/tests/chat/message-queue.spec.ts` share one keeper: “keyboard and button queue submissions preserve panel controls and deliver both messages.” The original cases are:

- “queued message indicator appears and message executes after agent turn” (26.2 seconds locally).
- “queue message via submit button click” (26.7 seconds).
- “queue chip stays collapsed by default and toggles via panel close button” (8.5 seconds).
- “Escape collapses an open queue panel” (8.0 seconds).

Their summed duration was 69.4 seconds; the combined keeper took 29.4 seconds. It keeps both real input paths, the busy/queue-mode precondition, absent Submit before typing, Cancel visibility, chip text, initially collapsed panel, open/close behavior, chip unmount while open, Escape with focus outside the composer, and final idle state. It additionally requires two distinct queued rows and each exact queued user/agent response in FIFO order. Lost queue content can no longer pass by merely rendering idle.

The opening mock prompt is the deterministic existing `/e2e:simple-message` scenario. Queued prompts use existing single-line `e2e:message` commands, so the tested input and actual provider response carry unique markers. This removes random mock-response work unrelated to queue behavior. The real busy predecessor remains; the further deterministic-fixture pass below replaces unrelated randomized tool activity, without a product timing override or new mock seam. An initial attempt used multiline script commands unsupported by the busy typing helper and failed at fixture typing; switching to the existing single-line command fixed that setup error. That failed attempt is not included in the passing comparison.

Task-page submission, plan mode/system-context exclusion, Clear all, queue merge, edit leases, Send Now, Auto-run, cancellation, FIFO and mobile paths stay separate because they reach different hosts, actions or lifecycle conditions. The chip behavior was introduced in PR #1049 on 2026-05-24; it remains asserted through the real queue UI.

### Session-tab cancellation before confirmation

`apps/web/e2e/tests/session/session-tab-management.spec.ts` combines “tab close button shows delete confirmation and removes session on confirm” and “tab close button delete confirmation can be cancelled.” The keeper first cancels and checks that the dialog closes, the target tab remains and the backend still has two sessions. It then reopens the same close-button dialog and confirms deletion, retaining the exact remaining session ID, disappearance of the target tab, sibling tab visibility and absence of misleading deletion toasts.

This avoids one repeated two-agent setup. The separate context-menu active/non-active deletion cases remain: they use the contextual popover rather than the tab-close dialog and protect different focus/recreation conditions. Hide-panel preference, shared-environment Changes continuity, sibling hydration, task switching, primary-session persistence and mobile interaction remain. The Cancel behavior originated in PR #1174 on 2026-05-30.

The two original close-button test bodies summed to 22.8 seconds. Their combined keeper took 10.7 seconds in the final run, saving approximately 12 seconds. Across the three changed groups, summed test-body time falls by about 176 seconds (roughly three minutes): LSP idle release saves about 125 seconds, Quick Chat saves about 39 seconds, and tab cancellation/confirmation saves about 12 seconds. These rounded observations do not measure a CI pipeline speedup. The final combined keeper/neighbor run passed seven tests in 1.6 minutes.

## Deterministic queue busy fixtures

The same queue file also uses `/slow` in ten cases solely to create a busy predecessor. The mock's slow handler adds randomized thinking, Read and search work on top of its requested duration; each atomic delay ranges from 10 to 50 milliseconds for `mock-fast`, and 0.5 to 3 seconds for `mock-slow`. The default seeded mock profile uses `mock-fast`, so large savings from this replacement are not assumed. Those tool events are not asserted by these queue cases. The existing `/sleep` command supplies a real, silent running turn without that unrelated random work. `deriveSessionInputMode` treats a running, non-background, non-steering session as queue mode; each keeper already requires that actual mode before submission.

The remaining six candidates passed before replacement in 1.7 minutes. Earlier baseline evidence covers Quick Chat, task-page/plan submission and Clear all. All ten predecessors now use existing `/sleep` commands. The initial experiment doubled two five-second submission windows to ten seconds. The full seventeen-case run passed in 4.9 minutes, but comparable test bodies showed no improvement; those two windows returned to their original five seconds before publication. The other requested durations remain unchanged. No busy precondition or FIFO, Auto-run, edit-lease, scroll, session-switch, merge or deletion assertion was removed. Explicit predecessor completion checks require the corresponding `Slept for Ns.` response.

Task-page and plan-mode submission also use single-line scripted payloads and require their own user/agent response markers before idle; the plan-mode system-context exclusion check remains. Their former idle-only endings could pass after payload loss. The full queue file passed seventeen cases without retries. Both submission cases passed with their original five-second windows in 46.2 seconds. The corresponding backend payload-substitution mutation made both fail at their exact response assertions. Their summed final bodies were 39.6 seconds versus 37.7 in the earlier baseline, so this fixture/assertion repair supplies no measured speedup and is not included as a runtime saving.

## Final verification

All three runtime-focused fault injections failed at the intended assertions:

| Mutation                                                                      | Keeper failure                                                     |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| Suppress the continuity client's idle lease release                           | The original language-server PID remains alive instead of exiting. |
| Replace an admitted queued keyboard payload during backend prompt composition | The exact expected keyboard response is absent after substitution. |
| Replace task-page and plan-mode queued payloads during backend prompt composition | Both distinct submission keepers fail because their exact responses are absent. |

The mutated production files were restored byte for byte, and corresponding E2E web/backend/mock/delivery/plugin artifacts rebuilt. The queue mutation exercised the real dispatch owner, not just the mock agent. An initial mutation build invocation could not find pnpm from the root shell; it restored the source without running a test. The corrected invocation used the Node-enabled web environment and E2E web build, retaining the pseudo QA locale. Only the subsequent assertion failures count as mutation evidence.

The final restored-source run passed all seven keepers/neighbors in 1.6 minutes. The seven additional distinct sibling checks passed in 3.7 minutes: Send Now interruption, task/plan submissions, shared LSP startup/configuration/Stop, real browser-close/reaper retention, independent-window leases and persisted panel hiding. Full queue-fixture verification passed seventeen cases in 4.9 minutes; the two restored five-second windows also passed their final checks after the payload mutation was restored. ESLint and Prettier pass for all three changed specs. Full discovery reports 3,988 configured instances with zero errors: Chromium 2,730; mobile Chrome 1,059; auth 37; routing 9; containers 152; Kubernetes compatibility 1. This is four fewer instances than the first pass and 125 fewer than the original inventory. The spec tree remains 1,369 files and has 247,147 lines. The [coverage CSV](e2e-test-value-removed-cases.csv) now maps 143 disappeared original titles across both passes; eighteen renamed/new instances account for the difference between that row count and the net reduction of 125.

No permanent production changes remain. Local evidence is retained under ignored `apps/backend/.build/e2e-runtime/`, including before-source copies, baseline/follow-up logs, CI timing input, mutation outcomes and measurement summaries. The user requested a draft PR and Actions timing comparison on 2026-10-12. Publication and CI measurement follow the local verification; the exact-parent baseline is E2E run 38162018356, which completed with seven failed and two retried tests.

## Remaining opportunities

Similar-looking outcomes are not sufficient evidence to remove a boundary. More savings may come from consolidating repeated session setup in config-mode MCP and from replacing random/default mock sequences with deterministic scenarios where the output is only setup. Those require auditing each resulting tool call, persistence assertion, fixture caller and observable host before changing them. Last-prompt tests distinguish loaded/unloaded pagination, viewport anchoring, disabled settings and input controls; they are not all equivalent button smokes.

The first audit's remaining assertion repairs are still relevant. Weak assertions need stronger keepers before deletion. No tests were moved into optional CI or nightly-only execution to manufacture faster required checks.
