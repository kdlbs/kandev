---
id: "03-host-platform-e2e"
title: "Prove host-platform behavior"
status: done
wave: 3
depends_on: ["02-filter-task-topbar-editors"]
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-windows-availability.md"
---

# Task 03: Prove host-platform behavior

## Acceptance

- A desktop Playwright scenario simulates a Windows backend host through the injected boot payload
  while leaving the browser non-Windows, and observes a compatible editor without observing
  **VS Code (Embedded)**.
- An inverse desktop scenario reports Windows only through `navigator.platform` while retaining the
  Linux backend payload, and observes **VS Code (Embedded)**.
- A mobile scenario with a Windows-host boot payload observes the intentional mobile task topbar
  without desktop editor controls or document-level horizontal overflow.

## Verification

Use E2E TDD: add the Windows-host desktop scenario and observe it fail before Task 02's behavior is
present, then run both focused production-build specs:

```bash
cd apps/web && pnpm e2e:run tests/task/windows-host-embedded-vscode-availability.spec.ts -- --project=chromium
cd apps/web && pnpm e2e:run tests/task/mobile-windows-host-embedded-vscode-availability.spec.ts -- --project=mobile-chrome
```

## Files likely touched

- `apps/web/e2e/tests/task/windows-host-embedded-vscode-availability.spec.ts`
- `apps/web/e2e/tests/task/mobile-windows-host-embedded-vscode-availability.spec.ts`

## Dependencies

- Task 02 must be complete.

## Parallelism

Parallel-safe with Task 04 after Task 02. The files are disjoint and no shared schema, package
configuration, generated contract, or lockfile is involved. User authorization is still required
before using subagents.

## Inputs

- Spec sections: **What** and **Scenarios**.
- Plan sections: **E2E Tests**, **Mobile design contract**, and **Risks**.
- Existing patterns:
  - `apps/web/e2e/tests/settings/vscode-open-panel.spec.ts`
  - `apps/web/e2e/tests/task/mobile-task-topbar-long-title.spec.ts`
  - `apps/web/e2e/fixtures/test-base.ts`

## Risks

- The initial-document interceptor must rewrite only the serialized `runtime.hostOS` value and
  preserve status, headers, and all other boot data.
- Seed the compatible editor before task-page navigation so the boot payload includes it.
- The inverse scenario must not rewrite the boot payload, or it will fail to prove that visitor
  platform is ignored.

## Output contract

Report the scenarios added, files changed, red and green E2E commands/results, artifacts or
blockers, and update this task to `done` plus its checkbox/status in `plan.md`.
