---
id: "02-async-folder-picker"
title: "Keep native folder selection responsive"
status: done
wave: 2
depends_on:
  - "01-direct-home-confirmation"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.5
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.10
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.12
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 02: Keep native folder selection responsive

## Summary

Keep the desktop process responsive to native events while macOS opens a
modal panel. Start the panel in a local workspace folder when possible.

## Red test

Add Rust tests for candidate order, missing folders, symlink rejection,
and the no-override fallback. Add a test for completion and cancellation
through the asynchronous result channel. These tests must fail before the
new helper and callback mapping exist.

## Implementation

- Replace `blocking_pick_folder()` with the dialog plugin's callback-based
  `pick_folder` in an async Tauri command. Await the callback through a
  one-shot channel. Preserve the owned-origin check and result shape.
- Start in the first existing non-symlink directory among `Projects`,
  `Developer`, `src`, `Code`, `workspace`, `Development`, and `repos` under
  the desktop user's Home. If none exists, omit `set_directory`.
- Preserve the selected/cancelled/failed frontend adapter contract. Keep
  the trigger disabled and busy while a panel is open.
- Test the dialog on macOS with Home containing cloud-provider mounts and
  many directories. Record whether the app remains responsive and the
  panel opens promptly. Do not claim a Linux smoke proves AppKit behavior.

## Acceptance

1. The command does not block a Tauri IPC worker while the user decides.
2. Selected, cancelled, and failed results preserve their current shape.
3. The native panel starts at a local workspace folder when available.
   Without one, it uses the OS default instead of forcing Home.
4. The user can still navigate to Home and choose it. A cancellation adds
   no discovery root. Other native picker uses keep working.

## Verification

```bash
(cd apps/desktop/src-tauri && cargo test --features desktop-runtime folder_picker --lib)
(cd apps && pnpm --filter @kandev/web test lib/desktop/folder-picker.test.ts components/folder-picker.test.tsx)
(cd apps && pnpm --filter @kandev/web run typecheck)
(cd apps && pnpm --filter @kandev/web run lint)
(cd apps/web && pnpm run i18n:check)
```

The integration check uses the real macOS application. Test selection,
cancellation, slow panels, and app responsiveness. The Linux Rust test
proves directory choice and async result mapping, not Finder behavior.

## Files likely touched

- `apps/desktop/src-tauri/src/folder_picker.rs`
- `apps/desktop/src-tauri/src/backend.rs`
- Rust tests next to the picker implementation
- `apps/web/lib/desktop/folder-picker.test.ts` if the outcome contract changes
- `docs/public/desktop-app.md`

## Dependencies

Task 01 removes the accidental picker call from Home confirmation. This
task then changes the native panel used by explicit folder selection.

## Risks

The dialog callback cannot force macOS or a Finder extension to respond.
The Tauri command must return a failure if its callback channel closes
without a result. It must not expose general filesystem access.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/local-repositories.md)
  and [design](../../specs/workspaces/system-design/local-repositories.md).
- Tauri dialog plugin `pick_folder` callback in the installed 2.7.2 crate.

## Results

Passed `cargo test --features desktop-runtime folder_picker --lib` (7 tests)
and the full desktop Rust library suite (81 tests). Coverage includes
workspace-directory order, missing candidates, symlink rejection, selection,
cancellation, and closed callback channels. The web folder-picker suite
passed (12 tests); web typecheck, lint, and i18n checks passed. The Linux
desktop release build and two-window smoke passed. Finder panel timing and UI
responsiveness remain to be checked on macOS.
