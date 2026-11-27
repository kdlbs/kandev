---
id: "01-enable-developer-tools"
title: "Enable native developer tools"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DEVELOPER-TOOLS-001
acceptance_criteria:
  - AC-DESKTOP-DEVELOPER-TOOLS-001.1
  - AC-DESKTOP-DEVELOPER-TOOLS-001.2
  - AC-DESKTOP-DEVELOPER-TOOLS-001.3
  - AC-DESKTOP-DEVELOPER-TOOLS-001.4
  - AC-DESKTOP-DEVELOPER-TOOLS-001.5
  - AC-DESKTOP-DEVELOPER-TOOLS-001.6
system_design:
  - ../../specs/desktop/system-design/developer-tools.md
---

# Task 01: Enable Native Developer Tools

## Summary

Enable release inspection and add View > Developer Tools with the platform
shortcut. Keep the inspector closed until requested and preserve the running page.

Implementation is complete; native platform verification is pending.

## In scope

- The Tauri dependency capability and explicit main web-view enablement.
- Native action routing, menu composition, and platform accelerators.
- Routing regression tests, release compilation, and native interaction checks.
- Public desktop instructions and scoped engineering guidance.

## Out of scope

Kandev runtime flags, shared web UI, backend changes, new Tauri script
permissions, automatic data collection, and the separate scroll correction.

## Acceptance

1. Release builds expose the inspector without opt-in through both native entry
   points. Startup and error pages remain inspectable. The inspector stays closed at startup.
2. Repeated opening preserves the page and current backend. Safari also exposes
   the web view on macOS 13.3+. Application-script permissions do not expand.
3. Routing tests, release builds, documentation checks, and native scenarios
   pass with recorded platform evidence. Missing platform checks remain pending.

## Implementation sequence

1. After explicit implementation authorization, mark this task `in_progress`.
2. Add a regression in `shell.rs::tests` using literal ID
   `desktop.v1.developer-tools`. Assert that it resolves to a native action.
   Before introducing the action, assert that routing is present and observe
   the failing `None` result. Then strengthen the assertion to the exact variant.
3. Add the typed action, platform accelerator, dependency capability, builder
   enablement, and View menu dispatch described in the design.
4. Test `developer_tools_is_a_native_action` and
   `developer_tools_uses_the_platform_accelerator`. Preserve unknown-ID rejection
   and existing menu-routing tests.
5. Run release compilation and the native scenarios below. A debug-only run is insufficient.
6. Update public instructions and `apps/desktop/AGENTS.md`. Record each result.

Use the existing native menu style. Keep the native action local to Rust and
avoid a generic inspector abstraction or an application-script bridge.

## ASCII UI preview

UI-01: Native View menu. See the [complete preview](plan.md#ascii-ui-preview).

```text
View
  ...existing zoom controls...
  Toggle Full Screen
  -----------------------------
  Developer Tools   Cmd+Option+I
```

Windows/Linux use Ctrl+Shift+I. No phone/browser control is added because this
action belongs to the native desktop shell. AC-DESKTOP-DEVELOPER-TOOLS-001.2 and .3 apply.

## Verification

Run these commands from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/desktop build:vite)
(cd apps/desktop/src-tauri && cargo fmt --check)
(cd apps/desktop/src-tauri && cargo test --locked --features desktop-runtime)
(cd apps/desktop/src-tauri && cargo check --locked --release --features desktop-runtime --bin kandev-desktop)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

On Linux, run the existing isolated startup smoke:

```bash
(cd apps && pnpm --filter @kandev/desktop e2e)
```

On macOS, prepare and build the real release app through the repository target:

```bash
make desktop-build
```

On Windows, use Git Bash with the matching Windows runtime bundle extracted
to `dist/kandev`, as in the existing release build. Run from the repository root:

```bash
bash scripts/release/prepare-desktop-runtime.sh --bundle-dir dist/kandev --platform windows-amd64
(cd apps/desktop && pnpm tauri build --features desktop-runtime --target x86_64-pc-windows-msvc --bundles nsis)
```

Record the artifact path and source revision. Do not change release workflows
just to add the Cargo capability. Existing desktop builds already enable `desktop-runtime`.

### Native scenario

Use a disposable home and release binary on each supported desktop OS.
Never perform test interaction in the user's live instance.

1. Launch the app with no inspector opt-in. Confirm the inspector starts closed.
2. Open View > Developer Tools. Confirm that the inspector targets the Kandev page.
3. In its console, set `window.__kandevInspectorProbe = "retained"`.
4. Close the inspector. Reopen it with the platform shortcut and read the marker.
5. Activate Developer Tools again. Confirm it remains open and the marker remains.
6. Confirm that the backend identity is unchanged and the app remains usable.
7. With an isolated startup failure, confirm that the same menu opens that page's inspector.
8. On macOS 13.3+, inspect the same page through Safari Develop. Capture a heap
   snapshot before any page reload and confirm the snapshot is available.
9. Quit only the owned disposable app and remove its temporary data.

Record OS, architecture, engine version, release artifact identity, both entry
points, page-marker result, and macOS snapshot result. Native inspector checks
are required delivery evidence. The existing startup smoke is not their substitute.

## Files likely touched

- `apps/desktop/src-tauri/Cargo.toml`
- `apps/desktop/src-tauri/Cargo.lock` only if dependency resolution requires it
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/src/shell.rs`, including its inline tests
- `apps/desktop/AGENTS.md`
- `docs/public/desktop-app.md`
- This package and its requirement/design status after verification

## Dependencies

None. The chat-scroll work is independent and its existing edits must be preserved.

## Risks

Native inspector behavior is platform-dependent. MacOS private API use requires
a packaged runtime check. The desktop binary has `test = false`, so routing unit
tests alone cannot prove menu construction or real inspector dispatch.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/desktop/requirements/developer-tools.md).
- [System design](../../specs/desktop/system-design/developer-tools.md).
- `apps/desktop/AGENTS.md` and the current `shell.rs` menu pattern.
- [ADR 0039](../../decisions/0039-native-desktop-integration-boundary.md).
- Pinned Tauri 2.11.5 and Wry 0.55.1 inspector implementation.

## Results

Implementation is complete; native platform verification is pending. Keep this
work order open until each required native check below has recorded evidence.

- Added the Tauri `devtools` capability to `desktop-runtime`, enabled it on the
  main webview, and routed the native View action to `open_devtools()`.
- Added the menu action and platform accelerators: Cmd+Option+I on macOS and
  Ctrl+Shift+I on Windows/Linux. The SPA bridge and capability permissions did
  not change.
- The new menu-routing test failed before implementation, then passed with the
  exact native action assertion. The accelerator test passes on Linux.
- All 111 Rust tests, release compilation, formatting, the desktop Vite build,
  and the isolated Linux desktop smoke passed.
- Public instructions and desktop engineering guidance were updated. Public-doc
  validators, catalog validation, specification lint, and diff checks pass.
- The Linux smoke did not activate Developer Tools or exercise the page marker.
  The full native inspector scenario remains pending on Linux, macOS, and
  Windows. Safari Develop and the macOS heap snapshot also require macOS.
