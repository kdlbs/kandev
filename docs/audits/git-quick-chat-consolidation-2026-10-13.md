# Git Changes and Quick Chat E2E consolidation

Status: consolidation implemented; complete local suites passed, hosted validation pending.

This batch owns `e2e/tests/git/git-changes-panel.spec.ts` and
`e2e/tests/chat/quick-chat.spec.ts` under `apps/web`. Compare their complete
Chromium suites with one managed worker, zero retries and the same portable
PTY implementation (`58eb30d5be`). The baseline has 26 Git and 24 Quick Chat
cases. All 50 passed in 381.425 seconds with zero skips or retries. The
candidate has 22 of each. No product behavior changes.

## Coverage decisions

| Original case | Decision and keeper | Preserved oracle / credible regression |
| --- | --- | --- |
| shows new untracked files in unstaged section | C: shows modified files in unstaged section | Create a genuinely untracked file, assert the unstaged section and filename, then delete it. Detects dropped untracked status independently of tracked modifications. |
| shows modified files with spaces in path | C: shows modified files in unstaged section | Commit and modify a path containing spaces, assert the unquoted filename, open its diff and require actual modified content in the shadow DOM. Detects porcelain path decoding and diff lookup failures. |
| shows commits after staging and committing | C: revert commit undoes commit and stages changes | Before reverting, require the commit section, message and exact short SHA. Detects absent commit metadata independently of a row test ID. |
| clicking commit opens diff view | C: revert commit undoes commit and stages changes | Open the explicit historical-detail action, assert the detail message and actual added line in the diff before reverting. Detects broken navigation or empty historical diff. |
| slash command menu populates before first message (eager agent init) | C: opens quick chat, selects agent, sends message and receives response | Subscribe before opening, require a nonempty available-commands frame, type slash and assert Commands, /slow and /error. Clear the menu before the keeper's follow-up prompt. Detects broken ACP command publication or menu consumption. |
| model selector shows dynamic session options before first message | C: same Quick Chat keeper | Require Mock Fast, open session model settings and its effort options. Close the popover before submitting the follow-up. Detects missing dynamic configuration independently of message delivery. |

All six contracts execute as named steps. Combining setup does not replace
their distinct assertions. The Quick Chat step names describe availability
**after the opening prompt**: the shared helper already submits an opening
prompt and waits for idle. The old titles and eager-start comments contradict
the implemented opening-composer contract, not its current assertions.

Keep Git's separate staged/unstaged facets, node_modules exclusion, copied
paths, viewport actions, live refresh, provider-only history fallback,
rewritten history, amend/reset, multi-commit and progress cases. Keep Quick
Chat's opening-composer rejection/creation rules, device-specific paths,
clarification keyboard routing, real reference-menu Escape handlers, restored
sessions, repository context, tabs and enhancement recovery. These expose
different state, protocol, transport or input-handler failures.

## Ownership and history

Git status and commit contracts are in workspace-git-status and
commit-file-navigation requirements. `commit-row.tsx` owns the explicit Open
action; `use-git-operations.ts` dispatches the real revert request. The server's
`process/git.go` validates HEAD and performs a soft reset, leaving changes
staged. The keeper retains this real mutation after inspecting that commit.
The original cases date to `b090a0157e`; later provider and live-refresh
regressions (`3a1ca57c0a`, `c4b4791de6`, `8ca01e0f7e`, `5928760745`) stay separate.

Quick Chat's opening-composer requirements and design, implemented in
`99f509743b` (#4173), forbid launching a session from setup alone. The opening
prompt creates the session before the helper returns. `tiptap-input.tsx`
consumes session commands; `model-selector.tsx` and `model-config-selector.tsx`
consume dynamic settings. Their command portal and controlled settings popover
remain real UI boundaries. The current Quick Chat file was introduced in
`376a2af` (#4404); no global helper behavior changes in this audit.

The Git file's private GitHelper duplicates the shared helper but deletes
`.git/index.lock` before commands despite claiming to respect active locks.
It has no production callers. Reuse the shared helper, which retries and
reports contention without removing the lock. Add a real temporary-repository
lock-preservation regression and prove it detects the old deletion behavior
with a temporary mutation restored byte for byte. Also reuse the already
tested shared mock-profile selector instead of choosing `agents[0]`. Keep the
file's navigation and repository-restoration helpers unchanged.

## Validation

Run the complete two-file baseline before editing, then the complete candidate.
Verify all original case identities remain either cases or successful named
steps (two obsolete Quick Chat titles map to accurate step names). Run the
shared Git helper unit suite, changed-file ESLint, formatting, web typecheck
and guarded E2E discovery. Local cohort savings are not a full-suite or hosted
critical-path claim. The full target remains at most 1,079.5 seconds for the
longest normal hosted E2E test step, against the fixed 2,159-second reference.

The real lock-preservation regression passes on the shared helper. Temporarily
restoring deletion of `index.lock` made its expected-contention assertion fail
(the Git command incorrectly succeeded). The helper was restored byte for byte;
all five helper unit tests then passed. Changed-file ESLint passes.

The complete candidate passed all 44 cases in 351.389 seconds, with zero skips
or retries: 30.036 seconds (7.87%) lower in this single full-file pair. JSON
reports confirm all six removed cases executed as successful named steps and
all 50 original scenario identities remain covered. The basic Quick Chat case
took about 41 seconds on both sides; no saving is attributed to that delay.

## Repository-context cleanup repair

The retained **explains quick chat and starts with repository context** case
removed its local and remote base branch before its task was cleaned up. A
following case then spent 30.088 seconds in workspace reset, failed to resolve
a containing base during worktree cleanup, and restarted the backend through
the existing transport-recovery path. A trace attributed 30.727 seconds to
`testPage`; temporary reset instrumentation and the actual backend log isolated
the failed cleanup. Instrumentation was restored byte for byte afterward.

F: preserve the branch until the real reset has finished deleting the owned
task and its worktree. Assert that reset succeeds directly, without fixture
recovery, before removing the branch in a nested finally. This strengthens the
cleanup oracle while retaining the request repository/base-branch assertions,
visible editor and unchanged source checkout assertions. No production reset
or transport-recovery behavior changes. The new assertion fails on the old
ordering (`expect(...).resolves.toBeUndefined()` rejects after about 30 seconds),
reproducing the defect instead of silently paying for backend recovery.

Three alternating repetitions of the nine affected original cases versus their
three combined keepers passed without skips/retries. Median elapsed time was
58.544s versus 25.982s (55.62% lower for **only these affected journeys**).
The complete two-file comparison above remains the broader measurement; this
small cohort must not be presented as the full E2E target result.

The corrected cleanup sequence passes both affected tests in 19.080 seconds;
the following basic-chat test now takes 10.670 seconds rather than about 41.
The **final complete two-file run** passes all 44 cases in **316.745 seconds**,
with zero skips/retries. Against the original 381.425-second baseline this is
**64.680 seconds (16.96%) lower**. All 50 original scenario identities remain
present as cases or successful steps. This final result supersedes the earlier
consolidation-only candidate timing above. It is a local full-file pair, not a
hosted full-suite result.

Final checks pass: five shared Git helper unit tests, changed-file ESLint and
Prettier, web typecheck, guarded E2E discovery (4013 instances across 1393 files),
document catalog validation and specification lint. No temporary instrumentation
or mutation remains. Native macOS and full hosted validation of the next PR head
remain required.

## Cancellation group retry scope

The three cases in `chat/quick-chat-cancel-palette.spec.ts` retain distinct
coverage: foreground composer cancellation, detached-background cancellation,
and command-palette targeting while an underlying task remains running. Each
case creates its own task/session state through the reset fixture. No case
consumes state created by an earlier case. The file's shared flag setup and
restoration remain in `beforeAll`/`afterAll`, including on worker replacement.

The serial declaration unnecessarily retries the entire group. Full hosted
runs 38405876470 and 38416314881 both replayed the first two passing cases when
the palette-targeting case failed. Its logs include cancellation escalation
and recovery-required state, so its assertions remain relevant and are kept.

A controlled local probe injected one failure at the start of only the third
case's first attempt, with one retry and one worker in both configurations.
The serial declaration ran six attempts in 200.376 seconds. The ordinary
declaration ran four attempts in 114.368 seconds. Every case's final attempt
passed all original assertions; the only failed attempt was the intentional
probe in each run. Both commands correctly returned failure under the local
fail-on-flaky policy. These are retry-scope measurements, not full-suite gains.

Remove only `.serial`. Keep all three cases, flag lifecycle hooks, assertions,
timeouts and retry counts. The deliberate failure and `testInfo` argument were
removed and the original source was verified byte-for-byte before applying
the one-line change. A fresh unmutated run with retries disabled passed all
three cases in 108.466 seconds, with no skipped or flaky cases.
Raw reports and restoration receipts are retained under
`apps/backend/.build/ci-half/e2e-focus/cancel-retry-*`.
