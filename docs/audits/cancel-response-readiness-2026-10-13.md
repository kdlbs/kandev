# Quick Chat response readiness after cancellation

Run 38439998051 retries both desktop and phone palette cancellation cases at
the final `Done. Emitted 2 messages` assertion. Both snapshots show the new
prompt and an agent still running four seconds into that turn. The desktop
backend log records prompt acceptance at 17:54:42.885Z and new agent messages
at 17:54:48.403Z, after the default five-second assertion window. Its prior
cancellation escalated before the next prompt was accepted; that evidence is
retained rather than being described as a clean cancellation.

Give only those asynchronous response assertions a 30-second maximum wait,
consistent with the existing response-readiness checks. Keep exact expected
text, every cancellation-pending and disabled-button assertion, the underlying
task's continued generating state, mobile touch target and overflow checks,
and all original scenarios. No fixed delay, retry increase, product timeout,
or shortcut around recovery is added. A response that never arrives still fails.

The complete desktop and phone files pass all five cases in the same CI
runtime image, one worker and retries disabled (2.3 minutes). Hosted failures
provide the failing evidence; this local run does not prove a full-suite
speedup or resolve every underlying cancellation/recovery observation. Source
snapshots, logs and blobs remain in ignored `e2e-focus/cancel-*`.
