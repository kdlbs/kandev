# E2E Git launcher overhead

The worker fixture routes every backend Git command through a generated
launcher. Previously, even an ordinary command started Node, imported the
instrumentation module, parsed two absent control files, then spawned Git.
This overhead applies to checkout preparation, status and diff refreshes, and
other Git-heavy browser journeys. It is independent of the assertions' value.

On POSIX, the launcher now checks the two captured control-file paths on every
invocation. If both are absent, it restores the captured environment and
original PATH and directly execs Git with the original arguments and inherited
streams. If either exists, the existing Node implementation handles the
command. Checks happen at invocation time because tests create controls after
worker startup. The Windows launcher remains on the cross-platform Node path.
Missing launcher configuration also retains the previous path.

No test selection, browser worker count, mock delay, retry, timeout, production
behavior, Git command, or active instrumentation logic changes. Shell values
are quoted as data, including spaces, apostrophes and dollar characters.

## Proof and measurement

- Baseline: the five existing launcher and real-Git integration tests pass.
- RED: a regression makes inactive instrumentation throw, then requires the
  external Git process to receive exact arguments, cwd, captured PATH/control
  environment, stdin, stdout, stderr and exit status. The old launcher fails
  because it loads the inactive module. The candidate passes.
- Dynamic controls: both independently created control files still load the
  instrumentation after the launcher has been written. A mutation bypassing
  those checks makes both regressions fail; the source was restored byte for
  byte. Existing real diff gating and three global-option push interception
  cases now execute through the actual generated launcher.
- Eight focused tests pass. Windows execution is not claimed from Linux.
- Browser comparison: identical 26-case selection, four CPU affinity, 8 GiB
  container budget, two resource-guard-admitted one-worker processes and pinned
  CI image `ghcr.io/kdlbs/kandev-ci@sha256:61bc3395791d25639eadc108c691863e122f2710ecd7caf6916fca188ab71c04`.
  All six samples pass the same 26 full test identities, with no retries/skips.

| Merged base | Launcher | Execution after discovery (seconds) | Whole local invocation (seconds) |
| --- | --- | ---: | ---: |
| `6888c980d5` | Original | 78.639 | 101.348 |
| `6888c980d5` | Direct Git | 56.377 | 81.231 |
| `6888c980d5` | Original restored | 79.505 | 103.404 |
| `6888c980d5` | Direct Git repeat, overlapping typecheck | 50.672 | 71.979 |
| `3c1001d3d0` | Original | 71.477 | 91.922 |
| `3c1001d3d0` | Direct Git | 48.078 | 68.044 |

The latest same-source pair reduces execution by **32.74%** and the whole
local invocation by **25.98%**. Do not combine samples from different merged
bases into a causal speed estimate. The overlapping-typecheck sample remains
recorded but is excluded from the isolated timing comparison. This is a shared
host, not a dedicated runner or a statistical fleet estimate.

Twelve desktop/mobile cases pass against base `6888c980d5`: long fetch
preparation, both diff renderers with a real Git gate, and GitLab MR creation
with push interception. Fresh web build, eight focused unit tests and typecheck
also pass against synthetic merge `5b41199663` with live main `3c1001d3d0`.
The same twelve control cases also pass on that refreshed build, with no
retries/skips. Candidate source hashes match the primary checkout exactly.

Raw reports, exact-identity comparison, source overlay hashes, logs and the
failed probe are retained under `apps/backend/.build/ci-half/e2e-focus/`.

A separate `GOMAXPROCS=1` experiment was rejected: the original setting passed
26 cases, while the lower setting failed workspace attachment and took longer.
It is not included in this change. Local samples cannot establish the full CI
50% target; that requires a passing complete hosted run at the candidate head.
