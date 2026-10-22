# Nono design evidence

Recorded: 2026-10-22. This is design evidence, not full product acceptance.

## Branch and instance ownership

Own workspace: `/home/zeval/.kandev/tasks/integrate-nono-sandb_gnabelid/kdlbs-kandev`.
Own branch: `feature/integrate-nono-sandb-7tq`.
Parent: `feature/investigate-pi-agent-6ac`, head `fff43904827d8b44bf2a33439bce7fee022fc893`.
[PR #4538](https://github.com/kdlbs/kandev/pull/4538) was open at that head.
The worktree inventory proves that the parent uses another directory.
An ordinary fetch failed on an unrelated corrupt shared ref, `feature/lead-pr-4371-to-merg-eeb`.
An isolated bare fetch verified the remote head. The existing origin tracking ref matched it.
Only this clean task branch advanced to that commit. An ancestry check passed before edits.

The main instance on port 9998 and retained instances were not changed.
The seed manifest still points to `/tmp/kw-knv1ryrw`, port 48351, source `f8fbb6136539d0209bc75812023c59a6a097c101`.
Read-only SQLite connections and bounded log reads supplied the incident evidence.
No restart, queue edit, authentication change, or platform-task mutation occurred.

## Task 04 chronology

The task is `c4dd54ef-c1e6-4546-8ce2-1aa5a5b45871`.
The original session is `5ad7b57a-66a0-4089-a9e5-b529768bc4a7`.
The replacement is `1ba05f83-fef1-4f8f-950c-c0df4c861fe7`.
Times below use the retained log's UTC+01:00 offset on October 19.

| Time | Observed event | Interpretation |
| --- | --- | --- |
| 14:25:45 | ACP `session/new` returned authentication required | The adapter reached ACP. This is not evidence of generic wrapper admission failure. |
| 14:25:45 | Error became “The bootstrap operation could not be completed.” | The public projection lost actionable authentication detail. Raw diagnostics must still remain private. |
| 14:37:42 | `fresh_start` failed: no executor record | Recovery did not occur. |
| 14:37:54 | Prompt queued as runtime unavailable, then failed and requeued | A terminal bootstrap failure shared the awaiting-launch classification. |
| 14:38:10 | Second `fresh_start` failed with the same cause | The retry did not repair the original session. |
| 14:38:27 onward | New session ran the sales case | Independent replacement success only. |
| 16:39:42 | Stall detection reported no live execution | Consistent with an unresolved original session. |

At inspection, both sessions were `WAITING_FOR_INPUT` and the task was `REVIEW`.
Only the replacement had an `executors_running` row.
The original retained the bootstrap error and one queued message, with content length 376.
The queued prompt's content was not exported.
This proves stale retained error and queue state. It does not prove which banner a current browser renders.
Missing credentials, invisible credentials, and credential expiry remain distinct possible causes of the original authentication failure.
The retained log cannot distinguish them. Do not broaden filesystem policy on that evidence alone.

Current source still requires an executor record for ordinary waiting sessions in `resumeRequiresExecutorRecord`.
`attemptColdResume` classifies missing records as awaiting launch.
These are generic task/runtime boundaries, independent of Nono.
Reproduce them on the current branch before changing code because the retained build predates the parent head.

## MCP evidence

The retained Greyproxy policy has one allow rule: `chatgpt.com:443`.
It has no explicit Kandev MCP destination rule.
Stored session text references failed MCP connectivity, and the parent acceptance record documents that limitation.
Proxy request history contains permitted provider traffic, but no retained denied MCP request.
The missing route is a demonstrated policy gap, not a packet-level diagnosis of every connection failure.
The follow-up must distinguish transport support, generated MCP configuration, endpoint resolution, authentication, and network denial.

## Pinned Nono evidence

Release: [v0.80.0](https://github.com/nolabs-ai/nono/releases/tag/v0.80.0), published October 13.
Source commit: `8bb041356d48f771a221e58c98d9b1ff442e76fe`.
Linux x86_64 musl archive SHA-256: `cae6755f1188b561ea02f62995feb8ad87d457f6d5255956ec54f41bbefca31e`.
The archive matched the release's published checksum. This was checksum verification, not independent signature verification.
The downloaded binary reports `nono 0.80.0`.
It was extracted under `/tmp/kandev-nono-design-evidence`, without global installation or real credentials.

Primary sources read:

- [Session lifecycle](https://nono.sh/docs/cli/features/session-lifecycle.md).
- [Profile authoring](https://nono.sh/docs/cli/features/profile-authoring.md).
- [Flags](https://nono.sh/docs/cli/usage/flags.md).
- [Pinned approval protocol](https://github.com/nolabs-ai/nono/blob/8bb041356d48f771a221e58c98d9b1ff442e76fe/docs/protocols/approval-webhook-v1.md).
- [Pinned approval implementation](https://github.com/nolabs-ai/nono/blob/8bb041356d48f771a221e58c98d9b1ff442e76fe/crates/nono-cli/src/approval_runtime.rs).

`inspect --json` exposes session records and audit rollups. It is not the request/decision channel.
The pinned supervisor guide says webhook approval is planned.
The pinned implementation and the following binary experiments establish that file-capability webhooks work.

## Temporary binary experiments

All children used disposable homes, working directories, synthetic files, and no provider credentials.
A local HTTP listener used a dynamically assigned port and closed after each experiment.
`security.approval_backends` selected a webhook, and `security.approval_defaults.backend` selected its name.
The profile enabled capability elevation. The child had blocked network access.

| Experiment | Result |
| --- | --- |
| Piped stdin/stdout with `nono run --silent --block-net` | Exact `reply:probe` line, exit 0 |
| Denied read outside permitted work directory | One `capability` request, exit 1, marker absent |
| Granted same read in a new attempt | One request, exit 0, marker present |
| Invalid decision value | Exit 1, marker absent |
| Two-second deadline with three-second reply | Exit 1, marker absent |
| Static permitted write | Succeeded |
| Static read outside grant | Denied |
| Static write outside grant | Denied |
| Static loopback network attempt | Socket creation denied before connection |

The webhook request includes capability type, request ID, session ID, child PID, path, access, and reason.
The tested responses used `decision: granted` and `decision: denied`.
These tests establish a usable file-approval exchange. They do not prove callback authentication or Kandev integration.
A first network probe failed before printing its result because socket creation itself was denied.
The corrected probe catches that denial and reports it alongside filesystem outcomes.

Pinned `supervised_runtime.rs::should_open_supervised_pty` enables a PTY for detached starts or when all three standard descriptors are terminals.
Piped ACP therefore avoids a Nono PTY. Native terminal mode requires a separate nested-PTY acceptance test.
The generated private session records include distinct supervisor PID, child PID, session ID, state, and exit code.
The audit metadata is separate from those registry records.
Temporary probe scripts remain at `/tmp/kandev-nono-design-evidence/probe.py` and `probe-timeout.py`.
They contain synthetic fixtures only. Their HTTP listeners and child processes exited.
No actual Nono ACP chat, model discovery, native agent terminal, restart, or cancellation acceptance ran during this design turn.
No macOS, signed-platform enrollment, command approval, or endpoint approval support is claimed.

## Required next evidence

Task 01 creates the permanent opt-in acceptance harness with these probe cases and ownership checks.
Later work orders prove Kandev integration, callback isolation, MCP routing, policy composition, and both positive and negative enforcement.
The successful replacement sales run cannot satisfy any same-session recovery criterion.
