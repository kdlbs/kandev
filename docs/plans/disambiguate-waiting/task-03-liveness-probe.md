# Task 03: Background-workload liveness probe (agentctl, cross-platform)

Spec: §"Background-workload liveness probe (agentctl)", §"Start-time source and
resolution", D5, D6. ACs: AC-27a, AC-27b, AC-70, AC-70a, AC-71, AC-72, AC-80.

## Shape

```go
package probe // colocated with the agent process runner, agentctl side

type Result string
const (
    ResultLive    Result = "live"
    ResultSettled Result = "settled"
    ResultUnknown Result = "unknown"
)

func ProbeBackgroundWorkloads(agentPID int, turnStart time.Time) (Result, error)
```

Enumerates the **transitive descendant set** of `agentPID` (not the process
group — §L forbids group membership as the predicate) and applies the
start-time predicate from D5:

- identify each descendant by `(pid, start time)`, never bare pid;
- comparison is inclusive (`start_time >= truncated_turn_start`);
- truncate `turnStart` down to the platform source's resolution before
  comparing (Build decision F3: this is the ONLY comparison used anywhere in
  this feature — AC-27b/AC-70/AC-72's prose says "strictly before the recorded
  turn start" but that reading contradicts AC-80 and D5; Build implements the
  truncated form everywhere, see plan.md's F3 entry);
- zombies excluded on every platform;
- one snapshot; a process that exits mid-walk is absent; an incomplete walk
  (e.g. permission error partway through) yields `unknown`, never a
  shortened set;
- the agent process itself is never a member of its own descendant set.

## Per-platform start-time source

| Platform | Source | Resolution | Boot anchor (round-5 F8) |
|---|---|---|---|
| Linux | `/proc/<pid>/stat` fields 4 (`ppid`) and 22 (`starttime`, ticks since boot) | ≥10ms | `/proc/uptime` (system uptime in seconds, itself relative to `time.Now()` at read time — NOT `/proc/stat` `btime`, which is whole-second and reproduces the `ps lstart` failure mode). Anchor: `bootTime := time.Now().Add(-uptimeDuration)`; `procStart := bootTime.Add(ticks * (time.Second / clockTicksPerSecond))`. `clockTicksPerSecond` via `unix.SysconfClockTicks` or the `_SC_CLK_TCK` sysconf equivalent (hardcode 100 with a comment if no cgo-free syscall is available in this repo's Go version — verify during implementation). |
| Darwin/BSD | `sysctl KERN_PROC_ALL` → `kinfo_proc.kp_proc.p_starttime` (`timeval`, already wall-clock, same clock as `time.Now()`) | 1µs | none needed — already wall-clock |
| Windows | not implemented | — | always `unknown` |

`ps -eo lstart` MUST NOT be used on Darwin (whole-second resolution breaks the
predicate's stated failure direction). No `ps`-shelling anywhere — read the
kernel structures directly via `golang.org/x/sys/unix` (Linux) and
`golang.org/x/sys/unix` / raw `sysctl(3)` cgo-free syscall (Darwin). Check
`go.mod`/`go.sum` for `golang.org/x/sys` before adding a new dependency — it is
very likely already present transitively; if not, it is the minimal correct
addition (no full `gopsutil`).

## Descendant enumeration

Build the full `pid -> (ppid, start_time, is_zombie)` table for the host in one
pass (Linux: iterate `/proc/[0-9]+`; Darwin: `sysctl KERN_PROC_ALL` gives the
whole table already), then compute the transitive closure of `agentPID`'s
children via the `ppid` edges. A process whose ancestor chain cannot be
resolved before the table was captured (raced) is simply absent from the
closure — consistent with "one snapshot".

## Round-5 F8/F10/F12 dispositions (closed here per plan.md)

- Linux boot anchor named above (`/proc/uptime`, not `/proc/stat` `btime`).
- CI: these five ACs run as native Go tests under
  `internal/agentctl/.../probe/probe_realtree_test.go`, gated with
  `if runtime.GOOS != "linux" && runtime.GOOS != "darwin" { t.Skip() }` inside
  each test (not a build-tag skip, so the test still compiles and is visible
  as skipped in CI output). Darwin-specific assertions additionally skip when
  `runtime.GOOS != "darwin"`; there is no macOS runner in this repo's CI, so
  the Darwin path is only exercised when a developer runs `go test` locally on
  a Mac. This is recorded, not hidden.

## Tests

- Synthetic table tests using an injectable process-table reader (interface
  seam so tests do not depend on real `/proc` or `sysctl`) for the descendant
  closure and start-time predicate logic — covers AC-70 (all descendants
  pre-turn → `settled`), AC-70a (one descendant at/after turn start →
  `live`), AC-71 (own-process-group descendant still counted because probe
  uses descendant set, not group), AC-72 (pre-turn-only → `settled`, then a
  new post-turn-start descendant → `live`), AC-80 (truncation: a descendant
  one nanosecond after the turn start but in the same source-resolution tick
  as the (truncated) turn start counts as `live`; an implementation using
  `ps`-style whole-second truncation is what this guards against).
- **Real-process-tree tests** (task's own subprocess tree, not a mock):
  `probe_realtree_test.go` spawns real child processes with `os/exec`,
  measures their actual start times via the platform-specific reader, and
  re-drives AC-70/70a/71/72/80 against them. Linux runs in CI; Darwin is
  developer-local only (see above).
- AC-27a: a descendant that is a zombie (test harness must actually produce
  one — spawn+exit without `Wait()`, or inject via the seam) yields `settled`.
