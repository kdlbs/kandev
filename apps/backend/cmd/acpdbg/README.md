# acpdbg

Headless CLI that speaks raw ACP JSON-RPC to any registered kandev agent (or an arbitrary binary) and records every wire frame to a JSONL file. Useful for answering "what models/modes does this agent advertise?", "does `session/load` work?", or "what does the prompt stream actually look like?" without spinning up the full kandev backend.

## Build

```bash
make acpdbg ARGS=list              # from repo root
make -C apps/backend build-acpdbg  # just build
```

Binary lands at `apps/backend/bin/acpdbg`.

## Usage

```bash
make acpdbg ARGS="<subcommand> [flags]"
```

Or invoke the binary directly: `apps/backend/bin/acpdbg <subcommand> [flags]`.

### Sub-commands

| Command | Purpose |
|---|---|
| `list` | Enumerate registered ACP agents with their spawn command |
| `probe <agent>` | `initialize` → `session/new` → close. Shows models, modes, auth methods. |
| `probe --exec "<cmd> [args...]"` | Same but against an arbitrary binary not in the registry |
| `prompt <agent> --prompt "..." [--model M] [--mode M]` | Full prompt round-trip, collects text chunks from `session/update` |
| `session-load <agent> --session-id <id>` | Reproduce `session/load` for an existing session |
| `matrix` | Probe every registered ACP agent in parallel, write one JSONL per agent + `matrix-summary.json` |

### Shared flags

- `--out DIR` — JSONL output directory (default `./acp-debug/`)
- `--file PATH` — exact JSONL path, overrides `--out`
- `--timeout DUR` — overall run timeout (default `30s`; use `60s+` for `matrix` since `npx`-spawned agents are slow to cold-start)
- `--workdir PATH` — child cwd (default: fresh `/tmp/kandev-acpdbg-<pid>-*`)
- `--verbose` — mirror frames to stderr as they're sent/received
- `--stderr` — capture child stderr into the JSONL (useful when an agent crashes before the handshake completes)

## JSONL format

One JSON object per line, chronologically ordered. `direction` is one of:

| `direction` | Meaning |
|---|---|
| `meta` | acpdbg marker (`event: start` with agent/command/workdir, `event: close` with exit_code/reason) |
| `sent` | Frame written to child stdin |
| `received` | Frame read from child stdout |
| `stderr` | Child stderr line (only with `--stderr`) |

### jq recipes

```bash
# Full initialize response
jq -c 'select(.direction == "received" and .frame.id == 1)' acp-debug/<file>.jsonl

# Just the advertised models
jq -r 'select(.direction == "received") | .frame.result.models.availableModels[]?.modelId' acp-debug/<file>.jsonl

# Close event (exit code + reason)
jq -c 'select(.direction == "meta" and .event == "close")' acp-debug/<file>.jsonl
```

## Design notes

- **Agent registry is the source of truth.** `acpdbg` imports `internal/agent/registry` directly, so adding/removing agents in `internal/agent/agents/*.go` automatically updates the CLI. No parallel config file.
- **Raw JSON-RPC, not the SDK.** Uses a custom line-delimited framer (`internal/agent/acpdbg/framer.go`) so the recorded frames are authoritative wire bytes, not SDK-parsed events.
- **Env inheritance.** The child process inherits the parent shell's env and credential files — if it works in kandev it works here. Auth failures are captured in the JSONL.
- **Agent-initiated requests** (`fs/read_text_file`, `session/request_permission`, etc.) are auto-replied to with `-32601 method not found` so sessions don't hang. For real permission flows, use the full kandev backend.

See `.agents/skills/acp-debug/SKILL.md` for the agent-facing usage guide.
