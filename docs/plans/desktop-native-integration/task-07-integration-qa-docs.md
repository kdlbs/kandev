---
id: "07-integration-qa-docs"
title: "Desktop integration QA and public docs"
status: done
wave: 6
depends_on:
  [
    "01-native-shell-commands",
    "02-spa-contextual-commands",
    "03-updater-release-artifacts",
    "04-updater-runtime-settings",
    "05-native-notifications",
    "06-external-links",
  ]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 07: Desktop Integration QA and Public Docs

## Acceptance

- Desktop smoke coverage exercises menu dispatch, contextual close no-op/window safety, zoom,
  settings routing, persisted window restore, focus/reopen, and backend cleanup on quit.
- Updater QA covers valid update, invalid signature, offline check, explicit consent, clean restart,
  and AppImage target selection without publishing a release.
- Notification QA covers both events, deduplication, denied permission, and focus without inferred
  routing.
- Desktop install/update docs identify AppImage as Linux in-app update format and `.deb`/`.rpm` as
  package-manager/manual formats, plus signing and failure recovery guidance.
- Browser/mobile settings behavior is regression-tested; native-only commands are explicitly
  justified as not applicable to mobile.
- Relevant `AGENTS.md`, public docs, spec index, decision index, and release documentation agree.

## Files Likely Touched

- Desktop smoke/E2E files under `apps/desktop/`
- Focused web E2E tests under `apps/web/e2e/`
- `docs/public/` desktop installation/update documentation
- Desktop signing/release documentation
- `apps/desktop/AGENTS.md` and `apps/web/AGENTS.md` only if conventions changed

## Verification

Run every command in the plan's Final Verification section, then inspect Playwright screenshots at
desktop and mobile settings viewports for clipping or native-only controls leaking into browsers.
On a supported GUI runner, verify window pixels are nonblank after restore and zoom operations.

## Output Contract

Record commands and platform coverage, update this task and the plan to `done`, and list any
platform behavior that could not be exercised locally before release.

## Verification Record

Completed on Linux x64 on 2026-07-15:

- `PATH=/root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin:$PATH RUSTC=/root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin/rustc RUSTDOC=/root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin/rustdoc rtk make fmt` passed.
- `cd apps/desktop/src-tauri && PATH=/root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin:$PATH RUSTC=/root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin/rustc RUSTDOC=/root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin/rustdoc /root/.rustup/toolchains/1.88.0-x86_64-unknown-linux-gnu/bin/cargo test --features desktop-runtime` passed (53 tests), including menu dispatch, contextual-close, zoom, window-state, notification, and lifecycle assertions.
- Web type checking, lint, and the full Vitest suite passed (4,889 passed, 4 skipped).
- The focused Updates-page Playwright suite passed its four applicable desktop/mobile workflows;
  changelog pagination skipped because the generated fixture had only one page.
- Backend formatting, tests, and lint passed.
- CLI release configuration tests and the complete desktop release/signature fixture matrix
  passed, including invalid, unsigned, wrong-key, partial-feed, and OS-signing warning paths.
- The Linux `.deb`/`.rpm` package build and native desktop launch smoke passed. That smoke proves backend startup and WebView navigation only; native interaction scenarios are covered by the focused cross-platform unit/integration tests above, not by the launch smoke itself.

macOS and Windows native menu rendering, notification permission prompts, OS code signing, and
real updater installation remain release-runner/manual platform checks; their config and dispatch
logic are covered by the cross-platform test suites.
