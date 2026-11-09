# Workflow prompt and handoff investigation

## Revision and scope

Date: 2026-11-08. Investigation checkout and remote main:
`9aba7b2edd020f6ba0f92c2bfe318429723ea2ed`.
Branch: `feature/preserve-workflow-pr-5bq`. The worktree was clean before
document authoring. The parent checkout was not changed.

The earlier parent inspection at `02b1267f7f279565cd2a9496caa429fd701607f0`
is background evidence. Current source was read again. The package uses the
current base, not assumptions about stock 0.99.1.

This is Feature / Analysis. No model/profile, autopilot, workflow step,
production code, permanent tests, generated contracts, or lockfiles changed.
No live deployment, credentials, user database, or helper journal was used.

## Canonical tracker checks

Both canonical issues were fetched with `gh issue view --json`, including
their full bodies and comments, at 20:51 UTC. Both were OPEN and unassigned.
Both had zero comments and no attachment links to fetch.

- [#5178](https://github.com/kdlbs/kandev/issues/5178):
  operator-reported explicit read-only review brief disappeared from the first
  message. The operator stopped the turn. The report does not establish publication.
- [#5173](https://github.com/kdlbs/kandev/issues/5173):
  operator-reported three completed turns, runtime state REVIEW, Build step,
  original primary session, and a later `already_signaled` response.
  A manual move is recovery evidence, not automatic-transition proof.

Canonical issue timelines were also fetched through
`gh api repos/kdlbs/kandev/issues/<number>/timeline --paginate`.
Neither timeline contained a linked fix PR. #5178's cross-reference was #5180,
the separately owned Claude API-key compatibility issue.

The authenticated identity was verified with
`gh api graphql -f query='{ viewer { login } }'`: `carlosflorencio`.
At 20:58 UTC, #5178 remained open/unassigned and a fresh PR search found no
explicit fix. At 20:59 UTC, authorized assignment succeeded:

```text
#5178 OPEN assignees=[carlosflorencio] updatedAt=2026-11-08T20:59:11Z
#5173 OPEN assignees=[] updatedAt=2026-11-08T05:51:16Z
```

No issue/PR comment or closure was made.

## PR and dependency coverage

The audit fetched 146 open PRs with titles, bodies, head refs and SHAs, plus
the 100 most recent merged PRs. Related live PRs were fetched separately for
their bodies and file lists. No mandatory unmerged prerequisite was found.
These are snapshots. implementation must resolve the heads again.

| Reference | Resolved status/head | Boundary decision |
| --- | --- | --- |
| #5198, fixing issue #5137 | MERGED. head `6e56327c86ad5d61b045bfc33f675c1dcc7c029c`. merge `02b1267f7f279565cd2a9496caa429fd701607f0` | Current base includes fresh-recovery workflow composition. Explicit `start_created` loss still reproduces. Do not replace recovery semantics. |
| #5144 | MERGED. head `b27e2110b184d228d30271c4c967cbd3f0d3aa3c`. merge `941d3797871df1ef2f280bb6eb194bd15d87d8cb` | Carries committed cross-step promotion entry ID. It does not prove later same-entry correction handling. |
| #3420 | OPEN. `c374ea0cfeab01c05f390b15db3a5e5b03488786` | Editor auto-start defaults and template documentation. No backend explicit-launch fix. Reconcile shared public template wording. |
| #4772 | OPEN. `471f82e976926eed0f320fca4bd6a4b905256cf6` | Profile destination allocation idempotency for #4477. Distinct from replacing a pending same-entry signal. |
| #4604 | OPEN. `3a5c209b8df19e2606c8fe966564d54df1caaf0a` | Queued turn-start profile-switch guard/deadlock. If #5173 reproduces only there, attribute it to that existing work. |
| #4764 | OPEN. `2789d904894cef436ba23a5bf9b6b1ce470c108a` | Exact assigned profile/model propagation. File overlap, not a prerequisite for prompt preservation. |
| #4809 | OPEN. `57518c3bb47aa86d94345d8ad18cb163adc28975` | Detached MCP waiting and uncertain transport outcome. Does not change signal first-writer identity. |
| #4714 | OPEN. `0bffae1e41e2e80e086d8c756c9abfad904a86dc` | Workflow prompt-source import preserves template identity. No launch-input repair. |

Open #5176, #5179, and #5184 retain their cold-resume executor, retained
executor-edit, and prepared-actor/shared-secret ownership. Their file overlap
does not authorize changing those guards. The sibling auth/model and
unavailable-runtime planning tasks remain independent. Issues #5191, #5186,
#5203, and #5169 were not absorbed.

## Issue 5178: confirmed

### Current source trace

1. `launchStartCreated` forwards `req.Prompt` to `startCreatedSession`.
   Its normal options contain only lifecycle ownership, not direct preservation.
2. `startCreatedSession` uses the supplied input, or the task description for
   exactly empty input. It calls
   `applyWorkflowAndPlanModeWithPromptContextOptions` with preservation false.
3. `stepPromptBodyWithOptions` substitutes a present `{{task_prompt}}`.
   Without that placeholder, a nonempty step template replaces the input
   unless `preserveDirectPrompt` is true.
4. `wrapCreatedSessionPrompt` adds server context to that reduced text.
   `executor.LaunchOptions.Prompt` and `postLaunchCreated` receive the same
   reduced result. The loss occurs before provider dispatch and recording.
5. The direct-message preserving seam already passes true.
   `seam2StartCreatedPayload`, ceiling replay and Send Now already carry
   `preserve_direct_prompt`. the launch producer fails to select it.

The condition requires a replacing step template. Empty and placeholder
templates are controls, not evidence that every launch loses its prompt.
The retained plan explains the operator-visible consequence, but is not
necessary for the demonstrated loss.

### Disposable probe

A Go overlay added a virtual `zz_planning_probe_test.go` to the existing
orchestrator package. All real files stayed unchanged. The corrected probe
used `setupTestRepo`, `seedTaskAndSessionWithStep`,
`createTestServiceWithScheduler`, `repoTurnService`, and
`repositoryBackedMessageCreator`.

The request went through public `LaunchSession`, with `IntentStartCreated`,
`AutoStart: true`, a concrete profile, and the explicit fixture instruction:

```text
Independent READ-ONLY review. Do not edit, rebase, commit, push, or publish.
```

The replacing template was `Read the retained implementation plan and continue.`
The manager callback captured `executor.LaunchAgentRequest.TaskDescription`.
The repository read all messages for the exact prepared session and required
one user row. The direct control used the existing preserving public seam.

Exact diagnostic command:

```bash
(cd apps/backend && go test -trimpath -overlay /tmp/workflow-prompt-handoff-probe/overlay.json ./internal/orchestrator -run '^TestPlanningProbe' -count=1 -v)
```

Corrected-fixture result:

```text
launch_replacing: manager_contains_brief=false stored_contains_brief=false manager_equals_stored=true
FAIL: explicit constraints must reach manager launch request
launch_placeholder: manager_contains_brief=true stored_contains_brief=true manager_equals_stored=true
PASS
direct_replacing: manager_contains_brief=true stored_contains_brief=true manager_equals_stored=true
PASS
TestPlanningProbe5173TurnStartReset: PASS
orchestrator: FAIL (expected #5178 assertion), 0.171s
```

The first attempt failed during fixture setup: missing explicit profile and
a message foreign key without a persisted turn. Those errors were corrected
before drawing any product conclusion. They are not defect reproductions.

**Confidence: high** for the current launch producer/composer defect and the
existing preserving seam as the repair. **Limits:** this was a SQLite-backed
service probe with a mocked agent manager. It did not launch a native provider,
seed an actual retained plan, inspect rendered UI, or exercise remote executors.
API raw-content behavior was source-traced, not asserted by this probe.
The permanent matrix and ACP trace in Task 01 must supply those missing proofs.

### Repair constraints

An unconditional `preserveDirectPrompt=true` is too broad.
`TaskHandlers.dispatchTaskSession` transports the task description in
`req.Prompt` during HTTP task creation. `AutoStart` is also set on the
reported explicit request, so it cannot distinguish origin.
The server-only description-origin flag in the owning design preserves
current automatic replacement semantics without guessing from text.

## Issue 5173: unconfirmed

### Verified current behavior

- `setSessionWaitingForInputWithHook` calls `writeTaskReviewState`.
  That path updates runtime state to REVIEW under its current-state guard;
  it does not move `workflow_step_id`. REVIEW plus Build can be valid.
- `handleStepComplete` resolves the latest turn's immutable step/entry stamp.
  `ClaimStepCompletionSignal` locks and validates the current task entry,
  then returns the first stored payload for that entry.
- `handleDuplicateStepComplete` returns only
  `accepted:false, reason:already_signaled`. A waiting-session retry can
  republish a wakeup. The response does not prove a committed destination.
- `prepareOnTurnStartSession` clears the pending bag for new admitted input.
  The clear is best effort. a failed database write is logged and not fatal.
- `matchingWorkflowSignal`, the transition CAS, and conditional consumed-signal
  cleanup protect current entry ownership. A late event is a durable-state
  wakeup, not authority to overwrite a successor signal.
- The payload requirement's first-signal rule, AC-001.13, is explicit. Its file
  remains draft. No new same-entry supersession policy was silently substituted.

### Disposable control

`TestPlanningProbe5173TurnStartReset` used a fresh SQLite repository and
the real orchestrator `ProcessOnTurnStart`:

1. Claim a Build-entry signal with `first handoff`.
2. Claim the same entry with a changed payload. The claim returns false and
   retains `first handoff`.
3. Run `ProcessOnTurnStart` without a configured move.
4. Claim that same entry again. The claim succeeds with `latest handoff`.

The control passed in the corrected overlay run. This proves the normal reset
seam can admit the later payload. It does not prove a full later message
admission or a Review transition, and it does not reproduce the report.

### Unresolved facts and hypotheses

The issue does not supply the workflow move configuration, turn/entry stamps,
admitted input operation IDs, or signal timestamps.
Transition claim outcome, carry token, and destination dispatch identity are
also absent.

Possible explanations remain:

- No runnable automatic move, an unmet guard, or a failed transition.
- A later turn continued without the new-input reset seam.
- Failed best-effort signal clearing left the original durable bag.
- New user/peer input cancelled or superseded the original completion.
- An ordering or recipient race after transition commit.

These are hypotheses. The successful reset control does not select among them.
The separate work order bounds the experiment and defines a decision checkpoint.
Do not claim or assign this issue on the strength of the #5178 reproduction.

**Confidence: insufficient for a corrective patch.** The requested structured
signal/destination readback is also an unresolved contract, not implemented
evidence. It requires explicit identities and failure semantics before design.

## Existing control suite

The following read-only diagnosis completed successfully against unchanged
production and permanent test files:

```bash
(cd apps/backend && go test -trimpath -p 1 ./internal/mcp/handlers ./internal/task/repository/sqlite ./internal/orchestrator -run '^(TestHandleStepComplete_(FirstCallAccepted|DuplicateCallDoesNotOverwriteHandoffOrBlockers|DedupWaitingRepublishes|ConcurrentCallsClaimOneSignal|StaleTurnRecoveryGuidance|FreshTurnAfterStepChange)|TestStepCompletionEntry(|ConcurrentClaims|EmptyMetadata|UnledgeredTask|HandoffRebase)|TestStepCompletionEntryHandoff|TestProcessOnTurnComplete_ConcurrentDuplicateCalls_OnlyOneTransitionApplies|TestStartCreatedSession_InitialTaskBrief)$' -count=1)
```

```text
ok internal/mcp/handlers            0.720s
ok internal/task/repository/sqlite  1.024s
ok internal/orchestrator            0.488s
```

These results cover existing duplicate claims, stale/fresh-turn handling,
entry-bound carry and transition controls, and the direct preserving seam.
They do not establish #5173's intended three-turn sequence. No race detector,
PostgreSQL, native ACP, browser E2E, or live deployment ran in this planning turn.

## Document checks

The final authoring checks ran from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node /tmp/workflow-prompt-handoff-probe/check-package.cjs
git diff --check -- docs/specs docs/decisions docs/plans/workflow-prompt-handoff-integrity
git status --short -- docs/plans/workflow-prompt-handoff-integrity
```

Results: 439 decisions and 1946 specifications validated. All 36 linter tests
passed. Full specification lint passed. The package appeared as untracked,
and the two existing initial-input files appeared modified. No staged change
or source/test change was present.

The temporary Node preflight called the repository's exported
`.github/scripts/pr-docs.cjs:validateCoverage`. It supplied both new work orders,
their sibling plan, the six referenced design/requirement pairs, and a projected
`session_launch.go` change. The projected source path forces coverage checks
instead of accepting the documentation-only exemption. It does not represent
an actual source edit. Result: `ok:true, status:covered, errors:[]`.

The first preflight loaded every specification and exceeded the helper's
4 MiB input limit. Loading only referenced documents corrected the invocation.
All relative Markdown file targets and trailing whitespace checks passed,
including the untracked files that ordinary `git diff` omits.
The temporary probes and preflight script are removed after this record.
They are not permanent tests or package dependencies.

## Implementation verification

Date: 2026-11-08. Base remained
`9aba7b2edd020f6ba0f92c2bfe318429723ea2ed`. The implemented change adds the
server-only `LaunchSessionRequest.PromptIsTaskDescription` marker, selects
`preserveDirectPrompt` in `launchStartCreated` for explicit nonempty input, and
marks ordinary task creation in `TaskHandlers.dispatchTaskSession` while leaving
Quick Chat explicit. Existing seam-2 serialization and ceiling replay carry the
value without a new recovery mechanism.

New tests:

- `apps/backend/internal/orchestrator/session_launch_prompt_integrity_test.go`:
  replacing, placeholder, and empty templates; explicit input equal to the
  description; description origin; empty and whitespace fallback; no-initial
  prompt; saved-prompt reference expansion; JSON server-only provenance;
  deferred round trip and replay consumer. It also covers one real
  `LaunchSession` with a replacing template plus workflow-level instructions,
  plan mode, canvas guidance allowed and absent, saved-prompt expansion, and a
  valid attachment identity; a matching pending initial-submission receipt
  that transitions after provider admission plus a forced CAS failure that
  blocks native dispatch and proves recovery keeps the explicit brief; and a
  server-only explicit-launch recovery marker.
- `apps/backend/internal/mcp/handlers/create_task_initial_prompt_workflow_test.go`:
  MCP initial task creation marks its description prompt as description-origin.
- `apps/backend/internal/task/handlers/task_create_prompt_origin_test.go`:
  ordinary creation marks the description origin; Quick Chat does not.
- `apps/web/e2e/helpers/start-created-prompt-integrity.ts` and the two session
  specs prepare a CREATED session with a retained plan, wait for exactly one
  stored user message, require its structured raw prompt, launch the explicit
  brief on replacing and placeholder step templates, compare the complete
  stored raw prompt with the native ACP prompt, match the prompt's
  session/process/connection identity to the `session_new` event, then reload
  desktop and phone Chat.

Exact results:

```text
ok  internal/orchestrator        (full package)
ok  internal/task/handlers       (full package)
ok  internal/prompts/service
ok  internal/sysprompt
ok  internal/task/models
go build ./...                   passed
golangci-lint                   0 issues
web typecheck                   passed
web lint                        passed
i18n ratchet                    no new UI copy
chromium E2E                    2 passed
mobile-chrome E2E               1 passed
public docs                     63 tests passed; 55 pages validated
spec catalog                    439 decisions, 1946 specifications
spec lint                       all files passed
```

Trace assertions required one `session_new`, one `prompt`, matching
session/process/connection identity between them, exact equality between the
native prompt and the stored structured raw prompt, the explicit brief once, and
no native prompt before launch. The UI assertion scoped to the single
`user-message-bubble`; the pinned last-prompt bar also echoes the text and is not
a second stored message. The screenshots from the first failing attempts showed
that duplicate DOM copy, not duplicate storage.

Task 02 re-ran the existing duplicate, stale-entry, concurrent-transition, and
later-reset controls with the race detector. They passed. No failing invariant
emerged, so this record keeps #5173 unassigned and states the missing evidence
rather than replacing the first-signal rule.

Review follow-up completed on the same base. The E2E helper now waits for exactly
one stored user message, requires its structured raw prompt, compares that raw
prompt byte-for-byte with the native ACP prompt, matches the prompt's
session/process/connection IDs to the `session_new` event before asserting
content, and rechecks the trace after reload. The Go matrix gained a real
`LaunchSession` mixed-context case (workflow instructions, plan mode, canvas
allowed/absent, saved-prompt expansion, attachment identity), a pending
submission receipt case with both admission success and a forced pre-provider
CAS failure that proves native dispatch stays at zero, and a recovery marker
that keeps the explicit brief during fresh-start recomposition. MCP initial
creation also marks its task-description prompt. Focused commands passed:

```text
go test -race ./internal/orchestrator -run '^TestLaunchStartCreated_'
go test -race ./internal/task/handlers -run '^(TestTaskCreateDispatch_|TestDispatchTaskSessionMarksExplicitCreationPrompt|TestStartAgentForNewTask_SetsDeferredStart)'
golangci-lint ./internal/orchestrator ./internal/task/handlers   0 issues
web typecheck, lint, i18n ratchet                               passed
chromium E2E                                                    2 passed
mobile-chrome E2E                                               1 passed
spec/docs validation                                            passed
```

Limits: local mock ACP only; no real vendor obedience, remote executor,
PostgreSQL, or historical incident reconstruction. No issue comment, PR, commit,
or publication was made in this implementation turn. The design package and code
changes remain unstaged and uncommitted.
