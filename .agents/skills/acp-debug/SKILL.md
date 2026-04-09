---
name: acp-debug
description: Debug an ACP agent CLI by spawning it, speaking raw JSON-RPC, and capturing every frame to a JSONL file. Use when the user asks to probe an agent's capabilities, compare agents, test a prompt against an agent, inspect raw ACP wire frames, or investigate why an agent fails to initialize.
---

# ACP Debug

Run a headless ACP JSON-RPC session against any registered kandev agent (or an arbitrary command), record every wire frame to a JSONL file, and summarize the handshake. Backed by the `acpdbg` binary at `apps/backend/bin/acpdbg`.

## When to use this skill

- "what models does auggie advertise"
- "probe claude-acp" / "probe all agents" / "run the matrix"
- "debug why copilot-acp isn't starting"
- "what does a `session/new` response actually look like for X"
- "try this prompt against auggie with mode=ask"
- "reproduce session/load for session <id> against claude-acp"
- User mentions inspecting raw ACP wire payloads or a JSONL file from an earlier run

## Before anything else: build the binary if missing

```bash
test -x apps/backend/bin/acpdbg || make -C apps/backend build-acpdbg
```

## Sub-commands

```text
acpdbg list                                            # enumerate registered ACP agents
acpdbg probe <agent>                                   # initialize + session/new + close
acpdbg probe --exec "<cmd> [args...]"                  # probe an arbitrary binary not in the registry
acpdbg prompt <agent> --prompt "..." [--model M] [--mode M]
acpdbg session-load <agent> --session-id <id>
acpdbg matrix                                          # probe every ACP agent in parallel
```

**Shared flags** (apply to every sub-command):
- `--out DIR` — JSONL output directory (default `./acp-debug/`)
- `--file PATH` — exact JSONL path, overrides `--out`
- `--timeout DUR` — overall run timeout (default `30s`)
- `--workdir PATH` — child cwd (default: fresh `/tmp/kandev-acpdbg-<pid>-*`)
- `--verbose` — mirror frames to stderr
- `--stderr` — capture child stderr into the JSONL

## Steps

**Create a task for each step below and mark them as completed as you go.**

### 1. Pick the sub-command that matches the user's intent

- "what models does X have" / "does X support modes" → `acpdbg probe X`
- "test a prompt against X" → `acpdbg prompt X --prompt "..."`
- "compare all agents" / "run the matrix" → `acpdbg matrix`
- "resume session <id>" → `acpdbg session-load X --session-id <id>`
- "try this random binary" → `acpdbg probe --exec "path/to/bin --acp"`

### 2. Run the command

Always capture stdout — it contains the JSONL file path (and for `matrix`, the summary table and `matrix-summary.json` path).

```bash
apps/backend/bin/acpdbg probe auggie --timeout 45s
```

For `matrix`, prefer `--timeout 60s` so `npx`-spawned agents have time to cold-start.

### 3. Read the JSONL file

The JSONL schema is:

| `direction` | Meaning | Extra fields |
|---|---|---|
| `meta` | acpdbg-generated marker | `event` (`start` / `close`), `meta` (map with `agent`, `command`, `workdir` for start; `exit_code`, `reason` for close) |
| `sent` | Frame written to child's stdin | `frame` (JSON-RPC request or reply) |
| `received` | Frame read from child's stdout | `frame` (JSON-RPC request, response, or notification) |
| `stderr` | Child stderr line (only when `--stderr`) | `line` |

Entries are strictly chronological. Each line is a single JSON object terminated by `\n`.

Useful `jq` recipes:

```bash
# Full initialize response
jq -c 'select(.direction == "received" and .frame.id == 1)' acp-debug/<file>.jsonl

# Full session/new response (models, modes, auth)
jq '.frame.result' acp-debug/<file>.jsonl | head -50

# Just the models advertised
jq -r 'select(.direction == "received") | .frame.result.models.availableModels[]?.modelId' acp-debug/<file>.jsonl

# Close event (exit code + reason)
jq -c 'select(.direction == "meta" and .event == "close")' acp-debug/<file>.jsonl

# All agent-initiated requests we auto-replied to
jq -c 'select(.direction == "received" and .frame.method and .frame.id)' acp-debug/<file>.jsonl
```

### 4. Summarize for the user

Give a concise markdown summary: agent, protocol version, models found (with currentModelId), modes found (with currentModeId), auth methods, any errors, and the JSONL path for deeper inspection. Example:

```text
**auggie** (protocol v1, auggie 0.20.1)
Models (11): claude-sonnet-4-6 (current), claude-opus-4-6, gpt-5-4, …
Modes  (2):  default (current), ask
Auth methods: (none advertised)
JSONL: acp-debug/auggie-probe-20260409-183104.jsonl
```

### 5. If something looks wrong, walk the frames

Common failure modes:

| Symptom | Likely cause | Next step |
|---|---|---|
| meta close `exit_code: 127` immediately | child binary not installed | Check `which <cmd>`; suggest install command |
| meta close before any `received` frame | child crashed on startup | Re-run with `--stderr` to capture the error |
| `initialize` response has populated `authMethods` but session/new fails | auth required | Surface the auth method ids; suggest setting env var / running CLI login |
| `session/new` hangs (context deadline exceeded) | agent waiting on an unanswered agent-initiated request | Check JSONL for `received` frames with `method` + `id` that we auto-replied to with `method not found` — the agent may be retrying |
| Response has no `models` / `modes` fields | agent doesn't expose them over ACP | Not a bug — document the gap |

Re-run with `--stderr` whenever the child exits before the handshake completes; the stderr lines land in the JSONL and usually contain the root cause.

### 6. For `matrix`, read `matrix-summary.json` too

```bash
jq '.' acp-debug/matrix-summary.json
```

One entry per agent with `status`, `models_count`, `current_model_id`, `auth_methods_count`, `duration_ms`, and `jsonl` (the full per-agent JSONL file). Useful for answering "which agents succeeded / failed / need auth" without re-reading every JSONL.

## What this skill does NOT do

- No interactive UI. For ad-hoc side-by-side comparison, read the `matrix-summary.json` output or build a separate visualization tool on top of the JSONL.
- No permission-request handling beyond canned replies. Agent-initiated requests (`fs/read_text_file`, `session/request_permission`, etc.) are answered with `-32601 method not found` so the session doesn't hang. If you need to exercise a real permission flow, use the full kandev backend.
- No automatic credential bootstrap. The child inherits the parent shell's env; if an auth check fails the skill reports which methods were advertised and lets the user fix their local credentials.
- No Docker / remote executor support. Standalone subprocess only — same as a manual `auggie --acp` invocation.
