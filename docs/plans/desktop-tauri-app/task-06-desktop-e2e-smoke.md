---
id: "06-desktop-e2e-smoke"
title: "Desktop E2E smoke"
status: done
wave: 4
depends_on: ["02-tauri-desktop-scaffold", "03-desktop-runtime-resources"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 06: Desktop E2E Smoke

## Acceptance

- CI has at least one Linux desktop smoke test that launches the Tauri app with a prepared runtime bundle.
- The smoke test verifies the startup screen transitions to the Kandev UI after backend health succeeds.
- A failure-path test or Rust integration test verifies child process cleanup on startup failure.

## Verification

```bash
rtk bash -lc 'cd apps && pnpm --filter @kandev/desktop e2e'
rtk bash -lc 'cd apps/desktop/src-tauri && cargo test'
```

## Files Likely Touched

- `apps/desktop/e2e/desktop-launch.spec.ts`
- `apps/desktop/e2e/desktop-single-instance.spec.ts`
- `apps/desktop/e2e/README.md`
- `apps/desktop/package.json`
- `apps/desktop/src-tauri/src/backend.rs`
- `.github/workflows/release.yml` or a dedicated desktop test workflow

## Dependencies

- Task 02.
- Task 03.

## Inputs

- Spec sections: State machine; Failure modes; Scenarios.
- Plan sections: E2E Tests.
- Existing web E2E patterns in `apps/web/e2e/README.md`; Tauri WebDriver guidance for desktop smoke coverage.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 4 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
