# Desktop App Guidance

`apps/desktop` is the Tauri v2 shell for Kandev. It should stay a thin desktop wrapper over the existing native runtime and Go-served SPA.

## Commands

Run from `apps/` unless noted:

- `pnpm --filter @kandev/desktop build:vite` builds the startup surface.
- `pnpm --filter @kandev/desktop build` builds the Linux desktop bundle locally.
- `pnpm --filter @kandev/desktop e2e` builds the app and runs the Linux smoke harness under Xvfb when needed.
- From `apps/desktop/src-tauri`, `cargo test` runs Rust unit tests without requiring the Tauri runtime feature by default.

## Runtime Resources

Release builds prepare `src-tauri/resources/kandev/` with:

```text
bin/kandev[.exe]
bin/agentctl[.exe]
bin/agentctl-linux-amd64
```

Use `scripts/release/prepare-desktop-runtime.sh` and `scripts/release/verify-desktop-runtime.sh`; do not commit runtime binaries. The tracked `.gitignore` files only keep the resource directory present for Tauri config validation.

## Architecture

- Frontend code in `src/` is only the startup/error surface. The real product UI is still served by the Go backend after `/health` succeeds.
- Rust code owns backend process spawning and cleanup. Do not expose broad shell or filesystem permissions to frontend JavaScript.
- Desktop launches force `KANDEV_SERVER_HOST=127.0.0.1`, pick a local port, and pass `KANDEV_BUNDLE_DIR` to the native launcher.
- `KANDEV_DESKTOP_RUNTIME_DIR` is a test/development override for runtime resources.

## Release

Desktop artifacts are built in `.github/workflows/release.yml` after the platform runtime bundles. Public macOS and Windows artifacts require signing unless `allow_unsigned_desktop=true` is explicitly selected for internal validation.
