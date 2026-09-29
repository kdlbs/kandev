# codexdbg

`codexdbg` is a local diagnostic command for the Codex app-server protocol. It starts a separate Codex process. It does not attach to a live Kandev session.

## Build

From the repository root, run:

```sh
make -C apps/backend build-codexdbg
apps/backend/bin/codexdbg --help
```

## Commands

Use `probe` to test startup and read model information. This command does not create a thread or start a model turn.

Use `prompt` to start a conversation. Use `thread-resume` to continue a thread that you name. Use `thread-fork` to fork through a completed turn that you name. These commands can send model requests.

Use `thread-read` to read a thread. Use `interrupt` to stop a prompt. Use `mcp-probe` to check the local MCP connection. Use `inspect` to read a saved capture without starting Codex.

## Captures

The command writes captures to `./codex-app-server-debug/` by default. It creates each file with owner-only access and does not replace an existing file. Captures can contain prompts and provider metadata. Keep them private.

Use `inspect --file PATH` to show request and response summaries. The inspector separates response, turn, and thread usage. It does not print raw frames or prompts.

Codex-reported thread estimates are not guaranteed billed charges. The command reports them separately from calculated prices.
