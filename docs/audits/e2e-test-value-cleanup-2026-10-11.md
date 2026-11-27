# E2E cleanup implementation and coverage ledger

Reviewed and implemented: 2026-10-11  
Baseline revision: `e8c1761cb5b7006b905c0aa995925372ce8664e1`  
Status: Cleanup implemented and focused verification passed

This implements the proven removals and consolidations from the [initial audit](e2e-test-value-2026-10-11.md), plus adjacent duplicate Office checks found while tracing their keepers. Product behavior is unchanged. This records the first uncommitted checkpoint; the user subsequently requested draft publication and Actions measurement on 2026-10-12.

This ledger preserves the first checkpoint. The [runtime follow-up](e2e-test-value-runtime-2026-10-11.md) records subsequent changes and measured time savings; the current removed-case CSV includes both passes.

The suite has **121 fewer configured Playwright instances**, from 4,113 to 3,992. Eight are obsolete skipped definitions; eight useful harness tests moved to Vitest's Node project; consolidation accounts for a further net reduction of 105 active browser instances. These are discovery counts, including skipped and opt-in cases, rather than counts of executed CI tests or unique behavioral contracts.

| Project                  | Before | After | Reduction |
| ------------------------ | -----: | ----: | --------: |
| Chromium                 |  2,854 | 2,734 |       120 |
| Mobile Chrome            |  1,060 | 1,059 |         1 |
| Auth                     |     37 |    37 |         0 |
| Routing                  |      9 |     9 |         0 |
| Containers               |    152 |   152 |         0 |
| Kubernetes compatibility |      1 |     1 |         0 |
| Total                    |  4,113 | 3,992 |       121 |

Spec files decreased from 1,382 to 1,369: fourteen deleted files and one new status-contract file. The spec tree decreased from 249,083 to 247,259 lines. Both complete discovery runs had zero collection errors.

## Coverage ownership after consolidation

Paths in this table are relative to `apps/web/e2e/tests/`. The first checkpoint mapped 137 original titles that disappeared to their keeper or dead-code rationale, with original line numbers. Sixteen renamed or new instances account for the difference between those 137 rows and the net reduction of 121. The [removed-case CSV](e2e-test-value-removed-cases.csv) now includes six additional rows from the runtime follow-up, with a pass column distinguishing the two batches. A removed title does not necessarily mean its assertions were removed.

| Area                               | Surviving owner and preserved proof                                                                                                                                                                                                                                                                                                                                                                      |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Dashboard and navigation           | `office/dashboard.spec.ts` combines API metrics, activity/task arrays, chart/card/section labels, onboarding URL and stat-card links. Functional Org chart, Preferences, Projects and Activity journeys own their own title checks.                                                                                                                                                                      |
| Inbox                              | `office/inbox.spec.ts` combines the Inbox title, Mine tab, inbox items/count envelope and approvals array. Removed realtime-named tests never performed an event-driven mutation.                                                                                                                                                                                                                        |
| Config Sync                        | `office/config-sync.spec.ts` traverses HTTP export, outgoing filesystem sync, incoming diff and import, checking workspace binding and canonical envelopes. Backend config tests retain detailed fields, import errors, guards, pruning and idempotence; provider reconciliation remains a separate E2E path.                                                                                            |
| Recovery and observation           | `office/recovery.spec.ts` checks the settings schema and persisted 42/48/72/24 values, restoring the original in `finally`. `office/run-observation.spec.ts` checks a seeded finished run's exact ID, agent, status and request timestamp, plus the actual activity label/page. An empty run list can no longer skip the assertions.                                                                     |
| Task status                        | New `office/task-status.spec.ts` retains all four active HTTP status/comment contracts from the retired stages file: in-progress success, invalid rejection, terminal workflow step and recorded comment. Modern quorum/approve/reject/re-entry tests remain unchanged.                                                                                                                                  |
| Governance and agents              | `office/agent-governance.spec.ts` persists all three flags in both directions, exercises admin creation with governance on/off and restores settings. `office/agents.spec.ts` retains CEO list/get/navigation/rename, worker create/list/UI/delete and paused/idle/pending/stopped transitions. Distinct recovery-control and paused-navigation cases remain separate.                                   |
| Roles and permissions              | `office/agent-roles.spec.ts` checks Security, QA and DevOps permissions and scoped list/detail badges. Worker permissions live in the worker lifecycle keeper. Invalid permission JSON fails directly. `office/permissions.spec.ts` retains default/schema checks, both persisted modes and invalid-value rejection, with restoration.                                                                   |
| Tasks and hierarchy                | `office/tasks.spec.ts` combines exact seeded API membership, multiple visible rows, identifier attachment and reload hydration of a new task. Parent/child identity and canonical `parentId` are checked in the expansion/indentation/collapse journey. Positive and zero-result search require the actual array. Both pointer-driven board moves remain and check exact persisted `in_progress` status. |
| Routines, Skills and Costs         | Real routine row/detail persistence absorbs name/description/list/title smoke checks; actual Create/Save, schedule/run and error paths remain. Skill CRUD retains exact slug/identity and deletion absence with cleanup; provider import stays separate. Cost endpoint availability joins actual project reassignment/refetch and the Costs title. Old zero/empty titles never asserted those outcomes.  |
| Onboarding, settings and Org chart | Completed onboarding metadata is combined; wizard/cancel/profile/second-workspace flows stay separate. Workspace rename absorbs Preferences/Danger zone presence. The chart re-parenting journey includes seeded CEO visibility. Project creation/repository persistence includes the Projects title.                                                                                                    |
| Task creation                      | `task/create-task.spec.ts` combines dialog/input/start and launch assertions into one existing journey: URL, panels, sidebar title, chat, idle, Review, Turn Finished, exactly one DONE session and ready task-linked environment. The REST session/environment association case remains separate. Eighteen instances become fifteen.                                                                    |
| Clarification                      | `chat/clarification.spec.ts` combines seven startups into one three-question journey covering structure, progress, Back disabled/restoration, auto-advance, direct navigation, incomplete Submit gating and submission unblocking. Keyboard, context, markdown, races, deferred answers, escape, custom text and failure paths remain separate. Thirty-eight instances become thirty-two.                |
| Git and archive confirmation       | Three-commit refresh retains all subjects and SHAs before/after reload, replacing the redundant single-commit case. Realtime history remains separate. `task/confirmation-text-hierarchy.spec.ts` retains the duplicate archive geometry checks plus animation/text-wrapping checks. Preference, warning and mobile/tablet paths remain.                                                                 |
| Model warnings on desktop/mobile   | Existing `settings/no-silent-model-fallback.spec.ts` and its mobile counterpart absorb exact saved-model labels and catalog-ready checks from PR-specific capture tests. Desktop hover/opacity/layer and mobile tap/drawer transform/layer/overflow assertions are retained in their respective interaction paths. Executor mismatch coverage remains separate.                                          |
| Harness ownership                  | Eight pure ChildProcess/filesystem/cleanup/SSH-owner tests moved unchanged to `apps/web/e2e/helpers/backend-fixture-lifecycle.test.ts`, with an explicit reviewed Node-project entry. The fake-DOM recovery test uses a lightweight Playwright page without starting the product backend; disabled-control and duplicate-click protection remain.                                                        |

## Dead definitions and misleading claims

The eight removed skipped definitions consist of four retired execution-policy transitions, one obsolete default-project onboarding expectation, two unconditional real-provider resume scaffolds, and one empty branch-fetch error scaffold referring to a removed selector. They supplied no executed proof.

Several removed titles overstated their behavior. The approval-decision tests only patched agent status; those transitions now live in the worker lifecycle keeper. The task WS test only checked continued card visibility; that assertion remains without claiming event delivery. The onboarding-task test created an ordinary task after setup; that API/list proof remains without claiming wizard task creation. Filter/sort smoke tests never interacted with filtering or asserted ordering. The ledger preserves their actual assertions and does not count their titles as evidence for the stronger claimed behavior.

## Verification and assertion mutation evidence

The original selected desktop baseline finished with **234 passed, 6 skipped and 1 failed in 13.7 minutes**. The failure was the retained local-rebase keyboard-focus assertion in `git/git-changes-panel.spec.ts`, where a provider collapse toggle was inactive rather than focused. An isolated rerun passed. That same assertion failed again in an intermediate combined keeper run; it was neither removed nor weakened. This is an unresolved baseline stability issue, not a passing full-suite claim.

The final desktop keeper/sibling run passed **61 tests in 2.4 minutes** after the cleanup selector corrections. It covers the changed Office suites, neighboring onboarding/project/breadcrumb/routine/recovery paths, the combined task launch and carousel, Git refresh, archive confirmation, desktop model selection, setup health and fake-DOM recovery. It excludes the known baseline focus case. The CLI uses one worker and retries zero; clarification retains its pre-existing describe-level retry setting. No retries occurred in this final run.

The final mobile model-selection run passed **all three keepers in 13.1 seconds**. The original mobile baseline passed four cases in 15.9 seconds, including the capture case subsequently absorbed into those keepers.

The migrated harness tests and Node-project selection tests passed: **two Vitest files, twelve tests** (eight migrated helper tests plus four selection tests). Web typecheck, ESLint for all twenty-eight changed/new TypeScript files, Prettier for changed/new web files, complete Playwright discovery and `git diff --check` passed. Web typecheck excludes `e2e/`; discovery and focused execution validate those test inputs.

Five reversible production mutations each failed at the intended keeper assertion:

| Mutation                                     | Detecting assertion                               |
| -------------------------------------------- | ------------------------------------------------- |
| Export the wrong workspace name              | Exact seeded workspace binding                    |
| Rename the inbox items field                 | Actual inbox items array required                 |
| Return no runs despite a seeded finished run | Exact run identity and metadata required          |
| Return `{}` for an empty task search         | Actual zero-result tasks array required           |
| Change the Security role badge to Worker     | Scoped Security badge on the created agent's card |

All mutated source files were restored byte for byte, and backend/mock/delivery/plugin artifacts were rebuilt before final verification. No production source diff remains. Detailed discovery, baseline, keeper, mutation and formatter logs are in the ignored local directory `apps/backend/.build/e2e-cleanup/`; the CSV and this ledger preserve the reviewable decisions. Baseline and final timings cover different selections and are **not a measured CI speedup**.

## Boundaries retained and work still requiring proof

This is the largest removal supported by the equivalence evidence gathered in this pass. It is not a claim that every remaining instance has unique coverage.

- Auth screenshots remain in their existing project. Inspection found distinct UI assertions for users, invitations, token minting and account security that API lifecycle tests do not replace. Capture gating needs those assertions migrated first.
- LSP lease/process continuity, real idle reapers, stall sweeps, parking, security, paid/live-provider, platform and transport distinctions remain. Similar UI outcomes do not make their integration boundaries equivalent. Browser-clock optimization was not attempted in this removal batch.
- The deleted live-provider resume scaffolds leave the same unimplemented continuity gap they had before. Mock resume does not establish live-provider continuity.
- Initial audit repairs outside these proven consolidation groups remain follow-up candidates: intervention in automatic-resume observers, vacuous workflow-column checks, property-picker success returns/CEO fallbacks, exact queued-message delivery, fixture prerequisite skips and API-only navigation setup ownership. Those tests stay until replacement or stronger assertion proof exists.

No full 3,992-instance suite, external-provider suite or CI pipeline was rerun for this cleanup. The reported gates are the focused checks above.
