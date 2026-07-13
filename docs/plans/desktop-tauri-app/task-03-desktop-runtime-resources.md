---
id: "03-desktop-runtime-resources"
title: "Desktop runtime resources"
status: done
wave: 2
depends_on: ["01-backend-desktop-launch-contract"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 03: Desktop Runtime Resources

## Acceptance

- A release helper extracts the existing platform runtime bundle into the deterministic Tauri resource layout.
- The helper validates `kandev`, `agentctl`, and the remote agentctl helpers consistently with the native launcher.
- The Tauri app resolves packaged resources without relying on `KANDEV_BUNDLE_DIR` or Node.js at runtime.

## Verification

```bash
rtk bash -lc 'scripts/release/prepare-desktop-runtime.sh --help'
rtk bash -lc 'cd apps/desktop/src-tauri && cargo test'
rtk bash -lc 'cd apps && pnpm --filter @kandev/desktop build'
```

## Files Likely Touched

- `scripts/release/prepare-desktop-runtime.sh`
- `scripts/release/verify-desktop-runtime.sh`
- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/src/backend.rs`
- `apps/desktop/src-tauri/resources/.gitignore`
- `apps/cli/src/release-config.test.ts` or a new script test if script behavior is tested from Node/Vitest

## Dependencies

- Task 01.

## Inputs

- Spec sections: API surface > Desktop artifact contract; Failure modes.
- Plan sections: Desktop App > Runtime resources.
- Existing bundle shape: `apps/cli/README_internal.md` and `scripts/release/package-bundle.sh`.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 2 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
