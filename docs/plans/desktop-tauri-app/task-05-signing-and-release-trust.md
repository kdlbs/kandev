---
id: "05-signing-and-release-trust"
title: "Signing and release trust"
status: done
wave: 3
depends_on: ["04-release-desktop-artifacts"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 05: Signing and Release Trust

## Acceptance

- macOS desktop release builds support Developer ID signing and notarization through GitHub Actions secrets.
- Windows desktop release builds support code signing through the selected signing mechanism.
- Unsigned desktop artifacts are clearly limited to dry runs/internal validation and are not treated as trusted public artifacts.

## Verification

```bash
rtk bash -lc 'cd apps && pnpm --filter kandev test -- release-config'
rtk git diff --check
```

Manual verification for maintainers:

```bash
rtk gh workflow run release.yml -f dry_run=true -f bump=patch
```

## Files Likely Touched

- `.github/workflows/release.yml`
- `apps/desktop/src-tauri/tauri.conf.json`
- `docs/desktop-tauri-signing.md`
- `apps/cli/src/release-config.test.ts`

## Dependencies

- Task 04.

## Inputs

- Spec sections: What; Permissions; Failure modes; Scenarios.
- Plan sections: Release Pipeline > Signing.
- Tauri signing docs for macOS, Windows, Linux, and updater signing constraints.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 3 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
