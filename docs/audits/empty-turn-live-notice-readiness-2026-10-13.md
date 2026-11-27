# Empty-turn live notice readiness

Hosted run 38472002693 at `690815cc5f` fails the slash-command hint case in
normal shard 4 on both attempts. Its retry trace spends about eight seconds
in the initial task-page navigation. The mock empty-turn scenario waits three
seconds before completing. The original fixture starts that turn before
navigating, so it does not ensure the browser receives the live completion.
The first attempt shows a generic empty-output message; the retry shows no
notice. Neither result satisfies the existing slash-command hint assertion.

`maybeEmitEmptyTurnNotice` in `lib/ws/handlers/empty-turn-notice.ts` runs from
the live turn handler and correlates the user prompt by turn ID. Loading a
completed historical turn does not replay the notice. Increasing the hint
assertion timeout cannot recover a completion event the browser never saw.

The unchanged two-case file passes locally. A controlled diagnostic waits for
the initial backend session to finish before opening the page. With the
original fixture, the original hint assertion fails. With the repaired
fixture, the same ordering passes: open a completed normal-output turn, verify
its output and idle state, and only then send `/e2e:empty-turn` through the
ready composer. The real agent completes without output and the original
hint assertion succeeds. Temporary diagnostic changes are restored after
each run.

The final unmodified candidate passes three repetitions with no retries or
skips. Blob reports verify the retained negative scenario executes successfully
in each repetition. Changed-file formatting and lint pass.

The original `does not show a hint when the turn produces output` case becomes
an exact-named first step in the retained positive case. Its mock-output,
idle-state and absent-hint assertions remain. Both scenarios now share one
task/session setup. The one-use seeding helper and its type-only imports are
removed. Command parsing, notice eligibility and message correlation retain
their independent unit coverage.

No product code, mock delay, timeout or retry configuration changes. This
repairs the reproduced fixture ordering problem; it does not add historical
notice replay or claim that every hosted failure cause is resolved. Complete
hosted validation remains required. Logs, original source, trace and
controlled RED/GREEN reports are retained under ignored `e2e-focus/empty-turn-*`.
