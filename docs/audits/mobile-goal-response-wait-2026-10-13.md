# Mobile goal response readiness

Run `38439998051` at `4ae0f5d` finished with one failed E2E case:
`mobile-agent-goal.spec.ts` / `opens the active goal in a touch drawer and
clears it`. The trace and screenshot show the clear prompt in the transcript,
the composer in its busy state and the agent still running when the default
five-second response assertion expires. Preceding goal turns in the same trace
take approximately five to six seconds.

Use the same 30-second response readiness allowance already present for the
active and completed goal responses. Preserve the exact clear-response text,
subsequent direct-input readiness, hidden goal chip, real touch drawer checks,
all submissions and the file's zero-retry policy. This increases only the
maximum wait for an asynchronous provider result; it adds no fixed sleep and
does not change product timeouts.

The complete two-case file passes in the pinned CI image with retries disabled.
Its delayed-acknowledgement single-submission regression also passes. Logs and
blob evidence are retained in ignored `e2e-focus/runtime-summary-candidate-*`
and `storage-disk-baseline-*`. Other experimental notification edits in that
first run were rejected and restored; they are not part of this repair.
