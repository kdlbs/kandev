---
status: draft
system: disambiguate-waiting
requirements:
  - REQ-DW-ORPHAN-001
  - REQ-DW-ORPHAN-002
---

# Orphaned background workloads system design

## Purpose and boundaries

This design records the evidence REQ-DW-ORPHAN-001 and REQ-DW-ORPHAN-002 were
written against, and the technical path that satisfies them. The acceptance
criteria under `../requirements/` are authoritative for behaviour.

Backend only, and inside `agentctl` only. No frontend change, no orchestrator
change, no WebSocket contract change, no schema change, no runtime flag.

## Prior art

**Our own prior reasoning — tool unavailable, in-repo sources used instead.**
`wiki-query` is not installed on this host: `which wiki-query wiki-switch`
returns "not found", there is no `wiki*` entry in `~/.claude/skills/` (57 skills)
or in the repository's `.claude/skills/` (35 skills), and `OBSIDIAN_VAULT_PATH`
and `QMD_COLLECTION` are both unset with no vault directory in `$HOME`. No
vault path or QMD collection could therefore be resolved, and this leg did not
run. The in-repository equivalent was read in full instead, and it carries the
decisive prior position: legacy `spec.md` section L, "What the agent's process
group actually contains — MEASURED, and it changes the design".

That section already measured and rejected the obvious design. Its conclusion is
contract: process-group **membership of the agent's group** answers a question
whose answer is permanently yes, because the ACP bridge, the Claude CLI and any
stdio MCP servers are unconditional members, and it simultaneously excludes the
only thing worth detecting, because the CLI puts each shell it spawns into its
own group. This design does not reopen that. What section L leaves open is the
**shell's own** group, which it measured but did not refute; the input inventory
below closes that question separately, and negatively.

**What other products shipped — tool unavailable.** The `saas-kb` MCP server and
its `search_fsm_docs` tool are not reachable from this session. No MCP servers
are configured in `~/.claude.json`, `~/.claude/settings.json` or
`.claude/settings.json`, and the only MCP tools exposed here are the
`mcp__kandev__*` family. No query was issued and this leg returned nothing. It
is named rather than skipped silently.

## Input inventory

Every shape below was sampled on the host, not inferred. Platform is
darwin 25.6.0 unless stated; the Linux rows were measured in
`debian:stable-slim` on kernel 6.12.76-linuxkit as uid 1000.

### The live session tree

Walking this session's own ancestry, which reproduces section L's structure on
current code:

```
38756 ppid=37602 pgid=38756  /bin/zsh -c ...          <- tool-call shell, OWN group
37602 ppid=37331 pgid=37141  claude                   <- Claude CLI
37331 ppid=37141 pgid=37141  node claude-agent-acp    <- ACP bridge
37141 ppid=37077 pgid=37141  npm exec claude-agent-acp<- agent, group leader, probe root
37077 ppid=1     pgid=37064  agentctl
```

The probe root is the agent process, `s.procMgr.AgentPID()`. The tool-call shell
is four hops below it and sits in its own process group, exactly as section L
reported.

### The defect

A temporary test in the `probe` package, run against the real platform reader and
then deleted, launched `/bin/sh -c 'sleep 45 & echo $!'` and let the wrapper
exit:

```
WORKLOAD  pid=35291 ppid=1 zombie=false startedAfterTurn=true
TESTPROC  pid=35289 (probe root)
IN DESCENDANT TREE OF PROBE ROOT: false
PROBE RESULT: "settled" (workload still alive and started in-turn)
```

The workload is alive, in-turn and not a zombie, and the probe reports `settled`.

### The capture window — why nothing can be captured at launch

An attested background shell in this session, launched through the real
`run_in_background` path, behaves as follows:

```
SHELL    pid=38830 pgid=38830      <- own group, as section L predicts
WORKLOAD pid=38843 ppid=1 pgid=38830 STAT=SN   <- reparented, group retained
wrapper shell 38830: GONE
```

The orphan keeps the shell's process group after the group leader dies, so a
process group **would** be a durable handle. The question is whether the group
can be learned while the shell still exists. It cannot. Measuring the wrapper
shell's lifetime over five runs of the exact failing pattern
(`/bin/sh -c 'nohup sleep 90 >/dev/null 2>&1 & echo $!'`):

| run | wrapper shell lifetime |
|---|---|
| 1 | 4.45 ms |
| 2 | 4.15 ms |
| 3 | 3.93 ms |
| 4 | 4.09 ms |
| 5 | 4.20 ms |

Mean 4.16 ms, max 4.45 ms. **Any design that has to observe the shell loses.**
The first probe sample is synchronous at turn settle (legacy D2), which is far
too late; and a mid-turn sampler would have to tick faster than 4 ms per session
to be reliable, which is not a sampling interval, it is a spin. This is the
measurement that rejects both "capture the shell's pgid when the tool call
executes" and "remember descendants seen live at an earlier sample". The
attestation itself carries no PID, and Kandev never spawns the shell — there is
no client-side ACP terminal in `internal/agentctl`, so the Claude CLI runs its
own Bash tool and Kandev only observes the notification.

### Platform capability

| Source | Linux | Darwin |
|---|---|---|
| Parent chain | `/proc/<pid>/stat` field 4 | `Eproc.Ppid` |
| Process group | `/proc/<pid>/stat` field 5 | `Eproc.Pgid` (`int32`) |
| Numeric session id | `/proc/<pid>/stat` field 6 | **unavailable** — `Eproc.Sess` is a `uintptr` kernel pointer, and `ps -o sess=` prints `0` for every process |
| Another process's environment | **readable** — `/proc/<pid>/environ`, same uid | **not readable** — `SysctlRaw("kern.procargs2", pid)` returns 29 bytes for another process and the environment is absent |

The Linux environment read was confirmed against a reparented orphan
(`ppid=1`): a token exported before the wrapper shell ran was found in
`/proc/<pid>/environ` after the shell exited. The Darwin result is the reason
REQ-DW-ORPHAN-002 exists rather than a platform-uniform guarantee.

### The identity already in place

A real Bash tool call in this session sees 18 inherited `KANDEV_*` variables,
among them `KANDEV_SESSION_ID`. Its value is byte-identical to the session id
this session was started with, which is the same identifier the probe request
already carries as `req.SessionID`.

The variable is injected on every executor path — `executor_standalone.go`,
`executor_kubernetes.go`, `manager_startup.go`, `manager_passthrough.go`,
`environment_resolution.go` — and is registered in
`internal/common/config/catalog.go` as a workspace injection with the reason
"session-owned child process context". No new variable is needed, and none is
added.

## Components and control flow

`probe.ProbeBackgroundWorkloads` gains a session identity argument and one new
pass. The flow, in order:

1. Read the process table in one snapshot, unchanged. A read error returns
   `unknown` (AC-DW-ORPHAN-002.3).
2. Confirm the agent process is present, unchanged. An absent root returns
   `unknown`.
3. Walk transitive descendants and test each against the truncated turn start,
   unchanged. If one qualifies, return `live` immediately — the orphan pass does
   not run and no environment is read (AC-DW-ORPHAN-002.5).
4. Otherwise, return `settled` without reading any environment if either
   precondition holds: the platform reader does not implement the
   environment-read capability (AC-DW-ORPHAN-002.1), or the session id on the
   probe request is empty (AC-DW-ORPHAN-002.4). Both are evaluable without
   touching a process, and both are checked before any candidate is considered.
5. Otherwise, for each process in the same snapshot that is not a zombie, is not
   already a descendant, is neither the agent process nor an ancestor of it, and
   started at or after the truncated turn start, read its environment and
   compare `KANDEV_SESSION_ID` against the probe's session id by exact string
   equality (AC-DW-ORPHAN-001.10).
6. On a match, and only on a match, re-validate the candidate's identity before
   returning: re-read that pid's start-time datum from the same platform source
   and require it to equal the datum the snapshot recorded for that pid. What is
   compared is the platform's own invariant datum, never a value re-derived from
   the current wall clock (AC-DW-ORPHAN-001.11). Equal data return `live`
   (AC-DW-ORPHAN-001.1). Two outcomes skip the candidate and continue the scan
   with the next one: a different datum, meaning the pid was recycled between the
   snapshot and the read; and a read that fails outright, meaning the candidate is
   gone (AC-DW-ORPHAN-002.2a). Neither is ever reported as `unknown` — that
   belongs to step 1 alone.
7. Otherwise return `settled`.

The agent process, the ACP bridge, the Claude CLI and any stdio MCP server all
carry `KANDEV_SESSION_ID` too. (So does agentctl on the containerized executors,
where it runs one server per session; in standalone mode a single control server
manages multiple agent instances and injects the variable per instance rather
than into its own environment. Nothing here depends on which, because
AC-DW-ORPHAN-001.9's exclusion is unconditional.) The descendants among them are
already governed by the start-time predicate, exactly as they are today. The
agent process and its ancestors are not descendants at all, so the identity
scan excludes them explicitly (AC-DW-ORPHAN-001.9) — without that exclusion an agent whose own
start time landed in the turn's truncation bucket would report `live` forever.
Exact-equality matching (AC-DW-ORPHAN-001.10) keeps one session on a shared host
from claiming another's workload.

**The agent's own environment is never read** (AC-DW-ORPHAN-002.4a). The match
compares a candidate against the session id already on the probe request, so the
agent's own variable is not an input to anything. An agent launched without
`KANDEV_SESSION_ID` therefore needs no special case: none of its descendants
carries one either, the scan matches nothing, and the descendant result stands.
An empty session id on the request is the one *request-carried* precondition
that skips the scan outright (AC-DW-ORPHAN-002.4), and it is evaluable without
touching any process.

**Computing the excluded set.** The agent's ancestors come from walking PPID
upward through the same snapshot: start at the agent process, follow each
entry's PPID to the entry carrying that PID, and stop on any of three
conditions — a PPID of 0 or below, no entry in the snapshot with that PID
(normal once a parent has exited), or a PID reached twice. The third is the same
cycle guard `transitiveDescendants` already carries against a racy snapshot and
it is required, not optional. The walk reads only the snapshot and never touches
a process outside it, so it adds no syscall and cannot fail.

**Why the match re-validates.** Legacy `spec.md` fixes the identity rule for this
whole feature: a process is identified by the pair (pid, start time), never by a
bare pid, because a recycled pid would otherwise inherit the wrong verdict. The
snapshot obeys that rule, but an environment is necessarily read by bare pid
afterwards, so a candidate that exits and has its pid reused between the two
could be matched on the dead process's start-time datum and the live process's
environment — and on this host the reused pid plausibly does carry the session's
`KANDEV_SESSION_ID`, since the agent spawns processes constantly. Re-reading that
datum closes that window. It costs one extra read per probe at most,
because re-validation runs only on a match and a passing match ends the scan.

The scan is a filter over one immutable snapshot, so its outcome cannot depend
on enumeration order (AC-DW-ORPHAN-001.5), and it holds no state between calls
(AC-DW-ORPHAN-001.6, AC-DW-ORPHAN-001.7). The re-validation read is the one
step that consults live state rather than the snapshot, and it does not weaken
that: a pid recycled before the pass began reports a start-time datum different
from the snapshot's no matter when it is read, so such a candidate is rejected
under every enumeration order. A process that exits *during* the pass can be observed
differently by two orderings, which is why AC-DW-ORPHAN-001.5 is scoped to a
process set whose liveness does not change while the pass runs.

The start-time predicate and its truncation are reused exactly as they are: this
design adds no second rule for deciding whether a process started in-turn
(AC-DW-ORPHAN-001.8). Re-validation is not such a rule. It never asks when a
process started relative to the turn; it asks only whether the process at a pid
is still the one the snapshot saw, which is why it compares a different value on
a different basis (AC-DW-ORPHAN-001.11). The existing zombie exclusion applies to
both passes (AC-DW-ORPHAN-001.4).

### Platform seam

Environment reading joins the existing `processTableReader` seam as an optional
capability. The Linux reader implements it over `/proc/<pid>/environ`. The
Darwin reader does not implement it, and the Windows build stays as it is: a nil
reader, `unknown` everywhere. When the capability is absent the orphan pass is
skipped entirely and the descendant result stands (AC-DW-ORPHAN-002.1).

The capability answers one question — does this pid carry this session id — and
it owns the parsing, because the blob's shape is the platform's business rather
than the probe's. On Linux `/proc/<pid>/environ` is a sequence of NUL-terminated
`NAME=VALUE` entries: split the blob on NUL, split each entry on its **first**
`=`, compare the name for exact equality with `KANDEV_SESSION_ID`, and compare
that entry's value for exact equality with the probe's session id. Never
substring-search the raw blob; `MY_KANDEV_SESSION_ID=...` must not match
(AC-DW-ORPHAN-001.10). If the name appears more than once the first entry
decides and later entries are ignored, so the answer never depends on how far
the scan ran. A trailing empty field after the final NUL is not an entry.

The same capability supplies the start-time re-read that re-validation needs,
since only the platform reader knows where a start time comes from. Ordering is normative:
read the environment first, then the start-time datum, then compare against the
snapshot. Read in that order, a pid recycled at any point before the comparison
yields a datum that differs from the snapshot's and is skipped.

**What re-validation compares, and what it must never compare.** The compared
value is the platform's own start-time datum: the number the kernel reports for a
process's creation and keeps reporting, unchanged, for that process's whole life.
It is **not** the probe's derived start time. The distinction is load-bearing on
Linux, the one platform where the identity pass runs today. There the derived
value is *boot anchor plus start ticks*, and the boot anchor is itself computed
from the current wall clock against `/proc/uptime`; two independent derivations
for one unchanged process therefore disagree by however far the anchor moved. An
implementation that compared derived values would reject **every** genuine match
and report `settled` for exactly the live orphan this capability exists to find.
The invariant datum is the raw `starttime` field of `/proc/<pid>/stat`, expressed
in clock ticks since boot and needing no anchor at all; on Darwin and any
BSD-shaped source it is the `p_starttime` timeval, which is already absolute.
The snapshot therefore retains, per process, whatever value this comparison
needs, and re-validation compares that retained value against a fresh read of the
same datum (AC-DW-ORPHAN-001.11).

This is deliberately a *different* comparison from the turn-start predicate, which
keeps using the derived start time and its truncation exactly as it does today
(AC-DW-ORPHAN-001.8). One compares a process against a moment; the other compares
a process against itself. A platform that implements the environment-read
capability must supply the invariant datum as well; a platform that supplies
neither never reaches the identity scan at all (AC-DW-ORPHAN-002.1), so no
platform is left able to match but unable to re-validate.

A re-validation read that fails outright is not a probe failure. It means the
candidate exited, which is the same answer as "does not contribute": skip it and
continue (AC-DW-ORPHAN-002.2a).

### Cost

The environment read happens only when the descendant walk found nothing, and
only for processes that started in-turn and are not already descendants. The
read is one `open`/`read`/`close` per candidate, bounded by the snapshot taken
in step 1; no second process-table enumeration is performed. Re-validation
adds one start-time read, and only on a match.

The candidate count is measured, not assumed, because the whole pass sits inside
the legacy 250 ms `KANDEV_PARKED_PROBE_BUDGET` (legacy D2, AC-40, AC-81) and
exceeding that budget yields `unknown`, which un-parks the session — the same
visible failure this capability exists to remove. Sampled on a developer host
busy enough to be running two agent sessions (`ps -eo pid,etime`): **770
processes in total, 13 of them started within the previous 5 minutes and 25
within the previous 60**. Candidates are a strict subset of that second number,
since descendants, zombies and the agent's own ancestors are all removed first,
so a realistic pass is tens of reads — on the order of a millisecond against a
250 ms budget, against a WebSocket round trip that already dominates it.

That measurement is why no cap, no in-probe deadline and no candidate ordering
are specified; the exclusion is recorded in the requirements' *Out of scope*.
The cost is also structurally self-limiting in the direction that matters: a
host with many in-turn processes is a busy host, and on a busy host the
descendant walk usually hits first and the identity scan never runs at all
(AC-DW-ORPHAN-002.5). One caveat for whoever revisits this:
`probe.ProbeBackgroundWorkloads` takes no `context.Context` and cannot be
interrupted mid-pass, so the budget bounds the orchestrator's call and not the
loop itself. Re-measure before assuming the numbers above still hold on a
materially different host.

## Failure and recovery

| Condition | Behaviour |
|---|---|
| Candidate exited between snapshot and environment read | Skip the candidate, continue. It is not live. |
| Candidate owned by another user | Skip the candidate, continue. It is not ours. |
| Environment unreadable for any other reason | Skip the candidate, continue (AC-DW-ORPHAN-002.2). |
| Platform cannot read environments at all | Descendant-only result, unchanged (AC-DW-ORPHAN-002.1). |
| Process-table read fails | `unknown`, unchanged (AC-DW-ORPHAN-002.3). |
| Probe session id on the request is empty | Descendant-only result; the identity scan is skipped and no environment is read (AC-DW-ORPHAN-002.4). |
| Agent launched without `KANDEV_SESSION_ID` | Descendant-only result, reached by ordinary non-matching rather than a check. No descendant carries the variable, so the identity scan runs and matches nothing (AC-DW-ORPHAN-002.4a). |
| Candidate's pid recycled between the snapshot and the environment read | The re-read start-time datum differs from the snapshot's, so the candidate is skipped rather than matched, preserving the legacy (pid, start time) identity rule. |
| Re-validation read fails outright (candidate exited before it could be read) | Skip the candidate, continue the scan. Never `unknown` (AC-DW-ORPHAN-002.2a). |
| Candidate is unchanged and still running, but its start time was re-derived instead of re-read | Rejected as a false mismatch, and the live orphan reads `settled`. This is the failure AC-DW-ORPHAN-001.11 forbids by fixing the comparison to the platform's invariant datum. |
| Orphan leaked by an earlier turn is still alive | Excluded by the start-time predicate (AC-DW-ORPHAN-001.2). It reads `live` only for the turn that started it, which is the same bound the legacy leaked-orphan row already accepts. |

A skipped candidate must not escalate to `unknown`. That is deliberate and it
does not contradict legacy D5's "never a shortened descendant set": D5 protects
the *descendant walk*, where a missing process could hide live work. Here the
opposite holds — an unreadable candidate is either gone or not ours, and both
mean "does not contribute".

## Permissions and security

No new access. The probe already runs where the agent runs and already reads the
whole process table. Reading `/proc/<pid>/environ` is restricted by the kernel to
the same uid, which is the uid agentctl already runs as.

`KANDEV_SESSION_ID` is not a secret and is not treated as one. It is already
exported into every process the agent starts, so this design exposes nothing that
was not already exposed. A process that copied the value could cause a false
`live`, which is the cheap direction and costs a stale spinner, consistent with
the leaked-orphan row already in the legacy table.

## Observability

The probe's existing warn-level log on error is kept. No new metric is
introduced: the projection this feeds is in-memory, un-persisted and already
un-instrumented, and a counter here would have no consumer.

## Requirement mapping

| Requirement | Satisfied by |
|---|---|
| REQ-DW-ORPHAN-001 | Components and control flow, steps 3 and 5 to 7; Platform seam |
| REQ-DW-ORPHAN-002 | Components and control flow, steps 3, 4 and 6, and "The agent's own environment is never read"; Platform capability; Platform seam; Failure and recovery |

## E2E decision

**No Playwright coverage.** The behaviour depends on real process reparenting on
the host running the agent, not on any UI path, and the parked affordance it
restores is already covered by the legacy specification's own criteria. A browser
test could not create the condition.

Coverage belongs in the `probe` package's real-process-tree suite,
`probe_realtree_test.go`, which is gated at runtime and runs for real on Linux in
CI. Three cases are required by the requirements and none exists today: a child
that runs a workload and exits before the sample must read `live`; a workload
left over from an earlier turn must not; and an unchanged, still-running
candidate must re-validate successfully after a delay long enough to move the
wall clock (AC-DW-ORPHAN-001.11).

The third case must live here and not in the fake-reader unit suite, and the
reason is the point of the criterion: `fakeProcessTableReader` serves stored
`processInfo` values, so a re-read through it returns a byte-identical start time
and the test passes whether or not the implementation re-derives. Only the real
platform reader exercises the derivation, so only a real-tree case can fail
against a re-deriving implementation.

## Build notes

`scripts/lint-spec-files.py` uses `dict | None` annotations and needs Python
3.10 or newer. The default `python3` on the development host used here is 3.9.6
and fails with a `TypeError` before linting anything; `/opt/homebrew/bin/python3`
(3.14.6) runs it correctly. CI is unaffected.
