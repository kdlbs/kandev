---
created: 2026-10-30
status: implemented
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
legacy_specs: []
---

# Implementation plan: Continuation feedback after resumed activity

## Overview

Resolve [issue #4972](https://github.com/kdlbs/kandev/issues/4972).
After confirmed activity from an accepted continuation, replace the connection-loss
warning with neutral feedback. Keep Cancel available and preserve the recovery budget.
First implement the persisted presentation transition and renderer together.
Then prove the desktop and phone flows and update public guidance.

Platform owns this repair because it owns recovery admission, episode ownership,
and status projection. Agents supplies activity identity. Tasks persists messages.
The UI consumes that status. No separate UI requirement is needed.

## Evidence and root cause

Investigated the canonical issue and its comments on 2026-10-30. It contains no
image attachments and no comments. The issue is open and assigned to
`carlosflorencio`, the authenticated GitHub user.

The current source reproduces the reported predicate:

```ts
retryNoticeVisible("RUNNING", {
  retrying: true,
  recovery_mode: "continue",
  recovery_phase: "continuing",
}); // true
```

`TransientRetryNotice` always selects amber styling, `IconAlertTriangle`, and
the heading from `failure_code`. Accepted continuation updates its phase to
`continuing`. Output observation does not change its presentation.
`resetTransientRetry` later clears both ownership and persisted notices.
Calling that reset on output replenishes the budget on subsequent failures.

The feedback test named “keeps an owned continuation notice reachable while
running” explicitly requires visibility. This is encoded presentation behavior,
not evidence of a lost WebSocket update. The existing hold fixtures emit
“Mock continuation accepted” as actual assistant text before they wait.
They can prove the resumed state while the turn remains unfinished.

Verification during investigation:

- `(cd apps/web && pnpm exec vitest run components/task/chat/messages/interruption-recovery-feedback.test.ts)` passed: 1 file, 13 tests.
- A direct `tsx` call against the production helper returned `true` for the
  issue's metadata. Supplying a hypothetical confirmed-activity marker also
  returned `true`, because the current helper does not consume activity.
- Source tracing confirmed accepted execution/generation fields, guarded stream
  observation, persisted status updates, and an existing cancellation owner.
- No live-instance mutation, native-provider probe, or browser reproduction was
  needed to establish this deterministic presentation defect.

## Requirement reconciliation

The existing feedback contract distinguishes phases but does not define a
presentation transition after resumed activity. This repair adds `003.5` through
`003.7` to the existing requirement and qualifies `003.1` before that transition.
The existing budget, ownership, and internal-prompt criteria remain applicable.
The paired design describes the additive marker and current-attempt checks.

Settled scope: the issue explicitly permits neutral recovery information.
Keep the card and Cancel rather than introducing a separate cancellation surface.
No material intent question remains. Existing ADRs already establish the episode,
identity, and cancellation boundaries. This local projection change needs no new ADR.

## Scope

### In scope

- Backend confirmation of activity from the accepted continuation only.
- A persisted optional activity marker and neutral status content.
- A neutral card with localized feedback and the existing Cancel action.
- Waiting/reconnecting feedback, retry budgets, stale-event rejection, and cleanup.
- Reload, another viewer, desktop, phone, and public recovery guidance.

### Out of scope

- New provider support, recovery admission, tool reconciliation, or prompt wording.
- Original-request replay after effects, provider switching, or retry timing changes.
- Dynamic routing, Office, utilities, passthrough, and general connection detection.
- New flags, tables, durable retry jobs, transport protocol versions, or schedulers.
- Production implementation, permanent test changes, commits, pushes, and PRs in
  this design turn. A later explicit request starts implementation.

## Technical approach

### One presentation transition under the existing owner

Follow the design's “Confirmed continuation activity” section. Add bounded activity
and publication state for the current accepted prompt. Extend
`recordContinuationAcceptance` and guarded stream observation to reconcile it.
Match the current entry, execution, and nonzero prompt generation before publication.
Reject old executions, earlier generations, unaccepted output, diagnostics, empty
chunks, child/background activity, and unattributed tool updates.

Use `provider_interruption_activity.go` for the focused helper. Keep
`event_handlers_streaming.go` limited to routing its qualifying event inputs.
Use the existing status writer and notice mutex. A delayed continuing-phase writer
must retain confirmed state. A later failure must clear presentation state while
carrying forward started attempts and policy. Never call `resetTransientRetry`
to acknowledge output or change the owner/timer as part of presentation.

### Persisted message and renderer

Persist `recovery_activity_confirmed: true` on the current status message.
Keep `retrying: true`, continuation mode, continuing phase, failure diagnostics,
attempt metadata, and existing Cancel action. Persist neutral content/variant.
Use the existing `UpdateMessage` publication. No store slice or browser-local
acknowledgment is needed.

Extend `ActionMeta` and the feedback helper for the optional field. Add a pure
presentation predicate that requires boolean true and valid continuation metadata.
Keep the card visible during RUNNING. Use neutral tokens and an informational
icon with localized “Agent resumed” text. Omit the historical failure heading,
warning styles, countdown, and attempt counter in that state. Retain provider/model
context, history-preserved text, Cancel, and one polite status announcement.
Generate Traditional Chinese and pseudo catalogs with repository commands.

### Compatibility matrix

| Producer or shape | Identity and transport | Behavior and evidence | Fallback |
| --- | --- | --- | --- |
| Supported Cursor continuation, retained or restored | ACP execution plus accepted prompt generation | Neutral after attributed output or foreground activity. Backend stream tests and hold-fixture E2E | Missing identity keeps the existing warning |
| Cursor V3 with inherited pending tools | Same retained ACP runtime, fresh accepted generation | Only activity attributable to the new prompt confirms recovery. Mixed-state backend regression | A late inherited tool result alone leaves the warning |
| Existing retained capacity continuation | Existing supported ACP capacity policy and accepted identity | Same presentation marker, unchanged capacity budget/window. Targeted capacity regressions | No marker retains current capacity feedback |
| Legacy message or older backend | Marker absent, false, malformed, or unknown phase | Existing warning/visibility behavior. Helper and component tests | No inferred confirmation from RUNNING |
| Legacy original-prompt replay | Existing replay mode | Existing presentation and retry owner | Continuation marker cannot activate outside continuation mode |
| Dynamic, Office, utility, passthrough, unsupported provider | Their existing owner/capability | No new admission or recovery path. Existing negative tests | Existing policy |

### Related packages and public docs

The original continuation, hidden-continuation, graduation, and pending-tool
packages are completed historical records. Their dispatch, cancellation, and
identity guarantees remain inputs. Their screenshots and test counts do not
prove this new neutral state. This package replaces only their post-activity
warning expectations. Do not rewrite historical verification results.

Update the interruption-continuation explanation in
`docs/public/sessions-and-review.md` during Task 02. Explain the neutral state,
Cancel, and unchanged automatic-attempt budget. This design turn changes internal
specifications and delivery records only.

## ASCII UI preview

UI-01: Desktop Task > Chat, continuation accepted with actual resumed output.
The before state is supported by source and existing tests, not a fresh screenshot.

```text
Before
[!] Agent connection lost. Continuing previous request... [Cancel]
    Attempt 1 of 5
    <new agent output and tool cards>

After
[i] Agent resumed                         [Cancel]
    Cursor / selected model
    Previous history and tool results are preserved.
    <new agent output and tool cards>
```

UI-02: Phone Task > Chat, the same confirmed state in the existing inline card.

```text
+--------------------------------------+
| [i] Agent resumed                    |
| Cursor / selected model              |
| Previous history and tool results    |
| are preserved.                       |
| [              Cancel              ] |
+--------------------------------------+
| <new agent output and tool cards>    |
```

UI-03: Before resumed activity, desktop and phone retain waiting/reconnecting
feedback. Acceptance or RUNNING alone does not select UI-01/UI-02.

```text
[!] Agent connection lost. Continuing in 0:05
    Attempt 1 of 5                       [Cancel]

[!] Agent connection lost. Reconnecting...
    Attempt 1 of 5                       [Cancel]
```

The phone uses the existing stacked action for UI-03. These are illustrative
localized labels. Required structure: neutral resumed styling, one current card,
visible Cancel, and no old failure heading or counter in the resumed state.
The card stays in the existing Chat scroller. The composer and navigation retain
their current placement. Phone Cancel has an actual hit area of at least 44px.
Technical context wraps and document overflow remains zero. No overlay or new
scroll region is introduced. The nearest exemplar is the shipped
`TransientRetryNotice` inside the dedicated mobile task Chat layout.

Map UI-01/UI-02 to `003.4` through `003.7`. Map UI-03 to `003.1` and `003.6`.

## Tests

Task 01 begins with failing regressions in
`apps/backend/internal/orchestrator/provider_interruption_activity_test.go` and
the existing frontend feedback/component suites.

| Criteria | Named regression and evidence |
| --- | --- |
| `003.5`, `003.6` | `TestInterruptionContinuationActivityConfirmsAcceptedPrompt`: stream assistant, reasoning, and attributable foreground tool variants. Assert neutral persisted metadata before completion |
| `003.6`, `003.7` | `TestInterruptionContinuationActivityRejectsStaleEvidence`: old execution/generation, missing identity, unaccepted output, diagnostics, empty chunks, nested/background work, inherited pending result |
| `003.5`, `003.7` | `TestInterruptionContinuationActivityPublicationOrdering`: activity before acceptance callback, late phase write, cancellation, replacement, and retry after a failed message update |
| `002.1`, `002.2`, `003.7` | `TestInterruptionContinuationActivityPreservesEpisodeBudget`: resumed output followed by another eligible failure retains actual attempts, delay progression, and exactly one dispatch owner |
| `003.1`, `003.5`, `003.6` | Frontend “renders neutral feedback only after attested resumed activity”: marker parsing and rendered heading/icon/styles/counter/Cancel, plus absent or malformed metadata |
| `002.2`, `003.3` | Existing cancellation, shutdown, supersession, and terminal cleanup tests, extended to confirmed notices |

Retain one completed-plus-pending V3 tool case. A late result for the pending
predecessor tool must not confirm recovery. A fresh accepted-prompt event must.
Existing capacity fixtures verify that presentation does not change their window.

## E2E tests

Task 02 updates the existing desktop `read-hold` and phone `pending-v3-hold`
flows in the continuation spec files. Their mock emits actual resumed output
and then holds the turn open. Assert a neutral card and Cancel while the session
is still RUNNING. Assert the absence of the old active failure label and attempt
counter. Assert the persisted marker through `listSessionMessages`.
Reload and open a second viewer. Confirm the same message ID and neutral state,
then cancel through the existing UI and prove the turn settles.

Retain waiting cancellation, refusal, hidden internal prompt, and native-identity
assertions. On the phone, measure Cancel's hitbox and check overflow. Capture
desktop and phone screenshots and compare their structure with UI-01/UI-02.
Projects: `chromium` and `mobile-chrome`. Map to `001.6`, `002.2`, `002.3`,
`003.1`, `003.3` through `003.7`.

## Work orders

- [x] [Task 01: Persist and render confirmed continuation activity](task-01-confirmed-activity.md)
- [x] [Task 02: Prove resumed feedback across viewers and viewports](task-02-viewer-and-mobile-proof.md)

Execute Task 01, then Task 02 in the primary session. Neither task authorizes delegation.

## Verification results

Design validation passed on 2026-10-30:

- `python3 scripts/list-docs.py validate`: 424 decisions and 1861 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The local `validateCoverage` invocation documented in Task 02 returned
  `covered` for both work orders, with zero reference errors. The input included
  representative backend and frontend production paths to require coverage.
- The Task 02 Playwright discovery command found 20 tests in the intended two
  files: 16 in `chromium` and 4 in `mobile-chrome`. It did not run browser tests.
- `git diff --check -- docs/specs docs/plans/continuation-resumed-feedback` passed.
- `git status --short -- docs/specs docs/plans/continuation-resumed-feedback`
  showed the two edited specs and the new plan directory. All five documentation
  files remain unstaged and uncommitted.

Investigation results appear in the evidence section.

Implementation and rendered verification passed on 2026-10-30:

- Task 01 backend activity tests passed, including the race detector run. The
  targeted continuation, transient retry, capacity, and prompt-evidence suite
  passed. The full orchestrator package also passed.
- Task 01 frontend feedback and action-message suites passed 75 tests; the full
  chat message suite passed 657 tests across 61 files. ESLint, typecheck,
  Traditional Chinese generation, pseudo generation, catalog checks, and the
  i18n ratchet passed.
- Task 01 Go lint on the changed files reported 0 issues.
- Task 02 discovery found 20 tests: 16 in `chromium` and 4 in `mobile-chrome`.
  Both managed suites passed, 16 and 4 respectively.
- Public documentation, specification catalog, specification linter, and
  documentation-reference checks passed. The local coverage preflight returned
  `covered` for both work orders.

## Risks

- Acceptance and stream callbacks can race. Reconcile current-attempt evidence at
  both boundaries and prevent a late phase write from restoring the warning.
- Existing stream observation includes tool updates. Reuse its identity evidence
  without treating every tool update as foreground resumed activity.
- A failed persistence update can temporarily leave stale feedback. Preserve
  diagnostics and allow later valid activity to retry publication.
- Older frontends ignore additive metadata and can retain their old presentation
  until upgraded. Older messages remain readable without invented activity.
- The existing native provider support is reused. Mock browser proof validates
  Kandev's projection and ownership, not new provider recovery guarantees.
