---
spec: docs/specs/desktop-tauri-app/spec.md
created: 2026-07-15
status: done
---

# Implementation Plan: Desktop Native Integration

## Goal

Add native menus and contextual shortcuts, signed prompt-before-install updates, native
notifications, persistent window state, focus/reopen behavior, and safe external links to the
existing Tauri shell without creating a second product UI or broadening WebView authority.

## Architecture

- Product contract: `docs/specs/desktop-tauri-app/spec.md`
- Shell decision: `docs/decisions/0026-tauri-desktop-shell.md`
- Integration boundary: `docs/decisions/0039-native-desktop-integration-boundary.md`
- Rust owns OS and lifecycle behavior; the SPA owns navigation and contextual UI semantics.
- The loopback page gets a narrow typed desktop bridge, not generic Tauri plugin access.
- Desktop update presentation extends the existing `/settings/system/updates` surface. Browser,
  service, and mobile behavior remains unchanged.

### Frozen bridge contract

Wave 1 uses a shared versioned adapter contract so the Rust and SPA tasks can proceed independently:

- native events: `close-context`, `open-settings`, `new-task`, and `check-for-updates`;
- SPA commands: `get-update-state`, `check-for-updates`, `install-update`, `show-notification`,
  and `open-external`;
- every bridge message is namespaced as desktop protocol `v1` and carries only its task-specific
  structured payload;
- unavailable or unsupported bridge operations return typed errors and never fall back to generic
  plugin, shell, filesystem, URL, or window authority.

The implementation may choose Tauri-safe literal identifiers, but both sides must expose these
stable adapter names and payload types. A protocol change requires updating this plan before the
parallel Wave 1 tasks diverge.

## Execution Rules

- Tasks in a wave may run in parallel only where shown.
- Tasks touching `apps/desktop/src-tauri/Cargo.toml`, `tauri.conf.json`, capabilities, or
  `src/main.rs` are intentionally serialized.
- Every behavioral change starts with a failing focused test where the local test harness supports
  it. Config-only release assertions may be added directly to existing release tests.
- Do not begin a later wave until all dependencies are verified and task status is updated.

## Waves

### Wave 1: Command foundations

- [x] [Task 01: Native shell commands and window lifecycle](task-01-native-shell-commands.md)
- [x] [Task 02: SPA contextual command resolver](task-02-spa-contextual-commands.md)

These tasks may run in parallel. Task 01 owns desktop Rust/config files; Task 02 owns web files.

### Wave 2: Update release supply chain

- [x] [Task 03: Signed updater release artifacts](task-03-updater-release-artifacts.md)

Depends on Task 01 so the finalized Tauri target/config baseline is available.

### Wave 3: Updater runtime and existing settings UI

- [x] [Task 04: Desktop updater runtime and settings adapter](task-04-updater-runtime-settings.md)

Depends on Tasks 01-03. This task is the next owner of shared Tauri manifests/configuration.

### Wave 4: Native notification routing

- [x] [Task 05: Native notifications and duplicate suppression](task-05-native-notifications.md)

Depends on Task 04 and owns the next shared Tauri/plugin edit.

### Wave 5: External links

- [x] [Task 06: Scoped native external-link handling](task-06-external-links.md)

Depends on Task 05 and owns the final shared Tauri capability/plugin edit.

### Wave 6: Integrated QA and documentation

- [x] [Task 07: Desktop integration QA and public docs](task-07-integration-qa-docs.md)

Depends on Tasks 01-06.

## Cross-Cutting Acceptance

- `Cmd/Ctrl+W` never quits or closes the main window and closes only one eligible context.
- macOS red close and Quit cleanly stop the backend and exit.
- Zoom, Settings, New Task, Help, Check for Updates, focus/reopen, and window restore work through
  native menus/lifecycle.
- Updates require valid Tauri signatures and explicit install/restart confirmation; Linux
  auto-update is AppImage-only.
- Apple notarization and Windows code signing add publisher identity when configured but do not
  block a complete Tauri-signed updater feed when absent; release warnings remain mandatory.
- Waiting-input and session-failure events yield one native desktop notification; generic focus
  and activation events never infer a task route because Tauri exposes no desktop click payload.
- Browser/service/mobile settings and notification paths retain their current behavior.
- The loopback WebView has no shell/filesystem capability and unsupported external schemes fail
  closed.

## Final Verification

Run formatting first, then focused and broad verification:

```bash
cd apps && rtk pnpm --filter @kandev/desktop build
cd apps/desktop/src-tauri && rtk cargo test
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
cd apps && rtk pnpm --filter @kandev/web lint
cd apps && rtk pnpm --filter @kandev/desktop e2e
```

Release workflow validation must additionally run the updater manifest/signature tests and
`scripts/release/verify-desktop-assets.sh` against representative artifacts for every target.

## Approval Checkpoint

The product decisions and implementation plan were approved on 2026-07-15. Any later change to
close priority, quit semantics, update consent, Linux update format, notification event scope, or
bridge permissions requires updating the spec/ADR first.
