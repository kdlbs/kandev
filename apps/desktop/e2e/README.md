# Desktop E2E

`pnpm --filter @kandev/desktop e2e` builds the Linux desktop bundle, launches the compiled Tauri app under `xvfb-run` when no display is available, and points it at a fake native runtime.

The fake runtime exposes `/health` and `/`. The smoke test passes only after the WebView requests `/`, which proves the startup screen transitioned to the backend UI after health succeeded.
