---
id: "07-docs-and-release-notes"
title: "Docs and release notes"
status: done
wave: 4
depends_on: ["04-release-desktop-artifacts", "05-signing-and-release-trust", "06-desktop-e2e-smoke"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 07: Docs and Release Notes

## Acceptance

- User docs explain desktop install artifacts, runtime requirements, update expectations, and signing/trust status.
- User docs explain GUI-launch environment differences and what to do when an agent CLI is not discoverable from the desktop app.
- Engineering docs mention the `apps/desktop` package and release workflow ownership.
- The spec status and plan task statuses are reconciled with the implementation state.

## Verification

```bash
rtk git diff --check
rtk bash -lc 'cd apps && pnpm format:check'
```

## Files Likely Touched

- `docs/specs/desktop-tauri-app/spec.md`
- `docs/plans/desktop-tauri-app/plan.md`
- `docs/desktop-tauri-signing.md`
- `docs/cli.md`
- `docs/features.md`
- `docs/remote-cloud-environment.md` if desktop build dependencies affect cloud dev setup
- `AGENTS.md`
- `apps/desktop/AGENTS.md`
- `apps/cli/README_internal.md`

## Dependencies

- Task 04.
- Task 05.
- Task 06.

## Inputs

- Spec sections: What; Out of scope.
- Plan sections: Release Pipeline; E2E Tests.
- Root `AGENTS.md` maintenance rule for changed package layout/release conventions.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 4 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
