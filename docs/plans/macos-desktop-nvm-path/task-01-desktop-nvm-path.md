---
id: "01-desktop-nvm-path"
title: "Add the macOS nvm PATH fallback"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
acceptance_criteria:
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.18
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.19
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.20
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
---

# Task 01 Add the macOS nvm PATH fallback

## Summary

Append the bin directory for the standard nvm default to the macOS desktop
PATH when that directory provides the Node tools.

## In scope

- In `backend.rs`, read `~/.nvm/alias/default` from the OS user home on macOS.
  Accept `node` and one to three numeric version components, with an optional
  `v` prefix. Check the directories under `~/.nvm/versions/node` by numeric
  version. Use the highest matching version for `node` or a partial number.
- If the selected bin directory has executable `node`, `npm`, and `npx`, add
  it as the last common path entry. Use the existing PATH join and deduplication.
- Add a short explanation of the supported defaults to the desktop guide.

## Out of scope

- Custom roots, named and LTS aliases, and shell initialization.
- Service PATH and remote execution.

## Acceptance

- Write a regression that launches a stub with a sparse PATH. Check that the
  child sees the nvm bin after the existing entries. The baseline omits it.
- Use a table test for `node`, a numeric prefix, an exact pin, and a missing or
  unusable default. Verify numeric ordering with versions `v22.9.0` and
  `v22.10.0`.
- On macOS, use the packaged app to run the nvm-provided `npx` through a Local
  executor.

## Verification

Run from the repository root with the desktop Rust toolchain. Run the launcher
regression on macOS.

```bash
(cd apps/desktop/src-tauri && cargo test --locked --lib desktop_nvm_)
(cd apps/desktop/src-tauri && cargo test --locked --lib desktop_environment_preserves_path_and_adds_gui_paths)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/desktop docs
```

### Finder smoke test on macOS

1. Use a disposable macOS account or VM with Node installed only through nvm.
   Run `nvm alias default node`. Keep `~/.local/bin` free of Node symlinks.
2. From the repository root, build `Kandev.app` with the desktop build commands.

   ```bash
   (cd apps && pnpm install --frozen-lockfile)
   make desktop-build DESKTOP_BUNDLES=app
   ```

3. In Finder, open the built `Kandev.app` in
   `apps/desktop/src-tauri/target/release/bundle/macos/`. Check app readiness.
4. Add a custom Terminal agent with command `npx --version`. Use it in a
   disposable Local executor task. Check that Kandev finds the command and
   the agent terminal prints the npm tool version. Compare it with the
   version from the configured nvm default in a terminal.

## Files likely touched

- `apps/desktop/src-tauri/src/backend.rs`
- `docs/public/desktop-app.md`
- `docs/desktop-app.md`

## Dependencies

None.

## Risks

- If an earlier PATH entry supplies a tool in a mixed installation, that
  entry controls selection for that executable.

## Parallelism

Sequential.

## Inputs

- [Desktop requirements](../../specs/desktop/requirements/desktop-tauri-app.md)
- [Desktop launch design](../../specs/desktop/system-design/desktop-tauri-app.md#macos-agent-path)
- `command_spec_uses_headless_launcher_and_loopback_env` and
  `desktop_environment_preserves_path_and_adds_gui_paths` in `backend.rs`.
- [Custom Terminal agent registration](../../public/add-agent-cli.md#register-a-local-agent)
- [Normal nvm alias creation](https://github.com/nvm-sh/nvm/blob/v0.40.4/nvm.sh#L3489-L3496)

## Results

- Added macOS nvm default discovery to the desktop PATH builder. The resolver
  accepts `node` and one to three numeric version components, sorts installed
  versions numerically, and requires executable `node`, `npm`, and `npx`.
- The resolver now reads only regular alias files. It skips blank and comment
  lines, strips inline comments, and scans only canonical `vMAJOR.MINOR.PATCH`
  version directories.
- Added `desktop_nvm_launch_resolves_nvm_tools`. It checks inherited PATH
  precedence, version selection, invalid and missing defaults, incomplete bins,
  and the PATH printed by a launcher stub spawned through
  `spawn_backend_command(&spec)`.
- Added focused regressions for FIFO aliases, commented numeric defaults, and
  unprefixed or malformed version directory names.
- Added the supported nvm defaults to the public and repository desktop guides.
- All four macOS-only nvm regressions passed with the macOS target condition
  simulated on Linux.
- `cargo +1.90 test --locked --features desktop-runtime` passed all 120 tests.
  `cargo +1.90 build --locked --features desktop-runtime` passed on Linux.
  The targeted PATH preservation test, public documentation validation, Rust
  formatting, and whitespace checks also passed.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed after the specification
  changes.
- The packaged Finder smoke test was not run because the current host runs
  Linux. Run it on a macOS host before closing the work order.
