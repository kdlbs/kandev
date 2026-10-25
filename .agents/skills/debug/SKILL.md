---
name: debug
description: Diagnose Kandev bugs, running-instance issues, UI/browser failures, and runtime behavior. Use when the user reports unexpected behavior, asks to investigate, asks to add logs/instrumentation, or when a fix needs root-cause evidence before implementing. Triage first, gather evidence safely, then hand off to /fix for code changes.
allowed-tools: Bash(curl:*) Bash(jq:*) Bash(mktemp:*) Bash(unzip:*) Bash(pnpm:*) Bash(scripts/kandev-instances:*) Bash(scripts/kandev-logs:*) Bash(scripts/dev-isolated:*) Bash(scripts/kandev-kill:*) Bash(go:*) Bash(rg:*) Bash(grep:*)
---

# Debug

Diagnose efficiently and safely. Debugging produces evidence and a root-cause hypothesis; `/fix` turns that into a regression-tested patch.

## Planner Entry

Perform triage, evidence gathering, and diagnosis directly in the primary
conversation. Keep production edits out of the diagnostic phase, then proceed
through `/fix` when code changes are needed.

## First: Create The Pipeline

Create a visible task list:

1. **Triage** - classify the bug and choose the cheapest faithful path
2. **Gather evidence** - targeted test, source-selectable diagnostic bundle, browser state, or instrumentation
3. **Diagnose** - trace the failure to root cause
4. **Report** - summarize evidence and choose `/fix` when code changes are needed
5. **Clean up** - remove temporary logs, throwaway repro tests, isolated instances, and browser sessions

## Triage Gate

Pick one path before launching anything:

| Class | Signals | Reference |
|---|---|---|
| Backend logic | validation, dedup, data shaping, workflow routing, API/service behavior | `references/backend-repro.md` |
| Live instance | user has a running instance already misbehaving and you need read-only state/logs | `references/instance.md` |
| UI/browser | layout, focus, click flow, WS-driven UI, console/network behavior | `references/browser.md` plus `references/instance.md` |
| Needs logs | current evidence is insufficient and instrumentation is needed | `references/instrumentation.md` |

Rules:
- Triage before launching anything.
- Use logs and targeted tests before browser automation.
- Never mutate the user's live instance. Creating or downloading an owned diagnostic bundle is read-only; browser interaction must use your isolated instance.
- Tear down only what you started. Never `pkill kandev`.

## Evidence Strategy

Start with the cheapest faithful reproduction:

1. Backend logic: write a throwaway focused Go repro test against the real service path. If it reproduces, convert it via `/fix`.
2. Live instance in a task session: call `get_diagnostic_bundle_kandev` with `backend`, `frontend`, or `all`; inspect `manifest.json` before assuming a source is complete.
3. Host-side instance: use `scripts/kandev-logs <port> --source backend|frontend|all`; do not relaunch. Set `KANDEV_API_TOKEN` only when authentication is enabled.
4. UI/browser: launch `scripts/dev-isolated --web`, drive
   `pnpm --dir apps exec playwright-cli`, and correlate console/network state
   with a fresh all-source bundle.
5. Unknown: trace from the symptom backward through code and add temporary instrumentation only where it will split the search space.

When logs show repeated calls to the same endpoint or action, classify each
request by transport, method/action, query or body cursor, caller, and cadence
before diagnosing a loop. A periodic newest-window refresh and cursor
pagination can share a route while serving different purposes. Verify
pagination by capturing the directional cursor request and its response. If no
such request exists, investigate the UI trigger or lifecycle; repeated
uncursored refreshes do not prove that the server failed to advance a cursor.

Before any restart or mutating reproduction, capture the baseline diagnostic
bundle and record the exact database and log pointers. Treat the live database
as latest state, not historical evidence; preserve the baseline and reconstruct
the lifecycle from timestamped logs, events, and transition tables before
comparing later state.

For a GitHub merge-queue ejection, a timeline removal event is not the cause.
Inspect the ruleset's `check_response_timeout_minutes`, the current
`mergeQueueEntry`, and `merge_group` runs. Record the synthetic run/job IDs,
`head_sha`, start/end timestamps, configured workflow/job timeout, and logs
before classifying the failure as CI capacity or changing product code. Use the
PR-fixup merge-queue and CI-troubleshooting references for the query details.

For workflow-routing failures, inspect the workflow's `on` activity types,
job-level `if` gates, permissions, exact PR and head SHA, and the actor that
added a label. Verify token-trigger behavior against GitHub's
[GITHUB_TOKEN documentation](https://docs.github.com/en/actions/concepts/security/github_token):
events created with `GITHUB_TOKEN` generally do not create another workflow run,
and a label-only trigger does not cover a later `synchronize` update. Confirm
the observed run and event rather than inferring that a missing run means the
workflow logic was skipped.

### File-first log triage

Start with the retained backend files before asking for a broad export. Each
Kandev home has `logs/backend-logs.log` plus the two preceding UTC daily files
(`backend-logs-YYYY-MM-DD.log`). The active file appends across same-day
restarts and each daily file is bounded, so search the exact files rather than
loading an entire log into memory:

```bash
rg --fixed-strings '<task-id>' '<home>/logs' -g 'backend-logs*.log'
rg --fixed-strings '<session-id>' '<home>/logs' -g 'backend-logs*.log'
```

Prefer a task ID, session ID, or exact route/error string. Add a bounded time
window only after the exact search; do not use a broad `rg` over the whole home
directory because task workspaces and ACP files can contain unrelated private
content. A zero-match task search is inconclusive when the event is an
install-wide startup/API event.

Request only the needed bundle sources. Standard bundles contain backend and
frontend diagnostic events; a custom bundle can add the allow-listed runtime
index. These sources do not read stored chat transcripts, session messages, or
agent messages. If the
maintainer explicitly needs agent protocol evidence, use the debug-only ACP
source and select the exact authorized sessions; ACP raw/normalized frames may
contain prompts, responses, tool calls, file/MCP data, environment-derived
values, and secrets. Always inspect `manifest.json` and its warnings before
assuming a source is complete, and grep task/session IDs inside the extracted
ZIP before broadening to route text or timestamps.

**Cancellation intent is separate from event serialization:** A generic
per-session event-serialization mutex only orders work; it is not evidence
that cancellation was requested. Model cancellation intent with separate state
or a refcount, and mark it only around real cancellation operations. During
concurrency debugging, inspect that state independently before attributing a
queued or dropped event to cancellation.

**Provider diagnostics:** Raw agent stderr may contain URLs, IDs, subscription
details, or other sensitive runtime data. Inspect it only in memory, sanitize it
before writing to generic logs, ring buffers, process-exit errors, persistence,
or the UI, and ensure bounded diagnostic consumers cannot block subprocess
stderr draining.

## Reference Files

Load only the reference needed for the selected path:

- `references/backend-repro.md` - targeted Go repro tests and backend-first debugging.
- `references/instance.md` - instance discovery, isolated launch, logs/export, and teardown.
- `references/browser.md` - workspace-pinned `playwright-cli` browser debugging against isolated instances.
- `references/instrumentation.md` - temporary vs persistent frontend/backend logging rules.

## When To Use Instrumentation

Use `references/instrumentation.md` before adding:
- `console.log`
- `logger.Warn("[DEBUG] ...")`
- `createDebugLogger(...)`
- backend `logger.Debug` / `logger.Info` for persistent diagnosis

Temporary logs must be stripped before `/commit` or `/pr`. Persistent instrumentation stays only when it has ongoing diagnostic value.

## Hand Off To Fix

When you can state:
- what fails,
- where it fails,
- why it fails,
- how to reproduce it,

then stop debugging and proceed through `/fix` in the same primary conversation.

## Final Report

Report:
- Bug class selected
- Evidence gathered
- Root cause or strongest hypothesis
- Suggested fix path and files
- Cleanup performed
- Any remaining unknowns
