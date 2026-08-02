# Browser Debugging

Use this only for UI, layout, focus, WS-driven, console, or click-flow bugs that cannot be faithfully reproduced from backend inputs.

## Rules

- Use an isolated instance from `references/instance.md`.
- Never drive a browser against the user's live instance.
- Use `npx playwright-cli`; there is no guaranteed bare `playwright-cli` binary.
- Reuse an existing browser session when possible.

## Commands

```bash
npx --no-install playwright-cli --version
npx playwright-cli --help
npx playwright-cli list
```

Open only if no suitable session exists:

```bash
npx playwright-cli open http://localhost:<your_web_port>
```

Common debugging:

```bash
npx playwright-cli snapshot
npx playwright-cli snapshot "#main"
npx playwright-cli snapshot --depth=4
npx playwright-cli console
npx playwright-cli console error
npx playwright-cli network
npx playwright-cli goto http://localhost:<your_web_port>/some/path
```

Correlate browser console and network activity with a fresh standard bundle or
the smallest custom source set that answers the question:

```bash
scripts/kandev-logs <your_backend_port> --source all
```

Kandev retains a bounded three-day console history in that browser. It uploads
the history only after an explicit bundle request; it does not stream console
calls continuously. Inspect the bundle manifest for missing browsers,
truncation, persistence fallback, or dropped entries before treating absence
as evidence.

The standard bundle contains frontend/backend diagnostic events. A custom
bundle can add the allow-listed runtime index, and neither source reads stored
chat/session/agent messages. ACP protocol frames require the debug-only
**Download with ACP** path,
explicit session selection, and a sensitive-data review; they can contain
prompts, responses, tools, files, MCP data, environment values, and secrets.
When correlating a browser failure with a task, grep the exact task/session ID
in the extracted ZIP and the backend daily files before searching generic
error text.

Close the browser when done unless the user asks to keep it open:

```bash
npx playwright-cli close
```
