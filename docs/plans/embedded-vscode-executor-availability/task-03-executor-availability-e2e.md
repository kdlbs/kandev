---
id: "03-executor-availability-e2e"
title: "Prove executor-specific behavior"
status: done
wave: 3
depends_on: ["02-wire-editor-availability"]
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-executor-availability.md"
---

# Task 03: Prove Executor-Specific Behavior

## Acceptance

- Desktop Playwright coverage proves unsupported capability hides embedded VS Code while retaining
  a compatible editor, and supported capability shows it independently of browser platform.
- Capability test routing rewrites only the correlated `task.session.status` response and proxies
  every unrelated WebSocket frame unchanged.
- Phone-viewport coverage retains the intentional mobile topbar, absence of desktop editor
  controls, and no horizontal overflow without relying on boot host metadata.

## Verification

From `apps/`:

```bash
pnpm --filter @kandev/web e2e:run -- tests/task/executor-embedded-vscode-availability.spec.ts -- --project=chromium
pnpm --filter @kandev/web e2e:run -- tests/task/mobile-executor-embedded-vscode-availability.spec.ts -- --project=mobile-chrome
```

If the WebSocket helper has extractable pure frame-rewrite logic, add and run its focused Vitest
test before Playwright. Do not add a test-only production API or runtime flag.

## Files likely touched

- `apps/web/e2e/helpers/session-capabilities.ts` (new)
- `apps/web/e2e/tests/task/executor-embedded-vscode-availability.spec.ts` (replace old
  host-platform spec)
- `apps/web/e2e/tests/task/mobile-executor-embedded-vscode-availability.spec.ts` (rename and update)
- `apps/web/e2e/tests/task/windows-host-embedded-vscode-availability.spec.ts` (remove)
- `apps/web/e2e/tests/task/mobile-windows-host-embedded-vscode-availability.spec.ts` (remove)

## Dependencies

- Task 02 supplies the active-session UI behavior and removes the boot-host helper.

## Parallelism

Parallel-safe with Task 04 only when the user explicitly authorizes subagents. Otherwise run
sequentially in the primary conversation.

## Inputs

- Spec scenarios 1–3, 8, and 9.
- Plan section: **E2E tests**.
- Existing WebSocket proxy pattern: `apps/web/e2e/helpers/ws-drop.ts`.
- Existing supported launch smoke:
  `apps/web/e2e/tests/settings/vscode-open-panel.spec.ts`.

## Risks

- Correlate request IDs; do not mutate every response containing a `capabilities` key.
- Preserve newline-delimited frames and binary frames exactly when they are not the target.
- Avoid starting code-server in menu-availability tests; the existing launch smoke owns that cost.

## Output contract

Report scenarios covered, files replaced/removed, exact Playwright results, and any fixture
limitations. Update this task to `done` and its plan checkbox/status.

## Completion record

- Replaced both host-platform specs with executor-capability desktop and mobile specs. Added a
  WebSocket proxy helper that records only `task.session.status` request IDs, rewrites only their
  response capability, and forwards all other frames unchanged.
- `./e2e/scripts/run-e2e.sh --no-build --project chromium -- tests/task/executor-embedded-vscode-availability.spec.ts` passed: 2 tests.
- `pnpm e2e:run -- tests/task/mobile-executor-embedded-vscode-availability.spec.ts --project=mobile-chrome` passed: 1 test.
- The desktop suite covers unsupported-session filtering with a retained custom editor and a
  supported capability under a Windows browser platform. The mobile suite confirms the compact
  header, no desktop editor control, and no horizontal overflow.
