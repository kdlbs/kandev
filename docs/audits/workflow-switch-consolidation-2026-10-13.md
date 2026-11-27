# Workflow-switch E2E consolidation

Status: consolidation implemented and locally verified; hosted measurement pending.

Owner: `apps/web/e2e/tests/workflow/workflow-agent-switch.spec.ts`.
The file is unchanged between completed E2E run 38381773780 at `5698b315ef`
and the merged parent `b706496d5b`. That hosted baseline contains 18 passing
attempts totaling 225.9 seconds. This is accumulated file time, not a promised
critical-path saving. The local comparison must use the same PTY implementation
on both sides so that test consolidation receives no credit for the cleanup fix.

The local baseline passed all 18 cases without skips or retries in 157.841s
with the candidate PTY implementation. Source SHA-256:
`5ee318ac7cbc494627d164e28748eb795173af2efbc1ea0beae61fd39cb0ce2d`.
The complete candidate passed all 13 cases in 135.525s, with zero skips or
retries: **14.14% lower for this file**. The JSON report confirms all five removed
titles executed as successful named steps, preserving all 18 original scenario
identities. Both sides used the initial Linux PTY implementation, so this comparison
attributes no cleanup-fix saving to consolidation. Changed-file ESLint passes.
Full discovery passes with 4019 configured instances across 1393 files on this
parent snapshot. This is a local file comparison, not a hosted critical-path claim.
After the portable poller-registration correction (`58eb30d5be`), all 13 cases
passed again in the complete 42-case desktop verification run. That correctness
run overlapped backend lint and is not a controlled timing comparison.

## Coverage decisions

The manual keeper is **manual step move promotes new agent session to primary**.
It creates the task directly in the start step, displays Profile A's tab, waits
for the first agent, moves to the Profile B step, then asserts the second tab
and primary star without reloading. The automatic keeper is
**on_turn_complete cascade promotes new agent session to primary**. Its task
starts in Inbox; a real first turn advances the workflow before the second
profile appears. Those entry paths remain separate.

| Original case | Decision | Remaining proof and credible regression |
| --- | --- | --- |
| manual step move updates chat UI to show new agent profile | Consolidate into manual keeper | Same direct-start task and A-to-B move. Retain initial A session/tab and resulting B session assertions; the keeper additionally requires B's rendered tab. Detects missing or mis-profiled replacement and stale browser tabs. |
| initial task creation uses step's agent profile for the session tab | Consolidate into manual keeper | Same explicitly supplied Profile A, start-step override, task creation and tab-by-session-ID assertion before moving. Detects missing or incorrect initial session/tab. The existing case does not independently test an omitted explicit profile. |
| primary star icon appears on new session tab after step move | Consolidate into manual keeper | Identical direct-start setup, A-to-B move and B tab/star assertions with no reload. Detects absent primary promotion or browser update. |
| new session inherits task_environment_id from old session | Consolidate into manual keeper | Capture the nonempty A environment before the move; require B to inherit that exact value afterward. Detects missing handoff or a different environment. This assertion is additional to tab/star coverage. |
| on_turn_complete transition creates new session with next step's agent profile | Consolidate into automatic keeper | Same Inbox-to-A start, real delayed prompt, on-turn-complete move and B auto-start. Retain minimum count and chronological A/B profile assertions before the keeper's live B tab/star assertions. Detects wrong first or second profile even if a primary tab renders. |

Keep the API-only **manual step move creates new session with step's agent
profile**: its Inbox-to-A start differs from direct create-and-start. Keep all
four reuse/new versus park/complete policy combinations, user-message
on-turn-start routing, prompt-triggered launch without auto-start, explicit
New Agent selection, no-override preservation, delayed duplicate-respawn
protection and reset-context control behavior. No mobile test is replaced by
a desktop keeper.

## Ownership and history

Runtime selection and handoff belong to `event_handlers_workflow.go`:
`switchSessionForStepWithPoliciesAndCandidateAndRoute` owns the replacement
and existing-session policy. `task_operations.go` publishes primary-session
updates through `publishPrimarySessionUpdate`. Desktop tabs and their primary
star consume the task/session projection; reloading would mask a missing live
update, so neither keeper reloads after its transition.

The star tests document the original raw-repository-write versus
`SetPrimarySession` broadcast regression. The manual promotion test also
documents the pinned-session behavior introduced in PR #743: primary ownership
can move while the inspected tab remains pinned. Both cases already asserted
the same star rather than active-tab switching. The present paths were
introduced in commit `b090a0157e`; later changes including `e96910ff9a` retained
their readiness guards, and `69e6c4f249` added the distinct policy matrix.
The consolidation preserves these boundaries and uses stable session IDs.

The inheritance polling helper has only this file's test caller and no
production caller. Its last-session observation and final exact environment
assertion remain meaningful; remove its timeout-swallowing wrapper only if the
keeper replaces it with an explicit exact-value poll. No runtime seam or
production logic is removed.

Contracts inspected: workflow profile-session lifecycle requirements, workflow
agent override routing design, the complete spec and its helper, real runtime
selection/primary publication, and the Chromium-only Playwright routing. The
independent Go lifecycle, authorization and storage tests remain unchanged.

## Verification

Run the complete file before and after with the guarded managed runner, one
Chromium worker and zero retries. All 18 original scenarios must remain as
retained cases or named steps across 13 cases. Compare every removed case's
assertions with its keeper before deleting it. Any failed baseline is separate
from a removal candidate. Report exact outcomes and duration without
extrapolating to the complete E2E workflow.
