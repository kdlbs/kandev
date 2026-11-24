---
created: 2026-11-24
status: in_progress
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
legacy_specs: []
---

# macOS desktop nvm discovery fix plan

## Overview

The macOS desktop launcher needs a way to find Node tools when nvm supplies
them and the app starts with a sparse PATH. The Desktop system owns the PATH
that the app supplies to its native launcher. Use one work order to add and
verify the fallback at that boundary.

## Evidence

- The new comment reports the missing `npx` on macOS desktop v0.103.1. The
  `~/.local/bin` symlink workaround worked for the reporter. [Issue comment](https://github.com/kdlbs/kandev/issues/1526#issuecomment-7127322186)
- The original repair captures Node tool paths from the service installer's
  environment. The macOS desktop app builds a separate PATH when it starts
  its bundled native launcher. Its fixed additions include `~/.local/bin`.
  The app relies on inheritance for an installed nvm bin directory. [Service fix](https://github.com/kdlbs/kandev/pull/1527)
- The desktop PATH builder and Go environment handoff match v0.103.1 on this
  checkout. A temporary Rust test called `build_backend_command` and
  `spawn_backend_command` with a sparse PATH on Linux. Its child exited 127
  while resolving the fixture's Node tools. The child executed `npx` through
  the fixture's `node` when it inherited the nvm directory. The same command
  succeeded with the sparse PATH after the test added the three symlinks.
  The test ran in a temporary copy and was removed with the copy.

## Scope

### In scope

- One PATH fallback for `~/.nvm` on macOS desktop. It covers a `node` or
  numeric default and applies to local agent discovery and execution.

### Out of scope

- Custom nvm roots, named and LTS aliases, and other version managers.
- Service installation, shell initialization, and remote executors.

## Technical approach

Add one lookup in the existing macOS PATH builder. Read `~/.nvm/alias/default`
and append the matching bin if it contains `node`, `npm`, and `npx`. The `node`
default covers the original `nvm install node` setup. [nvm defaults](https://github.com/nvm-sh/nvm/blob/v0.40.4/README.md#set-default-node-version)

[Desktop requirements](../../specs/desktop/requirements/desktop-tauri-app.md) ·
[macOS agent PATH design](../../specs/desktop/system-design/desktop-tauri-app.md#macos-agent-path)

## Tests

| Desktop acceptance criterion | Targeted evidence |
| --- | --- |
| `.18` | `desktop_nvm_launch_resolves_nvm_tools` checks what the launcher child receives. Cover `node` and the numeric selectors with a table test in `backend.rs`. |
| `.19` | Compare the resulting PATH against the original entry order in the same test. |
| `.20` | Include an unusable default in the table test and run `desktop_environment_preserves_path_and_adds_gui_paths`. |

Use `desktop_nvm_launch_resolves_nvm_tools` as the first regression test. The
baseline child PATH omits the fixture bin. The fixed child receives it after
the existing entries.

## End-to-end verification

Run the Finder smoke test in the work order with the packaged app on macOS.
A Terminal agent with `npx --version` exercises discovery and real invocation
through the local executor with no download or provider account. This check
covers `.18` at the OS launch boundary.

## Work orders

- [ ] [Task 01 Add the macOS nvm PATH fallback](task-01-desktop-nvm-path.md) (in progress)

## Verification results

- The temporary launcher subprocess test passed on Linux. It reproduced exit
  127 with the sparse PATH and executed `npx` with each workaround.
- The four macOS-only nvm regressions passed when compiled with the macOS
  target condition on this Linux host. They cover FIFO aliases, comments,
  canonical version directories, and the environment of a spawned launcher.
- `cargo +1.90 test --locked --features desktop-runtime` passed all 120 tests.
  The targeted desktop PATH test also passed.
- `cargo +1.90 build --locked --features desktop-runtime` passed on Linux.
- The public-doc validator tests passed all 63 cases, and the public-doc
  validator accepted all 56 pages. Rust formatting and whitespace checks passed.
- The packaged Finder smoke test remains pending because this host runs Linux.

## Risks

- Earlier PATH entries can supply `node` to npm scripts. The Finder smoke
  test uses the reported nvm-only setup.
