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
npx playwright-cli eval "JSON.stringify(window.__kandevLogBuffer?.snapshot?.() ?? [])"
npx playwright-cli goto http://localhost:<your_web_port>/some/path
```

Correlate browser console, frontend log buffer, network activity, and backend logs from:

```bash
scripts/kandev-logs <your_backend_port> --export --level error
```

Close the browser when done unless the user asks to keep it open:

```bash
npx playwright-cli close
```
