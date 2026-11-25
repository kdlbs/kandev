# Desktop E2E

`pnpm --filter @kandev/desktop e2e` builds the Linux desktop bundle, launches the compiled Tauri app under `xvfb-run` when no display is available, and points it at a fake native runtime.

The fake runtime exposes `/health` and `/`. The smoke test passes only after the WebView requests `/`, which proves the startup screen transitioned to the backend UI after health succeeded.


## Managed SSH native startup and restart

The opt-in Rust test uses the desktop command builder and process spawner with
a real built runtime. It generates a disposable managed key, authenticates to
an isolated loopback OpenSSH server, restarts the backend, and authenticates
again with the same identity. Neither launch inherits `SSH_AUTH_SOCK`.
This checks native process startup rather than WebView rendering.

Prepare runtime resources using `scripts/release/prepare-desktop-runtime.sh`.
Then run from the repository root, substituting absolute paths:

```bash
KANDEV_TEST_DESKTOP_RUNTIME_DIR=/absolute/path/to/runtime \
KANDEV_TEST_SSHD=/absolute/path/to/sshd \
KANDEV_TEST_SSH_FIXTURE_PARENT=/absolute/private/directory \
cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml --lib \
  native_desktop_managed_ssh_survives_restart_without_agent -- --ignored --nocapture
```

The fixture parent and its ancestors must meet OpenSSH ownership and permission
checks. A private user cache directory works on Linux. A world-writable `/tmp`
ancestor fails `StrictModes`; do not disable that check. The test creates and
removes a unique child directory, its own authorized-key file, and its own SSH
server. It does not use the developer's SSH configuration or authorization file.
The prepared runtime must have a release-shaped version accepted by the launcher.
