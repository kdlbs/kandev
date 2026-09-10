# Kandev Instance Debugging

Use this for live-instance state/logs or when a UI/browser repro needs an isolated app.

## Identify Instances

```bash
scripts/kandev-instances
```

Columns: `PID BACKEND_PORT WEB_PORT AGENTCTL_PORT HOME_DIR REPO_PATH`.

The user's live instance usually has `HOME_DIR=/home/<user>` or backend port `38429`. Never mutate it. Creating and downloading an owned diagnostic bundle is read-only.

## Launch Isolated Instance

Use an isolated instance for browser/UI repros and API probing that mutates data:

```bash
# Backend only:
scripts/dev-isolated

# Backend + web frontend:
scripts/dev-isolated --web
```

On a clean checkout, pass `--install` or run `make install` once so frontend dependencies exist.

`dev-isolated` prints a `READY` block with ports, log paths, pidfile, and teardown command. Save the backend/web port and pidfile.

## Diagnostic bundles

Inside a Kandev task session, request only the evidence needed:

```text
get_diagnostic_bundle_kandev source=backend
get_diagnostic_bundle_kandev source=frontend
get_diagnostic_bundle_kandev source=all
```

The tool returns an executor-local ZIP path. For a host-side instance, use:

```bash
scripts/kandev-logs <port> --source backend
scripts/kandev-logs <port> --source frontend
scripts/kandev-logs <port> --source all
```

When authentication is enabled, export a personal access token through
`KANDEV_API_TOKEN`; never place it in an argument or print it. No token is
needed when authentication is disabled.

Use the smallest source set that can answer the question. `backend` covers the
retained daily files; `frontend` asks connected tabs for their bounded local
console history. The task-session bundle currently accepts only the source
values exposed by its active schema (`backend`, `frontend`, or `all`); do not
assume that `runtime` or `acp` is available. Standard evidence does not include
stored chat/session/agent messages. For a delivery-versus-visibility question,
identify the exact sender and receiver sessions first, then use authorized task
conversation or ACP evidence when that source is available. Inspect
`manifest.json` warnings before drawing conclusions; if the active diagnostic
tool does not expose the requested chat/ACP source, report protocol evidence as
unavailable instead of inferring delivery from ordinary logs.

Extract into a newly created temporary directory, never a repository, home
directory, workspace root, or reused path. Reject ZIP entries that are
absolute or contain `..` before extraction. Inspect `manifest.json` first:
`partial` status, warnings, byte ranges, and loss counters mean a requested
source may be incomplete.

```bash
diagnostic_tmp="$(mktemp -d)"
if unzip -Z1 "$diagnostic_zip" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then
  echo "unsafe diagnostic ZIP paths" >&2
  exit 1
fi
unzip -q "$diagnostic_zip" -d "$diagnostic_tmp"
jq . "$diagnostic_tmp/manifest.json"
```

If a task ID is known, search it exactly before broadening. For a host-side
instance this is often faster than extracting a new ZIP:

```bash
rg --fixed-strings '<task-id>' '<extracted-directory>'
rg --fixed-strings '<session-id>' '<extracted-directory>'
rg --fixed-strings '<task-id>' '<HOME_DIR>/logs' -g 'backend-logs*.log'
rg --fixed-strings '<session-id>' '<HOME_DIR>/logs' -g 'backend-logs*.log'
```

A zero-match task-ID search is inconclusive: install-wide backend events and
browser pages outside recognized task routes may not carry `task_id`. Next
search a precise route/error string or a bounded timestamp window.

## Stalled launch triage

When a session stays `RUNNING` but produces no agent output, do not treat a
responsive browser or WebSocket as proof that the prompt reached the agent.
Correlate three independent facts for the exact session and time window:

1. Retained runtime state: current execution, turn, prompt generation, and
   session status.
2. Backend logs: launch admission, attachment claim/materialization, prompt
   dispatch, and any asynchronous error or terminal event.
3. Raw and normalized ACP frames for that authorized session, when protocol
   evidence is necessary.

No prompt frame means the failure happened before ACP admission. A prompt frame
without a terminal frame points to transport or provider settlement. A terminal
frame with durable state still `RUNNING` points to lifecycle reconciliation.
For staged attachments, trace upload, claim, materialization, and prompt
dispatch separately; an uploaded descriptor is not proof that launch claimed
it. Finally, check whether an asynchronous error only reached a log line or
also entered the correlated terminal execution path.

Backend files also live under `<HOME_DIR>/logs/backend-logs.log` with the two
previous UTC daily archives. Prefer a fresh bundle because its manifest records
completeness and includes requested browser evidence.

## Teardown

Tear down only instances you launched:

```bash
scripts/kandev-kill --pidfile /tmp/kandev-dev-isolated-<port>.pid --yes
# or:
scripts/kandev-kill <your_port> --yes
```

`kandev-kill` refuses protected ports without `--force`. Never use `pkill kandev`.
Remove only extraction directories you created.
