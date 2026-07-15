# Desktop App Guidance

`apps/desktop` is the Tauri v2 shell for Kandev. It should stay a thin desktop wrapper over the existing native runtime and Go-served SPA.

## Commands

Run from `apps/` unless noted:

- `pnpm --filter @kandev/desktop build:vite` builds the startup surface.
- `pnpm --filter @kandev/desktop build` builds the Linux desktop bundle locally.
- `pnpm --filter @kandev/desktop e2e` builds the app and runs the Linux smoke harness under Xvfb when needed.
- From `apps/desktop/src-tauri`, `cargo test --features desktop-runtime` runs the complete Rust
  suite, including native command/plugin integration.

## Runtime Resources

Release builds prepare `src-tauri/resources/kandev/` with:

```text
bin/kandev[.exe]
bin/agentctl[.exe]
bin/agentctl-linux-amd64
bin/agentctl-linux-arm64
bin/agentctl-darwin-arm64
bin/agentctl-darwin-amd64
```

Use `scripts/release/prepare-desktop-runtime.sh` and `scripts/release/verify-desktop-runtime.sh`; do not commit runtime binaries. The tracked `.gitignore` files only keep the resource directory present for Tauri config validation.

## Architecture

- Frontend code in `src/` is only the startup/error surface. The real product UI is still served by the Go backend after `/health` succeeds.
- Rust code owns backend process spawning and cleanup. Do not expose broad shell or filesystem permissions to frontend JavaScript.
- Native menus emit versioned `kandev-desktop-v1-*` events for SPA-owned context and navigation.
  Updater, notification, and external-link operations use narrow generated Tauri commands scoped
  to the owned loopback WebView; do not grant the SPA direct plugin permissions.
- Desktop launches force `KANDEV_SERVER_HOST=127.0.0.1`, prefer a stable desktop port with random fallback, and pass `KANDEV_BUNDLE_DIR` to the native launcher.
- Desktop launches set `KANDEV_DESKTOP_NATIVE_NOTIFICATIONS=true` so the owned backend suppresses
  only its duplicate System notification provider.
- `KANDEV_DESKTOP_RUNTIME_DIR` is a test/development override for runtime resources.

## Release

Desktop artifacts are built in `.github/workflows/release.yml` after the platform runtime bundles.
macOS and Windows signing is automatic: complete signing/notarization secrets produce signed
artifacts; missing or incomplete inputs produce unsigned desktop artifacts with a release-notes
warning. A separate Tauri updater key signs `.app.tar.gz`, `.AppImage.tar.gz`, and NSIS updater
bundles and enables `latest.json`; unsigned installers are never advertised by the updater. Use
`desktop_validation_only=true` for artifact-only validation runs that skip the release PR, tag,
publish jobs, and public container tags.
