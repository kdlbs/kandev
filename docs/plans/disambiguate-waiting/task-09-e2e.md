# Task 09: E2E coverage

Spec: §"Notes for implementation" (injectable probe seam), AC-73.

## Injectable probe seam

AC-62, AC-68, AC-73 need the probe's answer to change mid-test without a real
12-minute wait. Add a test-only override path (mirroring how the mock agent /
`KANDEV_E2E_MOCK` profile already injects scripted behaviour elsewhere) that
lets an E2E spec drive `BackgroundProbe.Probe` through a scripted sequence for
a given session. Keep this entirely inside the e2e/mock profile gate — it
must not be reachable in `prod`.

## Scenarios

- AC-73: three consecutive `live` samples then `settled`, with a self-resume
  on the transition — assert the background test id renders at every `live`
  sample and neither `waiting-for-input` nor `turn-finished` renders while
  parked (board and sidebar).
- AC-62: `live → settled` transition clears the affordance and the task row
  updates without a page reload (WS-driven).
- AC-68: agent self-resumes (session leaves `WAITING_FOR_INPUT`) while the
  probe is still `live` — affordance clears immediately.
- AC-51/AC-34: session switcher and sidebar precedence — parked session with
  and without a concurrent pending clarification/permission.

Real process-tree ACs (AC-70/70a/71/72/80) are covered by task-03's Go tests,
not E2E — no real 12-minute-shell E2E scenario is required by the spec.

Follow the repo's e2e conventions (`apps/web/e2e/README.md`); this feature
does not need the `containers` project (no Docker/SSH executor dependency).
