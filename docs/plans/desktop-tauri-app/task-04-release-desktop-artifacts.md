---
id: "04-release-desktop-artifacts"
title: "Release desktop artifacts"
status: done
wave: 3
depends_on: ["02-tauri-desktop-scaffold", "03-desktop-runtime-resources"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 04: Release Desktop Artifacts

## Acceptance

- The release workflow builds desktop artifacts for macOS arm64, macOS x64, Linux x64, Linux arm64, and Windows x64.
- `publish-release` attaches desktop artifacts and checksums without changing npm/Homebrew runtime publishing behavior.
- Release verification fails when a required desktop artifact is missing.

## Verification

```bash
rtk bash -lc 'cd apps && pnpm --filter kandev test -- release-config'
rtk git diff --check
```

## Files Likely Touched

- `.github/workflows/release.yml`
- `scripts/release/verify-desktop-assets.sh`
- `scripts/release/prepare-desktop-runtime.sh`
- `apps/cli/src/release-config.test.ts`
- `apps/desktop/package.json`

## Dependencies

- Task 02.
- Task 03.

## Inputs

- Spec sections: API surface > Desktop artifact contract; Scenarios.
- Plan sections: Release Pipeline > Build matrix; Release Pipeline > GitHub release publishing.
- Existing workflow: `.github/workflows/release.yml`.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 3 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
