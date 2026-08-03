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
| `probe [flags] <agent>` | `initialize` → `session/new` → close. Shows models, modes, auth methods. |
| `probe --exec "<cmd> [args...]"` | Same but against an arbitrary binary not in the registry |
| `mcp-probe [flags] <agent>` | Inject a temporary HTTP MCP sentinel into `session/new` and report delivery separately from observed initialize, `tools/list`, and tool use. |
| `prompt --prompt "..." [--model M] [--mode M] <agent>` | Full prompt round-trip, collects text chunks from `session/update` |
| `session-load --session-id <id> [--prompt "..."] <agent>` | Load an existing session in `--workdir`, then optionally verify it with a prompt |
| `matrix` | Probe every registered ACP agent in parallel, write one JSONL per agent + `matrix-summary.json` |

> **Flags go before the agent name.** Parsing uses the standard library `flag`
> package, which stops at the first non-flag argument — so
> `session-load opencode-acp --session-id X` silently drops `--session-id` and
> fails with `--session-id is required`.

### Shared flags

- `--out DIR` — JSONL output directory (default `./acp-debug/`)
- `--file PATH` — exact JSONL path, overrides `--out`
- `--timeout DUR` — overall run timeout (default `30s`; use `60s+` for `matrix` since `npx`-spawned agents are slow to cold-start)
- `--workdir PATH` — child cwd (default: fresh `/tmp/kandev-acpdbg-<pid>-*`)
- `--verbose` — mirror frames to stderr as they're sent/received
- `--stderr` — capture child stderr into the JSONL (useful when an agent crashes before the handshake completes)

### Verify resume under a different working directory

Create a session in one directory, then load it from another and ask the agent
to report its working directory:

```bash
apps/backend/bin/acpdbg prompt \
  --workdir /tmp/folder-a \
  --prompt "Reply with pwd only" \
  --file /tmp/codex-new.jsonl \
  codex-acp

apps/backend/bin/acpdbg session-load \
  --session-id <session-id-from-first-run> \
  --workdir /tmp/folder-b \
  --prompt "Reply with pwd only" \
  --file /tmp/codex-load.jsonl \
  codex-acp
```

`session-load` sends the selected `--workdir` as the ACP `cwd` parameter. The
JSONL wire frames are authoritative when a provider replays earlier session
messages during load.

### Probe MCP attachment

```bash
apps/backend/bin/acpdbg mcp-probe --timeout 45s auggie
```

The summary lists the agent's advertised HTTP/SSE capability and the sentinel
milestones separately. `sentinel_delivered: true` means `session/new` accepted
the injected configuration. `initialize_observed` and `tools_list_observed`
are direct traffic seen by the sentinel. If the result is `unobserved`, the
agent did not contact the temporary endpoint during the bounded probe window;
that is not a portable failure because agents may attach lazily or use a
different transport. Sentinel JSONL metadata includes only opaque connection
IDs and timestamps, while ordinary raw frames remain available because this is
an explicit developer command.

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
