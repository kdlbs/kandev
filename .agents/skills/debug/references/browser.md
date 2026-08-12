# Browser Debugging

Use this only for UI, layout, focus, WS-driven, console, or click-flow bugs that cannot be faithfully reproduced from backend inputs.

## Rules

- Use an isolated instance from `references/instance.md`.
- Never drive a browser against the user's live instance.
- Use the exact `@playwright/cli` dependency from the workspace via
  `pnpm --dir apps exec playwright-cli`; do not fetch a CLI dynamically.
- Reuse an existing browser session when possible.

## Commands

```bash
pnpm --dir apps exec playwright-cli --version
pnpm --dir apps exec playwright-cli --help
pnpm --dir apps exec playwright-cli list
```

Open only if no suitable session exists:

```bash
pnpm --dir apps exec playwright-cli open http://localhost:<your_web_port>
```

Common debugging:

```bash
pnpm --dir apps exec playwright-cli snapshot
pnpm --dir apps exec playwright-cli snapshot "#main"
pnpm --dir apps exec playwright-cli snapshot --depth=4
pnpm --dir apps exec playwright-cli console
pnpm --dir apps exec playwright-cli console error
pnpm --dir apps exec playwright-cli network
pnpm --dir apps exec playwright-cli goto http://localhost:<your_web_port>/some/path
```

For browser/API failures that may be caused by origin capabilities, capture
these values in the isolated browser session before attributing the symptom to
WebSocket or network behavior:

```bash
pnpm --dir apps exec playwright-cli eval 'JSON.stringify({
  origin: location.origin,
  secure_context: window.isSecureContext,
  random_uuid: typeof crypto === "undefined" ? "undefined" : typeof crypto.randomUUID,
  subtle: typeof crypto === "undefined" ? "undefined" : typeof crypto.subtle,
  clipboard_write: typeof navigator === "undefined" || !navigator.clipboard
    ? "undefined"
    : typeof navigator.clipboard.writeText,
})'
```

Verify the failing code's capability probe and fallback in that same session;
do not infer support from a different origin or from a test-only stub.

Correlate browser console, frontend log buffer, network activity, and backend logs from:

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
pnpm --dir apps exec playwright-cli close
```
